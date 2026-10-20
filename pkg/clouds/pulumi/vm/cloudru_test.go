// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/simple-container-com/api/pkg/clouds/vm"
)

const (
	tokenSbercloudProvider = "pulumi:providers:sbercloud"
	tokenVpc               = "sbercloud:index/vpc:Vpc"
	tokenVpcSubnet         = "sbercloud:index/vpcSubnet:VpcSubnet"
	tokenSecgroup          = "sbercloud:index/networkingSecgroup:NetworkingSecgroup"
	tokenSecgroupRule      = "sbercloud:index/networkingSecgroupRule:NetworkingSecgroupRule"
	tokenVpcEip            = "sbercloud:index/vpcEip:VpcEip"
	tokenComputeKeypair    = "sbercloud:index/computeKeypair:ComputeKeypair"
	tokenComputeInstance   = "sbercloud:index/computeInstance:ComputeInstance"
	tokenTlsPrivateKey     = "tls:index/privateKey:PrivateKey"

	tokenGetAvailabilityZones = "sbercloud:index/getAvailabilityZones:getAvailabilityZones"
	tokenGetComputeFlavors    = "sbercloud:index/getComputeFlavors:getComputeFlavors"
	tokenGetImagesImage       = "sbercloud:index/getImagesImage:getImagesImage"

	testEipAddress     = "203.0.113.42"
	testFlavorID       = "s6.large.2"
	testImageID        = "11111111-2222-3333-4444-555555555555"
	testGeneratedKey   = "-----BEGIN OPENSSH PRIVATE KEY-----\nGENERATED\n-----END OPENSSH PRIVATE KEY-----\n"
	testGeneratedPub   = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGENERATED\n"
	testCloudruAccess  = "AKTESTACCESSKEY"
	testCloudruSecret  = "SKTESTSECRETKEY"
	testCloudruProject = "forge-staging"
)

func cloudruConfig() *vm.VmConfig {
	cfg := &vm.VmConfig{
		Driver:   vm.DriverCloudru,
		Region:   "ru-moscow-1",
		PublicIP: true,
	}
	// What `credentials: ${auth:cloudru}` resolves to by the time the provisioning
	// tier sees it: an opaque JSON blob that api.ConvertAuth rehydrates.
	cfg.Credentials.Credentials = `{"accessKey":"` + testCloudruAccess + `","secretKey":"` + testCloudruSecret +
		`","projectName":"` + testCloudruProject + `"}`
	return cfg
}

// mockedID is the id vmMocks hands back for an SC resource name — which is not the
// name the driver passed in: ToResName suffixes the environment, and an assertion
// written against the bare name passes only until somebody runs the suite in another
// environment.
func mockedID(scName string) string {
	input := resourceInput(cloudruConfig())
	return input.ToResName(scName) + "-id"
}

// cloudruMocks answers the three datasources the driver filters on and fills in the
// outputs a real provider computes: the public address, the keypair's name, and the
// two generated SSH keys (without which the cloud-init ApplyT never runs).
func cloudruMocks() *vmMocks {
	m := newVmMocks()
	m.callResults = map[string]resource.PropertyMap{
		tokenGetAvailabilityZones: {
			"id":     resource.NewStringProperty("zones"),
			"region": resource.NewStringProperty("ru-moscow-1"),
			"names": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("ru-moscow-1a"),
				resource.NewStringProperty("ru-moscow-1b"),
			}),
		},
		tokenGetImagesImage: {
			"id":      resource.NewStringProperty(testImageID),
			"imageId": resource.NewStringProperty(testImageID),
			"name":    resource.NewStringProperty("Ubuntu 24.04 server 64bit"),
		},
		tokenGetComputeFlavors: {
			"id": resource.NewStringProperty("flavors"),
			// Deliberately NOT in sorted order: the datasource's own ids are unsorted,
			// and a driver that took them as given would pick a different flavour on a
			// later run and replace the machine.
			"ids": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("s6.xlarge.2"),
				resource.NewStringProperty(testFlavorID),
			}),
		},
	}
	m.extraOutputs = map[string]resource.PropertyMap{
		tokenVpcEip: {"address": resource.NewStringProperty(testEipAddress)},
		tokenComputeKeypair: {
			"name": resource.NewStringProperty("portal-vm-keypair"),
		},
		tokenTlsPrivateKey: {
			"privateKeyOpenssh": resource.MakeSecret(resource.NewStringProperty(testGeneratedKey)),
			"publicKeyOpenssh":  resource.NewStringProperty(testGeneratedPub),
		},
	}
	return m
}

