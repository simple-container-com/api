// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_secrets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/secrets/scoped"
	"github.com/simple-container-com/api/pkg/provisioner"
)

// scopeCmd carries the shared flags for the `secrets scope` verbs. Scoped secrets
// live in <stacksDir>/<stack>/secrets.<scope>.yaml (default .sc/stacks), governed by .sc/scopes.yaml:
// a per-scope recipient set so a pull_request CI job can hold a key that opens
// only its scope, not the whole-file store.
type scopeCmd struct {
	*secretsCmd
	scope   string
	stack   string
	keyFile string
	dir     string
}

// scDir returns the .sc config directory for the current repo.
func (s *scopeCmd) scDir() string {
	return filepath.Join(s.Root.Provisioner.Cryptor().Workdir(), api.ScConfigDirectory)
}

// stacksDir resolves the stacks root exactly as deploys do (--dir, else the config
// file's stacksDir, else .sc/stacks), so scope files are written where they are read.
func (s *scopeCmd) stacksDir() (string, error) {
	root := s.Root.Provisioner.Cryptor().Workdir()
	var cfg *api.ConfigFile
	if s.dir == "" {
		profile := provisioner.DefaultProfile
		if s.Root.Params != nil && s.Root.Params.Profile != "" {
			profile = s.Root.Params.Profile
		}
		_, statErr := os.Stat(api.ConfigFilePath(root, profile))
		if os.Getenv(api.ScConfigEnvVariable) != "" || statErr == nil {
			c, err := api.ReadConfigFile(root, profile)
			if err != nil {
				return "", errors.Wrap(err, "failed to resolve the stacks directory from the config file")
			}
			cfg = c
		}
	}
	return provisioner.ResolveStacksDir(root, cfg, s.dir), nil
}

func (s *scopeCmd) scopeFilePath() (string, error) {
	if err := scoped.ValidateScopeName(s.scope); err != nil {
		return "", err
	}
	if err := scoped.ValidateStackName(s.stack); err != nil {
		return "", err
	}
	stacksDir, err := s.stacksDir()
	if err != nil {
		return "", err
	}
	return scoped.ScopeFilePath(stacksDir, s.stack, s.scope), nil
}

func (s *scopeCmd) addDirFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&s.dir, "dir", "d", "", "root directory for stack configurations (default: stacksDir from the config file, else .sc/stacks)")
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
		return parsedKey(string(b), "--key-file "+s.keyFile)
	}
	if s.scope != "" {
		envName := scoped.ScopeKeyEnvName(s.scope)
		if v := os.Getenv(envName); strings.TrimSpace(v) != "" {
			return parsedKey(v, envName)
		}
	}
	if v := os.Getenv("SC_SCOPE_KEY"); strings.TrimSpace(v) != "" {
		return parsedKey(v, "SC_SCOPE_KEY")
	}
	pk := s.Root.Provisioner.Cryptor().PrivateKey()
	if strings.TrimSpace(pk) == "" {
		return "", errNoPrivateKey
	}
	return pk, nil
}

// errNoPrivateKey means no key was given at all, which is fine for a scope whose
// recipients are KMS keys. Any other privateKey error is a key that was given and
// cannot be used, and the command stops on it.
var errNoPrivateKey = errors.New("no private key available: set --key-file, SC_KEY_<SCOPE> / SC_SCOPE_KEY, or SIMPLE_CONTAINER_CONFIG")

// optionalKeys returns the key privateKey resolves, if any, failing only on a
// key that was given but cannot be parsed.
func (s *scopeCmd) optionalKeys() ([]string, error) {
	pk, err := s.privateKey()
	if errors.Is(err, errNoPrivateKey) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []string{pk}, nil
}

// parsedKey returns key if it parses, and otherwise an error naming where it came
// from: a key that was given explicitly and cannot be used must not read as "not
// a recipient".
func parsedKey(key, source string) (string, error) {
	if err := scoped.ValidatePrivateKey(key); err != nil {
		return "", errors.Wrapf(err, "%s cannot be parsed as a private key", source)
	}
	return key, nil
}

// declaredScopes lists the scopes in scopes.yaml, so only their SC_KEY_*
// variables are taken for scope keys; nil when it cannot be read.
func (s *scopeCmd) declaredScopes() []string {
	sc, err := scoped.LoadScopes(scoped.ScopesPath(s.scDir()))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(sc.Scopes))
	for name := range sc.Scopes {
		names = append(names, name)
	}
	return names
}

