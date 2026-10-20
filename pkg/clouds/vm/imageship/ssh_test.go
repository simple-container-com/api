// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package imageship

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	. "github.com/onsi/gomega"
)

func testKeyPair(passphrase string) (privPEM string, authorizedKey string) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	Expect(err).To(BeNil())

	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	Expect(err).To(BeNil())

	sshPub, err := ssh.NewPublicKey(pub)
	Expect(err).To(BeNil())
	return string(pem.EncodeToMemory(block)), string(ssh.MarshalAuthorizedKey(sshPub))
}

func TestDialSSHRefusesAnIncompleteTarget(t *testing.T) {
	RegisterTestingT(t)

	_, err := DialSSH(context.Background(), SshTarget{User: "sc"})
	Expect(err).To(MatchError(ContainSubstring("requires an address")))

	_, err = DialSSH(context.Background(), SshTarget{Address: "203.0.113.10"})
	Expect(err).To(MatchError(ContainSubstring("requires a user")))
}

func TestAuthMethodRefusesWhatIsNotSupported(t *testing.T) {
	RegisterTestingT(t)

	_, err := authMethod(SshTarget{Address: "203.0.113.10", User: "sc"})
	Expect(err).To(HaveOccurred())
	// Naming what is NOT supported matters more than naming what is missing: the
	// operator's next move is otherwise to try an agent or a password.
	Expect(err.Error()).To(ContainSubstring("ssh-agent"))

	priv, _ := testKeyPair("hunter2")
	_, err = authMethod(SshTarget{Address: "203.0.113.10", User: "sc", PrivateKey: priv})
	Expect(err).To(MatchError(ContainSubstring("passphrase-protected")))

	_, err = authMethod(SshTarget{
		Address: "203.0.113.10", User: "sc",
		PrivateKey: priv, PrivateKeyPassword: "hunter2",
	})
	Expect(err).To(BeNil())
}

// A malformed PEM in an error message is still key material, and this error lands in
// a CI log. The whole key, or any line of it, must never be echoed back.
func TestAuthMethodNeverEchoesKeyMaterial(t *testing.T) {
	RegisterTestingT(t)

	const marker = "SUPER-SECRET-KEY-MATERIAL"
	broken := "-----BEGIN OPENSSH PRIVATE KEY-----\n" + marker + "\n-----END OPENSSH PRIVATE KEY-----\n"

	_, err := authMethod(SshTarget{Address: "203.0.113.10", User: "sc", PrivateKey: broken})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).NotTo(ContainSubstring(marker))
	Expect(err.Error()).To(ContainSubstring("cannot parse the private key"))

	// Nor may the target's own description, which goes into every log line.
	r := &SshRemote{target: SshTarget{Address: "203.0.113.10", User: "sc", Port: 22, PrivateKey: broken}}
	Expect(r.Describe()).NotTo(ContainSubstring(marker))
	Expect(r.Describe()).To(Equal("sc@203.0.113.10:22"))
}

func TestHostKeyCallbackPinsAGivenKey(t *testing.T) {
	RegisterTestingT(t)

	_, authorized := testKeyPair("")
	cb, _, err := hostKeyCallback(SshTarget{Address: "203.0.113.10", HostKey: authorized})
	Expect(err).To(BeNil())

	pinned, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorized))
	Expect(err).To(BeNil())
	addr := &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 22}
	Expect(cb("203.0.113.10:22", addr, pinned)).To(BeNil())

	// A different machine answering on that address must not pass.
	_, other := testKeyPair("")
	otherKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(other))
	Expect(err).To(BeNil())
	Expect(cb("203.0.113.10:22", addr, otherKey)).To(HaveOccurred())
}

// What an operator will actually paste. `ssh-keyscan` prints known_hosts format —
// `<address> <type> <base64>` — not the authorized_keys line this field documents, and
// both this package and pulumi-command parse the value with ssh.ParseAuthorizedKey. It
// is accepted, because the leading field falls through to ParseAuthorizedKey's
// options-parsing branch; this test is here so the tolerance the docs rely on is
// asserted rather than assumed.
//
// The second half is the part that matters: the leading address is DISCARDED, not
// verified. Pinning host A's key under host B's address pins the key and says nothing
// about the address — the comparison is on key material alone.
func TestHostKeyCallbackAcceptsRawSshKeyscanOutput(t *testing.T) {
	RegisterTestingT(t)

	_, authorized := testKeyPair("")
	pinned, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorized))
	Expect(err).To(BeNil())
	addr := &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 22}

	for _, form := range []string{
		"203.0.113.10 " + authorized,               // ssh-keyscan <address>
		"[203.0.113.10]:2222 " + authorized,        // ssh-keyscan -p 2222
		"203.0.113.10,vm.example.ru " + authorized, // several names for one machine
		"198.51.100.7 " + authorized,               // a DIFFERENT address, same key
	} {
		cb, _, err := hostKeyCallback(SshTarget{Address: "203.0.113.10", HostKey: form})
		Expect(err).To(BeNil(), form)
		Expect(cb("203.0.113.10:22", addr, pinned)).To(BeNil(), form)
	}
}

