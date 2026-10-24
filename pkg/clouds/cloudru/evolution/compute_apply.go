// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// Defaults for the VM converge below.
const (
	// DefaultVmWaitTimeout is how long a VM is given to reach `running` and its
	// floating IP to attach. A measured create reaches `running` in ~45 s; ten
	// minutes is the point at which "still booting" stopped being plausible.
	DefaultVmWaitTimeout = 10 * time.Minute
	// DefaultVmPollInterval is how often the VM and floating-IP state are re-read.
	DefaultVmPollInterval = 5 * time.Second

	// DefaultVmDiskGiB is the boot disk SC asks for when the config names no
	// size. Ubuntu 24.04 declares `min_disk: 11`, so this is not a minimum — it
	// is enough room for the images a compose stack pulls.
	DefaultVmDiskGiB = 30

	// vmDiskSuffix and vmFloatingIPSuffix name a VM's satellite objects. They are
	// derived rather than configurable so that a destroy can find them by name
	// after a create that failed halfway.
	vmDiskSuffix       = "-disk"
	vmFloatingIPSuffix = "-ip"

	// metadataLogin and metadataPublicKey are the image_metadata slots SC fills.
	metadataHostname  = "hostname"
	metadataLogin     = "name"
	metadataPublicKey = "public_key"
)

// IngressRule is one port SC opens on the VM's security group.
type IngressRule struct {
	// Protocol is `tcp` or `udp`.
	Protocol string `json:"protocol,omitempty"`
	// Port is a single port; PortRange ("30000:32767") takes precedence.
	Port      int    `json:"port,omitempty"`
	PortRange string `json:"portRange,omitempty"`
	// CIDR defaults to 0.0.0.0/0.
	CIDR        string `json:"cidr,omitempty"`
	Description string `json:"description,omitempty"`
}

func (r IngressRule) portRange() string {
	if r.PortRange != "" {
		return r.PortRange
	}
	return fmt.Sprintf("%d:%d", r.Port, r.Port)
}

func (r IngressRule) protocol() string {
	if r.Protocol == "" {
		return "tcp"
	}
	return strings.ToLower(r.Protocol)
}

func (r IngressRule) cidr() string {
	if r.CIDR == "" {
		return "0.0.0.0/0"
	}
	return r.CIDR
}

