// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build !unix

package scoped

import "os"

// Releases are built for Linux and macOS only. Elsewhere the store is not locked,
// and concurrent changes can lose updates.
func tryLock(*os.File) (bool, error) { return true, nil }
func unlock(*os.File) error          { return nil }
