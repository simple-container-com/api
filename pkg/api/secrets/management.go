package secrets

import (
	"crypto/rsa"
	"encoding/asn1"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/go-git/go-billy/v5"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger/color"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/util"
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
	if lo.IndexOf(c.secrets.Registry.Files, filePath) < 0 {
		c.secrets.Registry.Files = append(c.secrets.Registry.Files, filePath)
	}
	if err := c.EncryptChanged(true, false); err != nil {
		return errors.Wrapf(err, "failed to re-encrypt all secrets")
	}
	if err := c.MarshalSecretsFile(); err != nil {
		return err
	}
	if err := c.gitRepo.AddFileToIgnore(filePath); err != nil {
		return err
	}
	return nil
}

func (c *cryptor) RemovePublicKey(pubKey string) error {
	delete(c.secrets.Secrets, TrimPubKey(pubKey))
	err := c.EncryptChanged(true, false)
	if err != nil {
		return err
	}
	return c.MarshalSecretsFile()
}

func (c *cryptor) GetKnownPublicKeys() []string {
	return lo.Keys(c.secrets.Secrets)
}

func (c *cryptor) AddPublicKey(pubKey string) error {
	defer c.withWriteLock()()
	if err := c.initData(); err != nil {
		return err
	}
	c.secrets.Secrets[TrimPubKey(pubKey)] = EncryptedSecrets{}
	err := c.EncryptChanged(true, false)
	if err != nil {
		return err
	}
	return c.MarshalSecretsFile()
}

