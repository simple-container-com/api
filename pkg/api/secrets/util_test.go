package secrets

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestTrimPubKey(t *testing.T) {
	RegisterTestingT(t)

	t.Run("RSA key with alias should be trimmed", func(t *testing.T) {
		keyWithAlias := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA user@example.com"
		keyWithoutAlias := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA"

		result := TrimPubKey(keyWithAlias)
		Expect(result).To(Equal(keyWithoutAlias))
	})

	t.Run("ed25519 key with alias should be trimmed", func(t *testing.T) {
		keyWithAlias := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKrlWq1Ly0vfk0S79H2f1hZJDB6jkUZvuyrx58bI+AaA user@host"
		keyWithoutAlias := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKrlWq1Ly0vfk0S79H2f1hZJDB6jkUZvuyrx58bI+AaA"

		result := TrimPubKey(keyWithAlias)
		Expect(result).To(Equal(keyWithoutAlias))
	})

	t.Run("key without alias should remain unchanged", func(t *testing.T) {
		keyWithoutAlias := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA"

		result := TrimPubKey(keyWithoutAlias)
		Expect(result).To(Equal(keyWithoutAlias))
	})

	t.Run("key with multiple word alias should be trimmed", func(t *testing.T) {
		keyWithMultiAlias := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA user@example.com deployment key"
		keyWithoutAlias := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA"

		result := TrimPubKey(keyWithMultiAlias)
		Expect(result).To(Equal(keyWithoutAlias))
	})

	t.Run("keys with same data but different aliases should normalize identically", func(t *testing.T) {
		keyWithAlias1 := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA user1@host1"
		keyWithAlias2 := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA user2@host2"
		keyWithAlias3 := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA different-alias"

		result1 := TrimPubKey(keyWithAlias1)
		result2 := TrimPubKey(keyWithAlias2)
		result3 := TrimPubKey(keyWithAlias3)

		// All should normalize to the same result
		Expect(result1).To(Equal(result2))
		Expect(result2).To(Equal(result3))
		Expect(result1).To(Equal("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA"))
	})

	t.Run("key with leading/trailing whitespace should be trimmed", func(t *testing.T) {
		keyWithSpaces := "  ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA user@example.com  "
		keyWithoutAlias := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA"

		result := TrimPubKey(keyWithSpaces)
		Expect(result).To(Equal(keyWithoutAlias))
	})

	t.Run("malformed key with only one part should return as-is", func(t *testing.T) {
		malformedKey := "invalid-key-data"

		result := TrimPubKey(malformedKey)
		Expect(result).To(Equal("invalid-key-data"))
	})

	t.Run("empty key should return empty", func(t *testing.T) {
		emptyKey := ""

		result := TrimPubKey(emptyKey)
		Expect(result).To(Equal(""))
	})

	t.Run("key with only whitespace should return empty", func(t *testing.T) {
		whitespaceKey := "   \t\n   "

		result := TrimPubKey(whitespaceKey)
		Expect(result).To(Equal(""))
	})

	t.Run("different key types should be handled correctly", func(t *testing.T) {
		dsaKey := "ssh-dss AAAAB3NzaC1kc3MAAACBAKyE user@example.com"
		ecdsaKey := "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAI user@example.com"

		dsaResult := TrimPubKey(dsaKey)
		ecdsaResult := TrimPubKey(ecdsaKey)

		Expect(dsaResult).To(Equal("ssh-dss AAAAB3NzaC1kc3MAAACBAKyE"))
		Expect(ecdsaResult).To(Equal("ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAI"))
	})
}

func TestTrimPrivKey(t *testing.T) {
	RegisterTestingT(t)

	t.Run("private key with whitespace should be trimmed", func(t *testing.T) {
		privKeyWithSpaces := "  -----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQ...\n-----END PRIVATE KEY-----  "
		expectedKey := "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQ...\n-----END PRIVATE KEY-----"

		result := TrimPrivKey(privKeyWithSpaces)
		Expect(result).To(Equal(expectedKey))
	})

	t.Run("private key without whitespace should remain unchanged", func(t *testing.T) {
		privKey := "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQ...\n-----END PRIVATE KEY-----"

		result := TrimPrivKey(privKey)
		Expect(result).To(Equal(privKey))
	})
}