// The flavour filter is an EXACT match on a whole number of GiB (the provider compares
// `memorySize * 1024` against the flavour's RAM in MB), so a quantity that does not
// convert cleanly has to be refused at graph-build time. Rounding it would present as
// "no flavour in this region has that shape", which sends an operator looking at the
// wrong thing entirely.
func TestGibibytes(t *testing.T) {
	RegisterTestingT(t)

	for quantity, expected := range map[string]int{
		"4Gi":    4,
		"8Gi":    8,
		"60Gi":   60,
		"1024Mi": 1,
		"1Ti":    1024,
	} {
		got, err := gibibytes(quantity, "size.memory")
		Expect(err).ToNot(HaveOccurred(), "quantity %q", quantity)
		Expect(got).To(Equal(expected), "quantity %q", quantity)
	}

	for _, quantity := range []string{
		"",
		"lots",
		"4gigs",
		// Not a whole number of GiB. Both are plausible operator input and both are
		// refused by name rather than rounded.
		"1536Mi",
		"512Mi",
		// Decimal GB is 4e9 bytes, which is not 4 GiB. Refused rather than taken as
		// close enough: SC's DSL spells the binary unit `Gi` everywhere else, and
		// quietly accepting one and filtering on the other matches no flavour.
		"4GB",
		"0Gi",
	} {
		_, err := gibibytes(quantity, "size.memory")
		Expect(err).To(HaveOccurred(), "quantity %q", quantity)
		// The field name is in every message: this value reaches the driver from two
		// different config keys, and "is not a size" without one is unactionable.
		Expect(err.Error()).To(ContainSubstring("size.memory"), "quantity %q", quantity)
	}
}

// `image:` accepts three shapes and the driver has to tell them apart without asking
// the cloud, because two of them take different (conflicting) lookup arguments.
func TestCloudruImageInput(t *testing.T) {
	RegisterTestingT(t)

	Expect(cloudruUUID.MatchString("8a7b6c5d-4e3f-2a1b-9c8d-7e6f5a4b3c2d")).To(BeTrue())
	Expect(cloudruUUID.MatchString("Ubuntu 24.04 server 64bit")).To(BeFalse())
	// A prefix that merely starts like one is not one: the id goes to the API verbatim,
	// so a near-miss has to fall through to the name lookup rather than be sent.
	Expect(cloudruUUID.MatchString("8a7b6c5d-4e3f-2a1b-9c8d-7e6f5a4b3c2d-extra")).To(BeFalse())
	Expect(cloudruUUID.MatchString("8a7b6c5d4e3f2a1b9c8d7e6f5a4b3c2d")).To(BeFalse())

	// SC's own default must be an alias this driver knows, or every Cloud.ru machine
	// created without an explicit `image:` is a name lookup for the literal string
	// "ubuntu-2404-lts".
	Expect(cloudruImageAliases).To(HaveKey(vm.DefaultImage))
	Expect(cloudruImageAliasNames()).To(Equal([]string{"ubuntu-2204-lts", "ubuntu-2404-lts"}))

	// The aliases are regexes over names like "Ubuntu 24.04 server 64bit", matched
	// case-insensitively and anchored at the start so that "24.04" cannot match inside
	// a build number.
	Expect(cloudruImageAliases[vm.DefaultImage]).To(MatchRegexp(`^\(\?i\)\^ubuntu`))
}

// Every map this driver iterates is iterated in sorted order, because the iteration
// produces Pulumi resource NAMES. Go's map order is randomised per run, so an
// unsorted loop renames resources between deploys — and a renamed resource is a
// delete-and-create, here of a security-group rule that is holding SSH open.
func TestSortedKeys(t *testing.T) {
	RegisterTestingT(t)

	Expect(sortedKeys(map[string]int{"https": 443, "ssh": 22, "http": 80})).
		To(Equal([]string{"http", "https", "ssh"}))
	Expect(sortedKeys(map[string]string{})).To(BeEmpty())
}

