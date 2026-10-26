// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
)

// DefaultCacheRoot is where SC caches fetched plugin binaries. Can be
// overridden per-call to ResolveBinaryPath; the default follows the
// Terraform-ish convention most operators already have in muscle memory.
const DefaultCacheRoot = "~/.sc/plugins"

// ResolveBinaryPath resolves a PluginSpec to a concrete executable binary
// path on disk. Supported modes:
//
//   - spec.Path set → return it verbatim (dev / air-gapped operator path).
//   - spec.Source set with type "github-release" → download from
//     github.com/<repo>/releases/download/<version>/<binary>, verify
//     sha256, cache under cacheRoot/<repo>/<version>/, chmod +x, return.
//
// If spec.SHA256 is set, it is enforced. If not, the fetcher looks for
// a sha256sums.txt asset in the release and uses the matching entry.
//
// cacheRoot may begin with "~" and is expanded to the user's home dir.
func ResolveBinaryPath(spec api.PluginSpec, cacheRoot string) (string, error) {
	if spec.Path != "" {
		return expandHome(spec.Path)
	}
	if spec.Source == nil {
		return "", errors.New("plugin spec has neither path nor source")
	}
	switch spec.Source.Type {
	case "github-release":
		return resolveGithubRelease(spec, cacheRoot)
	default:
		return "", errors.Errorf("unknown plugin source type %q", spec.Source.Type)
	}
}

func resolveGithubRelease(spec api.PluginSpec, cacheRoot string) (string, error) {
	src := spec.Source
	if src.Repository == "" {
		return "", errors.New("plugin source.repository is empty")
	}
	if src.Version == "" {
		return "", errors.New("plugin source.version is empty (SC does not auto-resolve \"latest\" — pin a specific release)")
	}
	root, err := expandHome(cacheRoot)
	if err != nil {
		return "", err
	}
	binName := binaryName(src)
	cached := filepath.Join(root, src.Repository, src.Version, binName)

	// Resolve the expected sha256 — either from spec.SHA256 or from the
	// release's sha256sums.txt. We do this BEFORE checking the cache so a
	// cached-but-stale binary gets replaced.
	expectedSHA, err := resolveExpectedSHA(spec, binName)
	if err != nil {
		return "", err
	}

	if ok, _ := checkCached(cached, expectedSHA); ok {
		return cached, nil
	}

	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		return "", errors.Wrap(err, "mkdir plugin cache")
	}

	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", src.Repository, src.Version, binName)
	if err := downloadFile(url, cached); err != nil {
		return "", errors.Wrapf(err, "download %q", url)
	}

	got, err := fileSHA256(cached)
	if err != nil {
		return "", err
	}
	if expectedSHA != "" && !strings.EqualFold(got, expectedSHA) {
		_ = os.Remove(cached)
		return "", errors.Errorf("plugin SHA256 mismatch for %s@%s/%s: want %s, got %s",
			src.Repository, src.Version, binName, expectedSHA, got)
	}
	if err := os.Chmod(cached, 0o755); err != nil {
		return "", errors.Wrap(err, "chmod plugin binary")
	}
	return cached, nil
}

// binaryName derives the per-platform binary filename inside a release:
//
//	sc-plugin-cloudflare-linux-amd64
//
// Overridable via spec.Source.BinaryName (template with {goos}/{goarch} if
// an operator pins a different naming convention).
func binaryName(src *api.PluginSource) string {
	if src.BinaryName != "" {
		s := src.BinaryName
		s = strings.ReplaceAll(s, "{goos}", runtime.GOOS)
		s = strings.ReplaceAll(s, "{goarch}", runtime.GOARCH)
		return s
	}
	// "simple-container-com/sc-plugin-cloudflare" → "sc-plugin-cloudflare".
	repoLeaf := src.Repository
	if idx := strings.LastIndex(repoLeaf, "/"); idx >= 0 {
		repoLeaf = repoLeaf[idx+1:]
	}
	return fmt.Sprintf("%s-%s-%s", repoLeaf, runtime.GOOS, runtime.GOARCH)
}

func resolveExpectedSHA(spec api.PluginSpec, binName string) (string, error) {
	if spec.SHA256 != "" {
		return strings.TrimSpace(spec.SHA256), nil
	}
	// Try to fetch sha256sums.txt from the release.
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/sha256sums.txt", spec.Source.Repository, spec.Source.Version)
	body, err := httpGet(url)
	if err != nil {
		// Not an error — operator may have omitted sha256sums.txt. SC proceeds
		// without the integrity check and logs a WARN at spawn time.
		return "", nil
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Lines look like: "<sha256>  <path>" — `path` may be "../sc-plugin-cloudflare-linux-amd64"
		// (what sha256sum writes when invoked from a subdir). Match on suffix.
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.HasSuffix(fields[1], binName) || filepath.Base(fields[1]) == binName {
			return fields[0], nil
		}
	}
	return "", nil
}

func checkCached(path, expectedSHA string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		return false, err
	}
	if expectedSHA == "" {
		return true, nil
	}
	got, err := fileSHA256(path)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(got, expectedSHA), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadFile(url, dst string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url) //nolint:noctx // allow — tool-level call
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