// VmSpec is the desired state of one Cloud.ru Evolution VM: the document the
// provisioning tier writes into `sc cloudru compute-vm apply`'s stdin.
//
// What is NOT here is as deliberate as what is. There is no `userData` and no
// `cloudInit`, because `POST /vms` refuses them (measured 2026-10-24): the image
// renders a fixed template from its own declared `image_metadata` slots and
// nothing else reaches the first boot. Everything SC needs on the machine —
// docker, compose, k3s — is installed afterwards over SSH.
type VmSpec struct {
	// Name is the VM. Satellite objects are named from it.
	Name string `json:"name"`
	// Hostname defaults to Name.
	Hostname string `json:"hostname,omitempty"`
	// Zone is an availability-zone NAME (`ru.AZ-3`); empty picks the project
	// default.
	Zone string `json:"zone,omitempty"`

	// Cores and MemoryGiB resolve a flavor. Flavor, when set, names one
	// explicitly and wins — but flavor names on this cloud are not stable
	// (`low-2-4` became `lowcost10-2-4` within a day), so cores+memory is the
	// supported spelling.
	Cores     int    `json:"cores,omitempty"`
	MemoryGiB int    `json:"memoryGiB,omitempty"`
	Flavor    string `json:"flavor,omitempty"`

	// Image is a UUID, a catalogue name (`Ubuntu-24.04`) or an SC alias
	// (`ubuntu-2404-lts`).
	Image string `json:"image,omitempty"`

	DiskGiB  int    `json:"diskGiB,omitempty"`
	DiskType string `json:"diskType,omitempty"`

	// SubnetID and SecurityGroupIDs are accepted so the config can carry them and
	// get a clear refusal: this cloud does not let a create choose either.
	SubnetID         string   `json:"subnetId,omitempty"`
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`

	// IngressRules are opened on whichever security group the VM actually joined.
	IngressRules []IngressRule `json:"ingressRules,omitempty"`

	// SSHUser is the login the image creates; PublicKey is the key it authorises.
	SSHUser   string `json:"sshUser,omitempty"`
	PublicKey string `json:"publicKey,omitempty"`

	// PublicIP allocates and attaches a floating IP.
	PublicIP bool `json:"publicIp,omitempty"`
}

// VmResult is the VM half of Result — the facts the provisioning tier cannot
// know before the call.
type VmResult struct {
	VmID            string `json:"vmId,omitempty"`
	DiskID          string `json:"diskId,omitempty"`
	FloatingIPID    string `json:"floatingIpId,omitempty"`
	Address         string `json:"address,omitempty"`
	InternalAddress string `json:"internalAddress,omitempty"`
	User            string `json:"user,omitempty"`
	Zone            string `json:"zone,omitempty"`
	FlavorName      string `json:"flavorName,omitempty"`
	ImageID         string `json:"imageId,omitempty"`
}

// ApplyVm makes one VM exist, reachable, and in the shape the spec asks for.
//
// Find-or-create at every step, keyed by name, because this runs under a
// pulumi-command resource: a retried deploy, or a deploy that follows a create
// which failed between the disk and the VM, must converge rather than pile up a
// second disk. Nothing here is ever recreated on SC's own initiative — a size
// change is reported as an error, not silently replaced, since replacing a VM
// destroys its data.
func (c *Client) ApplyVm(ctx context.Context, req Request, progress func(string, ...any)) (*Result, error) {
	if err := c.creds.Validate(); err != nil {
		return nil, err
	}
	if req.VmSpec == nil {
		return nil, errors.Errorf("`vmSpec` is required")
	}
	if progress == nil {
		progress = func(string, ...any) {}
	}
	spec := *req.VmSpec
	if spec.Name == "" {
		spec.Name = req.Name
	}
	spec.Name = toComputeName(spec.Name)
	if err := ValidateComputeName("vm", spec.Name); err != nil {
		return nil, err
	}
	if spec.SubnetID != "" || len(spec.SecurityGroupIDs) > 0 {
		return nil, errors.Errorf("cloud.ru Evolution cannot place a VM in a chosen subnet or security group at create time: "+
			"`POST /vms` refuses `subnet_id`, `security_group_ids`, `interfaces`, `networks` and `nics` alike, and the VM lands "+
			"in its availability zone's DEFAULT subnet and DEFAULT security group. Remove `network`/`securityGroupIds` from the "+
			"VM config (SC opens the ports it needs on whichever group the VM joined), or create the machine by hand. "+
			"VM %q", spec.Name)
	}
	if spec.PublicKey == "" {
		return nil, errors.Errorf("cloud.ru VM %q needs an SSH public key: the image authorises a key through its "+
			"`image_metadata.public_key` slot and there is no other way in (no user-data, no serial console)", spec.Name)
	}

	timeout, err := parseDuration(req.WaitTimeout, DefaultVmWaitTimeout, "waitTimeout")
	if err != nil {
		return nil, err
	}
	interval, err := parseDuration(req.PollInterval, DefaultVmPollInterval, "pollInterval")
	if err != nil {
		return nil, err
	}

	zone := spec.Zone
	if zone == "" {
		if zone, err = c.DefaultZone(ctx); err != nil {
			return nil, err
		}
		progress("resolved default availability zone %s", zone)
	} else if err := c.AssertZonePlaceable(ctx, zone); err != nil {
		// Checked here, before the boot disk: `POST /vms` is where the cloud would
		// otherwise refuse, and by then the disk exists — so every retry of a mis-zoned
		// config leaves another orphan behind. DefaultZone needs no such check because
		// it only ever returns a placeable zone.
		return nil, err
	}

	user := spec.SSHUser
	if user == "" {
		return nil, errors.Errorf("cloud.ru VM %q needs an `sshUser`", spec.Name)
	}

	image, err := c.FindImage(ctx, spec.Image)
	if err != nil {
		return nil, err
	}
	for _, slot := range []string{metadataLogin, metadataPublicKey} {
		if !image.supportsMetadata(slot) {
			return nil, errors.Errorf("cloud.ru image %q (%s) declares no `%s` metadata slot, so SC cannot authorise an SSH "+
				"key on it and the VM would boot unreachable; pick an image whose `image_metadata` includes `name` and `public_key`",
				image.Name, image.ID, slot)
		}
	}

	flavor, err := c.resolveFlavor(ctx, zone, spec)
	if err != nil {
		return nil, err
	}

	diskGiB := spec.DiskGiB
	if diskGiB == 0 {
		diskGiB = DefaultVmDiskGiB
	}
	if image.MinDisk > 0 && diskGiB < image.MinDisk {
		return nil, errors.Errorf("cloud.ru image %q needs at least %d GiB of disk, the config asks for %d",
			image.Name, image.MinDisk, diskGiB)
	}
	diskType, err := c.FindDiskType(ctx, spec.DiskType)
	if err != nil {
		return nil, err
	}

	vm, found, err := c.FindVm(ctx, spec.Name)
	if err != nil {
		return nil, err
	}
	if found {
		progress("vm %s already exists (%s, %s)", spec.Name, vm.ID, vm.State)
		if err := checkVmSizeDrift(vm, flavor); err != nil {
			return nil, err
		}
	} else {
		diskName := derivedName(spec.Name, vmDiskSuffix)
		disk, diskFound, err := c.FindDisk(ctx, diskName)
		if err != nil {
			return nil, err
		}
		if diskFound {
			progress("boot disk %s already exists (%s, %s)", diskName, disk.ID, disk.State)
		} else {
			progress("creating %d GiB %s boot disk %s from image %s in %s",
				diskGiB, diskType.Name, diskName, image.Name, zone)
			if disk, err = c.CreateDisk(ctx, diskName, diskGiB, diskType.Name, zone, image.ID); err != nil {
				return nil, errors.Wrapf(err, "failed to create boot disk for cloud.ru VM %q", spec.Name)
			}
		}
		// Both branches wait: a fresh disk is `creating` for a few seconds, and a
		// disk left behind by a run that died between the two creates can be found
		// in exactly that state.
		if disk, err = c.waitDiskAvailable(ctx, disk, timeout, interval, progress); err != nil {
			return nil, err
		}

		hostname := spec.Hostname
		if hostname == "" {
			hostname = spec.Name
		}
		progress("creating vm %s (%s: %d vCPU / %d GiB) in %s", spec.Name, flavor.Name, flavor.CPU, flavor.RAM, zone)
		vm, err = c.CreateVm(ctx, createVmRequest{
			ProjectID: c.creds.ProjectID,
			Name:      spec.Name,
			Zone:      zone,
			FlavorID:  flavor.ID,
			Disks:     []createVmDisk{{DiskID: disk.ID}},
			ImageMetadata: map[string]string{
				metadataHostname:  hostname,
				metadataLogin:     user,
				metadataPublicKey: strings.TrimSpace(spec.PublicKey),
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create cloud.ru VM %q", spec.Name)
		}
	}

	if vm, err = c.waitVmRunning(ctx, vm.ID, timeout, interval, progress); err != nil {
		return nil, err
	}

	res := &Result{
		Name: spec.Name,
		ID:   vm.ID,
		VmResult: VmResult{
			VmID:            vm.ID,
			Address:         vm.PrivateAddress(),
			InternalAddress: vm.PrivateAddress(),
			User:            user,
			Zone:            vm.Zone.Name,
			FlavorName:      vm.Flavor.Name,
			ImageID:         vm.ImageID,
		},
		Status: vm.State,
	}
	if len(vm.Disks) > 0 {
		res.DiskID = vm.Disks[0].ID
	}

	if spec.PublicIP {
		fip, err := c.ensureFloatingIP(ctx, spec.Name, zone, vm, timeout, interval, progress)
		if err != nil {
			return nil, err
		}
		res.FloatingIPID = fip.ID
		res.Address = fip.IPAddress
		res.URL = "ssh://" + user + "@" + fip.IPAddress
	}

	if err := c.ensureIngress(ctx, vm, zone, spec.IngressRules, progress); err != nil {
		return nil, err
	}
	return res, nil
}

// resolveFlavor turns the spec's size into a flavor.
func (c *Client) resolveFlavor(ctx context.Context, zone string, spec VmSpec) (*Flavor, error) {
	if spec.Flavor != "" {
		return c.FindFlavorByName(ctx, zone, spec.Flavor)
	}
	if spec.Cores <= 0 || spec.MemoryGiB <= 0 {
		return nil, errors.Errorf("cloud.ru VM %q needs either `flavor` or both `cores` and `memoryGiB`", spec.Name)
	}
	return c.FindFlavor(ctx, zone, spec.Cores, spec.MemoryGiB)
}

// checkVmSizeDrift refuses a resize instead of performing it.
//
// Compared by cores and memory rather than by flavor id, deliberately: the
// catalogue renames flavors (the 2 vCPU / 4 GiB size was `low-2-4` on 2026-10-23
// and `lowcost10-2-4` on 2026-10-24), so an id comparison would declare drift on
// a machine nobody touched and offer to destroy it.
func checkVmSizeDrift(existing *Vm, desired *Flavor) error {
	if existing.Flavor.CPU == desired.CPU && existing.Flavor.RAM == desired.RAM {
		return nil
	}
	return errors.Errorf("cloud.ru VM %q is running %d vCPU / %d GiB (flavor %q) and the config asks for %d vCPU / %d GiB "+
		"(flavor %q). SC will not resize it: Cloud.ru has no in-place resize, so this would mean destroying the machine and "+
		"everything on its disk. Destroy the stack deliberately (`sc destroy`) and deploy it again, or put the size back",
		existing.Name, existing.Flavor.CPU, existing.Flavor.RAM, existing.Flavor.Name,
		desired.CPU, desired.RAM, desired.Name)
}

// waitVmRunning polls until the VM serves, re-reading it by id so the returned
// object carries its interfaces.
func (c *Client) waitVmRunning(ctx context.Context, id string, timeout, interval time.Duration, progress func(string, ...any)) (*Vm, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		vm, err := c.GetVm(ctx, id)
		if err != nil {
			return nil, err
		}
		if vm.State != last {
			progress("vm %s is %s", vm.Name, vm.State)
			last = vm.State
		}
		if vm.State == VmStateRunning {
			return vm, nil
		}
		if isTerminalVmFailure(vm.State) {
			return nil, errors.Errorf("cloud.ru VM %q entered state %q and will not become %q",
				vm.Name, vm.State, VmStateRunning)
		}
		if time.Now().After(deadline) {
			return nil, errors.Errorf("cloud.ru VM %q is still %q after %s", vm.Name, vm.State, timeout)
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return nil, err
		}
	}
}

// waitDiskAvailable polls until the boot disk can be attached.
//
// `POST /disks` returns immediately with the disk in `creating`, and `POST /vms`
// refuses to attach one in that state — `422 disk_can_not_be_attached_from_
// current_state`, which names neither the disk nor the state it wanted. Nothing
// in the create response says the disk is not ready yet, so the only way to know
// is to read it back.
//
// An `in_use` disk is a different problem and is refused rather than waited on:
// the name SC derives for the boot disk belongs to one VM, so finding it attached
// to another means two stacks have picked the same VM name, and attaching it
// anywhere else would be destructive.
func (c *Client) waitDiskAvailable(
	ctx context.Context, disk *Disk, timeout, interval time.Duration, progress func(string, ...any),
) (*Disk, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		if disk.State != last {
			progress("boot disk %s is %s", disk.Name, disk.State)
			last = disk.State
		}
		switch {
		case disk.State == DiskStateAvailable:
			return disk, nil
		case disk.State == DiskStateInUse:
			return nil, errors.Errorf(
				"cloud.ru boot disk %q (%s) is already attached to %s. SC derives this disk's name from the VM "+
					"name, so another VM holding it means two stacks want the same VM — rename one of them",
				disk.Name, disk.ID, attachedVmNames(disk))
		case isTerminalDiskFailure(disk.State):
			return nil, errors.Errorf("cloud.ru boot disk %q entered state %q and will not become %q",
				disk.Name, disk.State, DiskStateAvailable)
		}
		if time.Now().After(deadline) {
			return nil, errors.Errorf("cloud.ru boot disk %q is still %q after %s", disk.Name, disk.State, timeout)
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return nil, err
		}
		fresh, err := c.GetDisk(ctx, disk.ID)
		if err != nil {
			return nil, err
		}
		disk = fresh
	}
}

