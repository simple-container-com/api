// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_image

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/clouds/vm"
	"github.com/simple-container-com/api/pkg/clouds/vm/imageship"
)

type shipFlags struct {
	images            []string
	host              string
	user              string
	port              int
	sshKey            string
	sshKeyPassword    string
	hostKey           string
	knownHosts        string
	insecureHostKey   bool
	compression       string
	force             bool
	pull              bool
	allowArchMismatch bool
	timeout           string
}

// NewShipCmd creates the ship command
func NewShipCmd() *cobra.Command {
	flags := &shipFlags{}

	cmd := &cobra.Command{
		Use:   "ship",
		Short: "Ship local images to a remote docker host over SSH",
		Long: `Copy locally held images straight to a machine's docker, with no registry in between.

The images are streamed (docker save -> compress -> docker load) over one SSH
connection: nothing is written to disk on either side, and the machine needs only
docker and sshd. Shipping is idempotent per image — a machine that already holds this
exact image id under this reference is left alone.

Pass --image more than once to ship several images in one stream. That is faster than
one invocation each, because docker save deduplicates the layers they share.

--pull fetches an image the local docker does not have before shipping it, which is
what makes this usable for images you did not build: a mongo or a redis from someone's
compose file, or the ingress Caddy. It pulls only what is absent, so a private
third-party image can be made shippable with a plain "docker login && docker pull"
first (the engine API takes no credentials from ~/.docker/config.json).

The machine's identity is verified against ~/.ssh/known_hosts unless --host-key pins
it (what CI wants: a freshly created machine is in nobody's known_hosts).

Examples:
  # Ship to a machine whose key is already known
  sc image ship --image ghcr.io/myorg/myapp:v1.0.0 --host 203.0.113.10

  # Ship an app and the third-party images beside it, fetching what is missing
  sc image ship --image ghcr.io/myorg/myapp:v1.0.0 --image mongo:7 --image redis:7 \
    --pull --host 203.0.113.10

  # Ship from CI, pinning the host key instead of trusting known_hosts
  sc image ship --image ghcr.io/myorg/myapp:v1.0.0 --host 203.0.113.10 \
    --ssh-key ./deploy_key --host-key "$(ssh-keyscan -t ed25519 203.0.113.10)"

  # Re-ship an image the machine already has
  sc image ship --image ghcr.io/myorg/myapp:v1.0.0 --host 203.0.113.10 --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShip(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringArrayVar(&flags.images, "image", nil,
		"Image reference to ship; repeat to ship several in one stream (required)")
	cmd.Flags().StringVar(&flags.host, "host", "", "Address of the machine to ship to (required)")
	cmd.Flags().StringVar(&flags.user, "user", vm.DefaultSshUser, "SSH user on the machine")
	cmd.Flags().IntVar(&flags.port, "port", vm.DefaultSshPort, "SSH port on the machine")
	cmd.Flags().StringVar(&flags.sshKey, "ssh-key", "", "Path to the SSH private key (default $SC_SSH_KEY)")
	cmd.Flags().StringVar(&flags.sshKeyPassword, "ssh-key-password", os.Getenv("SC_SSH_KEY_PASSWORD"),
		"Passphrase of the SSH private key (default $SC_SSH_KEY_PASSWORD)")
	cmd.Flags().StringVar(&flags.hostKey, "host-key", "",
		"Pin the machine's public key, as an authorized_keys line (`ssh-keyscan -t ed25519 <host>`)")
	cmd.Flags().StringVar(&flags.knownHosts, "known-hosts", "", "known_hosts file to verify against (default ~/.ssh/known_hosts)")
	cmd.Flags().BoolVar(&flags.insecureHostKey, "insecure-skip-host-key-check", false,
		"Accept any host key. The deploy key and every shipped byte go to whoever answers on that address")
	cmd.Flags().StringVar(&flags.compression, "compression", imageship.CompressionZstd,
		"Wire compression: zstd, gzip (for a docker engine older than 23) or none")
	cmd.Flags().BoolVar(&flags.force, "force", false, "Ship even when the machine already holds this image id")
	cmd.Flags().BoolVar(&flags.pull, "pull", false,
		"Pull an image the local docker does not have before shipping it (only what is absent)")
	cmd.Flags().BoolVar(&flags.allowArchMismatch, "allow-arch-mismatch", false,
		"Ship an image built for a different architecture than the machine runs")
	cmd.Flags().StringVar(&flags.timeout, "timeout", "30m", "Timeout for the whole transfer")

	_ = cmd.MarkFlagRequired("image")
	_ = cmd.MarkFlagRequired("host")

	return cmd
}

func runShip(ctx context.Context, flags *shipFlags) error {
	// Every reference is validated before any of them is interpolated into a remote
	// shell command, and before the first byte moves: a bad ref in position three
	// must not be discovered after two images have landed on the machine.
	for _, img := range flags.images {
		if err := validateImage(img); err != nil {
			return err
		}
	}

	timeout, err := time.ParseDuration(flags.timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	// An image transfer is minutes, not seconds, and a dead link otherwise hangs the
	// deploy forever: a hung `docker load` produces no output to notice.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	key, err := readSshKey(flags)
	if err != nil {
		return err
	}

	docker, err := imageship.NewLocalDocker()
	if err != nil {
		return err
	}
	defer func() { _ = docker.Close() }()

	logf := func(format string, args ...any) { fmt.Printf(format+"\n", args...) }
	remote, err := imageship.DialSSH(ctx, imageship.SshTarget{
		Address:                  flags.host,
		User:                     flags.user,
		Port:                     flags.port,
		PrivateKey:               key,
		PrivateKeyPassword:       flags.sshKeyPassword,
		HostKey:                  flags.hostKey,
		KnownHostsFile:           flags.knownHosts,
		InsecureSkipHostKeyCheck: flags.insecureHostKey,
		Log:                      logf,
	})
	if err != nil {
		return err
	}
	defer func() { _ = remote.Close() }()

	res, err := imageship.Ship(ctx, docker, remote, imageship.Options{
		Images:            flags.images,
		Compression:       flags.compression,
		Force:             flags.force,
		Pull:              flags.pull,
		AllowArchMismatch: flags.allowArchMismatch,
		Log:               logf,
	})
	if err != nil {
		return err
	}

	if res.SkippedAll() {
		fmt.Printf("\n✓ %s already on %s\n", plural(len(res.Images), "image", "images"), flags.host)
		return nil
	}
	fmt.Printf("\n✓ Shipped %s to %s\n", plural(len(res.Shipped()), "image", "images"), flags.host)
	for _, img := range res.Images {
		switch {
		case img.Skipped:
			fmt.Printf("  %s: already there (%s)\n", img.Image, img.ImageID)
		default:
			fmt.Printf("  %s: %s%s\n", img.Image, img.ImageID, lo.If(img.Pulled, " (pulled here)").Else(""))
		}
	}
	fmt.Printf("  Platform: %s\n", res.RemotePlatform)
	fmt.Printf("  Transferred: %s compressed (%s uncompressed) in %s\n",
		humanize.IBytes(uint64(res.BytesCompressed)), humanize.IBytes(uint64(res.BytesRaw)),
		res.Duration.Round(time.Millisecond))
	return nil
}

func plural(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, lo.If(n == 1, one).Else(many))
}

// readSshKey loads the private key material. The key is read here rather than inside
// imageship so that the path handling stays in the CLI and the transport takes key
// material it never has to guess the provenance of (the provisioning tier passes a
// `${secret:...}` value, not a path).
func readSshKey(flags *shipFlags) (string, error) {
	path := flags.sshKey
	if path == "" {
		path = os.Getenv("SC_SSH_KEY")
	}
	if path == "" {
		return "", fmt.Errorf("--ssh-key is required (or set SC_SSH_KEY); password and ssh-agent authentication are not supported")
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read the ssh private key %q: %w", path, err)
	}
	return string(key), nil
}
