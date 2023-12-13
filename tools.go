//go:build tools
// +build tools

//go:generate go build -o ./bin/mockery github.com/vektra/mockery/v2
//go:generate go build -o ./bin/gofumpt mvdan.cc/gofumpt

// this file references indirect dependencies that are used during the build

package main

import (
	_ "github.com/atombender/go-jsonschema/pkg/generator"
	_ "github.com/vektra/mockery/v2"
	_ "mvdan.cc/gofumpt"
)
