// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// The rotation period is a cost multiplier, not a cosmetic setting: Cloud KMS
// bills every active key version, rotation never re-encrypts existing
// ciphertext (so old versions stay load-bearing and billed), and provisioning
// creates one key per stack. A too-short period therefore accrues billed
// versions for the lifetime of the key. These tests pin the default and the
// floor so a regression shows up here rather than on an invoice.
func TestSecretsProviderConfig_EffectiveKeyRotationPeriod(t *testing.T) {
	RegisterTestingT(t)

	Expect((&SecretsProviderConfig{}).EffectiveKeyRotationPeriod()).To(Equal(DefaultKeyRotationPeriod),
		"unset rotation period must fall back to the default")
	Expect((&SecretsProviderConfig{KeyRotationPeriod: "31536000s"}).EffectiveKeyRotationPeriod()).
		To(Equal("31536000s"), "an explicit value must win over the default")
}

func TestDefaultKeyRotationPeriodIsSane(t *testing.T) {
	RegisterTestingT(t)

	secs, err := strconv.Atoi(strings.TrimSuffix(DefaultKeyRotationPeriod, "s"))
	Expect(err).To(BeNil(), "default must be a parseable seconds value")
	Expect(secs).To(BeNumerically(">=", MinKeyRotationPeriodSeconds),
		"the default must itself satisfy the validation floor")
	// Guards against reintroducing a value like 100000s (27.8 hours), which is
	// above GCP's own 86400s floor and so passes provider validation while
	// minting a billed key version roughly every day.
	Expect(secs).To(BeNumerically(">", 86400),
		"default must be well clear of GCP's 1-day minimum")
}

func TestSecretsProviderConfig_ValidateKeyRotationPeriod(t *testing.T) {
	tests := []struct {
		name      string
		period    string
		errSubstr string
	}{
		{name: "unset is valid and means default", period: ""},
		{name: "90 days", period: "7776000s"},
		{name: "exactly the 30-day floor", period: "2592000s"},
		{name: "one year", period: "31536000s"},
		{name: "below floor: 27.8 hours", period: "100000s", errSubstr: "below the minimum"},
		{name: "below floor: GCP minimum of one day", period: "86400s", errSubstr: "below the minimum"},
		{name: "below floor: one second under", period: "2591999s", errSubstr: "below the minimum"},
		{name: "missing seconds suffix", period: "7776000", errSubstr: "'s' suffix"},
		// "ninetydays" ends in 's', so it reaches the numeric branch, not the
		// suffix branch. Asserting the numeric message keeps the two branches
		// distinguishable — otherwise a regression that collapsed them would
		// still pass.
		{name: "not a number but ends in s", period: "ninetydays", errSubstr: "whole number of seconds"},
		{name: "suffix only", period: "s", errSubstr: "whole number of seconds"},
		{name: "negative", period: "-100s", errSubstr: "below the minimum"},
		{name: "fractional seconds", period: "2592000.5s", errSubstr: "whole number of seconds"},
		{name: "duration shorthand is not accepted", period: "90d", errSubstr: "'s' suffix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := (&SecretsProviderConfig{Provision: true, KeyRotationPeriod: tt.period}).
				ValidateKeyRotationPeriod()
			if tt.errSubstr == "" {
				Expect(err).To(BeNil())
				return
			}
			Expect(err).NotTo(BeNil(), "expected %q to be rejected", tt.period)
			Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
		})
	}
}

// A deliberate sub-30-day rotation must remain expressible: this is a shared
// library, and a compliance requirement for faster rotation is legitimate. The
// opt-out is what keeps the floor a typo-catcher rather than a policy imposed on
// every consumer.
func TestSecretsProviderConfig_AllowShortKeyRotationOptsOutOfTheFloor(t *testing.T) {
	RegisterTestingT(t)

	short := &SecretsProviderConfig{Provision: true, KeyRotationPeriod: "604800s"} // 7 days
	Expect(short.ValidateKeyRotationPeriod()).NotTo(BeNil(),
		"a short period must fail by default so typos surface")
	Expect(short.ValidateKeyRotationPeriod().Error()).To(ContainSubstring("allowShortKeyRotation"),
		"the error must name the escape hatch")

	deliberate := &SecretsProviderConfig{Provision: true, KeyRotationPeriod: "604800s", AllowShortKeyRotation: true}
	Expect(deliberate.ValidateKeyRotationPeriod()).To(BeNil(),
		"an explicit opt-out must be honoured")

	// The opt-out must not disable the malformed-value checks: it is about the
	// floor, not about accepting garbage.
	malformed := &SecretsProviderConfig{Provision: true, KeyRotationPeriod: "90d", AllowShortKeyRotation: true}
	Expect(malformed.ValidateKeyRotationPeriod()).NotTo(BeNil())
}
