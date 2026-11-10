// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/vm"
)

const (
	// acmeStoreWaitAttempts × acmeStoreWaitInterval is how long the store gets to
	// answer on 127.0.0.1 after `compose up -d` returned. SeaweedFS's `server -s3`
	// starts a master, a volume server, a filer and the S3 gateway in sequence and the
	// gateway is last; measured on the stand it is ready in 3-6 s from a cold start,
	// so 60 s is generous rather than a guess at an unknown.
	acmeStoreWaitAttempts = 20
	acmeStoreWaitInterval = 3

	// acmeStoreReachAttempts × acmeStoreReachInterval is the budget for a NODE to reach
	// the store across the mesh. Shorter than the above because by this point the store
	// has already answered on host 0 — what is being proven here is the route, and a
	// route either exists or does not.
	acmeStoreReachAttempts = 10
	acmeStoreReachInterval = 3

	// acmeStoreKeyLength and acmeStoreSecretLength are S3 credential shapes rather than
	// security choices: 20 and 40 characters is what every S3 implementation's examples
	// use, and certmagic-s3 passes them to the AWS SDK, which has rejected odd lengths
	// in some signing modes.
	acmeStoreKeyLength    = 20
	acmeStoreSecretLength = 40
)

// AcmeStoreCredentials are the three values generated for a store SC runs itself. All
// three are Outputs, so this type is passed by POINTER everywhere: a zero
// sdk.StringOutput has no OutputState and panics inside sdk.All's gatherJoinSet, and
// "there is no managed store" is the common case.
//
// AccessKey and SecretKey authenticate to SeaweedFS's S3 gateway. EncryptionKey is
// certmagic-s3's at-rest key for the objects in the bucket, and is a separate secret on
// purpose: the bucket holds the fleet's private keys, and the store runs on a machine
// the fleet's own workloads also run on.
type AcmeStoreCredentials struct {
	AccessKey     sdk.StringOutput
	SecretKey     sdk.StringOutput
	EncryptionKey sdk.StringOutput
}

