// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
)

const (
	ScConfigDirectory                  = ".sc"
	EnvConfigFileTemplate              = "cfg.%s.yaml"
	ScConfigEnvVariable                = "SIMPLE_CONTAINER_CONFIG"
	ScContainerResourceTypeEnvVariable = "SIMPLE_CONTAINER_RESOURCE_TYPE"
)

type ConfigFile struct {
	ProjectName        string `json:"projectName" yaml:"projectName"`
	PrivateKeyPath     string `yaml:"privateKeyPath,omitempty" json:"privateKeyPath,omitempty"`
	PublicKeyPath      string `yaml:"publicKeyPath,omitempty" json:"publicKeyPath,omitempty"`
	PrivateKey         string `yaml:"privateKey,omitempty" json:"privateKey,omitempty"`
	PrivateKeyPassword string `yaml:"privateKeyPassword,omitempty" json:"privateKeyPassword,omitempty"`
	PublicKey          string `yaml:"publicKey,omitempty" json:"publicKey,omitempty"`
	StacksDir          string `yaml:"stacksDir,omitempty" json:"stacksDir,omitempty"`
	ParentRepository   string `yaml:"parentRepository,omitempty" json:"parentRepository,omitempty"`
}

type InitParams struct {
	ProjectName         string `json:"projectName" yaml:"projectName"`
	RootDir             string `json:"rootDir,omitempty" yaml:"rootDir"`
	Profile             string `json:"profile,omitempty" yaml:"profile"`
	SkipInitialCommit   bool   `json:"skipInitialCommit" yaml:"skipInitialCommit"`
	SkipProfileCreation bool   `json:"skipProfileCreation" yaml:"skipProfileCreation"`
	SkipScDirCreation   bool   `json:"skipScDirCreation" yaml:"skipScDirCreation"`
	IgnoreWorkdirErrors bool   `json:"skipCreateConfigDir" yaml:"skipCreateConfigDir"`
	GenerateKeyPair     bool   `json:"generateKeyPair" yaml:"generateKeyPair"`
}

func ReadConfigFile(workDir, profile string) (*ConfigFile, error) {
	configFromEnv := os.Getenv(ScConfigEnvVariable)
	if configFromEnv != "" {
		// Accept both raw YAML and base64-encoded YAML. Operator-side some
		// workflows run `--body "$(cat cfg.github.yaml)"` (raw YAML) and
		// others `--body "$(base64 -w0 cfg.github.yaml)"`; keeping only one
		// path has bitten the fleet enough times (five live-caught before
		// this fix) that both should Just Work. The detection is a single
		// try-decode: a well-formed YAML config always contains a newline
		// and a `:` — valid base64 has neither.
		configStr := strings.TrimSpace(configFromEnv)
		if !strings.ContainsAny(configStr, ":\n") {
			if decoded, err := base64.StdEncoding.DecodeString(configStr); err == nil {
				configFromEnv = string(decoded)
			}
		}
		if res, err := UnmarshalDescriptor[ConfigFile]([]byte(configFromEnv)); err != nil {
			return nil, errors.Wrapf(err, "%q env variable is set, but failed to unmarshal config", ScConfigEnvVariable)
		} else {
			return res, nil
		}
	}
	res, err := ReadDescriptor(ConfigFilePath(workDir, profile), &ConfigFile{})
	if err != nil {
		return nil, errors.Wrapf(err, "profile does not exist: %q", profile)
	}
	return res, nil
}

func ConfigFilePath(workDir string, profile string) string {
	return path.Join(workDir, ScConfigDirectory, fmt.Sprintf(EnvConfigFileTemplate, profile))
}

func (f *ConfigFile) WriteConfigFile(workDir, profile string) error {
	bytes, err := f.ToYaml()
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFilePath(workDir, profile), bytes, 0o644)
}

func (f *ConfigFile) ToYaml() ([]byte, error) {
	res, err := yaml.Marshal(f)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal config")
	}
	return res, nil
}