func (c *cryptor) RemoveFile(filePath string) error {
	defer c.withWriteLock()()
	if err := c.initData(); err != nil {
		return err
	}
	c.secrets.Registry.Files = lo.Filter(c.secrets.Registry.Files, func(s string, _ int) bool {
		return s != filePath
	})
	if err := c.EncryptChanged(true, false); err != nil {
		return errors.Wrapf(err, "failed to re-encrypt all secrets")
	}
	err := c.MarshalSecretsFile()
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
	file, err = c.gitRepo.OpenFile(secretsFilePath, os.O_RDONLY, os.ModePerm)
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

func (c *cryptor) MarshalSecretsFile() error {
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

func (c *cryptor) DecryptAll(forceChanged bool) error {
	defer c.withReadLock()()

	if c.currentPublicKey == "" {
		return errors.New("public key is not configured")
	}

	if _, ok := c.secrets.Secrets[c.currentPublicKey]; !ok {
		return errors.Errorf("current public key (%s) is not found in secrets: no decryption can be made", c.currentPublicKey)
	}

	for _, sFile := range c.secrets.Secrets[c.currentPublicKey].Files {
		if _, err := c.decryptSecretDataToFile(sFile.EncryptedData, sFile.Path, forceChanged); err != nil {
			return errors.Wrapf(err, "failed to decrypt secret file %q with configured public key %q", sFile.Path, c.currentPublicKey)
		}
	}

	return nil
}

func (c *cryptor) EncryptChanged(force bool, forceChanged bool) error {
	c.secrets.Secrets = lo.MapKeys(c.secrets.Secrets, func(_ EncryptedSecrets, key string) string {
		return TrimPubKey(key)
	})
	for publicKey := range c.secrets.Secrets {
		filteredSecrets := c.secrets.Secrets[publicKey]
		filteredSecrets.Files = lo.Filter(filteredSecrets.Files, func(file EncryptedSecretFile, _ int) bool {
			return lo.Contains(c.secrets.Registry.Files, file.Path)
		})
		c.secrets.Secrets[publicKey] = filteredSecrets
	}

	acceptedChanges := make(map[string]bool)
	for _, relFilePath := range c.secrets.Registry.Files {
		secretData, err := c.readSecretFile(relFilePath)
		if err != nil {
			return errors.Wrapf(err, "failed to read secret file %q", relFilePath)
		}
		secrets := c.secrets.Secrets[c.currentPublicKey]

		currentContent, _ := c.decryptSecretData(secrets.GetEncryptedContent(relFilePath))
		if currentContent != nil && string(secretData) == string(currentContent) && !force {
			// skip re-encrypting for unchanged secret
			continue
		}

		// for all other public keys
		for publicKey := range c.secrets.Secrets {
			pKeySecrets := c.secrets.Secrets[publicKey]

			sFile, err := c.encryptSecretsFileWith(publicKey, relFilePath)
			if err != nil {
				return err
			}

			if accepted := acceptedChanges[sFile.Path]; !accepted {
				if err := c.ensureDiffAcceptable(sFile.Path, currentContent, secretData, forceChanged); err != nil {
					return errors.Wrapf(err, "diff is not acceptable")
				}
				acceptedChanges[sFile.Path] = true
			}

			if string(secretData) != string(currentContent) {
				pKeySecrets.RemoveFile(sFile)
			}

			pKeySecrets.AddFileIfNotExist(sFile)
			c.secrets.Secrets[publicKey] = pKeySecrets
		}

		sFile, err := c.encryptSecretsFileWith(c.currentPublicKey, relFilePath)
		if err != nil {
			return err
		}
		if accepted := acceptedChanges[sFile.Path]; !accepted {
			if err := c.ensureDiffAcceptable(sFile.Path, currentContent, secretData, forceChanged); err != nil {
				return errors.Wrapf(err, "diff is not acceptable")
			}
		}
		acceptedChanges[sFile.Path] = true
		if string(secretData) != string(currentContent) {
			secrets.RemoveFile(sFile)
		}
		secrets.AddFileIfNotExist(sFile)
		c.secrets.Secrets[c.currentPublicKey] = secrets
	}
	return nil
}

func (c *cryptor) ensureDiffAcceptable(fileName string, currentContent, newContent []byte, skipCheck bool) error {
	if skipCheck {
		return nil
	}
	currentString := string(currentContent)
	newString := string(newContent)

	type fileLine = lo.Tuple2[int, string]

	oldLines := lo.Map(strings.Split(currentString, "\n"), func(s string, i int) fileLine {
		return fileLine{A: i, B: s}
	})
	newLines := lo.Map(strings.Split(newString, "\n"), func(s string, i int) fileLine {
		return fileLine{A: i, B: s}
	})

	oldLines, newLines = lo.Difference(oldLines, newLines)

	oldDiffLines := lo.Filter(oldLines, func(oldS fileLine, _ int) bool {
		return !lo.ContainsBy(newLines, func(newS fileLine) bool {
			return oldS.B == newS.B
		})
	})
	newDiffLines := lo.Filter(newLines, func(newS fileLine, _ int) bool {
		return !lo.ContainsBy(oldLines, func(oldS fileLine) bool {
			return oldS.B == newS.B
		})
	})

	if len(oldDiffLines) > 0 {
		c.consoleWriter.Println("================================")
		c.consoleWriter.Println(color.RedFmt("Lines removed from"), color.MagentaFmt("`%s`", fileName))
		for _, removedString := range oldDiffLines {
			c.consoleWriter.Println(
				color.Red("--"),
				color.BlueBgFmt("%d\t:", removedString.A),
				color.RedFmt("%s", removedString.B),
			)
		}
	}
	if len(newDiffLines) > 0 {
		c.consoleWriter.Println("================================")
		c.consoleWriter.Println(color.GreenFmt("Lines added to"), color.MagentaFmt("`%s`", fileName))
		for _, addedString := range newDiffLines {
			c.consoleWriter.Println(
				color.Green("++"),
				color.BlueBgFmt("%d\t:", addedString.A),
				color.GreenFmt("%s", addedString.B),
			)
		}
	}
	if len(oldDiffLines) == 0 && len(newDiffLines) == 0 {
		return nil
	}
	c.consoleWriter.Println("================================")
	var readString string
	var attempts int
	for strings.ToLower(readString) != "y" && strings.ToLower(readString) != "n" {
		c.consoleWriter.Print("do you accept changes [Y/N]? >")
		readString, _ = c.confirmationReader.ReadLine()
		attempts++
		if attempts > 3 {
			return errors.Errorf("'Y' or 'N' expected, but got %q after 3 attempts", readString)
		}
	}
	if strings.ToLower(readString) == "y" {
		return nil
	}
	return errors.Errorf("Change is not accepted")
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

	if _, err := ssh.ParseRawPrivateKey([]byte(c.currentPrivateKey)); errors.As(err, new(*ssh.PassphraseMissingError)) && c.privateKeyPassphrase == "" {
		c.consoleWriter.Println("Enter password: ")
		if passphrase, err := c.consoleReader.ReadLine(); err != nil {
			return nil, errors.Wrapf(err, "failed to read password for passpharse-protected key")
		} else {
			c.privateKeyPassphrase = passphrase
		}
	}

	var rawKey any
	if c.privateKeyPassphrase != "" {
		if rawKey, err = ssh.ParseRawPrivateKeyWithPassphrase([]byte(c.currentPrivateKey), []byte(c.privateKeyPassphrase)); err != nil {
			return nil, errors.Wrapf(err, "failed to parse private key with passphrase")
		}
	} else {
		rawKey, err = ssh.ParseRawPrivateKey([]byte(c.currentPrivateKey))
	}

	if err != nil && errors.As(err, new(*asn1.StructuralError)) {
		return nil, errors.Wrapf(err, "invalid key format")
	} else if err != nil && errors.As(err, new(*ssh.PassphraseMissingError)) {
		return nil, errors.Wrapf(err, "failed to parse private key with passphrase (did you configure privateKeyPassword?)")
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

func (c *cryptor) decryptSecretDataToFile(encryptedData []string, relFilePath string, forceChanged bool) ([]byte, error) {
	decrypted, err := c.decryptSecretData(encryptedData)
	if err != nil {
		return nil, err
	}

	var file, existingFile billy.File
	var currentContent []byte
	if !c.gitRepo.Exists(relFilePath) {
		file, err = c.gitRepo.CreateFile(relFilePath)
	} else {
		existingFile, err = c.gitRepo.OpenFile(relFilePath, os.O_RDONLY, fs.ModePerm)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open existed secret file %q", relFilePath)
		}
		currentContent, err = io.ReadAll(existingFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read existed secret file %q", relFilePath)
		}
		if err := c.ensureDiffAcceptable(relFilePath, currentContent, decrypted, forceChanged); err != nil {
			return nil, errors.Wrapf(err, "changes are not accepted")
		}
		file, err = c.gitRepo.OpenFile(relFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, fs.ModePerm)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "failed to open/create secret file: %q", relFilePath)
	}
	defer func() { _ = file.Close() }()

	if _, err := io.WriteString(file, string(decrypted)); err != nil { // nolint: staticcheck
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
		workDir:            workDir,
		options:            opts,
		consoleReader:      util.DefaultConsoleReader,
		confirmationReader: util.DefaultConsoleReader,
		consoleWriter:      util.DefaultConsoleWriter,
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

	c.currentPublicKey = TrimPubKey(string(mPubKey))

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
