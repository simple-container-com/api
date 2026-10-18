// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/fs"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// The hook has to be REGISTERED, not merely written: login.go looks it up by
// provider type and only logs a warning when it is missing, so an unregistered
// hook reproduces the original failure with no error of its own.
func TestInitFileSystemStateStore_IsRegistered(t *testing.T) {
	RegisterTestingT(t)

	_, ok := pApi.InitStateStoreFuncByType[fs.StateStorageTypeFileSystem]
	Expect(ok).To(BeTrue(),
		"no init hook for %q: `sc provision` will warn and then fail in Pulumi with "+
			"`unable to open bucket …: no such file or directory`", fs.StateStorageTypeFileSystem)
}

func TestInitFileSystemStateStore(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()
	log := logger.New()

	t.Run("creates a missing directory, 0700", func(t *testing.T) {
		RegisterTestingT(t)
		dir := filepath.Join(t.TempDir(), "nested", "state")

		Expect(initFileSystemStateStore(ctx, &fs.FileSystemStateStorage{Path: dir}, log)).To(Succeed())

		st, err := os.Stat(dir)
		Expect(err).ToNot(HaveOccurred())
		Expect(st.IsDir()).To(BeTrue())
		// The checkpoint holds the stack's secrets; other local users have no
		// business listing it.
		Expect(st.Mode().Perm()).To(Equal(os.FileMode(0o700)))
	})

	t.Run("an explicit file url works the same way", func(t *testing.T) {
		RegisterTestingT(t)
		dir := filepath.Join(t.TempDir(), "state")

		Expect(initFileSystemStateStore(ctx, &fs.FileSystemStateStorage{Path: "file://" + dir}, log)).To(Succeed())

		st, err := os.Stat(dir)
		Expect(err).ToNot(HaveOccurred())
		Expect(st.IsDir()).To(BeTrue())
	})

	t.Run("an existing directory is left alone", func(t *testing.T) {
		RegisterTestingT(t)
		dir := t.TempDir()
		marker := filepath.Join(dir, ".pulumi-stays")
		Expect(os.WriteFile(marker, []byte("keep"), 0o600)).To(Succeed())

		Expect(initFileSystemStateStore(ctx, &fs.FileSystemStateStorage{Path: dir}, log)).To(Succeed())

		// MkdirAll on an existing path is a no-op, which is the point: this runs on
		// every provision, not only the first.
		Expect(marker).To(BeAnExistingFile())
	})

	t.Run("an empty path is reported, not resolved to the working directory", func(t *testing.T) {
		RegisterTestingT(t)
		err := initFileSystemStateStore(ctx, &fs.FileSystemStateStorage{}, log)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty path"))
	})

	t.Run("a non-file scheme is not ours to create", func(t *testing.T) {
		RegisterTestingT(t)
		// No mkdir, no error: the operator gets Pulumi's own message about the
		// bucket rather than a directory named after it.
		Expect(initFileSystemStateStore(ctx, &fs.FileSystemStateStorage{Path: "s3://bucket/prefix"}, log)).To(Succeed())
		Expect("bucket").ToNot(BeADirectory())
	})
}