// attachedVmNames describes who holds a disk, for the refusal above. The API
// reports the holders but has been seen answering an empty list for a disk it
// nonetheless calls `in_use`, so the fallback has to read as a sentence too.
func attachedVmNames(disk *Disk) string {
	if len(disk.Vms) == 0 {
		return "another VM"
	}
	return strings.Join(lo.Map(disk.Vms, func(v namedRef, _ int) string {
		return fmt.Sprintf("%s (%s)", v.Name, v.ID)
	}), ", ")
}

// isTerminalDiskFailure mirrors isTerminalVmFailure: listed, not inferred, so an
// undocumented transient state keeps polling instead of failing the deploy.
func isTerminalDiskFailure(state string) bool {
	switch strings.ToLower(state) {
	case "error", "failed", "deleted", "deleting":
		return true
	}
	return false
}

// isTerminalVmFailure reports a state from which no amount of waiting helps.
// Listed rather than inferred because an unknown state must keep polling: the
// API has no documented state machine and a new transient state must not be read
// as a failure.
func isTerminalVmFailure(state string) bool {
	switch strings.ToLower(state) {
	case "error", "failed", "deleted", "deleting":
		return true
	}
	return false
}

// ensureFloatingIP allocates the VM's public address and attaches it.
//
// The attach is `PUT /floating-ips/{id}` with the interface id, and it 422s while
// the address is still provisioning — so the address is polled to `available`
// first, and an already-attached address is left alone.
func (c *Client) ensureFloatingIP(
	ctx context.Context, vmName, zone string, vm *Vm, timeout, interval time.Duration, progress func(string, ...any),
) (*FloatingIP, error) {
	name := derivedName(vmName, vmFloatingIPSuffix)
	fip, found, err := c.FindFloatingIP(ctx, name)
	if err != nil {
		return nil, err
	}
	if !found {
		progress("allocating floating ip %s in %s", name, zone)
		if fip, err = c.CreateFloatingIP(ctx, name, zone); err != nil {
			return nil, errors.Wrapf(err, "failed to allocate floating ip for cloud.ru VM %q", vmName)
		}
	}

	iface := vm.primaryInterface()
	if iface == nil {
		return nil, errors.Errorf("cloud.ru VM %q reports no network interface, so its floating ip cannot be attached", vmName)
	}
	if iface.FloatingIP != nil && iface.FloatingIP.ID == fip.ID {
		progress("floating ip %s (%s) already attached", fip.Name, fip.IPAddress)
		return fip, nil
	}
	if iface.FloatingIP != nil {
		return nil, errors.Errorf("cloud.ru VM %q already has floating ip %q (%s) attached, which is not the one SC manages (%q); "+
			"detach it in the console or rename it to %s", vmName, iface.FloatingIP.Name, iface.FloatingIP.IPAddress, fip.Name, name)
	}

	deadline := time.Now().Add(timeout)
	for fip.State != FloatingIPStateAvailable && fip.State != FloatingIPStateInUse {
		if time.Now().After(deadline) {
			return nil, errors.Errorf("cloud.ru floating ip %q is still %q after %s", fip.Name, fip.State, timeout)
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return nil, err
		}
		if fip, err = c.GetFloatingIP(ctx, fip.ID); err != nil {
			return nil, err
		}
	}

	progress("attaching floating ip %s (%s) to interface %s", fip.Name, fip.IPAddress, iface.ID)
	if err := c.AttachFloatingIP(ctx, fip.ID, iface.ID); err != nil {
		return nil, errors.Wrapf(err, "failed to attach floating ip %q to cloud.ru VM %q", fip.Name, vmName)
	}
	if fip, err = c.GetFloatingIP(ctx, fip.ID); err != nil {
		return nil, err
	}
	if fip.IPAddress == "" {
		return nil, errors.Errorf("cloud.ru floating ip %q is attached but reports no address", fip.Name)
	}
	return fip, nil
}