// The whole graph, built against mocks. Everything asserted below is a fact about
// Cloud.ru that cost a reading of the upstream provider's Go sources to establish, and
// each one would otherwise be learned from a failed live deploy — against an account
// that bills for the attempt.
func TestCloudruDriver_BuildsTheMachine(t *testing.T) {
	RegisterTestingT(t)

	mocks := cloudruMocks()
	out, err := provisionVm(cloudruConfig(), mocks)
	Expect(err).To(BeNil())
	Expect(out.Hosts).To(HaveLen(1))

	// One of each, and exactly one instance: a vm resource is one machine.
	Expect(mocks.countOf(tokenSbercloudProvider)).To(Equal(1))
	Expect(mocks.countOf(tokenVpc)).To(Equal(1))
	Expect(mocks.countOf(tokenVpcSubnet)).To(Equal(1))
	Expect(mocks.countOf(tokenSecgroup)).To(Equal(1))
	Expect(mocks.countOf(tokenVpcEip)).To(Equal(1))
	Expect(mocks.countOf(tokenComputeKeypair)).To(Equal(1))
	Expect(mocks.countOf(tokenComputeInstance)).To(Equal(1))
	// Two keys, both generated by SC: the deploy key it connects with, and the
	// machine's own host key — which exists so that the first connection can verify
	// the machine instead of trusting whoever answers on that address.
	Expect(mocks.countOf(tokenTlsPrivateKey)).To(Equal(2))
	// 22 only. 80 and 443 belong to the ingress, and a host that is not running one
	// has no business answering on them.
	Expect(mocks.countOf(tokenSecgroupRule)).To(Equal(1))
	Expect(mocks.inputsOf(tokenSecgroupRule)["portRangeMin"].NumberValue()).To(BeEquivalentTo(vm.DefaultSshPort))

	inst := mocks.inputsOf(tokenComputeInstance)
	Expect(inst).NotTo(BeNil())
	Expect(inst["availabilityZone"].StringValue()).To(Equal("ru-moscow-1a"))
	Expect(inst["imageId"].StringValue()).To(Equal(testImageID))
	Expect(inst["flavorId"].StringValue()).To(Equal(testFlavorID))
	// The address is bound at CREATION (`eip_id`, ForceNew), not attached afterwards:
	// cloud-init installs docker over the network, so an instance that boots without
	// egress runs its user-data to a failure and the readiness gate then waits out its
	// full timeout on a machine that will never become ready.
	Expect(inst["eipId"].StringValue()).To(Equal(mockedID("portal-vm-host-0-eip")))
	// Upstream defaults this to TRUE, which would have the instance delete an address
	// Pulumi owns — leaving a checkpoint entry for a resource that no longer exists.
	Expect(inst["deleteEipOnTermination"].BoolValue()).To(BeFalse())
	// The NIC attaches to the SUBNET: `networks[].uuid` is Cloud.ru's network id, and
	// in its vocabulary the subnet is the network.
	networks := inst["networks"].ArrayValue()
	Expect(networks).To(HaveLen(1))
	Expect(networks[0].ObjectValue()["uuid"].StringValue()).To(Equal(mockedID("portal-vm-subnet")))

	// user-data carries the host key's PRIVATE half, so it is a credential. It is
	// secret here because secretness propagates through the ApplyT that renders it —
	// not because anything remembered to mark it.
	Expect(inst["userData"].ContainsSecrets()).To(BeTrue(),
		"cloud-init carries the machine's host private key and entered Pulumi unmarked")
	userData, _ := unwrapSecret(inst["userData"])
	Expect(userData.StringValue()).To(HavePrefix("#cloud-config\n"))
	Expect(userData.StringValue()).To(ContainSubstring(strings.TrimSpace(testGeneratedPub)))
	Expect(userData.StringValue()).To(ContainSubstring("BEGIN OPENSSH PRIVATE KEY"))

	// The flavour filter, which is the one input that decides what the machine costs.
	flavorArgs := mocks.callArgsOf(tokenGetComputeFlavors)
	Expect(flavorArgs["cpuCoreCount"].NumberValue()).To(BeEquivalentTo(vm.DefaultCores))
	// Whole GiB, matched exactly: the provider compares `memorySize * 1024` against
	// the flavour's RAM in MB.
	Expect(flavorArgs["memorySize"].NumberValue()).To(BeEquivalentTo(4))
	Expect(flavorArgs["availabilityZone"].StringValue()).To(Equal("ru-moscow-1a"))

	// The default image reaches the lookup as a REGEX over public image names, not as
	// the literal alias — Cloud.ru's names carry a build number and it rebuilds them.
	imageArgs := mocks.callArgsOf(tokenGetImagesImage)
	Expect(imageArgs["nameRegex"].StringValue()).To(Equal(cloudruImageAliases[vm.DefaultImage]))
	Expect(imageArgs).ToNot(HaveKey("name"), "`name` and `nameRegex` conflict in the upstream datasource")
	Expect(imageArgs["visibility"].StringValue()).To(Equal("public"))

	// The security group keeps its default EGRESS rules. Deleting them is the
	// tidy-looking choice and it breaks the driver outright: cloud-init needs egress.
	secgroup := mocks.inputsOf(tokenSecgroup)
	if deleteDefaults, present := secgroup["deleteDefaultRules"]; present {
		Expect(deleteDefaults.BoolValue()).To(BeFalse(),
			"deleting the default egress rules leaves cloud-init unable to install docker")
	}
}

