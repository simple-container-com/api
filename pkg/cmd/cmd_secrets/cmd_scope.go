// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
)

// scopeCmd carries the shared flags for the `secrets scope` verbs. Scoped secrets
// live in .sc/stacks/<stack>/secrets.<scope>.yaml, governed by .sc/scopes.yaml —
// a per-scope recipient set so a pull_request CI job can hold a key that opens
// only its scope, not the whole-file store.
type scopeCmd struct {
	*secretsCmd
	scope   string
	stack   string
	keyFile string
}

// scDir returns the .sc config directory for the current repo.
func (s *scopeCmd) scDir() string {
	return filepath.Join(s.Root.Provisioner.Cryptor().Workdir(), api.ScConfigDirectory)
}

// privateKey resolves the private key used to decrypt scoped values, in order:
//  1. --key-file (an explicit PEM file);
//  2. env SC_KEY_<SCOPE> (e.g. SC_KEY_PR) — the per-scope CI key described by the
//     rollout — then the generic SC_SCOPE_KEY;
//  3. the ambient cryptor key (SIMPLE_CONTAINER_CONFIG / profile) for local use.
//
// This lets a pull_request scan job hold ONLY the scope key without also carrying
// a full SIMPLE_CONTAINER_CONFIG.
func (s *scopeCmd) privateKey() (string, error) {
	if s.keyFile != "" {
		b, err := os.ReadFile(s.keyFile)
		if err != nil {
			return "", errors.Wrapf(err, "failed to read --key-file %s", s.keyFile)
		}
		return string(b), nil
	}
	if s.scope != "" {
		envName := "SC_KEY_" + strings.ToUpper(strings.ReplaceAll(s.scope, "-", "_"))
		if v := os.Getenv(envName); strings.TrimSpace(v) != "" {
			return v, nil
		}
	}
	if v := os.Getenv("SC_SCOPE_KEY"); strings.TrimSpace(v) != "" {
		return v, nil
	}
	pk := s.Root.Provisioner.Cryptor().PrivateKey()
	if strings.TrimSpace(pk) == "" {
		return "", errors.New("no private key available: set --key-file, SC_KEY_<SCOPE> / SC_SCOPE_KEY, or SIMPLE_CONTAINER_CONFIG")
	}
	return pk, nil
}

func NewScopeCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scope",
		Short: "Manage per-scope secrets (secrets.<scope>.yaml) with per-scope recipients",
		Long: "Per-scope secrets let a narrow CI context (e.g. pull_request scan jobs) hold a key " +
			"that decrypts only one scope instead of the whole-file store. Recipients are governed " +
			"by .sc/scopes.yaml (CODEOWNERS-gated); values live in .sc/stacks/<stack>/secrets.<scope>.yaml.",
		SilenceUsage: true,
	}
	cmd.AddCommand(
		newScopeSetCmd(sCmd),
		newScopeGetCmd(sCmd),
		newScopeListCmd(sCmd),
		newScopeDeleteCmd(sCmd),
		newScopeAllowCmd(sCmd),
		newScopeDisallowCmd(sCmd),
		newScopeLintCmd(sCmd),
		newScopeDoctorCmd(sCmd),
	)
	return cmd
}

// addScopeStackFlags registers --scope (required) and -s/--stack (required) on a
// value-level command.
func (s *scopeCmd) addScopeStackFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&s.scope, "scope", "", "scope name (e.g. pr)")
	cmd.Flags().StringVarP(&s.stack, "stack", "s", "", "stack name (secrets.<scope>.yaml under .sc/stacks/<stack>)")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("stack")
}

// openForWrite loads (or creates) the scope file for --stack/--scope, and verifies
// its recipients match the authoritative scopes.yaml set — refusing to write into
// a file whose audience has drifted (reconcile via allow/disallow first).
func (s *scopeCmd) openForWrite() (*scoped.ScopeFile, *scoped.Scopes, string, error) {
	if err := scoped.ValidateScopeName(s.scope); err != nil {
		return nil, nil, "", err
	}
	sc, err := scoped.LoadScopes(scoped.ScopesPath(s.scDir()))
	if err != nil {
		return nil, nil, "", err
	}
	recipients, err := sc.Recipients(s.scope)
	if err != nil {
		return nil, nil, "", errors.Wrapf(err, "declare the scope first with `sc secrets scope allow`")
	}
	path := scoped.ScopeFilePath(s.scDir(), s.stack, s.scope)
	var f *scoped.ScopeFile
	if _, statErr := os.Stat(path); statErr == nil {
		if f, err = scoped.LoadScopeFile(path); err != nil {
			return nil, nil, "", err
		}
		if err := scoped.SameRecipients(f.Recipients, recipients); err != nil {
			return nil, nil, "", errors.Wrapf(err, "%s recipients drifted from %s — reconcile with `sc secrets scope allow/disallow`", filepath.Base(path), scoped.ScopesFileName)
		}
	} else if os.IsNotExist(statErr) {
		if f, err = scoped.NewScopeFile(s.scope, recipients); err != nil {
			return nil, nil, "", err
		}
	} else {
		return nil, nil, "", errors.Wrapf(statErr, "failed to stat %s", path)
	}
	return f, sc, path, nil
}

func newScopeSetCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "set KEY [VALUE]",
		Short: "Seal a value into a scope (VALUE from arg, or '-'/omitted reads stdin)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value, err := readValueArg(cmd, args)
			if err != nil {
				return err
			}
			f, _, path, err := s.openForWrite()
			if err != nil {
				return err
			}
			if err := f.Set(key, value); err != nil {
				return err
			}
			if err := f.Save(path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sealed %q into scope %q (%d recipient(s))\n", key, s.scope, len(f.Recipients))
			return nil
		},
	}
	s.addScopeStackFlags(cmd)
	return cmd
}

func newScopeGetCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Decrypt one scoped value with the ambient key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := scoped.ValidateScopeName(s.scope); err != nil {
				return err
			}
			pk, err := s.privateKey()
			if err != nil {
				return err
			}
			path := scoped.ScopeFilePath(s.scDir(), s.stack, s.scope)
			f, err := scoped.LoadScopeFile(path)
			if err != nil {
				return err
			}
			val, err := f.Get(args[0], pk)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), val)
			return nil
		},
	}
	s.addScopeStackFlags(cmd)
	cmd.Flags().StringVar(&s.keyFile, "key-file", "", "PEM private key to decrypt with (else SC_KEY_<SCOPE> / SC_SCOPE_KEY / ambient config)")
	return cmd
}

func newScopeListCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secret names in a scope (values are never printed)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := scoped.ScopeFilePath(s.scDir(), s.stack, s.scope)
			f, err := scoped.LoadScopeFile(path)
			if err != nil {
				return err
			}
			for _, k := range f.Keys() {
				fmt.Fprintln(cmd.OutOrStdout(), k)
			}
			return nil
		},
	}
	s.addScopeStackFlags(cmd)
	return cmd
}

func newScopeDeleteCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "delete KEY",
		Short: "Remove a value from a scope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := scoped.ScopeFilePath(s.scDir(), s.stack, s.scope)
			f, err := scoped.LoadScopeFile(path)
			if err != nil {
				return err
			}
			if !f.Delete(args[0]) {
				return errors.Errorf("secret %q not found in scope %q", args[0], s.scope)
			}
			return f.Save(path)
		},
	}
	s.addScopeStackFlags(cmd)
	return cmd
}

func newScopeAllowCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "allow PUBKEY",
		Short: "Add an SSH recipient to a scope (updates scopes.yaml and reseals its files)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.reconcileRecipients(cmd, args[0], true)
		},
	}
	cmd.Flags().StringVar(&s.scope, "scope", "", "scope name (e.g. pr)")
	_ = cmd.MarkFlagRequired("scope")
	return cmd
}

func newScopeDisallowCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "disallow PUBKEY",
		Short: "Remove an SSH recipient from a scope (reseals; prints rotate-values warning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.reconcileRecipients(cmd, args[0], false)
		},
	}
	cmd.Flags().StringVar(&s.scope, "scope", "", "scope name (e.g. pr)")
	_ = cmd.MarkFlagRequired("scope")
	return cmd
}

