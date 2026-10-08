// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scopedstore

import (
	"os"
	"strings"
	"testing"

	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

func scopeArgs(a ...string) []string { return append([]string{"secrets", "scope"}, a...) }

// A scope file that is not parseable YAML (merge-conflict markers, truncation, empty)
// is classified as a look-alike: lint exits 0, deploy ignores it, allow/disallow skip it.
func TestScopeRegression_CorruptScopeFileFailsOpen(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	bob := newRSA(t)
	r.allow(t, ae, "pr", admin)
	r.allow(t, ae, "pr", bob)
	r.set(t, ae, "pr", "V", "1")
	p := r.stackFile("infra", "secrets.pr.yaml")
	orig, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte("<<<<<<< HEAD\n"+string(orig)+"=======\n"+string(orig)+">>>>>>> main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := r.sc(t, ae, "", scopeArgs("lint")...); err == nil {
		t.Errorf("lint exit 0 on a conflicted secrets.pr.yaml: %s", out)
	}
	if v, err := r.secretGet(t, ae, "V"); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Errorf("deploy read ignores the corrupt scope file (v=%q err=%v), want a hard integrity error", v, err)
	}
	if out, err := r.sc(t, ae, "", scopeArgs("disallow", "--scope", "pr", bob.pub)...); err == nil && strings.Contains(out, "resealed 0 file(s)") {
		t.Errorf("disallow silently skipped the corrupt scope file: %s", out)
	}
}

// The error from set/lint says to reconcile with allow/disallow, but allow of a
// recipient already in scopes.yaml (or disallow of one already absent) is a no-op.
func TestScopeRegression_AllowCannotReconcileDrift(t *testing.T) {
	admin, bob := newEd25519(t), newRSA(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	r.set(t, ae, "pr", "K1", "v1")
	sy := r.path(".sc/scopes.yaml")
	ss, _ := scoped.LoadScopes(sy)
	sp := ss.Scopes["pr"]
	sp.Recipients = append(sp.Recipients, bob.pub)
	ss.Scopes["pr"] = sp
	if err := ss.Save(sy); err != nil {
		t.Fatal(err)
	}
	r.sc(t, ae, "", scopeArgs("allow", "--scope", "pr", bob.pub)...)
	if out, err := r.sc(t, ae, "", scopeArgs("lint")...); err != nil {
		t.Errorf("drift (scopes.yaml has bob, file does not) not reconciled by `allow bob`: %v %s", err, out)
	}
}

// "-v" is also the root verbose flag. Omitting VALUE used to read stdin, so
// "set K -v" stored stdin. VALUE is now required; '--' lets it start with '-'.
func TestScopeRegression_ValueDashVSwallowed(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	if out, err := r.sc(t, ae, "FROM-STDIN", scopeArgs("set", "--scope", "pr", "-s", "infra", "DASH", "-v")...); err == nil || !strings.Contains(err.Error(), "VALUE is required") {
		t.Fatalf("set K -v must be refused, got %v: %s", err, out)
	}
	if out, err := r.sc(t, ae, "", scopeArgs("list", "--scope", "pr", "-s", "infra")...); err == nil && strings.Contains(out, "DASH") {
		t.Fatal("a refused set still stored the key")
	}
	r.mustSC(t, ae, "", scopeArgs("set", "--scope", "pr", "-s", "infra", "--", "DASH", "-v")...)
	if got := lastLine(r.mustSC(t, ae, "", scopeArgs("get", "--scope", "pr", "-s", "infra", "DASH")...)); got != "-v" {
		t.Errorf("stored value = %q, want -v", got)
	}
	r.mustSC(t, ae, "piped\n", scopeArgs("set", "--scope", "pr", "-s", "infra", "PIPED", "-")...)
	if got := lastLine(r.mustSC(t, ae, "", scopeArgs("get", "--scope", "pr", "-s", "infra", "PIPED")...)); got != "piped" {
		t.Errorf("stdin value = %q, want piped", got)
	}
}

// Stack names are never validated: a/b, .., . write files outside <stacksDir>/<stack>
// that no command can load afterwards.
func TestScopeRegression_StackNameNotValidated(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	for _, st := range []string{"a/b", "..", "."} {
		if out, err := r.sc(t, ae, "", scopeArgs("set", "--scope", "pr", "-s", st, "K", "v")...); err == nil {
			_, gerr := r.sc(t, ae, "", scopeArgs("get", "--scope", "pr", "-s", st, "K")...)
			t.Errorf("set -s %q accepted (%s) and the file is then unreadable: %v", st, strings.TrimSpace(out), gerr)
		}
	}
}

// allow stores the raw argument: leading spaces, CRLF, and a second key on another line.
func TestScopeRegression_RecipientNotNormalized(t *testing.T) {
	admin, bob, eve := newEd25519(t), newRSA(t), newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	r.sc(t, ae, "", scopeArgs("allow", "--scope", "pr", "  "+bob.pub+"\r\n")...)
	r.sc(t, ae, "", scopeArgs("allow", "--scope", "x", admin.pub+"\n"+eve.pub)...)
	ss, _ := scoped.LoadScopes(r.path(".sc/scopes.yaml"))
	for _, rc := range ss.Scopes["pr"].Recipients {
		if rc != strings.TrimSpace(rc) {
			t.Errorf("recipient stored with surrounding whitespace: %q", rc)
		}
	}
	for _, rc := range ss.Scopes["x"].Recipients {
		if strings.ContainsAny(rc, "\n\r") {
			t.Errorf("multi-line recipient accepted: scopes.yaml lists a second key (eve) that is never sealed to: %q", rc)
		}
	}
}

// list/delete do not validate --scope (get/set do): a traversal-shaped scope escapes the file naming.
func TestScopeRegression_ListDeleteScopeNotValidated(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	_, err := r.sc(t, ae, "", scopeArgs("list", "--scope", "../../../x", "-s", "infra")...)
	if err != nil && !strings.Contains(err.Error(), "invalid scope name") {
		t.Errorf("list --scope ../../../x reached the filesystem instead of being refused: %v", err)
	}
}

// A malformed SC_KEY_<SCOPE> shadows a valid ambient key in `get`, and the error blames
// recipiency rather than the unparseable key.
func TestScopeRegression_BadScopeKeyEnvShadowsAmbient(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	r.set(t, ae, "pr", "K", "v")
	env := adminEnv(admin, map[string]string{"SC_KEY_PR": "not a key"})
	out, err := r.sc(t, env, "", scopeArgs("get", "--scope", "pr", "-s", "infra", "K")...)
	if err != nil && !strings.Contains(err.Error(), "parse") {
		t.Errorf("get fails with a valid ambient key because SC_KEY_PR is junk, and says %q", err)
	}
	_ = out
}

// lint opens values but never validates `auth:` entries it can open; deploy then hard-fails.
func TestScopeRegression_LintMissesBrokenAuthEntry(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "prod", admin)
	r.set(t, ae, "prod", "P", "1")
	p := r.stackFile("infra", "secrets.prod.yaml")
	f, _ := scoped.LoadScopeFile(p)
	_ = f.Set("auth:broken", "type: [unclosed")
	_ = f.Save(p)
	if _, err := r.secretGet(t, ae, "P"); err == nil {
		t.Skip("deploy read tolerates it")
	}
	if out, err := r.sc(t, ae, "", scopeArgs("lint")...); err == nil {
		t.Errorf("lint passes though deploy-time resolution fails on auth:broken: %s", out)
	}
}
