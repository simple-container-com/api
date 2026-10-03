// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func fakeGcloud(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	prev := runGcloud
	runGcloud = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return nil, nil
	}
	t.Cleanup(func() { runGcloud = prev })
	return &calls
}

// A keyless deploy reaches its cluster through gke-gcloud-auth-plugin, which
// asks gcloud for a token; gcloud must be signed in with the federated
// credentials Application Default Credentials name.
func TestInitStateStoreSignsGcloudInWithAmbientCredentials(t *testing.T) {
	calls := fakeGcloud(t)
	credFile := filepath.Join(t.TempDir(), "gha-creds-1.json")
	if err := os.WriteFile(credFile, []byte(`{"type":"external_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credFile)
	t.Setenv("GOOGLE_CREDENTIALS", "")

	if err := InitStateStore(context.Background(), &gcloud.StateStorageConfig{}, logger.New()); err != nil {
		t.Fatal(err)
	}

	want := [][]string{{"auth", "login", "--cred-file=" + credFile, "--quiet"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("gcloud calls = %v; want %v", *calls, want)
	}
	if os.Getenv("GOOGLE_CREDENTIALS") != "" {
		t.Error("ambient mode must not set GOOGLE_CREDENTIALS")
	}
}

func TestInitStateStoreAmbientWithoutCredentialFile(t *testing.T) {
	calls := fakeGcloud(t)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "absent.json"))

	if err := InitStateStore(context.Background(), &gcloud.StateStorageConfig{}, logger.New()); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("gcloud called with no credentials file: %v", *calls)
	}
}