// lock holds the scope store lock for one read-modify-write command.
func (s *scopeCmd) lock(cmd *cobra.Command) (func(), error) {
	return scoped.LockStore(s.scDir(), func() {
		fmt.Fprintf(cmd.OutOrStderr(), "waiting for another sc process to finish changing %s...\n", s.scDir())
	})
}

// validateNames checks --scope and --stack before a command locks the store, so
// a mistyped call fails without touching it.
func (s *scopeCmd) validateNames() error {
	if err := scoped.ValidateScopeName(s.scope); err != nil {
		return err
	}
	return scoped.ValidateStackName(s.stack)
}

func NewScopeCmd(sCmd *secretsCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scope",
		Short: "Manage per-scope secrets (secrets.<scope>.yaml) with per-scope recipients",
		Long: "Per-scope secrets let a narrow CI context (e.g. pull_request scan jobs) hold a key " +
			"that decrypts only one scope instead of the whole-file store. Recipients are governed " +
			"by .sc/scopes.yaml (CODEOWNERS-gated); values live in <stacksDir>/<stack>/secrets.<scope>.yaml (default .sc/stacks).",
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
	cmd.Flags().StringVarP(&s.stack, "stack", "s", "", "stack name (secrets.<scope>.yaml under the stacks dir)")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("stack")
	s.addDirFlag(cmd)
}