// ensureAcmeStore stands the fleet's shared certificate store up on host 0 and proves
// every other host can reach it.
//
// Returns nil credentials and nil gates when `ingress.acmeStore` is absent, which is
// every single-machine vm resource and every fleet whose operator named a real bucket
// in `ingress.acmeStorage` instead. The caller passes both straight into ensureIngress,
// so the no-store path adds nothing to the graph.
//
// WHY THIS EXISTS AT ALL, since it is a database on a VM and that is usually the wrong
// shape: SC's Caddy image carries no `dns.*` provider, so HTTP-01 is the only ACME
// challenge it can answer. A name with N A records sends the CA's validator to an
// arbitrary machine, which 404s a token a sibling minted — so a fleet that terminates
// its own TLS CANNOT issue a certificate without shared challenge state. certmagic
// shares that state through its storage module, which turns N machines into one ACME
// client holding one certificate. On a cloud that sells a bucket (`yc-bucket`,
// `gcp-bucket`) the operator points `acmeStorage` at it and none of this runs. On
// Cloud.ru, MWS or a rented VPS there is no bucket to point at, and this is the option.
//
// WHAT IT IS NOT: highly available. One container on one machine. Losing host 0 does
// not take the fleet down — Caddy serves from its local cache — but it stops renewals,
// and a fleet that cannot renew fails 60 days later, which is the kind of outage nobody
// connects to the machine that died in March. Said in the operator-facing log below,
// not just here.
func ensureAcmeStore(
	ctx *sdk.Context,
	input api.ResourceInput,
	cfg *vm.VmConfig,
	hosts []HostFacts,
	// ready is the per-host readiness gate, and mesh is the per-host mesh command when
	// there is a mesh (nil otherwise). Both are indexed by host. The store depends on
	// host 0's docker AND on the whole mesh, because the address it binds to does not
	// exist as an interface until `wg-quick up` has run.
	dockerGates []*remote.Command,
	meshGates []*remote.Command,
	params pApi.ProvisionParams,
) (*AcmeStoreCredentials, []sdk.Resource, error) {
	if cfg == nil || !cfg.AcmeStoreEnabled() {
		return nil, nil, nil
	}
	if len(hosts) == 0 {
		return nil, nil, errors.Errorf("the managed ACME store needs at least one host")
	}
	endpoint, err := cfg.AcmeStoreEndpoint()
	if err != nil {
		return nil, nil, err
	}
	host0 := hosts[0]

	creds, err := acmeStoreCredentials(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	// Host 0's gates: its docker (which supersedes its readiness) and the whole mesh.
	// The whole mesh and not just host 0's own `wg-quick up`, because the reachability
	// gates below run from the OTHER hosts and a half-configured mesh would fail them
	// for a reason that has nothing to do with the store.
	storeGates := []sdk.Resource{}
	if dockerGates[0] != nil {
		storeGates = append(storeGates, dockerGates[0])
	}
	for _, m := range meshGates {
		if m != nil {
			storeGates = append(storeGates, m)
		}
	}

	// The identity file first, and over stdin: it holds the credential every node of the
	// fleet authenticates with, so it must not be a command-line argument.
	s3Config := sdk.All(creds.AccessKey, creds.SecretKey).ApplyT(func(vals []any) (string, error) {
		accessKey, _ := vals[0].(string)
		secretKey, _ := vals[1].(string)
		return vm.RenderAcmeStoreS3Config(accessKey, secretKey)
	}).(sdk.StringOutput)

	identity, err := remote.NewCommand(ctx, input.ToResName(host0.Name+"-acme-store-identity"), &remote.CommandArgs{
		Connection:             ConnectionArgs(host0),
		Create:                 sdk.String(writeFileScript(vm.AcmeStoreS3ConfigPath(), vm.EnvFileMode)),
		Update:                 sdk.String(writeFileScript(vm.AcmeStoreS3ConfigPath(), vm.EnvFileMode)),
		Stdin:                  sdk.ToSecret(s3Config).(sdk.StringOutput).ToStringPtrOutput(),
		Triggers:               TriggersForGuardedChange(host0.Address, s3Config.ApplyT(func(s string) string { return hashOf(s) }).(sdk.StringOutput)),
		Logging:                remote.LoggingNone.ToLoggingPtrOutput(),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn(storeGates))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to write the ACME store identity of vm %q", input.Descriptor.Name)
	}

	compose, err := vm.RenderAcmeStoreCompose(cfg)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to render the ACME store of vm %q", input.Descriptor.Name)
	}
	upScript := acmeStoreUpScript(acmeStoreUpSpec{
		Image:   cfg.AcmeStoreImage(),
		Compose: compose,
		Bucket:  cfg.AcmeStoreBucket(),
		Port:    cfg.AcmeStorePort(),
	})
	up, err := remote.NewCommand(ctx, input.ToResName(host0.Name+"-acme-store-up"), &remote.CommandArgs{
		Connection: ConnectionArgs(host0),
		Create:     sdk.String(upScript),
		Update:     sdk.String(upScript),
		// Guarded change, so the nonce re-runs it every provision: the container, the
		// bucket and the compose file are all things an operator or a `docker system
		// prune` can remove without any input here moving, and every one of them being
		// gone is an outage 60 days later rather than today. The script is idempotent
		// end to end — `up -d` on an unchanged project does nothing, and the bucket
		// create is a create-if-absent.
		Triggers:               TriggersForGuardedChange(host0.Address, sdk.String(hashOf(upScript))),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}, sdk.DependsOn([]sdk.Resource{identity}))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to start the ACME store of vm %q", input.Descriptor.Name)
	}

	// Teardown on its own no-triggers resource, for the reason the ingress teardown
	// gives at length: pulumi-command performs a replacement as create-then-delete, so a
	// `Delete` on the command above would run on any provision that changed the script
	// — after the create, reporting green, having removed the fleet's store.
	if _, err := remote.NewCommand(ctx, input.ToResName(host0.Name+"-acme-store-teardown"), &remote.CommandArgs{
		Connection:             ConnectionArgs(host0),
		Create:                 sdk.String("true"),
		Delete:                 sdk.String(acmeStoreTeardownScript()),
		AddPreviousOutputInEnv: sdk.Bool(false),
	}); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to register the ACME store teardown of vm %q", input.Descriptor.Name)
	}

	// Every host proves it can reach the store BEFORE its Caddy is configured, host 0
	// included (it reaches the same address over the same interface, and a firewall rule
	// that broke the mesh address would break it there too).
	//
	// This FAILS the provision rather than warning. The alternative was tried in spirit
	// on the stand and is exactly the trap this whole slice exists to avoid: a Caddy that
	// cannot read the store does not fail — it falls back to its own file storage, mints
	// its own ACME account, and the fleet silently becomes N clients racing one rate
	// limit. The failure surfaces as "no certificate" days later, on whichever node lost
	// the race.
	gates := make([]sdk.Resource, 0, len(hosts))
	reachScript := acmeStoreReachScript(endpoint)
	for i, h := range hosts {
		deps := []sdk.Resource{up}
		if dockerGates[i] != nil {
			deps = append(deps, dockerGates[i])
		}
		cmd, err := remote.NewCommand(ctx, input.ToResName(h.Name+"-acme-store-reach"), &remote.CommandArgs{
			Connection: ConnectionArgs(h),
			Create:     sdk.String(reachScript),
			Update:     sdk.String(reachScript),
			// TriggersForRead, not guarded change: this command writes nothing, so
			// there is no drift for it to repair, and the nonce in it would still make
			// it run every provision — which is what is wanted. Reading the address is
			// what makes it re-run when a host moves.
			Triggers:               TriggersForRead(h.Address),
			AddPreviousOutputInEnv: sdk.Bool(false),
		}, sdk.DependsOn(deps))
		if err != nil {
			return nil, nil, errors.Wrapf(err,
				"failed to verify that host %q of vm %q can reach the ACME store", h.Name, input.Descriptor.Name)
		}
		gates = append(gates, cmd)
	}

	params.Log.Info(ctx.Context(),
		"vm %q runs its own shared certificate store on host %q (%s, bucket %q, image %q): every node reads and "+
			"writes ACME state there, so the fleet holds ONE certificate and one ACME account instead of %d",
		input.Descriptor.Name, host0.Name, endpoint, cfg.AcmeStoreBucket(), cfg.AcmeStoreImage(), len(hosts))
	params.Log.Warn(ctx.Context(),
		"vm %q keeps the fleet's certificate store on a single machine (host %q). Losing that machine does not "+
			"take the fleet down — each Caddy serves from its local cache — but it stops RENEWALS, which surfaces "+
			"as expired certificates up to 60 days later. Point `ingress.acmeStorage` at a managed bucket instead "+
			"wherever the cloud sells one",
		input.Descriptor.Name, host0.Name)
	return creds, gates, nil
}

