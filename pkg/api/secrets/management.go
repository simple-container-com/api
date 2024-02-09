package secrets

import (
	"crypto/rsa"
	"encoding/asn1"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"

	"github.com/simple-container-com/api/pkg/api"

	"github.com/go-git/go-billy/v5"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"golang.org/x/crypto/ssh"
)

func (c *cryptor) ReadProfileConfig() error {
	return WithKeysFromCurrentProfile().f(c)
}

func (c *cryptor) GetSecretFiles() EncryptedSecretFiles {
	defer c.withReadLock()()
	res := c.secrets
	return res
}

func (c *cryptor) ReadSecretFiles() error {
	defer c.withWriteLock()()
	return c.unmarshalSecretsFile()
}

func (c *cryptor) GetAndDecryptFileContent(relPath string) ([]byte, error) {
	defer c.withReadLock()()

	if f, found := c.secrets.Secrets[c.currentPublicKey]; !found {
		return nil, errors.Errorf("secret file %q not found", relPath)
	} else if encrypted, found := lo.Find(f.Files, func(item EncryptedSecretFile) bool {
		return item.Path == relPath
	}); !found {
		return nil, errors.Errorf("encrypted secret file %q not found", relPath)
	} else if content, err := c.decryptSecretData(encrypted.EncryptedData); err != nil {
		return nil, errors.Wrapf(err, "failed to decrypt secret file %q with configured public key %q", relPath, c.currentPublicKey)
	} else {
		return content, nil
	}
}

func (c *cryptor) Options() []Option {
	return c.options
}

func (c *cryptor) AddFile(filePath string) error {
	defer c.withWriteLock()()

	if err := c.initData(); err != nil {
		return err
	}
	if lo.IndexOf(c.registry.Files, filePath) < 0 {
		c.registry.Files = append(c.registry.Files, filePath)
	}
	if err := c.EncryptChanged(); err != nil {
		return errors.Wrapf(err, "failed to re-encrypt all secrets")
	}
	if err := c.marshalSecretsFile(); err != nil {
		return err
	}
	if err := c.gitRepo.AddFileToIgnore(filePath); err != nil {
		return err
	}
	return nil
}

func (c *cryptor) RemovePublicKey(pubKey string) error {
	delete(c.secrets.Secrets, pubKey)
	err := c.EncryptChanged()
	if err != nil {
		return err
	}
	return c.marshalSecretsFile()
}

func (c *cryptor) GetKnownPublicKeys() []string {
	return lo.Keys(c.secrets.Secrets)
}

func (c *cryptor) AddPublicKey(pubKey string) error {
	c.secrets.Secrets[pubKey] = EncryptedSecrets{}
	err := c.EncryptChanged()
	if err != nil {
		return err
	}
	return c.marshalSecretsFile()
}

func (c *cryptor) RemoveFile(filePath string) error {
	defer c.withWriteLock()()
	if err := c.initData(); err != nil {
		return err
	}
	c.registry.Files = lo.Filter(c.registry.Files, func(s string, _ int) bool {
		return s != filePath
	})
	if err := c.EncryptChanged(); err != nil {
		return errors.Wrapf(err, "failed to re-encrypt all secrets")
	}
	err := c.marshalSecretsFile()
	if err != nil {
		return err
	}
	err = c.gitRepo.RemoveFileFromIgnore(filePath)
	if err != nil {
		return err
	}
	return nil
}

func (c *cryptor) unmarshalSecretsFile() error {
	secretsFilePath := path.Join(api.ScConfigDirectory, EncryptedSecretFilesDataFileName)

	var err error
	var file billy.File
	file, err = c.gitRepo.OpenFile(secretsFilePath, os.O_CREATE|os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}
	if file == nil {
		return errors.New("file is nil")
	}
	defer func() { _ = file.Close() }()
	secretsFileData, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrapf(err, "failed to read secret file: %q", secretsFilePath)
	}
	if res, err := api.UnmarshalDescriptor[EncryptedSecretFiles](secretsFileData); err != nil || res == nil {
		return errors.Wrapf(err, "failed to unmarshal secrets file: %q", secretsFilePath)
	} else {
		c.secrets = *res
	}
	return nil
}

