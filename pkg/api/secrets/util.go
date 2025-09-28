package secrets

import (
	"strings"
)

// TrimPubKey normalizes SSH public keys by ignoring aliases/comments
// SSH keys have format: <key-type> <key-data> [optional-comment/alias]
// We only keep the key-type and key-data parts to ensure keys with different
// aliases but same key data are treated identically for encryption/decryption
func TrimPubKey(pubKey string) string {
	parts := strings.Fields(strings.TrimSpace(pubKey))

	// SSH public keys should have at least 2 parts: key-type and key-data
	if len(parts) < 2 {
		return strings.TrimSpace(pubKey)
	}

	// Return only the first two parts (key-type and key-data), ignoring alias/comment
	return strings.Join(parts[:2], " ")
}

func TrimPrivKey(privKey string) string {
	return strings.TrimSpace(privKey)
}
