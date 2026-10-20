// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// HostRole is what the machine is being prepared for. The cloud-init body is the
// same for both — the difference is what gets installed on top afterwards — but the
// role is carried explicitly so a future k3s-specific tweak has a place to go that
// is not a flag on VmConfig.
type HostRole string

const (
	// RoleCompose is a machine that runs stacks under `docker compose`.
	RoleCompose HostRole = "compose"
	// RoleK3sServer is a k3s control-plane node.
	RoleK3sServer HostRole = "k3s-server"
	// RoleK3sAgent is a k3s worker node.
	RoleK3sAgent HostRole = "k3s-agent"
)

// DefaultDockerInstallScript is the fallback way to get docker onto a fresh machine.
//
// It reaches out to get.docker.com, which is one of the four egress paths whose
// reachability from inside Russia is being measured before anything is promised
// about it (roadmap item sc-ru-market-readiness, B0).
//
// PackageMirror does NOT cover this: it rewrites apt's sources, and this is a curl to
// a host of Docker's own. The script then adds Docker's apt repository and installs
// from THERE, not from the distribution — so a machine that cannot reach
// download.docker.com cannot be fixed by an apt mirror either. The two answers for
// such a machine are an image that already ships docker (the install is skipped —
// see InstallDocker below) or a private mirror of Docker's repository; neither is
// expressible as a one-line field, which is why there is no flag pretending to be
// one.
const DefaultDockerInstallScript = "curl -fsSL https://get.docker.com | sh"

// dockerLogOpts caps the json-file driver.
//
// Not optional, and not tidiness. A VM — unlike Lambda or a Serverless Container —
// does not rotate container logs for you. An uncapped compose portal fills its disk
// over a few weeks and then fails in a way that looks like the application broke
// (vm-matrix.md §3.1).
var dockerLogOpts = map[string]string{
	"max-size": "50m",
	"max-file": "3",
}

// cloudInitTemplate is one template for every cloud. Only the transport differs
// per driver (YC's metadata["user-data"], OpenStack's user_data, MWS's
// os.metadata.attributes, Timeweb's cloud_init) — never the body.
//
// Two deviations from the sketch in vm-matrix.md §3.1, both deliberate:
//
//   - the user is NOT created with `groups: [docker]`. cloud-init's users module
//     runs before runcmd, so at that point the docker group does not exist yet; it
//     logs "cannot add user to group" and carries on, leaving an account that
//     cannot talk to the daemon. The group membership is granted after the install
//     instead.
//   - `sysctl --system` is called explicitly. Writing /etc/sysctl.d/99-sc.conf only
//     takes effect on the next boot otherwise, and the first deploy happens on this
//     one.
const cloudInitTemplate = `#cloud-config
{{- if .PackageMirror }}
# Rewrites the distribution's package sources before any module that installs a
# package runs. cloud-init's apt module does this by rewriting the sources files
# itself, which is why it is expressed here rather than as a runcmd — by the time
# runcmd runs, package_update has already tried the default mirror and the boot has
# already spent its timeout on it.
#
# Both the main archive and the security one are set, from the same value: a mirror
# that carries one and not the other leaves every boot waiting on
# security.ubuntu.com, which is the slow failure this field exists to avoid. Point it
# at a mirror that carries both (mirror.yandex.ru/ubuntu does).
apt:
  primary:
    - arches: [default]
      uri: {{ quote .PackageMirror }}
  security:
    - arches: [default]
      uri: {{ quote .PackageMirror }}
{{- end }}
users:
  - name: {{ .User }}
    sudo: ["ALL=(ALL) NOPASSWD:ALL"]
    shell: /bin/bash
    lock_passwd: true
    ssh_authorized_keys:
{{- range .AuthorizedKeys }}
      - {{ quote . }}
{{- end }}
{{- if .HostPrivateKey }}
# Pins the machine's SSH host key to one SC generated before the machine booted,
# which is the only way a deploy can verify the identity of a machine that did not
# exist when the config was written. See HostKeys on RenderCloudInit.
#
# cloud-init's ssh module treats a supplied ssh_keys as an ELSE to generating any
# (cc_ssh.py), so this leaves the machine with exactly one host key, of exactly the
# algorithm the client pins. That is deliberate: an extra ecdsa host key is an extra
# algorithm the handshake could negotiate and the pin could not cover.
ssh_keys:
  ed25519_private: |
{{ indent 4 .HostPrivateKey }}
  ed25519_public: {{ quote .HostPublicKey }}
{{- end }}
write_files:
  - path: /etc/docker/daemon.json
    permissions: "0644"
    content: |
      {{ .DaemonJSON }}
  - path: /etc/sysctl.d/99-sc.conf
    permissions: "0644"
    content: |
      net.ipv4.ip_forward=1
      net.bridge.bridge-nf-call-iptables=1
runcmd:
  - [bash, -c, {{ quote .InstallDocker }}]
  - [bash, -c, {{ quote .PostInstall }}]
  - [bash, -c, {{ quote .MarkDone }}]
`

type cloudInitData struct {
	User           string
	AuthorizedKeys []string
	HostPrivateKey string
	HostPublicKey  string
	PackageMirror  string
	DaemonJSON     string
	InstallDocker  string
	PostInstall    string
	MarkDone       string
}

var cloudInitTpl = template.Must(template.New("cloud-init").
	Funcs(template.FuncMap{"quote": strconv.Quote, "indent": indent}).
	Parse(cloudInitTemplate))