func (c *cryptor) marshalSecretsFile() error {
	secretsFilePath := path.Join(api.ScConfigDirectory, EncryptedSecretFilesDataFileName)

	bytes, err := api.MarshalDescriptor(&c.secrets)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal secrets")
	}
	var file billy.File
	file, err = c.gitRepo.OpenFile(secretsFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	if file == nil {
		return errors.New("file is nil")
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(bytes); err != nil {
		return errors.Wrapf(err, "failed to write secrets file %q", secretsFilePath)
	}
	return nil
}

func (c *cryptor) DecryptAll() error {
	defer c.withReadLock()()

	if c.currentPublicKey == "" {
		return errors.New("public key is not configured")
	}

	if _, ok := c.secrets.Secrets[c.currentPublicKey]; !ok {
		return errors.New("current public key is not found in secrets: no decryption can be made")
	}

	for _, sFile := range c.secrets.Secrets[c.currentPublicKey].Files {
		if _, err := c.decryptSecretDataToFile(sFile.EncryptedData, sFile.Path); err != nil {
			return errors.Wrapf(err, "failed to decrypt secret file %q with configured public key %q", sFile.Path, c.currentPublicKey)
		}
	}

	return nil
}

func (c *cryptor) EncryptChanged() error {
	for publicKey := range c.secrets.Secrets {
		filteredSecrets := c.secrets.Secrets[publicKey]
		filteredSecrets.Files = lo.Filter(filteredSecrets.Files, func(file EncryptedSecretFile, _ int) bool {
			return lo.Contains(c.registry.Files, file.Path)
		})
		c.secrets.Secrets[publicKey] = filteredSecrets
	}
	for _, relFilePath := range c.registry.Files {
		secretData, err := c.readSecretFile(relFilePath)
		if err != nil {
			return errors.Wrapf(err, "failed to read secret file %q", relFilePath)
		}
		secrets := c.secrets.Secrets[c.currentPublicKey]

		// for all other public keys
		for publicKey := range c.secrets.Secrets {
			pKeySecrets := c.secrets.Secrets[publicKey]

			currentContent, _ := c.decryptSecretData(pKeySecrets.GetEncryptedContent(relFilePath))
			_, found := lo.Find(pKeySecrets.Files, func(file EncryptedSecretFile) bool {
				return file.Path == relFilePath
			})
			if currentContent != nil && string(secretData) == string(currentContent) && found {
				// skip re-encrypting for unchanged secret
				continue
			}

			sFile, err := c.encryptSecretsFileWith(publicKey, relFilePath)
			if err != nil {
				return err
			}
			pKeySecrets.AddFileIfNotExist(sFile)
			c.secrets.Secrets[publicKey] = pKeySecrets
		}

		if currentContent, _ := c.decryptSecretData(secrets.GetEncryptedContent(relFilePath)); currentContent != nil && string(secretData) == string(currentContent) {
			// skip re-encrypting for unchanged secret
			continue
		}

		sFile, err := c.encryptSecretsFileWith(c.currentPublicKey, relFilePath)
		if err != nil {
			return err
		}
		secrets.AddFileIfNotExist(sFile)
		c.secrets.Secrets[c.currentPublicKey] = secrets
	}
	return nil
}

func (c *cryptor) encryptSecretsFileWith(publicKey string, relFilePath string) (EncryptedSecretFile, error) {
	file := EncryptedSecretFile{}
	encryptedData, err := c.encryptSecretFile(publicKey, relFilePath)
	if err != nil {
		return file, err
	}
	return EncryptedSecretFile{
		Path:          relFilePath,
		EncryptedData: encryptedData,
	}, nil
}

func (c *cryptor) encryptSecretFile(keyData string, relFilePath string) ([]string, error) {
	secretData, err := c.readSecretFile(relFilePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read secret file %q", relFilePath)
	}

	parsed, err := ciphers.ParsePublicKey(keyData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse public key: %q", keyData)
	}

	var encryptedData []string
	encryptedData, err = ciphers.EncryptLargeString(parsed, string(secretData))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encrypt secret file: %q with publicKey %q", relFilePath, keyData[0:15])
	}

	return encryptedData, nil
}