// reconcileRecipients updates scopes.yaml then reseals every scope file of the
// scope to the new recipient set. Resealing requires the ambient key to be a
// current recipient (it decrypts to re-encrypt).
func (s *scopeCmd) reconcileRecipients(cmd *cobra.Command, pubKey string, allow bool) error {
	if err := scoped.ValidateScopeName(s.scope); err != nil {
		return err
	}
	scopesPath := scoped.ScopesPath(s.scDir())
	sc, err := scoped.LoadScopes(scopesPath)
	if err != nil {
		return err
	}
	if allow {
		changed, aErr := sc.Allow(s.scope, pubKey)
		if aErr != nil {
			return aErr
		}
		if !changed {
			fmt.Fprintf(cmd.OutOrStdout(), "recipient already present in scope %q; nothing to do\n", s.scope)
			return nil
		}
	} else {
		removed, dErr := sc.Disallow(s.scope, pubKey)
		if dErr != nil {
			return dErr
		}
		if !removed {
			fmt.Fprintf(cmd.OutOrStdout(), "recipient not present in scope %q; nothing to do\n", s.scope)
			return nil
		}
	}
	recipients, err := sc.Recipients(s.scope)
	if err != nil {
		return err
	}
	// Phase 1: load + reseal every scope file of this scope IN MEMORY. The private
	// key is fetched lazily — only files that actually hold values need decrypting,
	// so declaring the first recipient of an empty scope needs no key. Nothing is
	// written until every reseal succeeds, so a mid-way decrypt/parse failure can
	// never leave some files resealed and scopes.yaml/other files behind (drift).
	files, err := scoped.ListScopeFiles(s.scDir())
	if err != nil {
		return err
	}
	type pendingSave struct {
		f    *scoped.ScopeFile
		path string
	}
	var pending []pendingSave
	var pk string
	for _, path := range files {
		if scoped.ScopeNameFromFile(path) != s.scope {
			continue
		}
		f, lErr := scoped.LoadScopeFile(path)
		if lErr != nil {
			return lErr
		}
		if len(f.Values) > 0 {
			if pk == "" {
				if pk, err = s.privateKey(); err != nil {
					return err
				}
			}
			if err := f.Reencrypt(recipients, pk); err != nil {
				return err
			}
		} else {
			f.Recipients = recipients
		}
		pending = append(pending, pendingSave{f: f, path: path})
	}
	// Phase 2: persist. Write the resealed files first, then scopes.yaml last, so a
	// reader never sees scopes.yaml advertise a recipient a file hasn't been
	// resealed for.
	for _, p := range pending {
		if err := p.f.Save(p.path); err != nil {
			return err
		}
	}
	if err := sc.Save(scopesPath); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "scope %q now has %d recipient(s); resealed %d file(s)\n", s.scope, len(recipients), len(pending))
	if !allow {
		fmt.Fprintf(cmd.OutOrStderr(), "WARNING: removing a recipient does NOT revoke access to values already committed in git history. Rotate every value in scope %q now.\n", s.scope)
	}
	return nil
}

func newScopeLintCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Verify every scope file: encryption, recipient set vs scopes.yaml, scope/filename binding",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scDir := s.scDir()
			sc, err := scoped.LoadScopes(scoped.ScopesPath(scDir))
			if err != nil {
				return err
			}
			files, err := scoped.ListScopeFiles(scDir)
			if err != nil {
				return err
			}
			var problems []string
			for _, path := range files {
				f, lErr := scoped.LoadScopeFile(path)
				if lErr != nil {
					problems = append(problems, lErr.Error())
					continue
				}
				if vErr := f.VerifyConsistency(); vErr != nil {
					problems = append(problems, vErr.Error())
				}
				want, rErr := sc.Recipients(f.Scope)
				if rErr != nil {
					problems = append(problems, fmt.Sprintf("%s: %s", filepath.Base(path), rErr.Error()))
					continue
				}
				if dErr := scoped.SameRecipients(f.Recipients, want); dErr != nil {
					problems = append(problems, fmt.Sprintf("%s: recipients drift vs %s: %s", filepath.Base(path), scoped.ScopesFileName, dErr.Error()))
				}
			}
			if len(problems) > 0 {
				for _, p := range problems {
					fmt.Fprintf(cmd.OutOrStderr(), "✗ %s\n", p)
				}
				return errors.Errorf("scoped secrets lint failed: %d problem(s)", len(problems))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %d scope file(s) OK\n", len(files))
			return nil
		},
	}
	return cmd
}

func newScopeDoctorCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report which scopes the ambient key can open",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pk, err := s.privateKey()
			if err != nil {
				return err
			}
			files, err := scoped.ListScopeFiles(s.scDir())
			if err != nil {
				return err
			}
			for _, path := range files {
				f, lErr := scoped.LoadScopeFile(path)
				if lErr != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "?  %s (unreadable: %v)\n", filepath.Base(path), lErr)
					continue
				}
				status := "no"
				if len(f.Keys()) == 0 {
					status = "empty"
				} else if _, gErr := f.Get(f.Keys()[0], pk); gErr == nil {
					status = "YES"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-5s scope=%s  file=%s\n", status, f.Scope, filepath.Base(path))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&s.keyFile, "key-file", "", "PEM private key to test with (else SC_KEY_<SCOPE> / SC_SCOPE_KEY / ambient config)")
	return cmd
}

// readValueArg returns the value from args[1], or reads stdin when args[1] is
// absent or "-".
func readValueArg(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 2 && args[1] != "-" {
		return args[1], nil
	}
	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", errors.Wrap(err, "failed to read value from stdin")
	}
	return strings.TrimRight(string(data), "\n"), nil
}
