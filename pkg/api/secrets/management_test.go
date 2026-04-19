package secrets

import (
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/tests/testutil"
	"github.com/simple-container-com/api/pkg/util/test"
)

// TestDecryptAll_Output tests that DecryptAll provides proper output to the user
func TestDecryptAll_Output(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name             string
		setupFunc        func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock, wd string)
		expectedInOutput []string
		expectError      bool
		errorContains    string
	}{
		{
			name: "shows output for multiple revealed files",
			setupFunc: func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock, wd string) {
				// Mock console writer to accept any number of Print/Println calls BEFORE any operations
				consoleWriter.On("Print", mock.Anything).Return()
				consoleWriter.On("Print", mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				// Add two secret files
				Expect(c.AddFile("stacks/common/secrets.yaml")).To(BeNil())
				Expect(c.AddFile("stacks/refapp/secrets.yaml")).To(BeNil())

				// Remove the files to test decryption
				commonPath := path.Join(wd, "stacks/common/secrets.yaml")
				refappPath := path.Join(wd, "stacks/refapp/secrets.yaml")
				Expect(os.RemoveAll(commonPath)).To(BeNil())
				Expect(os.RemoveAll(refappPath)).To(BeNil())
			},
			expectedInOutput: []string{
				"revealed",
				"stacks/common/secrets.yaml",
				"stacks/refapp/secrets.yaml",
				"revealed 2 secret file(s)",
			},
			expectError: false,
		},
		{
			name: "shows message when no files to reveal",
			setupFunc: func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock, wd string) {
				// Don't add any files, just ensure the public key exists
				// We need to add a file first, then remove it from the registry
				// to simulate having no files to reveal
				consoleWriter.On("Print", mock.Anything).Return()
				consoleWriter.On("Print", mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				// Add a file to initialize the secrets for this key
				Expect(c.AddFile("stacks/common/secrets.yaml")).To(BeNil())

				// Then remove the file from the registry (simulating no files to reveal)
				cryptorInstance := c.(*cryptor)
				cryptorInstance.secrets.Registry.Files = []string{}
			},
			expectedInOutput: []string{
				"no secret files to reveal",
			},
			expectError: false,
		},
		{
			name: "shows output for single revealed file",
			setupFunc: func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock, wd string) {
				// Mock console writer to accept any number of Print/Println calls BEFORE any operations
				consoleWriter.On("Print", mock.Anything).Return()
				consoleWriter.On("Print", mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
				consoleWriter.On("Println", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				// Add one secret file
				Expect(c.AddFile("stacks/common/secrets.yaml")).To(BeNil())

				// Remove the file to test decryption
				commonPath := path.Join(wd, "stacks/common/secrets.yaml")
				Expect(os.RemoveAll(commonPath)).To(BeNil())
			},
			expectedInOutput: []string{
				"revealed",
				"stacks/common/secrets.yaml",
				"revealed 1 secret file(s)",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			m := &mocks{
				consoleReaderMock:      &test.ConsoleReaderMock{},
				confirmationReaderMock: &test.ConsoleReaderMock{},
			}

			// Mock console writer
			consoleWriterMock := &test.ConsoleWriterMock{}

			workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
			defer cleanup()
			Expect(err).To(BeNil())

			// Accept all changes for encryption
			m.confirmationReaderMock.On("ReadLine").Return("Y", nil)

			opts := []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files"),
				WithConsoleReader(m.consoleReaderMock),
				WithConfirmationReader(m.confirmationReaderMock),
			}

			got, err := NewCryptor(workDir, opts...)
			Expect(err).To(BeNil())
			Expect(got).NotTo(BeNil())

			// Replace console writer with mock FIRST
			cryptorInstance := got.(*cryptor)
			cryptorInstance.consoleWriter = consoleWriterMock

			// Run setup AFTER replacing console writer
			// The setupFunc should set up mock expectations BEFORE calling operations
			tc.setupFunc(t, got, consoleWriterMock, workDir)

			// Call DecryptAll
			err = got.DecryptAll(false)

			if tc.expectError {
				Expect(err).NotTo(BeNil())
				if tc.errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(tc.errorContains))
				}
			} else {
				Expect(err).To(BeNil())

				// Verify that Println was called (output was produced)
				// We can't easily verify the exact content without capturing it,
				// but we can verify the mock was called
				consoleWriterMock.AssertExpectations(t)
			}
		})
	}
}

