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
// about it (roadmap item sc-ru-market-readiness, B0). Where it is unreliable, the
// answer is PackageMirror, not a retry loop.
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
users:
  - name: {{ .User }}
    sudo: ["ALL=(ALL) NOPASSWD:ALL"]
    shell: /bin/bash
    lock_passwd: true
    ssh_authorized_keys:
{{- range .AuthorizedKeys }}
      - {{ quote . }}
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
	DaemonJSON     string
	InstallDocker  string
	PostInstall    string
	MarkDone       string
}

var cloudInitTpl = template.Must(template.New("cloud-init").
	Funcs(template.FuncMap{"quote": strconv.Quote}).
	Parse(cloudInitTemplate))

// RenderCloudInit builds the user-data for one machine.
//
// deployPublicKey is the public half of the key pair SC generates inside the parent
// stack (D10, vm-matrix.md §3.3). It is required: a machine with no authorised key
// is a machine SC cannot deploy to, and discovering that after creation means
// destroying and recreating it.
func RenderCloudInit(cfg *VmConfig, role HostRole, deployPublicKey string) (string, error) {
	if cfg == nil {
		return "", errors.Errorf("vm config must not be nil")
	}
	if cfg.Driver == DriverByo {
		// Not a limitation — a definition. The operator created the machine; SC has
		// no first boot to hook into. Readiness on a byo host is a precondition
		// check, not a wait (pkg/clouds/pulumi/vm/ready.go).
		return "", errors.Errorf("cloud-init is not rendered for `provider: %s`: the machine already booted", DriverByo)
	}
	if strings.TrimSpace(deployPublicKey) == "" {
		return "", errors.Errorf("cloud-init requires the deploy public key: a machine with no authorised key cannot be deployed to")
	}
	if role == "" {
		return "", errors.Errorf("cloud-init requires a host role")
	}

	daemonJSON, err := DockerDaemonJSON(cfg.RegistryMirrors)
	if err != nil {
		return "", err
	}

	user := cfg.EffectiveSshUser()
	keys := append([]string{strings.TrimSpace(deployPublicKey)}, cfg.ExtraSshKeys...)

	var out bytes.Buffer
	if err := cloudInitTpl.Execute(&out, cloudInitData{
		User:           user,
		AuthorizedKeys: lo.Compact(keys),
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
