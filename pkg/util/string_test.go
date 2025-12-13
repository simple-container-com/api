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

func TestSanitizeK8sResourceName(t *testing.T) {
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
			input:          "simple-service",
			expectedLength: 14,
			shouldContain:  "simple-service",
		},
		{
			name:             "Long volume name truncated",
			input:            "celery-blockchain--vata-postgres--production-sidecarcsql--production-creds",
			expectedLength:   63,
			shouldContain:    "celery-blockchain",
			shouldNotContain: "--production-creds", // Should not have the full suffix
		},
		{
			name:             "Name with underscores and uppercase",
			input:            "My_Service_Name_With_Underscores",
			expectedLength:   32,
			shouldContain:    "my-service-name-with-underscores",
			shouldNotContain: "_", // Underscores should be replaced
		},
		{
			name:             "Name with invalid characters",
			input:            "service@name#with$invalid%chars",
			expectedLength:   63,
			shouldContain:    "servicename",
			shouldNotContain: "@", // Invalid chars should be removed
		},
		{
			name:           "Very long name exceeding 63 chars",
			input:          "this-is-a-very-long-kubernetes-resource-name-that-definitely-exceeds-the-sixty-three-character-limit",
			expectedLength: 63,
			shouldContain:  "this-is-a-very-long-kubernetes-resource-name-that-definit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			result := SanitizeK8sResourceName(tc.input)

			// Verify length constraint
			Expect(len(result)).To(BeNumerically("<=", tc.expectedLength))

			// Verify it contains expected content
			if tc.shouldContain != "" {
				Expect(result).To(ContainSubstring(tc.shouldContain))
			}

			// Verify it doesn't contain problematic patterns
			if tc.shouldNotContain != "" {
				Expect(result).ToNot(ContainSubstring(tc.shouldNotContain))
			}

			// Verify Kubernetes naming requirements
			Expect(result).To(MatchRegexp("^[a-z0-9]([a-z0-9-]*[a-z0-9])?$"))

			// Verify no double hyphens
			Expect(result).ToNot(ContainSubstring("--"))
		})
	}
}

func TestSanitizeK8sResourceName_RealWorldVolumeExample(t *testing.T) {
	RegisterTestingT(t)

	// Test the exact case from the Kubernetes error message
	problematicVolumeName := "celery-blockchain--vata-postgres--production-sidecarcsql--production"

	result := SanitizeK8sResourceName(problematicVolumeName)

	// Verify the fix
	Expect(len(result)).To(BeNumerically("<=", 63))
	Expect(result).To(ContainSubstring("celery-blockchain"))          // Should preserve meaningful prefix
	Expect(result).To(MatchRegexp("^[a-z0-9]([a-z0-9-]*[a-z0-9])?$")) // Should match K8s requirements
	Expect(result).ToNot(ContainSubstring("--"))                      // Should not have double hyphens
}