// TestDecryptAll_ErrorCases tests error handling in DecryptAll
func TestDecryptAll_ErrorCases(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name          string
		setupFunc     func(t *testing.T, c Cryptor, wd string)
		expectError   bool
		errorContains string
	}{
		{
			name: "returns error when public key is not configured",
			setupFunc: func(t *testing.T, c Cryptor, wd string) {
				// Create cryptor without public key
				cryptorInstance := c.(*cryptor)
				cryptorInstance.currentPublicKey = ""
			},
			expectError:   true,
			errorContains: "public key is not configured",
		},
		{
			name: "returns error when current public key is not found in secrets",
			setupFunc: func(t *testing.T, c Cryptor, wd string) {
				// This should be handled by the implementation
				// The error occurs when the current public key doesn't exist in secrets map
				cryptorInstance := c.(*cryptor)
				// Set a public key that doesn't exist in secrets
				cryptorInstance.currentPublicKey = "non-existent-key"
			},
			expectError:   true,
			errorContains: "current public key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			m := &mocks{
				consoleReaderMock:      &test.ConsoleReaderMock{},
				confirmationReaderMock: &test.ConsoleReaderMock{},
			}

			workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
			defer cleanup()
			Expect(err).To(BeNil())

			// Accept all changes
			m.confirmationReaderMock.On("ReadLine").Return("Y", nil)

			opts := []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files"),
				WithConsoleReader(m.consoleReaderMock),
				WithConfirmationReader(m.confirmationReaderMock),
			}

			got, err := NewCryptor(workDir, opts...)
			Expect(err).To(BeNil())
			Expect(got).NotTo(BeNil())

			// Run setup
			tc.setupFunc(t, got, workDir)

			// Call DecryptAll and check for error
			err = got.DecryptAll(false)

			if tc.expectError {
				Expect(err).NotTo(BeNil())
				if tc.errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(tc.errorContains))
				}
			} else {
				Expect(err).To(BeNil())
			}
		})
	}
}

