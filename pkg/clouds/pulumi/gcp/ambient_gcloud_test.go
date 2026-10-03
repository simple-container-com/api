// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

type gcloudCall struct {
	args      []string
	configDir string // CLOUDSDK_CONFIG when gcloud ran
	keyFile   string // contents of --key-file when gcloud ran
}

func fakeGcloud(t *testing.T, err error) *[]gcloudCall {
	t.Helper()
	var calls []gcloudCall
	prev := runGcloud
	runGcloud = func(args ...string) ([]byte, error) {
		c := gcloudCall{args: args, configDir: os.Getenv("CLOUDSDK_CONFIG")}
		for i, a := range args {
			if a == "--key-file" && i+1 < len(args) {
				data, _ := os.ReadFile(args[i+1])
				c.keyFile = string(data)
			}
		}
		calls = append(calls, c)
		return []byte("gcloud said no"), err
	}
	t.Cleanup(func() { runGcloud = prev })
	return &calls
}

func ambientEnv(t *testing.T) string {
	t.Helper()
	credFile := filepath.Join(t.TempDir(), "gha-creds-1.json")
	if err := os.WriteFile(credFile, []byte(`{"type":"external_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credFile)
	t.Setenv("GOOGLE_CREDENTIALS", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("CLOUDSDK_CONFIG", "/home/runner/shared-gcloud")
	t.Setenv("TMPDIR", t.TempDir())
	return credFile
}

func initAmbient(t *testing.T) {
	t.Helper()
	if err := InitStateStore(context.Background(), &gcloud.StateStorageConfig{}, logger.New()); err != nil {
		t.Fatalf("InitStateStore: %v", err)
	}
}

// A keyless deploy reaches its cluster through gke-gcloud-auth-plugin, which asks
// gcloud for a token; gcloud must be signed in with the federated credentials, in
// a configuration of its own so the account later steps share is not switched.
func TestInitStateStoreSignsGcloudInWithAmbientCredentials(t *testing.T) {
	calls := fakeGcloud(t, nil)
	credFile := ambientEnv(t)

	initAmbient(t)

	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0].args, []string{"auth", "login", "--cred-file=" + credFile, "--quiet"}) {
		t.Fatalf("gcloud calls = %+v; want one auth login with the credentials file", *calls)
	}
	dir := (*calls)[0].configDir
	if dir == "" || dir == "/home/runner/shared-gcloud" {
		t.Fatalf("gcloud signed in under CLOUDSDK_CONFIG=%q; want a private directory", dir)
	}
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("private gcloud config: %v; want an existing 0700 directory", err)
	}
	if os.Getenv("CLOUDSDK_CONFIG") != dir {
		t.Error("the processes this run starts (gke-gcloud-auth-plugin) would not see the signed-in configuration")
	}
	if os.Getenv("GOOGLE_CREDENTIALS") != "" {
		t.Error("ambient mode must not set GOOGLE_CREDENTIALS")
	}
}

// When the sign-in fails, or gcloud is not installed, the run keeps going with
// the configuration it had and leaves no private directory behind.
func TestInitStateStoreAmbientGcloudFailureRestoresConfig(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
	}{
		{"gcloud fails", errors.New("exit status 1")},
		{"gcloud missing", &exec.Error{Name: "gcloud", Err: exec.ErrNotFound}},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := fakeGcloud(t, c.err)
			ambientEnv(t)

			initAmbient(t)

			if len(*calls) != 1 {
				t.Fatalf("gcloud calls = %+v", *calls)
			}
			if got := os.Getenv("CLOUDSDK_CONFIG"); got != "/home/runner/shared-gcloud" {
				t.Errorf("CLOUDSDK_CONFIG = %q; want the previous value back", got)
			}
			if _, err := os.Stat((*calls)[0].configDir); !os.IsNotExist(err) {
				t.Errorf("private gcloud config left behind: %v", err)
			}
		})
	}
}

func TestInitStateStoreAmbientSkipsSignIn(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(t *testing.T)
	}{
		{"outside GitHub Actions", func(t *testing.T) { t.Setenv("GITHUB_ACTIONS", "") }},
		{"no credentials variable", func(t *testing.T) { t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "") }},
		{"credentials file absent", func(t *testing.T) {
			t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "absent.json"))
		}},
		{"credentials path is a directory", func(t *testing.T) { t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", t.TempDir()) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := fakeGcloud(t, nil)
			ambientEnv(t)
			c.set(t)

			initAmbient(t)

			if len(*calls) != 0 {
				t.Errorf("gcloud called: %+v", *calls)
			}
			if got := os.Getenv("CLOUDSDK_CONFIG"); got != "/home/runner/shared-gcloud" {
				t.Errorf("CLOUDSDK_CONFIG changed to %q", got)
			}
		})
	}
}

// Key mode activates the configured key and removes the temporary key file; it
// never signs in with ambient credentials.
func TestInitStateStoreKeyModeActivatesServiceAccount(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
	}{
		{"activates", nil},
		{"activation fails", errors.New("exit status 1")},
		{"gcloud missing", &exec.Error{Name: "gcloud", Err: exec.ErrNotFound}},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := fakeGcloud(t, c.err)
			ambientEnv(t)
			key := `{"type":"service_account","client_email":"ci@p.iam.gserviceaccount.com"}`
			cfg := &gcloud.StateStorageConfig{Credentials: gcloud.Credentials{Credentials: api.Credentials{Credentials: key}}}

			if err := InitStateStore(context.Background(), cfg, logger.New()); err != nil {
				t.Fatal(err)
			}

			if len(*calls) != 1 || (*calls)[0].args[0] != "auth" || (*calls)[0].args[1] != "activate-service-account" {
				t.Fatalf("gcloud calls = %+v; want one activate-service-account", *calls)
			}
			if (*calls)[0].keyFile != key {
				t.Errorf("gcloud read key file %q; want the configured key", (*calls)[0].keyFile)
			}
			keyPath := (*calls)[0].args[len((*calls)[0].args)-1]
			if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
				t.Errorf("temporary key file %s left behind: %v", keyPath, err)
			}
			if os.Getenv("GOOGLE_CREDENTIALS") != key {
				t.Error("key mode must set GOOGLE_CREDENTIALS for the Pulumi backend")
			}
		})
	}
}