// acmeStoreCredentials generates the three secrets, as Pulumi resources so they survive
// the next provision. Regenerating them would be worse than it looks: the encryption key
// is what every object in the bucket was sealed with, so a new one makes the fleet's
// whole certificate history unreadable and every node re-issues at once.
func acmeStoreCredentials(ctx *sdk.Context, input api.ResourceInput) (*AcmeStoreCredentials, error) {
	// Special characters off on all three. The access and secret keys ride an S3
	// SigV4 signature and a `+` or `/` in them has broken more than one S3
	// implementation's canonicalisation; the encryption key is passed through the
	// Caddyfile as an env placeholder, where `{`, `}` and `$` are Caddy's own syntax.
	generate := func(name string, length int) (sdk.StringOutput, error) {
		res, err := random.NewRandomPassword(ctx, input.ToResName("acme-store-"+name), &random.RandomPasswordArgs{
			Length:  sdk.Int(length),
			Special: sdk.Bool(false),
		})
		if err != nil {
			return sdk.StringOutput{}, errors.Wrapf(err,
				"failed to generate the ACME store %s of vm %q", name, input.Descriptor.Name)
		}
		return res.Result, nil
	}
	accessKey, err := generate("access-key", acmeStoreKeyLength)
	if err != nil {
		return nil, err
	}
	secretKey, err := generate("secret-key", acmeStoreSecretLength)
	if err != nil {
		return nil, err
	}
	// Exactly AcmeStorageEncryptionKeyLen, not "at least": certmagic-s3 requires a
	// 32-byte key and refuses anything else at config-load time, which CrashLoops
	// every replica of the fleet's ingress at once.
	encryptionKey, err := generate("encryption-key", vm.AcmeStorageEncryptionKeyLen)
	if err != nil {
		return nil, err
	}
	return &AcmeStoreCredentials{AccessKey: accessKey, SecretKey: secretKey, EncryptionKey: encryptionKey}, nil
}

// acmeStoreUpSpec is the non-secret half of the store: the identity file carrying the
// credential is delivered by its own command, over stdin.
type acmeStoreUpSpec struct {
	Image   string
	Compose string
	Bucket  string
	// Port is the host port the store is published on, used by the health probe below —
	// which probes 127.0.0.1 and therefore needs the port rather than the endpoint.
	Port int
}