// openForWrite loads (or creates) the scope file for --stack/--scope, and verifies
// its recipients match the authoritative scopes.yaml set — refusing to write into
// a file whose audience has drifted (reconcile via allow/disallow first).
func (s *scopeCmd) openForWrite() (*scoped.ScopeFile, *scoped.Scopes, string, error) {
	if err := scoped.ValidateScopeName(s.scope); err != nil {
		return nil, nil, "", err
	}
	if err := scoped.ValidateStackName(s.stack); err != nil {
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
	stacksDir, err := s.stacksDir()
	if err != nil {
		return nil, nil, "", err
	}
	path := scoped.ScopeFilePath(stacksDir, s.stack, s.scope)
	// A typo in --stack would otherwise create a stack directory nothing deploys.
	if st, dErr := os.Stat(filepath.Dir(path)); dErr != nil || !st.IsDir() {
		return nil, nil, "", errors.Errorf("stack %q has no directory %s; check --stack and --dir", s.stack, filepath.Dir(path))
	}
	var f *scoped.ScopeFile
	if _, statErr := os.Stat(path); statErr == nil {
		if f, err = scoped.LoadScopeFile(path); err != nil {
			return nil, nil, "", err
		}
		if err := scoped.SameRecipients(f.Recipients, recipients); err != nil {
			return nil, nil, "", errors.Wrapf(err, "%s recipients drifted from %s — reconcile with `sc secrets scope allow/disallow`", filepath.Base(path), scoped.ScopesFileName)
		}
	} else if os.IsNotExist(statErr) {
		if f, err = scoped.NewScopeFile(s.stack, s.scope, recipients); err != nil {
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
		Use:   "set KEY VALUE|-",
		Short: "Seal a value into a scope (VALUE from the argument, or '-' to read stdin)",
		Long: "Seal a value into a scope. The value is taken from the VALUE argument, or " +
			"read from stdin when VALUE is '-'. Put '--' before KEY when the value starts " +
			"with '-'. When read from stdin, a single " +
			"trailing newline is stripped (the usual echo/heredoc artifact); pipe binary or " +
			"exact-match data via the VALUE argument if that matters.\n\n" +
			"A KEY of auth:<name> seals an auth entry instead of a value: the YAML that " +
			"would sit under <name> in secrets.yaml's auth map. A deploy that can open the " +
			"scope but not the whole-file store gets its ${auth:<name>} from it. With " +
			"credentials left empty, a gcp-service-account entry uses the environment's " +
			"credentials (Workload Identity Federation in CI).",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				// Omitting the value used to read stdin, so "set K -v" stored stdin while
				// -v went to the verbose flag. The value is now always explicit.
				return errors.New("VALUE is required: pass it as the second argument, or '-' to read it from stdin (put '--' before KEY when the value starts with '-')")
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value, err := readValueArg(cmd, args)
			if err != nil {
				return err
			}
			if strings.HasPrefix(key, scoped.AuthKeyPrefix) {
				// Refuse a broken auth entry now rather than at the next deploy.
				if _, err := api.ParseAuthDescriptor(value); err != nil {
					return errors.Wrapf(err, "%s must be the YAML of one auth entry (type + config)", key)
				}
			}
			if err := s.validateNames(); err != nil {
				return err
			}
			unlock, err := s.lock(cmd)
			if err != nil {
				return err
			}
			defer unlock()
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
		Short: "Decrypt one scoped value with a scope key or ambient AWS (KMS) credentials",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := scoped.ValidateScopeName(s.scope); err != nil {
				return err
			}
			// An SSH key is optional: a KMS-recipient scope is opened via the ambient
			// credential chain (e.g. an OIDC role) with no private key at all.
			keys, err := s.optionalKeys()
			if err != nil {
				return err
			}
			path, err := s.scopeFilePath()
			if err != nil {
				return err
			}
			f, err := scoped.LoadScopeFile(path)
			if err != nil {
				return err
			}
			opener := scoped.NewOpener(keys, true)
			defer func() { _ = opener.Close() }()
			val, owned, err := f.Open(args[0], opener)
			if err != nil {
				return err
			}
			if !owned {
				if len(keys) == 0 {
					return errors.Wrapf(errNoPrivateKey, "no SSH key and no KMS recipient could open %q in scope %q", args[0], s.scope)
				}
				return errors.Errorf("key is not a recipient of %q in scope %q (and no KMS recipient was openable)", args[0], s.scope)
			}
			fmt.Fprintln(cmd.OutOrStdout(), val)
			return nil
		},
	}
	s.addScopeStackFlags(cmd)
	cmd.Flags().StringVar(&s.keyFile, "key-file", "", "PEM private key to decrypt with (else SC_KEY_<SCOPE> / SC_SCOPE_KEY / ambient config / ambient AWS for KMS recipients)")
	return cmd
}

func newScopeListCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secret names in a scope (values are never printed)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := s.scopeFilePath()
			if err != nil {
				return err
			}
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
			if err := s.validateNames(); err != nil {
				return err
			}
			unlock, err := s.lock(cmd)
			if err != nil {
				return err
			}
			defer unlock()
			path, err := s.scopeFilePath()
			if err != nil {
				return err
			}
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
		Use:   "allow RECIPIENT",
		Short: "Add a recipient — SSH pubkey, awskms://<key>?region=<r>, or gcpkms://projects/<p>/locations/<l>/keyRings/<r>/cryptoKeys/<k> — to a scope (updates scopes.yaml and reseals its files)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.reconcileRecipients(cmd, args[0], true)
		},
	}
	cmd.Flags().StringVar(&s.scope, "scope", "", "scope name (e.g. pr)")
	_ = cmd.MarkFlagRequired("scope")
	s.addDirFlag(cmd)
	return cmd
}

func newScopeDisallowCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "disallow RECIPIENT",
		Short: "Remove a recipient — SSH pubkey or awskms:// / gcpkms:// URL — from a scope (reseals; prints rotate-values warning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.reconcileRecipients(cmd, args[0], false)
		},
	}
	cmd.Flags().StringVar(&s.scope, "scope", "", "scope name (e.g. pr)")
	_ = cmd.MarkFlagRequired("scope")
	s.addDirFlag(cmd)
	return cmd
}

