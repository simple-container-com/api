package api

import (
	"fmt"
	"path"
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
	return ReadDescriptor(path.Join(workDir, ScConfigDirectory, fmt.Sprintf(EnvConfigFileTemplate, profile)), &ConfigFile{})
}