func acmeStoreUpScript(s acmeStoreUpSpec) string {
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	b.WriteString(shellHelpers())
	fmt.Fprintf(b, "install -d -m 0750 %s\n", shellQuote(vm.AcmeStoreDir()))
	fmt.Fprintf(b, "printf %%s %s | base64 -d > %s\n",
		shellQuote(encodeFile(s.Compose)), shellQuote(vm.AcmeStoreComposePath()+".new"))
	fmt.Fprintf(b, "chmod 0640 %s\n", shellQuote(vm.AcmeStoreComposePath()+".new"))
	fmt.Fprintf(b, "mv -f %s %s\n",
		shellQuote(vm.AcmeStoreComposePath()+".new"), shellQuote(vm.AcmeStoreComposePath()))

	// The data volume is `external: true` in the compose file, which is what stops a
	// `docker compose down -v` run by hand from deleting the fleet's certificates — and
	// is also why it has to be created here, since compose will not create an external
	// volume.
	fmt.Fprintf(b, "docker volume inspect %s >/dev/null 2>&1 || docker volume create %s >/dev/null\n",
		shellQuote(vm.AcmeStoreDataVolume), shellQuote(vm.AcmeStoreDataVolume))

	// Best-effort pull for the same reason the ingress pulls best-effort: the clouds
	// this store exists for are the ones most likely to have no route to Docker Hub, and
	// a hard pull would make the store unprovisionable there with the image already
	// loaded by hand.
	fmt.Fprintf(b, "{ docker pull %s || true; } >&2 2>&1\n", shellQuote(s.Image))
	fmt.Fprintf(b, "docker image inspect %s >/dev/null 2>&1 || { echo %s >&2; exit 1; }\n",
		shellQuote(s.Image),
		shellQuote(fmt.Sprintf("the ACME store image %q is not on the machine and could not be pulled; "+
			"point `ingress.acmeStore.image` at a registry this machine can reach, or `docker load` it by hand",
			s.Image)))

	fmt.Fprintf(b, "docker compose -p %s -f %s up -d --remove-orphans\n",
		shellQuote(vm.AcmeStoreProject), shellQuote(vm.AcmeStoreComposePath()))
	fmt.Fprintf(b, "sc_wait %s %s %s\n",
		shellQuote(vm.AcmeStoreProject), shellQuote(vm.AcmeStoreComposePath()), shellQuote("store"))

	// Then the store must actually ANSWER, which `sc_wait` does not prove: the compose
	// file declares no healthcheck (see RenderAcmeStoreCompose for why), so "running" is
	// all docker knows. Probed on 127.0.0.1 rather than on the mesh address, because
	// what is being established here is that the process serves; the mesh route is what
	// the per-host gates prove.
	//
	// A 403 counts as an answer. An unauthenticated GET of the bucket list is exactly
	// what a correctly-configured S3 gateway refuses, so insisting on a 2xx here would
	// mean the probe passes only while the store is misconfigured.
	fmt.Fprintf(b, `i=0; code=000
while [ "$i" -lt %d ]; do
  code=$(curl -s -o /dev/null -w '%%{http_code}' --max-time 5 %s 2>/dev/null || echo 000)
  case "$code" in 2??|40?) break ;; esac
  i=$((i+1)); sleep %d
done
case "$code" in
  2??|40?) echo "the ACME store answers on %s (HTTP $code)" ;;
  *) echo %s >&2
     { docker logs --tail 200 %s || true; } >&2 2>&1
     exit 1 ;;
esac
`,
		acmeStoreWaitAttempts,
		shellQuote(fmt.Sprintf("http://127.0.0.1:%d/", s.Port)),
		acmeStoreWaitInterval,
		"127.0.0.1",
		shellQuote(fmt.Sprintf("the ACME store container is running but did not answer on 127.0.0.1 within %ds; "+
			"its own log follows", acmeStoreWaitAttempts*acmeStoreWaitInterval)),
		shellQuote(vm.AcmeStoreContainer))

	// The bucket, create-if-absent. `weed shell` takes its command on STDIN and NOT as
	// `-c <cmd>` (which it answers with `flag provided but not defined: -c`, live-caught
	// on the stand 2026-11-09) — so the command is piped, and `-master localhost:9333`
	// is required because the shell defaults to a master it cannot find in this layout.
	//
	// `s3.bucket.create` on an existing bucket is an error rather than a no-op, which is
	// why the list is consulted first instead of swallowing the create's exit code: a
	// swallowed error would also swallow "the master is not up yet".
	fmt.Fprintf(b, `if echo %s | docker exec -i %s weed shell -master localhost:9333 2>/dev/null | grep -qx %s; then
  echo %s
else
  echo %s | docker exec -i %s weed shell -master localhost:9333 >&2 2>&1
  if ! echo %s | docker exec -i %s weed shell -master localhost:9333 2>/dev/null | grep -qx %s; then
    echo %s >&2
    exit 1
  fi
  echo %s
fi
`,
		shellQuote("s3.bucket.list"), shellQuote(vm.AcmeStoreContainer), shellQuote(s.Bucket),
		shellQuote(fmt.Sprintf("the ACME store bucket %q is present", s.Bucket)),
		shellQuote("s3.bucket.create -name "+s.Bucket), shellQuote(vm.AcmeStoreContainer),
		shellQuote("s3.bucket.list"), shellQuote(vm.AcmeStoreContainer), shellQuote(s.Bucket),
		shellQuote(fmt.Sprintf("the ACME store bucket %q could not be created; the `weed shell` output is above",
			s.Bucket)),
		shellQuote(fmt.Sprintf("created the ACME store bucket %q", s.Bucket)))
	fmt.Fprintf(b, "echo %s\n", shellQuote("the ACME store is up"))
	return b.String()
}

