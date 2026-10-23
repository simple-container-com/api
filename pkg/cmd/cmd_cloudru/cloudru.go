// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package cmd_cloudru holds the `sc cloudru` subcommands, which converge Cloud.ru
// Evolution resources that have no Pulumi provider resource behind them.
//
// Why a CLI subcommand at all. Container Apps exist in no Terraform or Pulumi
// provider (not even in Cloud.ru's own evo-terraform), and Pulumi's Go SDK has no
// dynamic providers — so the only way to put a Container App inside Pulumi's
// resource model is to drive something external. `pkg/clouds/pulumi/cloudru`
// therefore creates a pulumi-command `local.Command` triplet that runs these
// subcommands, exactly as the VM driver runs `sc image ship`.
//
// Doing the REST calls in-process from the provisioning tier would be shorter and
// is wrong twice over: it would run during `pulumi preview`, where nothing is
// supposed to change, and it would sit outside the resource model, so nothing
// downstream could depend on it and a repeat deploy would have no way to know it
// had already happened.
//
// Every subcommand here reads ONE JSON document from stdin and writes ONE JSON
// document to stdout, with progress on stderr. Stdin because the document carries
// the IAM key secret and the stack's own secrets, and a command-line argument is
// readable in `ps` by every other process on the runner.
package cmd_cloudru

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/clouds/cloudru/evolution"
)

// NewCloudruCmd creates the `cloudru` command group.
func NewCloudruCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloudru",
		Short: "Cloud.ru Evolution operations",
		Long: `Converge Cloud.ru Evolution resources that no Terraform or Pulumi provider covers.

These commands are what the cloudru-container-app driver runs; they are usable by
hand for debugging a deploy. Each reads a JSON request from stdin and prints a JSON
result to stdout.`,
	}

	cmd.AddCommand(NewRegistryCmd())
	cmd.AddCommand(NewContainerAppCmd())

	return cmd
}

// readRequest decodes the stdin document.
//
// DisallowUnknownFields is deliberate: the provisioning tier and this command share
// the Go type, so an unknown field means the two halves are at different versions —
// an `sc` on the runner older than the module that generated the request. Failing
// loudly beats silently ignoring half a spec and reporting a green deploy.
func readRequest(in io.Reader) (evolution.Request, error) {
	var req evolution.Request
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// Never wraps the input: it holds the key secret and the stack's secrets.
		return req, errors.Errorf("failed to decode the request from stdin (%T): check that this `sc` is new enough for the deploy that produced it", err)
	}
	return req, nil
}

// run is the shared body of every subcommand: read stdin, call the client, print
// the result as JSON.
func run(ctx context.Context, fn func(context.Context, *evolution.Client, evolution.Request) (*evolution.Result, error)) error {
	req, err := readRequest(os.Stdin)
	if err != nil {
		return err
	}
	client := evolution.New(req.Credentials)
	res, err := fn(ctx, client, req)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		return errors.Wrapf(err, "failed to encode result")
	}
	fmt.Println(string(encoded))
	return nil
}

// progress writes a line to stderr. stdout is reserved for the one JSON result.
func progress(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
}