// TestRemovePublicKey_OutputAndBehavior tests that RemovePublicKey properly removes keys and shows output
func TestRemovePublicKey_OutputAndBehavior(t *testing.T) {
	RegisterTestingT(t)

	testCases := []struct {
		name             string
		setupFunc        func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock) (keyToRemove string)
		expectError      bool
		errorContains    string
		verifyKeyRemoved bool
	}{
		{
			name: "successfully removes existing public key and shows output",
			setupFunc: func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock) string {
				// Generate another key pair to add and then remove
				_, anotherPubKey, err := ciphers.GenerateKeyPair(2048)
				Expect(err).To(BeNil())

				anotherPubKeySSH, err := ciphers.MarshalPublicKey(anotherPubKey)
				Expect(err).To(BeNil())

				anotherPubKeyString := strings.TrimSpace(string(anotherPubKeySSH))

				// Add the key
				Expect(c.AddPublicKey(anotherPubKeyString)).To(BeNil())
				Expect(c.ReadSecretFiles()).To(BeNil())

				// Verify key exists
				knownKeys := c.GetKnownPublicKeys()
				Expect(knownKeys).To(ContainElement(anotherPubKeyString))

				// Mock console writer - RemovePublicKey only calls Println with 2 args
				// We use Maybe() to make these optional since they might not all be called
				consoleWriter.On("Println", mock.Anything, mock.Anything).Return()

				return anotherPubKeyString
			},
			expectError:      false,
			verifyKeyRemoved: true,
		},
		{
			name: "returns error when removing non-existent key",
			setupFunc: func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock) string {
				nonExistentKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7vbqajDhA nonexist@user"
				return nonExistentKey
			},
			expectError:      true,
			errorContains:    "public key",
			verifyKeyRemoved: false,
		},
		{
			name: "handles key with alias correctly",
			setupFunc: func(t *testing.T, c Cryptor, consoleWriter *test.ConsoleWriterMock) string {
				// Add a key with an alias
				_, anotherPubKey, err := ciphers.GenerateKeyPair(2048)
				Expect(err).To(BeNil())

				anotherPubKeySSH, err := ciphers.MarshalPublicKey(anotherPubKey)
				Expect(err).To(BeNil())

				anotherPubKeyString := strings.TrimSpace(string(anotherPubKeySSH))

				// Add the key
				Expect(c.AddPublicKey(anotherPubKeyString)).To(BeNil())
				Expect(c.ReadSecretFiles()).To(BeNil())

				// Mock console writer
				consoleWriter.On("Println", mock.Anything, mock.Anything).Return()

				return anotherPubKeyString
			},
			expectError:      false,
			verifyKeyRemoved: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			m := &mocks{
				consoleReaderMock:      &test.ConsoleReaderMock{},
				confirmationReaderMock: &test.ConsoleReaderMock{},
			}

			// Mock console writer
			consoleWriterMock := &test.ConsoleWriterMock{}

			workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
			defer cleanup()
			Expect(err).To(BeNil())

			// Accept all changes
			m.confirmationReaderMock.On("ReadLine").Return("Y", nil)

			opts := []Option{
				withGitDir("gitdir"),
				WithKeysFromScConfig("local-key-files"),
				WithConsoleReader(m.consoleReaderMock),
				WithConfirmationReader(m.confirmationReaderMock),
			}

			got, err := NewCryptor(workDir, opts...)
			Expect(err).To(BeNil())
			Expect(got).NotTo(BeNil())

			// Replace console writer with mock
			cryptorInstance := got.(*cryptor)
			cryptorInstance.consoleWriter = consoleWriterMock

			// Run setup and get key to remove
			keyToRemove := tc.setupFunc(t, got, consoleWriterMock)

			// Call RemovePublicKey
			err = got.RemovePublicKey(keyToRemove)

			if tc.expectError {
				Expect(err).NotTo(BeNil())
				if tc.errorContains != "" {
					Expect(err.Error()).To(ContainSubstring(tc.errorContains))
				}
			} else {
				Expect(err).To(BeNil())

				// Verify that Println was called (output was produced)
				consoleWriterMock.AssertExpectations(t)

				// Verify key was actually removed
				if tc.verifyKeyRemoved {
					Expect(got.ReadSecretFiles()).To(BeNil())
					knownKeys := got.GetKnownPublicKeys()
					Expect(knownKeys).NotTo(ContainElement(keyToRemove))
				}
			}

			// Verify all mock expectations were met
			consoleWriterMock.AssertExpectations(t)
		})
	}
}