// With the ingress on, and only then, the machine answers on 80 and 443.
func TestCloudruDriver_OpensIngressPortsOnlyWithIngress(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudruConfig()
	cfg.Ingress = &vm.VmIngress{Enable: true, Email: "ops@example.com"}

	mocks := cloudruMocks()
	_, err := provisionVm(cfg, mocks)
	Expect(err).To(BeNil())

	Expect(mocks.countOf(tokenSecgroupRule)).To(Equal(3))
}

// An explicit `size` reaches the flavour filter, and `size.disk` reaches the root
// volume — in GiB, which is the unit both arguments are expressed in.
func TestCloudruDriver_HonoursSize(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudruConfig()
	cfg.Size = &vm.VmSize{Cores: 8, Memory: "16Gi", Disk: "60Gi", DiskType: "SSD"}

	mocks := cloudruMocks()
	_, err := provisionVm(cfg, mocks)
	Expect(err).To(BeNil())

	flavorArgs := mocks.callArgsOf(tokenGetComputeFlavors)
	Expect(flavorArgs["cpuCoreCount"].NumberValue()).To(BeEquivalentTo(8))
	Expect(flavorArgs["memorySize"].NumberValue()).To(BeEquivalentTo(16))

	inst := mocks.inputsOf(tokenComputeInstance)
	Expect(inst["systemDiskSize"].NumberValue()).To(BeEquivalentTo(60))
	Expect(inst["systemDiskType"].StringValue()).To(Equal("SSD"))
}

// Without `size.disk` the two disk arguments must be ABSENT, not zero: they are
// Optional+Computed upstream, so omitting them takes the image's own root disk and the
// account's default volume family. A zero would be a size.
func TestCloudruDriver_LeavesDiskToTheImage(t *testing.T) {
	RegisterTestingT(t)

	mocks := cloudruMocks()
	_, err := provisionVm(cloudruConfig(), mocks)
	Expect(err).To(BeNil())

	inst := mocks.inputsOf(tokenComputeInstance)
	Expect(inst).ToNot(HaveKey("systemDiskSize"))
	Expect(inst).ToNot(HaveKey("systemDiskType"))
}

// The operator's own VPC is used as given, and a subnet map that does not cover the
// zone the machine landed in is refused by name — the one shape of a BYO network that
// cannot be completed by guessing.
func TestCloudruDriver_ReusesAnExistingNetwork(t *testing.T) {
	RegisterTestingT(t)

	cfg := cloudruConfig()
	cfg.Network = &vm.VmNetwork{ID: "vpc-operator", Subnets: map[string]string{"ru-moscow-1a": "subnet-operator"}}

	mocks := cloudruMocks()
	_, err := provisionVm(cfg, mocks)
	Expect(err).To(BeNil())

	Expect(mocks.countOf(tokenVpc)).To(Equal(0), "SC created a VPC next to the one it was given")
	Expect(mocks.countOf(tokenVpcSubnet)).To(Equal(0))
	inst := mocks.inputsOf(tokenComputeInstance)
	Expect(inst["networks"].ArrayValue()[0].ObjectValue()["uuid"].StringValue()).To(Equal("subnet-operator"))

	// The zone the machine lands in is only known after the datasource answered, so
	// this cannot be caught by Validate — and a wrong guess here places the machine in
	// a zone, or behind a route table, the operator did not choose.
	wrongZone := cloudruConfig()
	wrongZone.Network = &vm.VmNetwork{ID: "vpc-operator", Subnets: map[string]string{"ru-moscow-1c": "subnet-operator"}}
	_, err = provisionVm(wrongZone, cloudruMocks())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`network.subnets` has no entry for it (has: ru-moscow-1c)"))
}

