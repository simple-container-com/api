// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

// The runtime (Deployment) proxy must expose its built-in health server so a startup
// probe can gate the app containers -- otherwise the app dials localhost:5432 before the
// proxy is listening and logs connection-refused on every pod (re)start.
func TestCloudsqlProxyCommandArgs_RuntimeEnablesHealthCheck(t *testing.T) {
	cmd, args := cloudsqlProxyCommandArgs("proj", "europe-north1", "inst", 0)

	assert.Equal(t, "/cloud-sql-proxy", cmd, "runtime proxy runs the binary directly (no shell wrapper)")
	assert.Contains(t, args, "--health-check", "runtime proxy must expose its health server for the startup probe")
	assert.Contains(t, args, "--http-address=0.0.0.0", "health server must bind 0.0.0.0 so kubelet probes can reach it")
	assert.Contains(t, args, "--http-port=9090")
	assert.Contains(t, args, "proj:europe-north1:inst", "instance connection name must be preserved")
	assert.NotContains(t, args, "-c", "runtime proxy must not be wrapped in a self-killing shell")
	// Base flags moved from the old inline builder must survive the refactor. --credentials-file
	// is the only flag that authenticates the proxy; it must equal the VolumeMount path +
	// /credentials.json, so a future mount-path change forces this flag to move in lockstep.
	assert.Contains(t, args, "--address")
	assert.Contains(t, args, "0.0.0.0")
	assert.Contains(t, args, "--credentials-file=/var/run/secrets/cloudsql/credentials.json",
		"credentials flag must match the mounted secret path")
}

// The init-Job proxy runs in a RestartPolicy: Never pod; it must self-terminate or the
// Job never completes. It must stay shell-wrapped and must NOT enable the health server.
func TestCloudsqlProxyCommandArgs_InitJobSelfKills(t *testing.T) {
	cmd, args := cloudsqlProxyCommandArgs("proj", "europe-north1", "inst", 30)

	assert.Equal(t, "sh", cmd, "init-Job proxy must be shell-wrapped so it can self-terminate")
	require.GreaterOrEqual(t, len(args), 2)
	assert.Equal(t, "-c", args[0])
	script := args[1]
	assert.Contains(t, script, "kill -9", "init-Job proxy must self-kill so the Job completes")
	assert.Contains(t, script, "proj:europe-north1:inst")
	assert.Contains(t, script, "--credentials-file=/var/run/secrets/cloudsql/credentials.json",
		"init-Job proxy still authenticates via the mounted secret")
	assert.NotContains(t, script, "--health-check", "init-Job proxy must not run a health server")
}

// Runtime proxy => native sidecar: an init container with RestartPolicy: Always plus
// startup/readiness/liveness probes. This is what eliminates the startup race and lets a
// hung-but-alive proxy self-heal.
func TestCloudsqlProxyContainerArgs_RuntimeIsNativeSidecar(t *testing.T) {
	c := cloudsqlProxyContainerArgs("creds", "proj", "reg", "inst", 0)

	assert.Equal(t, sdk.String("Always"), c.RestartPolicy,
		"runtime proxy must be a native sidecar (init container, RestartPolicy: Always)")
	assert.NotNil(t, c.StartupProbe, "startup probe must gate app containers until the proxy is listening")
	assert.NotNil(t, c.ReadinessProbe, "readiness probe keeps the pod out of rotation until the DB path is up")
	assert.NotNil(t, c.LivenessProbe, "liveness probe restarts a proxy that passed startup then hung")
	assert.NotNil(t, c.Ports, "health-check port must be declared for the probes to target")
}

// Init-Job proxy must NOT be a native sidecar: RestartPolicy: Always on a Job's container
// would keep the Job from ever completing, and it serves no health endpoints.
func TestCloudsqlProxyContainerArgs_InitJobIsNotSidecar(t *testing.T) {
	c := cloudsqlProxyContainerArgs("creds", "proj", "reg", "inst", 30)

	assert.Nil(t, c.RestartPolicy,
		"init-Job proxy must not carry RestartPolicy: Always -- it would hang the Job")
	assert.Nil(t, c.StartupProbe, "init-Job proxy has no health server, so no probe")
	assert.Nil(t, c.ReadinessProbe)
	assert.Nil(t, c.LivenessProbe)
	assert.Nil(t, c.Ports)
}