// reconcileRecipients updates scopes.yaml then reseals every scope file of the
// scope to the new recipient set. Resealing requires the ambient key to be a
// current recipient (it decrypts to re-encrypt).
func (s *scopeCmd) reconcileRecipients(cmd *cobra.Command, pubKey string, allow bool) error {
	if err := scoped.ValidateScopeName(s.scope); err != nil {
		return err
	}
	unlock, err := s.lock(cmd)
	if err != nil {
		return err
	}
	defer unlock()
	scopesPath := scoped.ScopesPath(s.scDir())
	sc, err := scoped.LoadScopes(scopesPath)
	if err != nil {
		return err
	}
	// changed: scopes.yaml itself changes. Even when it does not, files whose
	// recipients drifted from it are resealed: lint and set send the operator here
	// to reconcile exactly that.
	var changed bool
	if allow {
		var aErr error
		if changed, aErr = sc.Allow(s.scope, pubKey); aErr != nil {
			return aErr
		}
	} else {
		var dErr error
		if changed, dErr = sc.Disallow(s.scope, pubKey); dErr != nil {
			return dErr
		}
		// Refuse to strand a scope with no recipients — the resulting store would be
		// undecryptable and every value orphaned. Deleting the scope is the explicit
		// path for that.
		if left := sc.Scopes[s.scope].Recipients; changed && len(left) == 0 {
			return errors.Errorf("refusing to remove the last recipient of scope %q; delete the scope's files and its %s entry instead", s.scope, scoped.ScopesFileName)
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
	stacksDir, err := s.stacksDir()
	if err != nil {
		return err
	}
	files, err := scoped.ListScopeFiles(stacksDir)
	if err != nil {
		return err
	}
	type pendingSave struct {
		f    *scoped.ScopeFile
		path string
	}
	var pending []pendingSave
	var opener *scoped.Opener
	dropped := false // some file loses a recipient: its committed values need rotating
	defer func() {
		if opener != nil {
			_ = opener.Close()
		}
	}()
	for _, path := range files {
		if scoped.ScopeNameFromFile(path) != s.scope {
			continue
		}
		f, lErr := scoped.LoadScopeFile(path)
		if lErr != nil {
			return lErr
		}
		if !changed && scoped.SameRecipients(f.Recipients, recipients) == nil {
			continue
		}
		if scoped.DropsRecipients(f.Recipients, recipients) {
			dropped = true
		}
		if len(f.Values) > 0 {
			if opener == nil {
				// Reseal decrypts current values first: build an Opener from any SSH key
				// we have PLUS KMS (so a scope whose current recipient is a KMS key can
				// still be resealed by an operator with kms:Decrypt). A missing SSH key is
				// not fatal here — KMS may open it; Reencrypt fails closed if neither can.
				keys, kErr := s.optionalKeys()
				if kErr != nil {
					return kErr
				}
				opener = scoped.NewOpener(keys, true)
			}
			if err := f.Reencrypt(recipients, opener); err != nil {
				return err
			}
		} else {
			f.Recipients = recipients
		}
		pending = append(pending, pendingSave{f: f, path: path})
	}
	if !changed && len(pending) == 0 {
		verb := "already present in"
		if !allow {
			verb = "not present in"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "recipient %s scope %q and every file matches; nothing to do\n", verb, s.scope)
		return nil
	}
	// scopes.yaml is rendered before any file is written, so one this command
	// cannot edit stops it with every scope file untouched.
	var commitScopes func() error
	if changed {
		if commitScopes, err = sc.Prepare(scopesPath); err != nil {
			return err
		}
	}
	// Phase 2: persist. Write the resealed files first, then scopes.yaml last, so a
	// reader never sees scopes.yaml advertise a recipient a file hasn't been
	// resealed for.
	for _, p := range pending {
		if err := p.f.Save(p.path); err != nil {
			return err
		}
	}
	if commitScopes != nil {
		if err := commitScopes(); err != nil {
			return err
		}
	}
	if changed {
		fmt.Fprintf(cmd.OutOrStdout(), "scope %q now has %d recipient(s); resealed %d file(s)\n", s.scope, len(recipients), len(pending))
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "scopes.yaml unchanged; resealed %d file(s) of scope %q that had drifted from it\n", len(pending), s.scope)
	}
	if dropped || (changed && !allow) {
		fmt.Fprintf(cmd.OutOrStderr(), "WARNING: removing a recipient does NOT revoke access to values already committed in git history. Rotate every value in scope %q now.\n", s.scope)
	}
	return nil
}

func newScopeLintCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	var allowLegacyDuplicates bool
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
			stacksDir, err := s.stacksDir()
			if err != nil {
				return err
			}
			files, lookalikes, err := scoped.ListScopeFilesAndLookalikes(stacksDir)
			if err != nil {
				return err
			}
			var problems, warnings []string
			for _, p := range lookalikes {
				warnings = append(warnings, fmt.Sprintf("%s is not a scope file (no stack, scope or recipients field) and is ignored; if it is one, restore those fields", p))
			}
			// Files left under the default dir are invisible to deploys once another
			// stacks dir is configured; the write would otherwise "succeed" and never apply.
			defaultDir := provisioner.ResolveStacksDir(s.Root.Provisioner.Cryptor().Workdir(), nil, "")
			if filepath.Clean(defaultDir) != filepath.Clean(stacksDir) {
				if stray, sErr := scoped.ListScopeFiles(defaultDir); sErr == nil {
					for _, f := range stray {
						warnings = append(warnings, fmt.Sprintf("%s is under %s but the configured stacks dir is %s; deploys will not read it", f, defaultDir, stacksDir))
					}
				}
			}
			keys, err := s.optionalKeys()
			if err != nil {
				return err
			}
			// lint and doctor span every scope, so every scope key the job holds counts,
			// as it does at deploy time; one that does not parse is an error.
			if bad := scoped.InvalidEnvScopeKeys(s.declaredScopes()); len(bad) > 0 {
				return errors.Errorf("%s cannot be parsed as a private key", strings.Join(bad, ", "))
			}
			keys = append(keys, scoped.EnvScopeKeys()...)
			opener := scoped.NewOpener(keys, true)
			defer func() { _ = opener.Close() }()
			loaded := map[string]map[string]*scoped.ScopeFile{} // stack -> scope -> file
			// keyScopes[stack][key] = scopes that define it, to catch cross-scope
			// duplicates (the resolver hard-fails on these at deploy; lint catches
			// them first).
			keyScopes := map[string]map[string][]string{}
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
				} else if dErr := scoped.SameRecipients(f.Recipients, want); dErr != nil {
					problems = append(problems, fmt.Sprintf("%s: recipients drift vs %s: %s", filepath.Base(path), scoped.ScopesFileName, dErr.Error()))
				}
				// A broken auth entry fails every deploy that opens the scope, so check
				// the ones this run can open.
				for _, k := range f.Keys() {
					if !strings.HasPrefix(k, scoped.AuthKeyPrefix) {
						continue
					}
					v, owned, oErr := f.Open(k, opener)
					if oErr != nil || !owned {
						continue
					}
					// The parse error can quote the decrypted value, so it is not printed.
					if _, aErr := api.ParseAuthDescriptor(v); aErr != nil {
						problems = append(problems, fmt.Sprintf("%s: %s does not parse as an auth entry; inspect it with `sc secrets scope get` where its value may be shown", filepath.Base(path), k))
					}
				}
				if keyScopes[f.Stack] == nil {
					keyScopes[f.Stack] = map[string][]string{}
					loaded[f.Stack] = map[string]*scoped.ScopeFile{}
				}
				loaded[f.Stack][f.Scope] = f
				for _, k := range f.Keys() {
					keyScopes[f.Stack][k] = append(keyScopes[f.Stack][k], f.Scope)
				}
			}
			// A key must live in exactly one mode/scope so deploy-time resolution is
			// deterministic: flag a key present in >1 scope, or in both a scope and
			// the stack's legacy secrets.yaml (mode A, which wins silently at deploy).
			for stack, keys := range keyScopes {
				var legacy map[string]string
				legacyPath := filepath.Join(scoped.StackDir(stacksDir, stack), api.SecretsDescriptorFileName)
				if _, statErr := os.Stat(legacyPath); statErr == nil {
					if d, rErr := api.ReadDescriptor(legacyPath, &api.SecretsDescriptor{}); rErr == nil {
						legacy = d.Values
					}
				}
				for k, scopes := range keys {
					if len(scopes) > 1 {
						sort.Strings(scopes)
						// Ambiguous only if the copies differ. Compare them when this
						// run can open every copy; otherwise say so, since a deploy that
						// opens more than one fails if they differ.
						same, known := sameValue(loaded[stack], scopes, k, opener)
						switch {
						case known && !same:
							problems = append(problems, fmt.Sprintf("stack %q: key %q has different values in scopes %v (ambiguous at deploy)", stack, k, scopes))
						case !known:
							warnings = append(warnings, fmt.Sprintf("stack %q: key %q is in scopes %v; could not compare the copies without a key that opens all of them (a deploy that opens more than one fails if they differ)", stack, k, scopes))
						}
					}
					if _, inLegacy := legacy[k]; inLegacy {
						msg := fmt.Sprintf("stack %q: key %q is in both scope %q and the legacy secrets.yaml (mode A wins silently)", stack, k, scopes[0])
						if allowLegacyDuplicates {
							warnings = append(warnings, msg)
						} else {
							problems = append(problems, msg)
						}
					}
				}
			}
			for _, w := range warnings {
				fmt.Fprintf(cmd.OutOrStderr(), "! %s\n", w)
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
	s.addDirFlag(cmd)
	cmd.Flags().BoolVar(&allowLegacyDuplicates, "allow-legacy-duplicates", false,
		"report, not fail, keys that are also in the whole-file secrets.yaml: expected while client deploys move off it, "+
			"since the store keeps serving clients that have not moved")
	return cmd
}

