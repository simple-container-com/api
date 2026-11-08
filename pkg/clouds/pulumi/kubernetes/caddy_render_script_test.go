// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// The Caddy init container's whole job is shell, so these tests run the real
// caddyfileRenderScript against a fake kubectl. They cover the rollout race fixed in
// this commit: a pod rolled by a caddy-update-hash patch that lands before the Service
// annotation it hashed is visible renders the PREVIOUS config and serves it forever.

const (
	staleEntry = "\nhttp://smoke.example.com {\n  reverse_proxy http://app.ns.svc.cluster.local:8080\n}\n"
	freshEntry = "\nhttps://smoke.example.com {\n  reverse_proxy http://app.ns.svc.cluster.local:8080\n}\n"
)

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// renderHarness writes a fake kubectl that serves one Service (ns/app) whose
// caddyfile-entry annotation is whatever `bodies` says for that call number: call i uses
// bodies[min(i, len(bodies)-1)], so a single body is a stable cluster and two bodies
// model the annotation becoming visible on the second look.
type renderHarness struct {
	dir     string
	workDir string
	env     []string
}

func newRenderHarness(t *testing.T, g *WithT, bodies ...string) *renderHarness {
	t.Helper()
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	g.Expect(os.Mkdir(workDir, 0o755)).To(Succeed())

	// The embedded base config the init container copies first.
	srcCaddyfile := filepath.Join(dir, "base-Caddyfile")
	g.Expect(os.WriteFile(srcCaddyfile, []byte("# base config\n"), 0o644)).To(Succeed())

	for i, body := range bodies {
		g.Expect(os.WriteFile(filepath.Join(dir, fmt.Sprintf("body-%d", i)), []byte(body), 0o644)).To(Succeed())
	}

	// The body is `cat`ed straight out, never through a $(...) capture: the annotation
	// ends with a newline, kubectl -o jsonpath appends nothing, and command substitution
	// would strip it — which is precisely the byte difference that would make the
	// script's md5 never match. Getting this wrong in the fake made the first run of
	// this test fail in exactly the way a real cluster would not.
	binDir := filepath.Join(dir, "bin")
	g.Expect(os.Mkdir(binDir, 0o755)).To(Succeed())
	fakeKubectl := fmt.Sprintf(`#!/usr/bin/env bash
set -eu
DIR=%q
MAX=%d
for arg in "$@"; do
  if [ "$arg" == "--all-namespaces" ]; then
    printf '%%s\n' "2026-11-08T16:16:30Z ns app"
    exit 0
  fi
done
# Per-service annotation read. Advance a call counter so successive renders can see
# different cluster state.
n=0
if [ -f "$DIR/calls" ]; then n=$(cat "$DIR/calls"); fi
echo $((n + 1)) > "$DIR/calls"
if [ "$n" -ge "$MAX" ]; then n=$((MAX - 1)); fi
cat "$DIR/body-$n"
`, dir, len(bodies))
	g.Expect(os.WriteFile(filepath.Join(binDir, "kubectl"), []byte(fakeKubectl), 0o755)).To(Succeed())

	return &renderHarness{
		dir:     dir,
		workDir: workDir,
		env: []string{
			"PATH=" + binDir + ":" + os.Getenv("PATH"),
			"CADDY_SRC_CADDYFILE=" + srcCaddyfile,
			"CADDY_WORK_DIR=" + workDir,
			"CADDY_SYNC_INTERVAL=0",
			"CADDY_SYNC_ATTEMPTS=3",
			"USE_PREFIXES=false",
			"DEFAULT_ENTRY_START=http:// {",
			"DEFAULT_ENTRY=\n  respond 503\n",
			"CADDYFILE_PREFIX=",
		},
	}
}

