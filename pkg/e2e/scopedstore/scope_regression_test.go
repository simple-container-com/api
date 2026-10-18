// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scopedstore

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

func scopeArgs(a ...string) []string { return append([]string{"secrets", "scope"}, a...) }

// A scope file with merge-conflict markers fails lint, the deploy read and resealing.
func TestScopeRegression_DamagedScopeFileFailsLintDeployAndReseal(t *testing.T) {
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
	if out, err := r.sc(t, ae, "", scopeArgs("disallow", "--scope", "pr", bob.pub)...); err == nil {
		t.Errorf("disallow succeeded over a damaged scope file: %s", out)
	}
}

// allow of a recipient scopes.yaml already lists reseals the files that drifted
// from it, which is what lint and set tell the operator to do.
func TestScopeRegression_AllowReconcilesDrift(t *testing.T) {
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
	if out, err := r.sc(t, ae, "", scopeArgs("allow", "--scope", "pr", bob.pub)...); err != nil || !strings.Contains(out, "resealed 1 file(s)") {
		t.Fatalf("allow did not reseal the drifted file: %v %s", err, out)
	}
	if out, err := r.sc(t, ae, "", scopeArgs("lint")...); err != nil {
		t.Errorf("drift (scopes.yaml has bob, file does not) not reconciled by `allow bob`: %v %s", err, out)
	}
}

// "-v" is also the root verbose flag. Omitting VALUE used to read stdin, so
// "set K -v" stored stdin. VALUE is now required; '--' lets it start with '-'.
func TestScopeRegression_SetNeedsAnExplicitValue(t *testing.T) {
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

// A stack name is one directory under the stacks dir: a/b, .. and . are refused.
func TestScopeRegression_StackNameIsValidated(t *testing.T) {
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

// allow trims a recipient and refuses one that spans several lines.
func TestScopeRegression_RecipientIsNormalized(t *testing.T) {
	admin, bob, eve := newEd25519(t), newRSA(t), newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	if out, err := r.sc(t, ae, "", scopeArgs("allow", "--scope", "pr", "  "+bob.pub+"\r\n")...); err != nil {
		t.Fatalf("allow with surrounding whitespace failed: %v\n%s", err, out)
	}
	if _, err := r.sc(t, ae, "", scopeArgs("allow", "--scope", "x", admin.pub+"\n"+eve.pub)...); err == nil {
		t.Errorf("allow accepted a recipient that spans two lines")
	}
	ss, err := scoped.LoadScopes(r.path(".sc/scopes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ss.Scopes["pr"].Recipients, bob.pub) {
		t.Errorf("trimmed recipient not stored: %q", ss.Scopes["pr"].Recipients)
	}
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

// list and delete refuse a scope name shaped like a path.
func TestScopeRegression_ListDeleteValidateScope(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	for _, verb := range [][]string{{"list"}, {"delete", "K"}} {
		args := append([]string{verb[0], "--scope", "../../../x", "-s", "infra"}, verb[1:]...)
		if _, err := r.sc(t, ae, "", scopeArgs(args...)...); err == nil || !strings.Contains(err.Error(), "invalid scope name") {
			t.Errorf("%s --scope ../../../x: %v; want invalid scope name", verb[0], err)
		}
	}
}

// A malformed SC_KEY_<SCOPE> is reported as a key that cannot be parsed, naming it.
func TestScopeRegression_UnparseableScopeKeyIsReported(t *testing.T) {
	admin := newEd25519(t)
	r := newRepo(t)
	ae := adminEnv(admin, nil)
	r.allow(t, ae, "pr", admin)
	r.set(t, ae, "pr", "K", "v")
	env := adminEnv(admin, map[string]string{"SC_KEY_PR": "not a key"})
	_, err := r.sc(t, env, "", scopeArgs("get", "--scope", "pr", "-s", "infra", "K")...)
	if err == nil || !strings.Contains(err.Error(), "SC_KEY_PR") || !strings.Contains(err.Error(), "cannot be parsed") {
		t.Errorf("get with a junk SC_KEY_PR: %v; want it named as unparseable", err)
	}
}

// lint parses the auth: entries it can open, since a broken one fails every deploy.
func TestScopeRegression_LintCatchesBrokenAuthEntry(t *testing.T) {
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
		t.Fatal("the deploy read accepted a broken auth entry; lint must keep matching it")
	}
	if out, err := r.sc(t, ae, "", scopeArgs("lint")...); err == nil {
		t.Errorf("lint passes though deploy-time resolution fails on auth:broken: %s", out)
	}
}
