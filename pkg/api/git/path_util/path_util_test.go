// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package path_util

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestReplaceTildeWithHome_NoTilde(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty path", ""},
		{"absolute path", "/var/log/app.log"},
		{"relative path", "./config.yaml"},
		{"parent traversal", "../foo/bar"},
		{"path containing tilde mid-string", "/etc/some~thing/cfg"},
		{"bare tilde without slash", "~just-a-username-fragment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := ReplaceTildeWithHome(tc.in)
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.in))
		})
	}
}

func TestReplaceTildeWithHome_CurrentUser(t *testing.T) {
	RegisterTestingT(t)

	home, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred(), "test prereq: HOME must resolve")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"~ alone (no trailing /) is not expanded", "~", "~"}, // doc: only "~/" prefix triggers expansion
		{"~/", "~/", home + "/"},
		{"~/.config/app.yaml", "~/.config/app.yaml", filepath.Join(home, ".config/app.yaml")},
		{"~/nested/deep/file", "~/nested/deep/file", filepath.Join(home, "nested/deep/file")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := ReplaceTildeWithHome(tc.in)
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestReplaceTildeWithHome_NamedUser(t *testing.T) {
	RegisterTestingT(t)

	current, err := user.Current()
	Expect(err).ToNot(HaveOccurred(), "test prereq: current user must resolve")

	// Use the current user's username — we know it exists on this system.
	// Verifies the ~user/ expansion path through user.Lookup.
	in := "~" + current.Username + "/some/file"
	want := current.HomeDir + "/some/file"

	got, err := ReplaceTildeWithHome(in)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal(want))
}

func TestReplaceTildeWithHome_UnknownUserReturnsError(t *testing.T) {
	RegisterTestingT(t)

	// A username that should not exist on any sane test system.
	in := "~no-such-user-deadbeef-9c4f/some/file"
	got, err := ReplaceTildeWithHome(in)
	Expect(err).To(HaveOccurred())
	// On error path the contract is: return the original path + the error.
	Expect(got).To(Equal(in))
	Expect(err.Error()).To(ContainSubstring("no-such-user-deadbeef-9c4f"))
}

func TestReplaceTildeWithHome_PreservesPathSemantics(t *testing.T) {
	RegisterTestingT(t)

	home, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred())

	got, err := ReplaceTildeWithHome("~/foo")
	Expect(err).ToNot(HaveOccurred())

	// The expansion is a string replacement, not a path-cleaning operation.
	// Sanity-check the resulting path can be parsed by filepath without panic
	// and starts with the home directory.
	Expect(strings.HasPrefix(got, home)).To(BeTrue())
	Expect(filepath.IsAbs(got)).To(BeTrue())
}
