package yandex

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type AuthorizedKey struct {
	Id               string    `json:"id"`
	ServiceAccountId string    `json:"service_account_id"`
	CreatedAt        time.Time `json:"created_at"`
	KeyAlgorithm     string    `json:"key_algorithm"`
	PublicKey        string    `json:"public_key"`
	PrivateKey       string    `json:"private_key"`
	asString         string    `json:"-"`
}

func NewAuthorizedKey(auth string) (*AuthorizedKey, error) {
	result := &AuthorizedKey{}
	err := json.Unmarshal([]byte(auth), result)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal")
	}
	result.asString = auth
	return result, err
}

type keyFileStruct struct {
	PrivateKey string `json:"private_key"`
}

func (a *AuthorizedKey) GetIAMToken() (string, error) {
	signed, err := signedToken(a.asString, a.ServiceAccountId, a.Id)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create token to sign")
	}
	resp, err := http.Post(
		"https://iam.api.cloud.yandex.net/iam/v1/tokens",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"jwt":"%s"}`, signed)),
	)
	if err != nil {
		return "", errors.Wrapf(err, "unable to sign JWT token")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", errors.Errorf("unable to sign JWT token, received status code: %d, body: %s", resp.StatusCode, body)
	}
	var data struct {
		IAMToken string `json:"iamToken"`
	}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", errors.Wrapf(err, "unable to decode JWT token")
	}
	return data.IAMToken, nil
}

func signedToken(authorizedKey string, serviceAccountID string, publicKeyIdentifier string) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:    serviceAccountID,
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		NotBefore: jwt.NewNumericDate(time.Now().UTC()),
		Audience:  []string{"https://iam.api.cloud.yandex.net/iam/v1/tokens"},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodPS256, claims)
	token.Header["kid"] = publicKeyIdentifier

	privateKey, err := loadPrivateKey(authorizedKey)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to load private key")
	}
	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to sign token")
	}
	return signed, nil
}

func loadPrivateKey(authorizedKey string) (*rsa.PrivateKey, error) {
	var keyData keyFileStruct
	if err := json.Unmarshal([]byte(authorizedKey), &keyData); err != nil {
		return nil, errors.Wrapf(err, "unable to decode JWT token")
	}

	rsaPrivateKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(keyData.PrivateKey))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse jwt token private key")
	}
	return rsaPrivateKey, err
}
