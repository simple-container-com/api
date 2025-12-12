package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestSanitizeGCPServiceAccountName(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name             string
		input            string
		expectedLength   int
		shouldContain    string
		shouldNotContain string
	}{
		{
			name:           "Short name unchanged",
			input:          "telegram-bot-db",
			expectedLength: 15,
			shouldContain:  "telegram-bot-db",
		},
		{
			name:             "Long name truncated with hash",
			input:            "telegram-bot-connection-initcsql-sidecarcsql--production",
			expectedLength:   28,
			shouldContain:    "telegram-bot-connection-",
			shouldNotContain: "--ction-initcsql", // Should not have malformed middle truncation
		},
		{
			name:           "Very long name with environment suffix",
			input:          "very-long-service-name-that-exceeds-gcp-limits-production",
			expectedLength: 28,
			shouldContain:  "very-long-service-name",
		},
		{
			name:             "Name with underscores",
			input:            "telegram_bot_connection_initcsql_sidecarcsql",
			expectedLength:   28,
			shouldContain:    "telegram-bot-connection-",
			shouldNotContain: "_", // Underscores should be replaced
		},
		{
			name:             "Name with double hyphens",
			input:            "service--name--with--double--hyphens",
			expectedLength:   28,
			shouldNotContain: "--", // Double hyphens should be cleaned up
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			accountName := SanitizeGCPServiceAccountName(tc.input)

			// Verify length constraint
			Expect(len(accountName)).To(BeNumerically("<=", tc.expectedLength))

			// Verify it contains expected prefix
			if tc.shouldContain != "" {
				Expect(accountName).To(ContainSubstring(tc.shouldContain))
			}

			// Verify it doesn't contain problematic patterns
			if tc.shouldNotContain != "" {
				Expect(accountName).ToNot(ContainSubstring(tc.shouldNotContain))
			}

			// Verify GCP naming requirements
			Expect(accountName).To(MatchRegexp("^[a-z](?:[-a-z0-9]{4,28}[a-z0-9])?$"))

			// Verify no double hyphens
			Expect(accountName).ToNot(ContainSubstring("--"))
		})
	}
}

func TestSanitizeGCPServiceAccountName_RealWorldExample(t *testing.T) {
	RegisterTestingT(t)

	// Test the exact case from the error message
	problematicName := "telegram-bot-connection-initcsql-sidecarcsql--production"

	accountName := SanitizeGCPServiceAccountName(problematicName)

	// Verify the fix
	Expect(len(accountName)).To(BeNumerically("<=", 28))
	Expect(accountName).ToNot(ContainSubstring("--ction-initcsql"))            // Should not have the malformed pattern
	Expect(accountName).To(ContainSubstring("telegram-bot-connection"))        // Should preserve meaningful prefix
	Expect(accountName).To(MatchRegexp("^[a-z](?:[-a-z0-9]{4,28}[a-z0-9])?$")) // Should match GCP requirements
	Expect(accountName).ToNot(ContainSubstring("--"))                          // Should not have double hyphens
}