func (c *cryptor) readSecretFile(relFilePath string) ([]byte, error) {
	file, err := c.gitRepo.OpenFile(relFilePath, os.O_RDONLY, fs.ModePerm)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open secret file: %q", relFilePath)
	}
	secretData, err := io.ReadAll(file)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read secret file: %q", relFilePath)
	}
	return secretData, err
}

func (c *cryptor) decryptSecretData(encryptedData []string) ([]byte, error) {
	if c.currentPrivateKey == "" {
		return nil, errors.New("private key is not configured")
	}

	var key *rsa.PrivateKey
	var err error
	if rawKey, err := ssh.ParseRawPrivateKey([]byte(c.currentPrivateKey)); err != nil && errors.As(err, &asn1.StructuralError{}) {
		return nil, errors.Wrapf(err, "invalid key format")
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to parse private key")
	} else if castedKey, ok := rawKey.(*rsa.PrivateKey); !ok {
		return nil, errors.Errorf("unsupported private key type: %T", rawKey)
	} else {
		key = castedKey
	}

	decrypted, err := ciphers.DecryptLargeString(key, encryptedData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decrypt secret")
	}
	return decrypted, nil
}

func (c *cryptor) decryptSecretDataToFile(encryptedData []string, relFilePath string) ([]byte, error) {
	decrypted, err := c.decryptSecretData(encryptedData)
	if err != nil {
		return nil, err
	}

	var file billy.File
	if !c.gitRepo.Exists(relFilePath) {
		file, err = c.gitRepo.CreateFile(relFilePath)
	} else {
		file, err = c.gitRepo.OpenFile(relFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, fs.ModePerm)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "failed to open secret file: %q", relFilePath)
	}
	defer func() { _ = file.Close() }()
	if _, err := io.WriteString(file, string(decrypted)); err != nil {
		return nil, errors.Wrapf(err, "failed to write secret to file %q", relFilePath)
	}

	return decrypted, nil
}

func (c *cryptor) initData() error {
	if c.secrets.Secrets == nil {
		c.secrets.Secrets = make(map[string]EncryptedSecrets, 0)
	}
	if c.currentPublicKey == "" {
		return errors.New("public key is not configured")
	}
	if c.currentPrivateKey == "" {
		return errors.New("private key is not configured")
	}
	if c.gitRepo == nil {
		return errors.New("git repo is not configured")
	}
	return nil
}

func (c *cryptor) withReadLock() func() {
	c._lock.RLock()
	return func() {
		c._lock.RUnlock()
	}
}

func (c *cryptor) withWriteLock() func() {
	c._lock.Lock()
	return func() {
		c._lock.Unlock()
	}
}

func NewCryptor(workDir string, opts ...Option) (Cryptor, error) {
	c := &cryptor{
		workDir: workDir,
		options: opts,
	}

	beforeInitOpts := lo.Filter(opts, func(item Option, index int) bool {
		return item.beforeInit
	})
	afterInitOpts := lo.Filter(opts, func(item Option, index int) bool {
		return !item.beforeInit
	})
	if err := c.applyOpts(beforeInitOpts); err != nil {
		return nil, err
	}

	if err := c.applyOpts(afterInitOpts); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *cryptor) GenerateKeyPairWithProfile(projectName string, profile string) error {
	c.profile = profile
	privKey, pubKey, err := ciphers.GenerateKeyPair(2048)
	if err != nil {
		return errors.Wrapf(err, "failed to generate key pair")
	}

	c.currentPrivateKey = string(ciphers.PrivateKeyToBytes(privKey))

	mPubKey, err := ciphers.MarshalPublicKey(pubKey)
	if err != nil {
		return errors.Wrapf(err, "failed to serialize public key")
	}

	c.currentPublicKey = string(mPubKey)

	config := &api.ConfigFile{
		ProjectName: projectName,
		PrivateKey:  c.currentPrivateKey,
		PublicKey:   c.currentPublicKey,
	}
	if err := config.WriteConfigFile(c.workDir, c.profile); err != nil {
		return errors.Wrapf(err, "failed to write config file")
	}
	return nil
}

func (c *cryptor) applyOpts(opts []Option) error {
	for _, opt := range opts {
		if err := opt.f(c); err != nil {
			return err
		}
	}
	return nil
}
