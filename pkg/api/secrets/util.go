package secrets

import (
	"strings"
)

func TrimPubKey(pubKey string) string {
	if parts := strings.Fields(pubKey); len(parts) > 3 || len(parts) < 2 {
		return strings.TrimSpace(pubKey)
	} else {
		return strings.Join(parts, " ")
	}
}

func TrimPrivKey(privKey string) string {
	return strings.TrimSpace(privKey)
}