func TestHostKeyCallbackRefusesAnUnparseablePin(t *testing.T) {
	RegisterTestingT(t)

	_, _, err := hostKeyCallback(SshTarget{Address: "203.0.113.10", HostKey: "not-a-key"})
	Expect(err).To(MatchError(ContainSubstring("cannot parse the pinned host key")))
}

// The common CI situation: known_hosts has never seen this machine. The error has to
// hand over both ways out, because the default (verify) is the right default and the
// operator needs to know how to satisfy it rather than how to switch it off.
func TestHostKeyCallbackNamesTheWayOutWhenKnownHostsCannotBeRead(t *testing.T) {
	RegisterTestingT(t)

	missing := filepath.Join(t.TempDir(), "known_hosts")
	_, _, err := hostKeyCallback(SshTarget{Address: "203.0.113.10", KnownHostsFile: missing})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ssh-keyscan"))
	Expect(err.Error()).To(ContainSubstring(missing))
}

// Skipping verification is a real choice with a real consequence, so it is opt-in
// AND it says so out loud. A silent insecure default is how a deploy key reaches a
// machine that merely answered on the right address.
func TestHostKeyCallbackWarnsWhenVerificationIsSkipped(t *testing.T) {
	RegisterTestingT(t)

	var logged []string
	cb, _, err := hostKeyCallback(SshTarget{
		Address:                  "203.0.113.10",
		InsecureSkipHostKeyCheck: true,
		Log:                      func(format string, _ ...any) { logged = append(logged, format) },
	})
	Expect(err).To(BeNil())

	_, whatever := testKeyPair("")
	anyKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(whatever))
	Expect(err).To(BeNil())
	Expect(cb("203.0.113.10:22", &net.TCPAddr{}, anyKey)).To(BeNil())
	Expect(strings.Join(logged, "\n")).To(ContainSubstring("WARNING"))
}

func TestOneLineFlattensAndCapsRemoteStderr(t *testing.T) {
	RegisterTestingT(t)

	Expect(oneLine("")).To(Equal("no stderr"))
	Expect(oneLine("first\nsecond\n")).To(Equal("first; second"))
	Expect(len(oneLine(strings.Repeat("x", 1000)))).To(BeNumerically("<=", 404))
}

// The pin is only usable if the handshake is restricted to the pinned key's
// algorithm. Unrestricted, the client's own preference order asks an ed25519-pinned
// host for its ECDSA key, FixedHostKey compares two different keys, and the operator
// is told the host might be impersonated. Live-caught 2026-10-18.
func TestHostKeyCallbackRestrictsTheHandshakeToThePinnedAlgorithm(t *testing.T) {
	RegisterTestingT(t)

	_, authorized := testKeyPair("")
	_, algos, err := hostKeyCallback(SshTarget{Address: "203.0.113.10", HostKey: authorized})
	Expect(err).To(BeNil())
	Expect(algos).To(Equal([]string{ssh.KeyAlgoED25519}))
}

func TestHostKeyAlgorithmsAcceptsEveryRsaSignatureAlgorithm(t *testing.T) {
	RegisterTestingT(t)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).To(BeNil())
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	Expect(err).To(BeNil())

	// An RSA host key verifies under all three, and a server with SHA-1 disabled —
	// the default for years — refuses a bare `ssh-rsa` offer, so pinning only
	// key.Type() would break every RSA-keyed host.
	Expect(hostKeyAlgorithms(pub)).To(Equal([]string{
		ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA,
	}))
}

// known_hosts may hold several keys of different types for one host, any of which is
// a valid match, so that path must stay unrestricted.
func TestHostKeyCallbackDoesNotRestrictKnownHosts(t *testing.T) {
	RegisterTestingT(t)

	_, authorized := testKeyPair("")
	path := filepath.Join(t.TempDir(), "known_hosts")
	Expect(os.WriteFile(path, []byte("203.0.113.10 "+authorized), 0o600)).To(Succeed())

	_, algos, err := hostKeyCallback(SshTarget{Address: "203.0.113.10", KnownHostsFile: path})
	Expect(err).To(BeNil())
	Expect(algos).To(BeNil())
}