// Pin the exact probe<->port wiring. A NotNil check would pass even with a port-name typo
// or a wrong probe path -- a probe the kubelet can never satisfy, which loops the pod in
// Init then restarts it. The agreement assertions (probe Port == declared port Name) are
// what actually defend the named-port linkage the whole sidecar depends on.
func TestCloudsqlProxyContainerArgs_RuntimeProbeWiring(t *testing.T) {
	c := cloudsqlProxyContainerArgs("creds", "proj", "reg", "inst", 0)

	ports := c.Ports.(v1.ContainerPortArray)
	require.Len(t, ports, 1)
	p0 := ports[0].(*v1.ContainerPortArgs)
	assert.Equal(t, sdk.Int(9090), p0.ContainerPort)
	assert.Equal(t, sdk.String("csql-hc"), p0.Name)

	sp := c.StartupProbe.(*v1.ProbeArgs)
	spHG := sp.HttpGet.(v1.HTTPGetActionArgs)
	assert.Equal(t, sdk.String("/startup"), spHG.Path)
	assert.Equal(t, sdk.IntPtr(30), sp.FailureThreshold, "startup budget ~= period(2s) x 30 = 60s")
	assert.Equal(t, p0.Name, spHG.Port, "startup probe must target the declared health port by name")

	rp := c.ReadinessProbe.(*v1.ProbeArgs)
	rpHG := rp.HttpGet.(v1.HTTPGetActionArgs)
	assert.Equal(t, sdk.String("/readiness"), rpHG.Path)
	assert.Equal(t, p0.Name, rpHG.Port, "readiness probe must target the declared health port by name")

	lp := c.LivenessProbe.(*v1.ProbeArgs)
	lpHG := lp.HttpGet.(v1.HTTPGetActionArgs)
	assert.Equal(t, sdk.String("/liveness"), lpHG.Path)
	assert.Equal(t, p0.Name, lpHG.Port, "liveness probe must target the declared health port by name")
}

// The proxy mounts its credential Secret at a fixed path; the mount Name must equal the
// secret name (the credential Volume that compute_proc.go appends derives from the same
// Metadata.Name(), so a drift breaks `--credentials-file` auth).
func TestCloudsqlProxyContainerArgs_MountsCredentialSecret(t *testing.T) {
	c := cloudsqlProxyContainerArgs("creds", "proj", "reg", "inst", 0)

	mounts := c.VolumeMounts.(v1.VolumeMountArray)
	require.Len(t, mounts, 1)
	m := mounts[0].(*v1.VolumeMountArgs)
	assert.Equal(t, sdk.String("creds"), m.Name, "mount name must equal the secret name")
	assert.Equal(t, sdk.String("/var/run/secrets/cloudsql"), m.MountPath)
	assert.Equal(t, sdk.Bool(true), m.ReadOnly)
}

// The load-bearing wiring: the runtime proxy MUST be attached as a native sidecar (init
// container), never a regular container. RestartPolicy: Always on a regular container is
// rejected by the API server, and only the init-container placement gives the startup-probe
// ordering that removes the connection-refused race. Guards against a future refactor
// silently appending to SidecarOutputs.
func TestAttachCloudsqlProxyAsNativeSidecar_LandsInInitContainers(t *testing.T) {
	kubeArgs := &kubernetes.SimpleContainerArgs{}
	var proxy v1.ContainerOutput
	var vol v1.VolumeOutput

	attachCloudsqlProxyAsNativeSidecar(kubeArgs, proxy, vol)

	assert.Len(t, kubeArgs.InitContainerOutputs, 1, "proxy must be a native sidecar (init container)")
	assert.Empty(t, kubeArgs.SidecarOutputs, "proxy must NOT land in regular containers")
	assert.Len(t, kubeArgs.VolumeOutputs, 1, "credential secret volume must ride along")
}
