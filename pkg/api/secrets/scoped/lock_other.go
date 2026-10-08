// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build !unix

package scoped

import "os"

// Releases are built for Linux and macOS only; elsewhere the store is unlocked.
func lockFile(*os.File) error   { return nil }
func unlockFile(*os.File) error { return nil }
