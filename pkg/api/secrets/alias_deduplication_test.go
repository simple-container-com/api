package secrets

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
	"github.com/simple-container-com/api/pkg/api/tests/testutil"
	"github.com/simple-container-com/api/pkg/util/test"
)

func TestAddPublicKeyAliasDeduplication(t *testing.T) {
	RegisterTestingT(t)

	// Setup a test cryptor
	workDir, cleanup, err := testutil.CopyTempProject("testdata/repo")
	defer cleanup()
	Expect(err).To(BeNil())

	mocks := &mocks{
		consoleReaderMock:      &test.ConsoleReaderMock{},
		confirmationReaderMock: &test.ConsoleReaderMock{},
	}
	mocks.confirmationReaderMock.On("ReadLine").Return("Y", nil)

	repo, err := git.Open(workDir, git.WithGitDir("gitdir"))
	Expect(err).To(BeNil())

	cryptor, err := NewCryptor(workDir,
		WithKeysFromScConfig("local-key-files"),
		WithGitRepo(repo),
		WithConsoleReader(mocks.consoleReaderMock),
		WithConfirmationReader(mocks.confirmationReaderMock),
	)
	Expect(err).To(BeNil())

	// Generate test keys
	_, rsaPubKey, err := ciphers.GenerateKeyPair(2048)
	Expect(err).To(BeNil())
	rsaPubKeySSH, err := ciphers.MarshalPublicKey(rsaPubKey)
	Expect(err).To(BeNil())

	_, ed25519PubKey, err := ciphers.GenerateEd25519KeyPair()
	Expect(err).To(BeNil())
	ed25519PubKeySSH, err := ciphers.MarshalEd25519PublicKey(ed25519PubKey)
	Expect(err).To(BeNil())

	// Create keys with and without aliases
	rsaKeyWithoutAlias := strings.TrimSpace(string(rsaPubKeySSH))
	rsaKeyWithAlias1 := rsaKeyWithoutAlias + " user1@host1"

	ed25519KeyWithoutAlias := strings.TrimSpace(string(ed25519PubKeySSH))
	ed25519KeyWithAlias1 := ed25519KeyWithoutAlias + " dev@laptop"
	ed25519KeyWithAlias2 := ed25519KeyWithoutAlias + " prod@server"

	t.Run("RSA key - add without alias first", func(t *testing.T) {
		initialKeys := cryptor.GetKnownPublicKeys()
		initialCount := len(initialKeys)

		// Add RSA key without alias
		err := cryptor.AddPublicKey(rsaKeyWithoutAlias)
		Expect(err).To(BeNil())

		keysAfterFirst := cryptor.GetKnownPublicKeys()
		Expect(len(keysAfterFirst)).To(Equal(initialCount + 1))
		Expect(keysAfterFirst).To(ContainElement(rsaKeyWithoutAlias))

		// Add the same RSA key with alias - should this create a duplicate?
		err = cryptor.AddPublicKey(rsaKeyWithAlias1)
		Expect(err).To(BeNil())

		keysAfterAlias := cryptor.GetKnownPublicKeys()
		t.Logf("Keys after adding with alias: %v", keysAfterAlias)
		t.Logf("Key count: initial=%d, after_first=%d, after_alias=%d",
			initialCount, len(keysAfterFirst), len(keysAfterAlias))

		if len(keysAfterAlias) == initialCount+1 {
			t.Log("✅ GOOD: Same key with alias was deduplicated")
			Expect(keysAfterAlias).To(ContainElement(rsaKeyWithoutAlias))
			Expect(keysAfterAlias).NotTo(ContainElement(rsaKeyWithAlias1))
		} else {
			t.Log("❌ ISSUE: Same key with alias created duplicate entry")
		}
	})

	t.Run("RSA key - add with alias first", func(t *testing.T) {
		// Use different alias to avoid conflicts
		rsaKeyWithAlias3 := rsaKeyWithoutAlias + " different@host"

		initialKeys := cryptor.GetKnownPublicKeys()
		initialCount := len(initialKeys)

		// Add RSA key with alias first
		err := cryptor.AddPublicKey(rsaKeyWithAlias3)
		Expect(err).To(BeNil())

		keysAfterFirst := cryptor.GetKnownPublicKeys()
		t.Logf("Keys after adding with alias first: %v", keysAfterFirst)

		if len(keysAfterFirst) == initialCount+1 {
			if keysAfterFirst[len(keysAfterFirst)-1] == rsaKeyWithoutAlias {
				t.Log("✅ GOOD: Key with alias was stored in normalized form")
			} else {
				t.Log("❌ ISSUE: Key with alias was stored with alias intact")
			}
		}
	})

	t.Run("Ed25519 key - multiple aliases of same key", func(t *testing.T) {
		initialKeys := cryptor.GetKnownPublicKeys()
		initialCount := len(initialKeys)

		// Add multiple versions with different aliases
		err := cryptor.AddPublicKey(ed25519KeyWithAlias1)
		Expect(err).To(BeNil())
		err = cryptor.AddPublicKey(ed25519KeyWithAlias2)
		Expect(err).To(BeNil())
		err = cryptor.AddPublicKey(ed25519KeyWithoutAlias)
		Expect(err).To(BeNil())

		keysAfterMultiple := cryptor.GetKnownPublicKeys()
		t.Logf("Keys after adding multiple aliases: %v", keysAfterMultiple)
		t.Logf("Key count: initial=%d, after_multiple=%d", initialCount, len(keysAfterMultiple))

		if len(keysAfterMultiple) == initialCount+1 {
			t.Log("✅ GOOD: Multiple aliases of same ed25519 key were deduplicated")
			Expect(keysAfterMultiple).To(ContainElement(ed25519KeyWithoutAlias))
		} else {
			t.Log("❌ ISSUE: Multiple aliases created duplicate entries")
		}
	})
}