// The credential reaches the provider, the secret half marked. An unmarked provider
// input is printed verbatim in a Pulumi update summary and archived in the CI log —
// the exact mechanism by which a Yandex service-account private key reached a
// world-readable Actions log on 2026-09-30.
func TestCloudruProvider_MarksTheSecretKey(t *testing.T) {
	RegisterTestingT(t)

	mocks := cloudruMocks()
	_, err := provisionVm(cloudruConfig(), mocks)
	Expect(err).To(BeNil())

	prov := mocks.inputsOf(tokenSbercloudProvider)
	Expect(prov).NotTo(BeNil())
	Expect(prov["accessKey"].StringValue()).To(Equal(testCloudruAccess))

	Expect(prov["secretKey"].ContainsSecrets()).To(BeTrue(),
		"the Cloud.ru secret key entered Pulumi unmarked")
	secret, _ := unwrapSecret(prov["secretKey"])
	Expect(secret.StringValue()).To(Equal(testCloudruSecret))

	// Region picks the Keystone endpoint too, so the three travel together: setting
	// the region alone authenticates against one region's tenancy and creates in
	// another.
	Expect(prov["region"].StringValue()).To(Equal("ru-moscow-1"))
	Expect(prov["authUrl"].StringValue()).To(Equal("https://iam.ru-moscow-1.hc.sbercloud.ru/v3"))
	// An explicit project in the auth entry outranks the region-derived default.
	Expect(prov["projectName"].StringValue()).To(Equal(testCloudruProject))

	// A vm resource without credentials must say so, rather than building a provider
	// that fails its first call with a Keystone 401 naming no field.
	noCreds := cloudruConfig()
	noCreds.Credentials.Credentials = ""
	_, err = provisionVm(noCreds, cloudruMocks())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`accessKey` is required"))
}

// The host facts the layers above consume. Each is load-bearing somewhere else:
// HostKeyPinned decides whether the connection verifies the machine at all, and the
// deploy key is read by the vm-compose template in a different stack.
func TestCloudruDriver_HostFacts(t *testing.T) {
	RegisterTestingT(t)

	mocks := cloudruMocks()
	out, err := provisionVm(cloudruConfig(), mocks)
	Expect(err).To(BeNil())

	host := out.Hosts[0]
	Expect(host.Name).To(Equal("portal-vm-host-0"))
	Expect(host.User).To(Equal(vm.DefaultSshUser))
	Expect(host.Port).To(Equal(vm.DefaultSshPort))
	Expect(host.Role).To(Equal(vm.RoleCompose))
	// Always true on this driver, and that is the dividend of generating the host key:
	// unlike byo, SC created this machine and therefore created its identity.
	Expect(host.HostKeyPinned).To(BeTrue())

	// The readiness gate exists and is the thing templates DependsOn. A deploy that
	// waits for SSH alone fails intermittently on `docker: command not found`.
	Expect(out.Ready).To(HaveLen(1))
	Expect(out.Ready[0]).ToNot(BeNil())

	// The connection pins the host key, which is the entire point of generating one —
	// and pulumi-command checks `HostKey != nil`, not `!= ""`, so an empty value is
	// "provided and unparseable" and fails every connection to the machine.
	conn, _ := unwrapSecret(mocks.inputsOf(tokenRemoteCommand)["connection"])
	hostKey, _ := unwrapSecret(conn.ObjectValue()["hostKey"])
	Expect(hostKey.StringValue()).To(Equal(strings.TrimSpace(testGeneratedPub)))
	key, _ := unwrapSecret(conn.ObjectValue()["privateKey"])
	Expect(key.StringValue()).To(Equal(testGeneratedKey))
}
