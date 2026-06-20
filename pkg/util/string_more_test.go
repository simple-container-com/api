// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestTrimStringWithHash_RawTruncationFallback(t *testing.T) {
	RegisterTestingT(t)

	// maxLen=4 with sep="-" (1) and a 4-char hash makes prefixLen = 4-1-4 = -1,
	// which is < 1, so the helper falls back to raw truncation str[:maxLen].
	out := TrimStringWithHash("abcdefghijklmnop", 4, "-")
	Expect(out).To(Equal("abcd"))
}

func TestTrimStringWithHash_ExactLengthUnchanged(t *testing.T) {
	RegisterTestingT(t)

	// len(str) == maxLen hits the early `<= maxLen` return.
	s := "exactly-ten"
	Expect(TrimStringWithHash(s, len(s), "-")).To(Equal(s))
}

func TestTrimStringMiddle_LongInputComposition(t *testing.T) {
	RegisterTestingT(t)

	// Verify the actual middle-trim composition: first half + sep + last half.
	in := "abcdefghijklmnop" // len 16
	out := TrimStringMiddle(in, 8, "..")
	// maxLen/2 == 4 → first 4 + ".." + last 4
	Expect(out).To(Equal("abcd..mnop"))
}

func TestSanitizeK8sResourceName_TrimsLeadingTrailingHyphens(t *testing.T) {
	RegisterTestingT(t)

	// Leading/trailing non-alphanumerics must be stripped so the result matches
	// the RFC-1123-ish pattern.
	out := SanitizeK8sResourceName("--Foo_Bar--")
	Expect(out).To(Equal("foo-bar"))
	Expect(out).To(MatchRegexp("^[a-z0-9]([a-z0-9-]*[a-z0-9])?$"))
}

func TestSanitizeGCPServiceAccountName_DoubleHyphenCleanup(t *testing.T) {
	RegisterTestingT(t)

	// Underscores -> hyphens then double-hyphen collapse.
	out := SanitizeGCPServiceAccountName("svc__name")
	Expect(out).To(Equal("svc-name"))
	Expect(out).ToNot(ContainSubstring("--"))
}
