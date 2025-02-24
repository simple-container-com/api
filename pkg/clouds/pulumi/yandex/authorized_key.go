package yandex

import (
	"encoding/json"
	"time"
)

type AuthorizedKey struct {
	Id               string    `json:"id"`
	ServiceAccountId string    `json:"service_account_id"`
	CreatedAt        time.Time `json:"created_at"`
	KeyAlgorithm     string    `json:"key_algorithm"`
	PublicKey        string    `json:"public_key"`
	PrivateKey       string    `json:"private_key"`
}

func FromString(auth string) (AuthorizedKey, error) {
	result := &AuthorizedKey{}
	err := json.Unmarshal([]byte(auth), result)
	return *result, err
}
