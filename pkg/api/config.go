package api

import (
	"fmt"
	"os"
	"path"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const ScConfigDirectory = ".sc"
const EnvConfigFileTemplate = "cfg.%s.yaml"

type ConfigFile struct {
	PrivateKeyPath string `yaml:"privateKeyPath" json:"privateKeyPath"`
	PublicKeyPath  string `yaml:"publicKeyPath" json:"publicKeyPath"`
	PrivateKey     string `yaml:"privateKey" json:"privateKey"`
	PublicKey      string `yaml:"publicKey" json:"publicKey"`
}

func ReadConfigFile(workDir, profile string) (*ConfigFile, error) {
	res, err := ReadDescriptor(configFilePath(workDir, profile), &ConfigFile{})
	if err != nil {
		return nil, errors.Wrapf(err, "profile does not exist: %q", profile)
	}
	return res, nil
}

func configFilePath(workDir string, profile string) string {
	return path.Join(workDir, ScConfigDirectory, fmt.Sprintf(EnvConfigFileTemplate, profile))
}

func (f *ConfigFile) WriteConfigFile(workDir, profile string) error {
	bytes, err := f.ToYaml()
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePath(workDir, profile), bytes, 0644)
}

func (f *ConfigFile) ToYaml() ([]byte, error) {
	res, err := yaml.Marshal(f)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal config")
	}
	return res, nil
}