// acmeStoreReachScript is one host proving it can reach the store across the mesh.
//
// Same "a 403 is an answer" reasoning as the health probe above, and the same reason for
// curl rather than a TCP connect: a connect succeeding proves the route and NOT that the
// thing on the other end is the store — on a mesh address, a connect that lands on a
// stale iptables DNAT would pass.
func acmeStoreReachScript(endpoint string) string {
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	fmt.Fprintf(b, `i=0; code=000
while [ "$i" -lt %d ]; do
  code=$(curl -s -o /dev/null -w '%%{http_code}' --max-time 5 %s 2>/dev/null || echo 000)
  case "$code" in 2??|40?) break ;; esac
  i=$((i+1)); sleep %d
done
case "$code" in
  2??|40?) echo "this host reaches the ACME store at %s (HTTP $code)" ;;
  *) echo %s >&2; exit 1 ;;
esac
`,
		acmeStoreReachAttempts,
		shellQuote(strings.TrimSuffix(endpoint, "/")+"/"),
		acmeStoreReachInterval,
		endpoint,
		shellQuote(fmt.Sprintf("this host cannot reach the fleet's certificate store at %s within %ds. The store "+
			"is published on the overlay address of the first machine and on 127.0.0.1 only, so the usual cause is "+
			"the mesh: check `wg show %s` on both machines (a peer with no handshake), and that the first machine's "+
			"firewall admits %d/udp. The provision is failed here deliberately rather than letting Caddy start — a "+
			"Caddy that cannot read the store does not fail, it issues its own certificate from its own ACME "+
			"account, and the fleet silently becomes N clients racing one rate limit",
			endpoint, acmeStoreReachAttempts*acmeStoreReachInterval, vm.MeshInterface, vm.DefaultMeshPort)))
	return b.String()
}

// acmeStoreTeardownScript removes the store when the vm resource is destroyed.
//
// It keeps the data volume, for a sharper version of the reason the ingress teardown
// keeps its own: that volume holds the fleet's ACME account key and every certificate it
// has been issued, and Let's Encrypt allows 50 certificates per domain per week.
// Deleting it makes re-provisioning the same fleet a week-long outage.
func acmeStoreTeardownScript() string {
	b := &strings.Builder{}
	b.WriteString("set -eu\n")
	b.WriteString(shellHelpers())
	fmt.Fprintf(b, "sc_rm %s\n", shellQuote(vm.AcmeStoreProject))
	fmt.Fprintf(b, "rm -rf %s\n", shellQuote(vm.AcmeStoreDir()))
	fmt.Fprintf(b, "echo %s\n", shellQuote(fmt.Sprintf(
		"removed the ACME store and its configuration; the volume %s (ACME account and certificates) was kept",
		vm.AcmeStoreDataVolume,
	)))
	return b.String()
}
