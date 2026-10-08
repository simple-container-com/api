// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const reviewedScopes = `# Which keys open each secrets.<scope>.yaml.
# Edit with sc secrets scope allow / disallow.
schemaVersion: 1
scopes:
  # PR scan jobs
  pr:
    description: pull request scans
    recipients:
      - ssh-ed25519 AAAAbreakglass # break-glass, keep first
      - awskms://alias/pr?region=us-east-1
  alpha:
    recipients:
      - ssh-ed25519 AAAAalpha
`

func writeScopes(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ScopesFileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func saveAndRead(t *testing.T, s *Scopes, path string) string {
	t.Helper()
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// What allow does: one recipient more. Only that line may change.
func TestScopesSave_AddRecipientKeepsTheRestOfTheFile(t *testing.T) {
	path := writeScopes(t, reviewedScopes)
	s, err := LoadScopes(path)
	if err != nil {
		t.Fatal(err)
	}
	pr := s.Scopes["pr"]
	pr.Recipients = append(pr.Recipients, "gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/pr")
	s.Scopes["pr"] = pr

	got := saveAndRead(t, s, path)
	want := strings.Replace(reviewedScopes,
		"      - awskms://alias/pr?region=us-east-1\n",
		"      - awskms://alias/pr?region=us-east-1\n      - gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/pr\n", 1)
	if got != want {
		t.Fatalf("file changed beyond the added recipient:\n--- got\n%s--- want\n%s", got, want)
	}
}

// What disallow does: one recipient fewer. The comment on another stays.
func TestScopesSave_RemoveRecipientKeepsTheRestOfTheFile(t *testing.T) {
	path := writeScopes(t, reviewedScopes)
	s, _ := LoadScopes(path)
	pr := s.Scopes["pr"]
	pr.Recipients = []string{"ssh-ed25519 AAAAbreakglass"}
	s.Scopes["pr"] = pr

	got := saveAndRead(t, s, path)
	want := strings.Replace(reviewedScopes, "      - awskms://alias/pr?region=us-east-1\n", "", 1)
	if got != want {
		t.Fatalf("--- got\n%s--- want\n%s", got, want)
	}
}

// A new scope goes at the end; existing scopes keep their (non-alphabetical) order.
func TestScopesSave_NewScopeIsAppended(t *testing.T) {
	path := writeScopes(t, reviewedScopes)
	s, _ := LoadScopes(path)
	s.Scopes["zeta"] = Scope{Recipients: []string{"ssh-ed25519 AAAAz"}}
	s.Scopes["beta"] = Scope{Description: "b", Recipients: []string{"ssh-ed25519 AAAAb"}}

	got := saveAndRead(t, s, path)
	want := reviewedScopes + "  beta:\n    description: b\n    recipients:\n      - ssh-ed25519 AAAAb\n" +
		"  zeta:\n    recipients:\n      - ssh-ed25519 AAAAz\n"
	if got != want {
		t.Fatalf("--- got\n%s--- want\n%s", got, want)
	}
}

func TestScopesSave_RemovedScopeDisappears(t *testing.T) {
	path := writeScopes(t, reviewedScopes)
	s, _ := LoadScopes(path)
	delete(s.Scopes, "alpha")
	got := saveAndRead(t, s, path)
	want := strings.Replace(reviewedScopes, "  alpha:\n    recipients:\n      - ssh-ed25519 AAAAalpha\n", "", 1)
	if got != want {
		t.Fatalf("--- got\n%s--- want\n%s", got, want)
	}
}

func TestScopesSave_Description(t *testing.T) {
	path := writeScopes(t, reviewedScopes)
	s, _ := LoadScopes(path)
	pr, alpha := s.Scopes["pr"], s.Scopes["alpha"]
	pr.Description = ""
	alpha.Description = "now described"
	s.Scopes["pr"], s.Scopes["alpha"] = pr, alpha

	got := saveAndRead(t, s, path)
	if strings.Contains(got, "pull request scans") {
		t.Error("removed description is still there")
	}
	if !strings.Contains(got, "  alpha:\n    recipients:\n      - ssh-ed25519 AAAAalpha\n    description: now described\n") {
		t.Errorf("added description missing:\n%s", got)
	}
	back, err := LoadScopes(path)
	if err != nil || back.Scopes["alpha"].Description != "now described" || back.Scopes["pr"].Description != "" {
		t.Fatalf("reload: %+v, %v", back, err)
	}
}

// Keys this build does not know are someone's data, not noise.
func TestScopesSave_KeepsUnknownKeys(t *testing.T) {
	path := writeScopes(t, "schemaVersion: 1\nowner: platform-team\nscopes:\n  pr:\n    reviewers: [alice]\n    recipients:\n      - ssh-ed25519 AAAAa\n")
	s, _ := LoadScopes(path)
	pr := s.Scopes["pr"]
	pr.Recipients = append(pr.Recipients, "ssh-ed25519 AAAAb")
	s.Scopes["pr"] = pr
	got := saveAndRead(t, s, path)
	for _, want := range []string{"owner: platform-team", "reviewers: [alice]", "      - ssh-ed25519 AAAAb\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A flow-style list is rewritten as a block list rather than an unreadable line.
func TestScopesSave_FlowRecipientsBecomeBlock(t *testing.T) {
	path := writeScopes(t, "schemaVersion: 1\nscopes:\n  pr:\n    recipients: [ssh-ed25519 AAAAa]\n")
	s, _ := LoadScopes(path)
	pr := s.Scopes["pr"]
	pr.Recipients = append(pr.Recipients, "ssh-ed25519 AAAAb")
	s.Scopes["pr"] = pr
	got := saveAndRead(t, s, path)
	if !strings.Contains(got, "    recipients:\n      - ssh-ed25519 AAAAa\n      - ssh-ed25519 AAAAb\n") {
		t.Errorf("got:\n%s", got)
	}
}

func TestScopesSave_NewFileUsesTwoSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), ScopesFileName)
	s := &Scopes{Scopes: map[string]Scope{"pr": {Description: "d", Recipients: []string{"ssh-ed25519 AAAAa"}}}}
	got := saveAndRead(t, s, path)
	want := "schemaVersion: 1\nscopes:\n  pr:\n    description: d\n    recipients:\n      - ssh-ed25519 AAAAa\n"
	if got != want {
		t.Fatalf("--- got\n%s--- want\n%s", got, want)
	}
}

// A missing or empty file is written fresh.
func TestScopesSave_EmptyExistingFileIsWrittenFresh(t *testing.T) {
	for _, content := range []string{"", "# only a comment\n", "\n"} {
		path := writeScopes(t, content)
		s := &Scopes{Scopes: map[string]Scope{"pr": {Recipients: []string{"ssh-ed25519 AAAAa"}}}}
		if err := s.Save(path); err != nil {
			t.Fatalf("%q: %v", content, err)
		}
		back, err := LoadScopes(path)
		if err != nil || !reflect.DeepEqual(back.Scopes, s.Scopes) {
			t.Errorf("%q: reload %+v, %v", content, back, err)
		}
	}
}

// An existing file Save cannot edit safely is an error and stays untouched: a
// reviewed governance file is never replaced wholesale.
func TestScopesSave_RefusesFilesItCannotEditSafely(t *testing.T) {
	s := &Scopes{Scopes: map[string]Scope{"pr": {Recipients: []string{"ssh-ed25519 AAAAa"}}}}
	for name, content := range map[string]string{
		"unparseable": "scopes: [",
		"a list":      "- a list\n",
		"anchor":      "schemaVersion: 1\nbase: &keys\n  - ssh-ed25519 AAAAa\nscopes:\n  pr:\n    recipients: *keys\n",
		"merge key":   "schemaVersion: 1\ndefaults: &d\n  recipients: [ssh-ed25519 AAAAa]\nscopes:\n  pr:\n    <<: *d\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := writeScopes(t, content)
			if err := s.Save(path); err == nil {
				t.Fatal("Save edited a file it cannot edit safely")
			}
			if data, _ := os.ReadFile(path); string(data) != content {
				t.Errorf("file changed:\n%s", data)
			}
		})
	}
	if os.Geteuid() != 0 {
		path := writeScopes(t, reviewedScopes)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
		if err := s.Save(path); err == nil || !strings.Contains(err.Error(), "failed to read") {
			t.Errorf("unreadable file: %v", err)
		}
	}
}

// A description that did not change keeps its style: block scalars and quoting
// are the reviewer's, and re-styling them is diff noise.
func TestScopesSave_UnchangedDescriptionKeepsItsStyle(t *testing.T) {
	content := "schemaVersion: 1\nscopes:\n  pr:\n    description: |\n      line one\n      line two\n    recipients:\n      - ssh-ed25519 AAAAa\n"
	path := writeScopes(t, content)
	s, err := LoadScopes(path)
	if err != nil {
		t.Fatal(err)
	}
	pr := s.Scopes["pr"]
	pr.Recipients = append(pr.Recipients, "ssh-ed25519 AAAAb")
	s.Scopes["pr"] = pr
	got := saveAndRead(t, s, path)
	if want := content + "      - ssh-ed25519 AAAAb\n"; got != want {
		t.Fatalf("--- got\n%s--- want\n%s", got, want)
	}
}

// Any sequence of the edits allow and disallow make must leave a file that
// loads back to exactly the saved set, keeps the header comment, and changes no
// recipient that the edit did not touch.
func TestScopesSave_RandomEditsRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	path := writeScopes(t, reviewedScopes)
	pool := []string{"ssh-ed25519 AAAA1", "ssh-ed25519 AAAA2", "awskms://alias/x?region=eu-west-1", "gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/k"}
	names := []string{"pr", "alpha", "prod", "staging"}
	for step := 0; step < 200; step++ {
		s, err := LoadScopes(path)
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		name := names[rng.Intn(len(names))]
		sc := s.Scopes[name]
		switch r := pool[rng.Intn(len(pool))]; rng.Intn(4) {
		case 0, 1:
			if !contains(sc.Recipients, r) {
				sc.Recipients = append(sc.Recipients, r)
			}
			s.Scopes[name] = sc
		case 2:
			sc.Recipients = without(sc.Recipients, r)
			if len(sc.Recipients) == 0 {
				delete(s.Scopes, name)
			} else {
				s.Scopes[name] = sc
			}
		case 3:
			if len(sc.Recipients) > 0 {
				sc.Description = []string{"", "a", "b: c", "multi\nline"}[rng.Intn(4)]
				s.Scopes[name] = sc
			}
		}
		got := saveAndRead(t, s, path)
		if !strings.HasPrefix(got, "# Which keys open each secrets.<scope>.yaml.\n") {
			t.Fatalf("step %d: header comment lost:\n%s", step, got)
		}
		back, err := LoadScopes(path)
		if err != nil {
			t.Fatalf("step %d: reload: %v\n%s", step, err, got)
		}
		if !sameScopes(back.Scopes, s.Scopes) {
			t.Fatalf("step %d: reload differs\n got: %+v\nwant: %+v\n%s", step, back.Scopes, s.Scopes, got)
		}
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func without(list []string, v string) []string {
	out := []string{}
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func sameScopes(a, b map[string]Scope) bool {
	if len(a) != len(b) {
		return false
	}
	for name, sa := range a {
		sb, ok := b[name]
		if !ok || sa.Description != sb.Description {
			return false
		}
		ra, rb := append([]string{}, sa.Recipients...), append([]string{}, sb.Recipients...)
		sort.Strings(ra)
		sort.Strings(rb)
		if !reflect.DeepEqual(ra, rb) {
			return false
		}
	}
	return true
}