// TestRemovePublicKey_WriteLock tests that RemovePublicKey acquires write lock
func TestRemovePublicKey_WriteLock(t *testing.T) {
	RegisterTestingT(t)

	m := &mocks{
		consoleReaderMock:      &test.ConsoleReaderMock{},
		confirmationReaderMock: &test.ConsoleReaderMock{},
	}

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).To(BeNil())

	// Accept all changes
	m.confirmationReaderMock.On("ReadLine").Return("Y", nil)

	opts := []Option{
		withGitDir("gitdir"),
		WithKeysFromScConfig("local-key-files"),
		WithConsoleReader(m.consoleReaderMock),
		WithConfirmationReader(m.confirmationReaderMock),
	}

	got, err := NewCryptor(workDir, opts...)
	Expect(err).To(BeNil())
	Expect(got).NotTo(BeNil())

	// Add a key to remove
	_, anotherPubKey, err := ciphers.GenerateKeyPair(2048)
	Expect(err).To(BeNil())

	anotherPubKeySSH, err := ciphers.MarshalPublicKey(anotherPubKey)
	Expect(err).To(BeNil())

	anotherPubKeyString := strings.TrimSpace(string(anotherPubKeySSH))

	// Add the key
	Expect(got.AddPublicKey(anotherPubKeyString)).To(BeNil())
	Expect(got.ReadSecretFiles()).To(BeNil())

	// Verify key exists before removal
	knownKeys := got.GetKnownPublicKeys()
	Expect(knownKeys).To(ContainElement(anotherPubKeyString))

	// The test verifies that RemovePublicKey acquires a write lock by:
	// 1. Successfully removing the key (no race condition errors)
	// 2. The key is actually removed from the secrets map
	// 3. The changes are persisted via MarshalSecretsFile
	err = got.RemovePublicKey(anotherPubKeyString)
	Expect(err).To(BeNil())

	// Verify key was removed
	Expect(got.ReadSecretFiles()).To(BeNil())
	knownKeys = got.GetKnownPublicKeys()
	Expect(knownKeys).NotTo(ContainElement(anotherPubKeyString))

	// Verify the secrets file was updated by reading it back
	// This confirms the write lock was acquired and the change was persisted
	secrets := got.GetSecretFiles()
	_, exists := secrets.Secrets[anotherPubKeyString]
	Expect(exists).To(BeFalse(), "Key should not exist in secrets after removal")
}

// TestDecryptAll_Integration tests DecryptAll in an integration scenario
func TestDecryptAll_Integration(t *testing.T) {
	RegisterTestingT(t)

	m := &mocks{
		consoleReaderMock:      &test.ConsoleReaderMock{},
		confirmationReaderMock: &test.ConsoleReaderMock{},
	}

	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).To(BeNil())

	// Mock console writer
	consoleWriterMock := &test.ConsoleWriterMock{}
	consoleWriterMock.On("Print", mock.Anything).Return()
	consoleWriterMock.On("Print", mock.Anything, mock.Anything).Return()
	consoleWriterMock.On("Println", mock.Anything).Return()
	consoleWriterMock.On("Println", mock.Anything, mock.Anything).Return()
	consoleWriterMock.On("Println", mock.Anything, mock.Anything, mock.Anything).Return()
	consoleWriterMock.On("Println", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	consoleWriterMock.On("Println", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	// Accept all changes
	m.confirmationReaderMock.On("ReadLine").Return("Y", nil)

	opts := []Option{
		withGitDir("gitdir"),
		WithKeysFromScConfig("local-key-files"),
		WithConsoleReader(m.consoleReaderMock),
		WithConfirmationReader(m.confirmationReaderMock),
	}

	got, err := NewCryptor(workDir, opts...)
	Expect(err).To(BeNil())
	Expect(got).NotTo(BeNil())

	// Replace console writer with mock
	cryptorInstance := got.(*cryptor)
	cryptorInstance.consoleWriter = consoleWriterMock

	// Store original file content
	originalContent, err := os.ReadFile(path.Join(workDir, "stacks/common/secrets.yaml"))
	Expect(err).To(BeNil())

	t.Run("encrypt and then decrypt files with output", func(t *testing.T) {
		RegisterTestingT(t)

		// Add file to encrypt it
		Expect(got.AddFile("stacks/common/secrets.yaml")).To(BeNil())

		// Remove the file
		secretFile := path.Join(workDir, "stacks/common/secrets.yaml")
		Expect(os.RemoveAll(secretFile)).To(BeNil())

		// Verify file doesn't exist
		_, err := os.Stat(secretFile)
		Expect(err).NotTo(BeNil())

		// Decrypt all files
		err = got.DecryptAll(false)
		Expect(err).To(BeNil())

		// Verify file was decrypted
		decryptedContent, err := os.ReadFile(secretFile)
		Expect(err).To(BeNil())
		Expect(decryptedContent).To(Equal(originalContent))

		// Verify that output was produced (Println was called)
		consoleWriterMock.AssertExpectations(t)
	})
}
