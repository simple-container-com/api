//go:build tools
// +build tools

// this file references indirect dependencies that are used during the build

package main

import (
	_ "github.com/atombender/go-jsonschema/pkg/generator"
	_ "mvdan.cc/gofumpt"
)
