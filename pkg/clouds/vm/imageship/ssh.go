// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package imageship

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/pkg/errors"
)

const DefaultSshTimeout = 30 * time.Second

// SshTarget is everything needed to reach one machine. It is the live-connection
// twin of vm.VmHost (the declarative form in a parent stack's `hosts:`).
type SshTarget struct {
	Address string
	User    string
	Port    int
	// PrivateKey is the key material in PEM form, not a path. The caller decides
	// where it came from — a file for `sc image ship`, a `${secret:...}` for the
	// provisioning tier — and this package never logs it.
	PrivateKey         string
	PrivateKeyPassword string

	// HostKey pins the machine's public key, as one authorized_keys-style line
	// (`ssh-ed25519 AAAA…`). Preferred in CI, where known_hosts is empty and a
	// freshly created machine has never been seen before.
	HostKey string
	// KnownHostsFile is consulted when HostKey is empty. Defaults to
	// ~/.ssh/known_hosts.
	KnownHostsFile string
	// InsecureSkipHostKeyCheck accepts whatever key the far end presents. It has to
	// be asked for explicitly: without a host key, SSH authenticates the channel but
	// nothing authenticates the *machine*, and the deploy key plus every secret in
	// the compose env goes to whoever answered on that address.
	InsecureSkipHostKeyCheck bool

	Timeout time.Duration
	// Log receives the one warning this type can emit. Optional.
	Log func(format string, args ...any)
}

type SshRemote struct {
	client *ssh.Client
	target SshTarget
}

var _ Remote = (*SshRemote)(nil)

// DialSSH opens the connection. The caller closes it.
func DialSSH(ctx context.Context, t SshTarget) (*SshRemote, error) {
	if t.Address == "" {
		return nil, errors.Errorf("ssh target requires an address")
	}
	if t.User == "" {
		return nil, errors.Errorf("ssh target %q requires a user", t.Address)
	}
	if t.Port == 0 {
		t.Port = 22
	}
	if t.Timeout == 0 {
		t.Timeout = DefaultSshTimeout
	}
	if t.Log == nil {
		t.Log = func(string, ...any) {}
	}

	auth, err := authMethod(t)
	if err != nil {
		return nil, err
	}
	verifyHost, hostKeyAlgos, err := hostKeyCallback(t)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(t.Address, fmt.Sprint(t.Port))
	// ssh.Dial ignores ctx, so dial the socket ourselves to keep cancellation and
	// the deadline honest.
	conn, err := (&net.Dialer{Timeout: t.Timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot reach %s", addr)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User: t.User,
		Auth: []ssh.AuthMethod{auth},
		// HostKeyAlgorithms has to agree with the pinned key or the pin can never
		// match — see hostKeyAlgorithms. Nil (every supported algorithm, client
		// preference) is correct for the known_hosts and skip-check paths.
		HostKeyCallback:   verifyHost,
		HostKeyAlgorithms: hostKeyAlgos,
		Timeout:           t.Timeout,
	})
	if err != nil {
		_ = conn.Close()
		return nil, errors.Wrapf(err, "ssh handshake with %s@%s failed", t.User, addr)
	}
	return &SshRemote{client: ssh.NewClient(sshConn, chans, reqs), target: t}, nil
}

func (r *SshRemote) Close() error {
	return r.client.Close()
}

func (r *SshRemote) Describe() string {
	return fmt.Sprintf("%s@%s", r.target.User, net.JoinHostPort(r.target.Address, fmt.Sprint(r.target.Port)))
}

// Run executes cmd on the machine. stdin, when given, is streamed to the command —
// which is how a whole image gets there without a file at either end.
func (r *SshRemote) Run(ctx context.Context, cmd string, stdin io.Reader) (string, error) {
	sess, err := r.client.NewSession()
	if err != nil {
		return "", errors.Wrapf(err, "cannot open an ssh session on %s", r.Describe())
	}
	defer func() { _ = sess.Close() }()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	if stdin != nil {
		sess.Stdin = stdin
	}

	if err := sess.Start(cmd); err != nil {
		return "", errors.Wrapf(err, "cannot start %q on %s", cmd, r.Describe())
	}
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()

	select {
	case <-ctx.Done():
		// Closing the session is what actually interrupts a transfer that is still
		// streaming; Signal alone is advisory and several sshd builds ignore it.
		_ = sess.Signal(ssh.SIGTERM)
		_ = sess.Close()
		return "", ctx.Err()
	case err := <-done:
		if err != nil {
			return "", errors.Wrapf(err, "%q failed on %s: %s", cmd, r.Describe(), oneLine(stderr.String()))
		}
		return strings.TrimSpace(stdout.String()), nil
	}
}