// ensureIngress opens the requested ports on the security groups the VM's
// interface actually joined.
//
// Read off the VM rather than looked up as "the zone's default group", because
// those are not always the same object and a rule added to the wrong group is a
// deploy that reports success and a port that stays shut.
func (c *Client) ensureIngress(ctx context.Context, vm *Vm, zone string, rules []IngressRule, progress func(string, ...any)) error {
	if len(rules) == 0 {
		return nil
	}
	var groupIDs []string
	if iface := vm.primaryInterface(); iface != nil {
		for _, g := range iface.SecurityGroups {
			groupIDs = append(groupIDs, g.ID)
		}
	}
	if len(groupIDs) == 0 {
		groups, err := c.FindSecurityGroups(ctx, zone, nil)
		if err != nil {
			return err
		}
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
		}
	}

	existing, err := c.ListSecurityGroups(ctx)
	if err != nil {
		return err
	}
	byID := make(map[string]SecurityGroup, len(existing))
	for _, g := range existing {
		byID[g.ID] = g
	}

	for _, groupID := range groupIDs {
		group := byID[groupID]
		for _, rule := range rules {
			wanted := SecurityGroupRule{
				Direction:      "ingress",
				EtherType:      "IPv4",
				IPProtocol:     rule.protocol(),
				PortRange:      rule.portRange(),
				RemoteIPPrefix: rule.cidr(),
				Description:    rule.Description,
			}
			if hasIngressRule(group, wanted) {
				continue
			}
			progress("opening %s %s from %s on security group %s", wanted.IPProtocol, wanted.PortRange, wanted.RemoteIPPrefix, group.Name)
			if err := c.CreateSecurityGroupRule(ctx, groupID, wanted); err != nil {
				return errors.Wrapf(err, "failed to open %s %s on cloud.ru security group %q",
					wanted.IPProtocol, wanted.PortRange, groupID)
			}
		}
	}
	return nil
}

