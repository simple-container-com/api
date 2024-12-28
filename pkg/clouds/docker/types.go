package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type RegistryCredentials struct {
	DockerRegistryURL      *string `json:"dockerRegistryURL,omitempty" yaml:"dockerRegistryURL,omitempty"`
	DockerRegistryUsername *string `json:"dockerRegistryUsername,omitempty" yaml:"dockerRegistryUsername,omitempty"`
	DockerRegistryPassword *string `json:"dockerRegistryPassword,omitempty" yaml:"dockerRegistryPassword,omitempty"`
}

type ImagePullSecret struct {
	Auths map[string]ImagePullAuth `json:"auths"`
}

type ImagePullAuth struct {
	Auth     string `json:"auth"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c RegistryCredentials) RegistryRequiresAuth() bool {
	return c.DockerRegistryUsername != nil && c.DockerRegistryPassword != nil
}

func (c RegistryCredentials) ToImagePullSecret() (string, error) {
	if c.DockerRegistryUsername == nil || c.DockerRegistryPassword == nil {
		return "", errors.Errorf("docker registry username and password must not be empty")
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", lo.FromPtr(c.DockerRegistryUsername), lo.FromPtr(c.DockerRegistryPassword))))
	auths := map[string]ImagePullAuth{}
	auths[lo.FromPtr(c.DockerRegistryURL)] = ImagePullAuth{
		Auth:     auth,
		Username: lo.FromPtr(c.DockerRegistryUsername),
		Password: lo.FromPtr(c.DockerRegistryPassword),
	}
	resBytes, err := json.Marshal(ImagePullSecret{
		Auths: auths,
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to generate image pull secret")
	}
	return base64.StdEncoding.EncodeToString(resBytes), nil
}