func authMethod(t SshTarget) (ssh.AuthMethod, error) {
	if t.PrivateKey == "" {
		return nil, errors.Errorf(
			"no private key for %s@%s; password and ssh-agent authentication are not supported",
			t.User, t.Address,
		)
	}
	var signer ssh.Signer
	var err error
	if t.PrivateKeyPassword != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(t.PrivateKey), []byte(t.PrivateKeyPassword))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(t.PrivateKey))
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, errors.Errorf(
					"the private key for %s@%s is passphrase-protected; supply the passphrase",
					t.User, t.Address,
				)
			}
		}
	}
	if err != nil {
		// Deliberately not wrapping with the key: a malformed PEM in an error message
		// is still key material in a CI log.
		return nil, errors.Errorf("cannot parse the private key for %s@%s: %v", t.User, t.Address, err)
	}
	return ssh.PublicKeys(signer), nil
}

// hostKeyAlgorithms restricts the handshake to the algorithm of a pinned host key.
//
// ssh.FixedHostKey compares the key the server presents against the pinned one
// byte-for-byte, so the server has to be asked for a key of that type. With
// HostKeyAlgorithms unset the client offers its whole supported list in ITS OWN
// preference order, which puts the ECDSA algorithms ahead of ssh-ed25519 — so a host
// that has both (every stock sshd) presents its ECDSA key, and an ed25519 pin can
// never match. The failure is `ssh: host key mismatch`, which reads as "this is not
// the machine you pinned" — i.e. exactly the alarm the pin exists to raise, for a
// machine that is in fact the right one.
//
// Live-caught 2026-10-18: the byo-host README's own recipe is
// `ssh-keyscan -t ed25519`, so the documented path produced an unusable pin and the
// error told the operator their host might be impersonated.
func hostKeyAlgorithms(key ssh.PublicKey) []string {
	// An RSA host key verifies under any of the three; which one a server picks is
	// about the SIGNATURE hash, not the key, and a server with SHA-1 disabled (the
	// default for years) refuses a bare `ssh-rsa` offer. All three marshal to the
	// same bytes, so FixedHostKey is satisfied either way.
	if key.Type() == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{key.Type()}
}

// hostKeyCallback returns the host verifier and, when a key is pinned, the host-key
// algorithms the handshake must be restricted to (nil otherwise).
func hostKeyCallback(t SshTarget) (ssh.HostKeyCallback, []string, error) {
	if t.InsecureSkipHostKeyCheck {
		t.Log("WARNING: not verifying the host key of %s — the deploy key and every secret in this "+
			"transfer go to whoever answers on that address", t.Address)
		return ssh.InsecureIgnoreHostKey(), nil, nil //nolint:gosec // explicitly opted into by the caller
	}
	if t.HostKey != "" {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(t.HostKey))
		if err != nil {
			return nil, nil, errors.Wrapf(err, "cannot parse the pinned host key for %s", t.Address)
		}
		return ssh.FixedHostKey(key), hostKeyAlgorithms(key), nil
	}

	path := t.KnownHostsFile
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, errors.Wrap(err, "cannot locate the home directory to read known_hosts")
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}
	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, nil, errors.Wrapf(err,
			"cannot verify the identity of %s from %s; pin the key with the host-key option "+
				"(`ssh-keyscan -t ed25519 %s`), or skip the check explicitly",
			t.Address, path, t.Address)
	}
	// No restriction: known_hosts may hold several keys for this host, of different
	// types, and any of them is a valid match.
	return cb, nil, nil
}

// oneLine flattens a remote command's stderr for an error message: `docker load`
// failures arrive as several lines, and a multi-line error wrapped three times is
// unreadable by the time it reaches the operator.
func oneLine(s string) string {
	// Trim before flattening, or the trailing newline every command emits becomes a
	// dangling "; " at the end of the message.
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", "; ")
	const max = 400
	if len(s) > max {
		return s[:max] + "…"
	}
	return orElse(s, "no stderr")
}