// hasIngressRule reports whether the group already permits what a rule asks for.
//
// A group-wide `any`/`any` ingress rule (which the project's default group has,
// scoped to the group itself) does NOT count as covering a rule from 0.0.0.0/0 —
// it permits traffic from group members only, which is not the same reachability.
func hasIngressRule(group SecurityGroup, wanted SecurityGroupRule) bool {
	for _, r := range group.Rules {
		if r.Direction != "ingress" || r.RemoteIPPrefix != wanted.RemoteIPPrefix {
			continue
		}
		if r.IPProtocol == "any" && r.PortRange == "any" {
			return true
		}
		if r.IPProtocol == wanted.IPProtocol && r.PortRange == wanted.PortRange {
			return true
		}
	}
	return false
}

// DestroyVm removes a VM and the objects SC created alongside it.
//
// Order matters and is not negotiable: the VM holds the interface the floating IP
// is attached to and the disk is `in_use` until the VM is gone, so deleting the
// disk first fails and deleting the floating IP first strands it. Absence is
// success at every step — a destroy that runs twice, or after a create that
// failed halfway, must still converge.
func (c *Client) DestroyVm(ctx context.Context, req Request, progress func(string, ...any)) (*Result, error) {
	if err := c.creds.Validate(); err != nil {
		return nil, err
	}
	if progress == nil {
		progress = func(string, ...any) {}
	}
	name := req.Name
	if name == "" && req.VmSpec != nil {
		name = req.VmSpec.Name
	}
	name = toComputeName(name)
	if name == "" {
		return nil, errors.Errorf("`name` is required")
	}
	timeout, err := parseDuration(req.WaitTimeout, DefaultVmWaitTimeout, "waitTimeout")
	if err != nil {
		return nil, err
	}
	interval, err := parseDuration(req.PollInterval, DefaultVmPollInterval, "pollInterval")
	if err != nil {
		return nil, err
	}

	if vm, found, err := c.FindVm(ctx, name); err != nil {
		return nil, err
	} else if found {
		progress("deleting vm %s (%s)", vm.Name, vm.ID)
		if err := c.DeleteVm(ctx, vm.ID); err != nil {
			return nil, errors.Wrapf(err, "failed to delete cloud.ru VM %q", name)
		}
		if err := c.waitVmGone(ctx, vm.ID, timeout, interval, progress); err != nil {
			return nil, err
		}
	}

	if fip, found, err := c.FindFloatingIP(ctx, derivedName(name, vmFloatingIPSuffix)); err != nil {
		return nil, err
	} else if found {
		progress("releasing floating ip %s (%s)", fip.Name, fip.IPAddress)
		if err := c.DeleteFloatingIP(ctx, fip.ID); err != nil {
			return nil, errors.Wrapf(err, "failed to release cloud.ru floating ip %q", fip.Name)
		}
	}

	if disk, found, err := c.FindDisk(ctx, derivedName(name, vmDiskSuffix)); err != nil {
		return nil, err
	} else if found {
		progress("deleting boot disk %s", disk.Name)
		if err := c.DeleteDisk(ctx, disk.ID); err != nil {
			return nil, errors.Wrapf(err, "failed to delete cloud.ru boot disk %q", disk.Name)
		}
	}
	return &Result{Name: name}, nil
}

// waitVmGone polls until the VM no longer exists, so the disk delete that follows
// is not refused for an `in_use` volume.
func (c *Client) waitVmGone(ctx context.Context, id string, timeout, interval time.Duration, progress func(string, ...any)) error {
	deadline := time.Now().Add(timeout)
	for {
		vm, err := c.GetVm(ctx, id)
		if err != nil {
			if NotFound(err) {
				return nil
			}
			return err
		}
		if time.Now().After(deadline) {
			return errors.Errorf("cloud.ru VM %q is still %q after %s of waiting for it to be deleted", vm.Name, vm.State, timeout)
		}
		progress("waiting for vm %s to disappear (%s)", vm.Name, vm.State)
		if err := sleepCtx(ctx, interval); err != nil {
			return err
		}
	}
}

// sleepCtx waits, honouring cancellation.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
