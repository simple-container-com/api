// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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
	assert.NotContains(t, script, "--health-check", "init-Job proxy must not run a health server")
}

// Runtime proxy => native sidecar: an init container with RestartPolicy: Always plus a
// startup probe. This is what eliminates the startup race.
func TestCloudsqlProxyContainerArgs_RuntimeIsNativeSidecar(t *testing.T) {
	c := cloudsqlProxyContainerArgs("creds", "proj", "reg", "inst", 0)

	assert.Equal(t, sdk.String("Always"), c.RestartPolicy,
		"runtime proxy must be a native sidecar (init container, RestartPolicy: Always)")
	assert.NotNil(t, c.StartupProbe, "startup probe must gate app containers until the proxy is listening")
	assert.NotNil(t, c.ReadinessProbe, "readiness probe keeps the pod out of rotation until the DB path is up")
	assert.NotNil(t, c.Ports, "health-check port must be declared for the probes to target")
}

// Init-Job proxy must NOT be a native sidecar: RestartPolicy: Always on a Job's container
// would keep the Job from ever completing.
func TestCloudsqlProxyContainerArgs_InitJobIsNotSidecar(t *testing.T) {
	c := cloudsqlProxyContainerArgs("creds", "proj", "reg", "inst", 30)

	assert.Nil(t, c.RestartPolicy,
		"init-Job proxy must not carry RestartPolicy: Always -- it would hang the Job")
	assert.Nil(t, c.StartupProbe, "init-Job proxy has no health server, so no probe")
	assert.Nil(t, c.Ports)
}