// HostKeys is the key material a machine SC creates is born with. Both halves of
// both pairs are generated inside the parent stack and persisted in its checkpoint,
// so they are stable across deploys — which matters because user-data is
// replace-on-change on every cloud: a key that moved would rebuild the machine.
type HostKeys struct {
	// DeployPublicKey is the public half of the key pair SC connects with (D10,
	// vm-matrix.md §3.3). Required: a machine with no authorised key is a machine SC
	// cannot deploy to, and discovering that after creation means destroying and
	// recreating it. The PRIVATE half never appears in user-data.
	DeployPublicKey string

	// HostPrivateKey / HostPublicKey pin the machine's own ed25519 host key, in
	// OpenSSH formats (`-----BEGIN OPENSSH PRIVATE KEY-----…` and
	// `ssh-ed25519 AAAA…`). Optional as a pair — omit both and the machine generates
	// its own, which no deploy can then verify.
	//
	// Why SC supplies them rather than reading the machine's key afterwards: no cloud
	// API returns a host key, so the alternatives are to trust the first connection
	// (no protection against anyone who can answer on that address before SC does) or
	// to make the operator paste `ssh-keyscan` output for a machine that does not
	// exist yet. Generating the key first is the only one of the three that verifies
	// anything.
	//
	// The private half does ride in user-data, which anything on the instance can
	// read from the metadata service and any console reader can read from the API.
	// That is accepted: both of those already imply control of the machine. What it
	// protects against is a DIFFERENT machine answering on the address — which is
	// exactly the deploy-time risk, since the deploy carries the deploy key and every
	// `${secret:...}` of every stack.
	HostPrivateKey string
	HostPublicKey  string
}

// RenderCloudInit builds the user-data for one machine.
func RenderCloudInit(cfg *VmConfig, role HostRole, keys HostKeys) (string, error) {
	if cfg == nil {
		return "", errors.Errorf("vm config must not be nil")
	}
	if cfg.Driver == DriverByo {
		// Not a limitation — a definition. The operator created the machine; SC has
		// no first boot to hook into. Readiness on a byo host is a precondition
		// check, not a wait (pkg/clouds/pulumi/vm/ready.go).
		return "", errors.Errorf("cloud-init is not rendered for `provider: %s`: the machine already booted", DriverByo)
	}
	if strings.TrimSpace(keys.DeployPublicKey) == "" {
		return "", errors.Errorf("cloud-init requires the deploy public key: a machine with no authorised key cannot be deployed to")
	}
	// Refused rather than half-applied: a private half with no public half leaves
	// cloud-init writing a host key whose .pub does not match it, and sshd then
	// refuses to start — which presents as a machine that never answers SSH at all.
	if (strings.TrimSpace(keys.HostPrivateKey) == "") != (strings.TrimSpace(keys.HostPublicKey) == "") {
		return "", errors.Errorf("cloud-init needs both halves of the host key, or neither")
	}
	if role == "" {
		return "", errors.Errorf("cloud-init requires a host role")
	}

	daemonJSON, err := DockerDaemonJSON(cfg.RegistryMirrors)
	if err != nil {
		return "", err
	}

	user := cfg.EffectiveSshUser()
	authorized := append([]string{strings.TrimSpace(keys.DeployPublicKey)}, cfg.ExtraSshKeys...)

	var out bytes.Buffer
	if err := cloudInitTpl.Execute(&out, cloudInitData{
		User:           user,
		AuthorizedKeys: lo.Compact(authorized),
		HostPrivateKey: strings.TrimRight(keys.HostPrivateKey, "\n"),
		HostPublicKey:  strings.TrimSpace(keys.HostPublicKey),
		PackageMirror:  strings.TrimSpace(cfg.PackageMirror),
		DaemonJSON:     daemonJSON,
		// Idempotent on purpose: an image that already ships docker (several RU
		// marketplace images do) must not be sent through the convenience script.
		InstallDocker: fmt.Sprintf("command -v docker >/dev/null 2>&1 || { %s; }", DefaultDockerInstallScript),
		PostInstall: strings.Join([]string{
			"set -e",
			"groupadd -f docker",
			fmt.Sprintf("usermod -aG docker %s", user),
			"systemctl enable --now docker",
			"sysctl --system",
			fmt.Sprintf("install -d -m 0750 -o %s -g %s %s %s", user, user, ConfigDir, StateDir),
		}, " && "),
		// Written last, and only if everything above succeeded, because this file
		// is the readiness contract: its presence means "ready for a deploy".
		MarkDone: fmt.Sprintf("touch %s", CloudInitDoneMarker),
	}); err != nil {
		return "", errors.Wrapf(err, "failed to render cloud-init for role %q", role)
	}
	return out.String(), nil
}

// indent prefixes every line with n spaces, for embedding a multi-line value under a
// YAML block scalar.
//
// Blank lines are left blank rather than padded with trailing spaces: an OpenSSH
// private key ends with a newline, and a line of whitespace inside a `|` block is
// content — it would become part of the key file and sshd would refuse to parse it.
func indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

// DockerDaemonJSON renders /etc/docker/daemon.json: the mandatory log caps plus any
// registry mirrors. Built through encoding/json rather than string concatenation so
// a mirror URL cannot produce a file the daemon refuses to parse — which it answers
// by not starting at all.
func DockerDaemonJSON(registryMirrors []string) (string, error) {
	daemon := map[string]any{
		"log-driver": "json-file",
		"log-opts":   dockerLogOpts,
	}
	if mirrors := lo.Compact(registryMirrors); len(mirrors) > 0 {
		daemon["registry-mirrors"] = mirrors
	}
	rendered, err := json.Marshal(daemon)
	if err != nil {
		return "", errors.Wrapf(err, "failed to render docker daemon.json")
	}
	return string(rendered), nil
}