// sameValue opens key in every named scope. known is false when any copy could
// not be opened with the material at hand.
func sameValue(files map[string]*scoped.ScopeFile, scopes []string, key string, o *scoped.Opener) (same, known bool) {
	var first string
	for i, scope := range scopes {
		f := files[scope]
		if f == nil {
			return false, false
		}
		v, owned, err := f.Open(key, o)
		if err != nil || !owned {
			return false, false
		}
		if i == 0 {
			first = v
		} else if v != first {
			return false, true
		}
	}
	return true, true
}

func newScopeDoctorCmd(sCmd *secretsCmd) *cobra.Command {
	s := &scopeCmd{secretsCmd: sCmd}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report which scopes the current key or AWS (KMS) credentials can open",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// SSH key is optional — a KMS recipient is opened via ambient credentials.
			keys, err := s.optionalKeys()
			if err != nil {
				return err
			}
			// lint and doctor span every scope, so every scope key the job holds counts,
			// as it does at deploy time; one that does not parse is an error.
			if bad := scoped.InvalidEnvScopeKeys(s.declaredScopes()); len(bad) > 0 {
				return errors.Errorf("%s cannot be parsed as a private key", strings.Join(bad, ", "))
			}
			keys = append(keys, scoped.EnvScopeKeys()...)
			if len(keys) == 0 {
				fmt.Fprintf(cmd.OutOrStderr(), "! no private key found (--key-file, SC_KEY_<SCOPE>, SC_SCOPE_KEY or SIMPLE_CONTAINER_CONFIG); only KMS recipients are tested\n")
			}
			opener := scoped.NewOpener(keys, true)
			defer func() { _ = opener.Close() }()
			stacksDir, err := s.stacksDir()
			if err != nil {
				return err
			}
			files, err := scoped.ListScopeFiles(stacksDir)
			if err != nil {
				return err
			}
			for _, path := range files {
				f, lErr := scoped.LoadScopeFile(path)
				if lErr != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "?     scope=?  file=%s (unreadable: %v)\n", filepath.Base(path), lErr)
					continue
				}
				status := "no"
				if len(f.Keys()) == 0 {
					status = "empty"
				}
				// The first value this run can open settles it; lint checks the rest.
				for _, k := range f.Keys() {
					if _, owned, gErr := f.Open(k, opener); gErr != nil {
						status = "ERR"
						break
					} else if owned {
						status = "YES"
						break
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-5s scope=%s  file=%s\n", status, f.Scope, filepath.Base(path))
			}
			return nil
		},
	}
	s.addDirFlag(cmd)
	cmd.Flags().StringVar(&s.keyFile, "key-file", "", "PEM private key to test with (else SC_KEY_<SCOPE> / SC_SCOPE_KEY / ambient config / ambient AWS for KMS)")
	return cmd
}

// readValueArg returns the value from args[1], or reads stdin when args[1] is "-".
func readValueArg(cmd *cobra.Command, args []string) (string, error) {
	if len(args) < 2 {
		return "", errors.New("VALUE is required")
	}
	if args[1] != "-" {
		return args[1], nil
	}
	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", errors.Wrap(err, "failed to read value from stdin")
	}
	// Strip a SINGLE trailing newline — the usual `echo`/heredoc artifact — rather than
	// all trailing newlines, so a value with intentional trailing newlines (e.g. a PEM
	// key piped via `set KEY - < key.pem`) keeps all but the last. Also drop a trailing
	// CR so a CRLF line ending (Windows / some editors) does not leave a stray \r.
	return strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"), nil
}