// run executes the production script and returns its combined output plus the rendered
// Caddyfile.
func (h *renderHarness) run(t *testing.T, g *WithT, updateHash string) (string, string) {
	t.Helper()
	cmd := exec.Command("bash", "-c", caddyfileRenderScript)
	cmd.Env = append(append([]string{}, h.env...), "CADDY_UPDATE_HASH="+updateHash)
	out, err := cmd.CombinedOutput()
	g.Expect(err).ToNot(HaveOccurred(), "script failed: %s", string(out))

	rendered, readErr := os.ReadFile(filepath.Join(h.workDir, "Caddyfile"))
	g.Expect(readErr).ToNot(HaveOccurred())
	return string(out), string(rendered)
}

func TestCaddyfileRenderScript(t *testing.T) {
	g := NewWithT(t)

	t.Run("renders immediately when the rolled-for hash is already visible", func(t *testing.T) {
		h := newRenderHarness(t, g, freshEntry)
		out, rendered := h.run(t, g, md5Hex(freshEntry))

		g.Expect(rendered).To(ContainSubstring("https://smoke.example.com"))
		g.Expect(rendered).To(ContainSubstring("# base config"))
		// No retry: the hash matched on the first render. This also proves md5 is taken
		// over the annotation's exact bytes — a stripped trailing newline would not match.
		g.Expect(out).ToNot(ContainSubstring("retrying"))
	})

	t.Run("waits for a stale annotation to become visible, then renders the new config", func(t *testing.T) {
		// The measured bug: the pod is rolled for freshEntry but the Service still
		// serves staleEntry on the first look.
		h := newRenderHarness(t, g, staleEntry, freshEntry)
		out, rendered := h.run(t, g, md5Hex(freshEntry))

		g.Expect(rendered).To(ContainSubstring("https://smoke.example.com"))
		g.Expect(rendered).ToNot(ContainSubstring("http://smoke.example.com"),
			"the stale site block must not survive into the rendered Caddyfile")
		g.Expect(out).To(ContainSubstring("retrying (1/3)"))
	})

	t.Run("does not wait at all when no hash is set", func(t *testing.T) {
		// A Caddy no app has ever patched: the downward API resolves the missing
		// annotation to an empty string, and there is nothing to wait for.
		h := newRenderHarness(t, g, staleEntry)
		out, rendered := h.run(t, g, "")

		g.Expect(rendered).To(ContainSubstring("http://smoke.example.com"))
		g.Expect(out).ToNot(ContainSubstring("retrying"))
	})

	t.Run("fails open when the hash can never match", func(t *testing.T) {
		// Legitimate states that must not cost readiness: the app that set the hash has
		// been deleted, or a migrated stack whose Service annotation is re-rendered
		// against the legacy namespace while the hash is taken over the static render.
		h := newRenderHarness(t, g, staleEntry)
		out, rendered := h.run(t, g, md5Hex("an entry no Service carries"))

		g.Expect(out).To(ContainSubstring("WARNING"))
		g.Expect(out).To(ContainSubstring("rendering from current cluster state anyway"))
		// Exactly what today's code would have produced.
		g.Expect(rendered).To(ContainSubstring("http://smoke.example.com"))
		g.Expect(strings.Count(out, "retrying")).To(Equal(2), "should retry attempts-1 times before giving up")
	})

	t.Run("promotes the rendered file atomically", func(t *testing.T) {
		h := newRenderHarness(t, g, freshEntry)
		_, _ = h.run(t, g, md5Hex(freshEntry))

		// Caddy mounts this directory; a leftover partial render would be a second
		// Caddyfile-shaped file sitting next to the live one.
		_, err := os.Stat(filepath.Join(h.workDir, "Caddyfile.rendering"))
		g.Expect(os.IsNotExist(err)).To(BeTrue(), "Caddyfile.rendering should have been moved, not copied")
	})

	t.Run("dedups two Services claiming the same site address", func(t *testing.T) {
		// Pre-existing behaviour, asserted here because the render was restructured into
		// a function that runs more than once: the dedup state must be rebuilt per
		// render, or a second attempt would drop every entry as already-seen.
		h := newRenderHarness(t, g, staleEntry, freshEntry)
		_, rendered := h.run(t, g, md5Hex(freshEntry))

		g.Expect(strings.Count(rendered, "smoke.example.com")).To(Equal(1))
	})
}
