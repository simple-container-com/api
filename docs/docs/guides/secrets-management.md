# Secrets Management with Simple Container

This comprehensive guide covers how to manage secrets and confidential files using Simple Container's built-in secrets management system. Simple Container encrypts secrets to your team's SSH public keys (RSA and ed25519) so they can be stored and shared safely in your Git repository.

## Overview

Simple Container's secrets management provides:

- **Public-key encryption** to SSH recipients (RSA and ed25519) for secure secret storage
- **Team-based access control** with public key management
- **Git-native workflow** for secret versioning and collaboration
- **Built-in commands** for easy secret lifecycle management
- **Automatic encryption/decryption** during deployment
- **Multi-key encryption** - each secret is encrypted with all authorized public keys

## How Encryption Works

Simple Container uses a multi-key encryption approach:

1. **Public keys are registered** in `.sc/secrets.yaml` file
2. **Every secret file is encrypted** with ALL registered public keys
3. **Anyone with an authorized private key** can decrypt secrets using `sc secrets reveal`
4. **Adding/removing team members** requires re-encrypting all secrets

This means that when you run `sc secrets reveal`, the system uses your private key to decrypt secrets that were encrypted with your corresponding public key (along with all other team members' public keys).

### Cipher per recipient type

Each recipient public key gets its own encryption of the secret, using a scheme appropriate to the key type:

- **`ssh-rsa` recipients** — RSA-OAEP (SHA-256).
- **`ssh-ed25519` recipients** — an ephemeral-static **X25519 ECDH** sealed box (HKDF-SHA256 + ChaCha20-Poly1305): the content key is derived from the ECDH shared secret, so only the holder of the ed25519 private key can decrypt.

### Store format & version compatibility

The encrypted store `.sc/secrets.yaml` carries an optional `schemaVersion` field
that identifies its schema. The current format is **schema version 0** — written
without an explicit `schemaVersion:` field, so existing stores are unchanged.

`sc` is **fail-closed** on this: a build refuses to read a store whose
`schemaVersion` is newer than it understands, rather than silently dropping fields
it cannot model and corrupting the store on the next write. The error looks like:

```
secrets file ".sc/secrets.yaml" is schema version N, but this sc build supports up
to schema version M; upgrade sc (refusing to read to avoid data loss)
```

This guarantee holds on **every** path — the local CLI and the GitHub Actions
deploy/reveal flows (a too-new store halts the run instead of falling back to
"no secrets"). The practical implication: **keep `sc` up to date across your team
and CI** before any newer store format is adopted, so that no older binary is
left unable to read — or, worse, able to overwrite — the new on-disk schema.

## Prerequisites

Before working with secrets, ensure you have:

- Simple Container CLI installed
- An SSH key pair — RSA (2048-bit) or ed25519
- Access to the project repository

## Configuration Override

Simple Container supports configuration override via the `SIMPLE_CONTAINER_CONFIG` environment variable. This is particularly useful for CI/CD environments where you want to provide configuration without creating files:

```shell
# Override configuration via environment variable (using explicit keys)
export SIMPLE_CONTAINER_CONFIG="
projectName: your-project-name
privateKey: |
  -----BEGIN RSA PRIVATE KEY-----
  MIIEpAIBAAKCAQEA...
  -----END RSA PRIVATE KEY-----
publicKey: 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... user@host'
"

# Or using key paths
export SIMPLE_CONTAINER_CONFIG="
privateKeyPath: /path/to/ci/private/key
publicKeyPath: /path/to/ci/public/key
projectName: your-project-name
"

# Now sc commands will use the environment configuration
sc secrets reveal
```

**Configuration Content Options:**
Configuration files (`.sc/cfg.default.yaml` or `.sc/cfg.<profile>.yaml`) and the `SIMPLE_CONTAINER_CONFIG` environment variable can contain:

1. **Key Paths** (typical for local development):
   ```yaml
   privateKeyPath: ~/.ssh/id_rsa
   publicKeyPath: ~/.ssh/id_rsa.pub
   projectName: your-project-name
   ```

2. **Explicit Key Content** (typical for CI/CD environments):
   ```yaml
   projectName: your-project-name
   privateKey: |
     -----BEGIN RSA PRIVATE KEY-----
     MIIEpAIBAAKCAQEA...
     -----END RSA PRIVATE KEY-----
   publicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ..."
   ```

This approach is recommended for CI/CD pipelines as it avoids creating configuration files and allows for secure key management through environment variables.

### Install Simple Container CLI

```shell
bash <(curl -Ls "https://dist.simple-container.com/sc.sh") --version
```

## SSH Key Requirements

Simple Container currently supports SSH RSA keys of size 2048 bits. Your SSH key pair should be:

- **Private key**: `~/.ssh/id_rsa` (never commit or share)
- **Public key**: `~/.ssh/id_rsa.pub` (safe to share)
- **No passphrase**: Keys should not have a passphrase for automation

### Generate SSH Keys

If you don't have SSH keys, generate them:

```shell
# Using ssh-keygen
ssh-keygen -t rsa -b 2048 -f ~/.ssh/id_rsa -N ""

# Or using sc (generates and configures automatically)
sc secrets init -g
```

## Core Secrets Commands

### `sc secrets init`

Initialize secrets management in your project or configure your local environment.

```shell
# Initialize with existing SSH key
sc secrets init

# Generate new SSH key and initialize
sc secrets init -g

# Generate separate key pair for CI/CD environment
sc secrets init --profile github --generate

# Initialize with verbose output
sc secrets init -g --verbose

# Initialize and make initial commit
sc secrets init -g --commit
```

**What it does:**

- Creates `.sc/cfg.default.yaml` configuration file (or `.sc/cfg.<profile>.yaml` when using `--profile`)
- Configures your SSH key paths or embeds key contents
- Sets up local secrets environment
- When using `--profile`, creates separate configuration for different environments (e.g., CI/CD)
- Configuration can be overridden using `SIMPLE_CONTAINER_CONFIG` environment variable

**Example output:**
```yaml
# .sc/cfg.default.yaml (default profile - using key paths)
privateKeyPath: ~/.ssh/id_rsa
publicKeyPath: ~/.ssh/id_rsa.pub
projectName: simple-container-api

# .sc/cfg.github.yaml (when using --profile github - using key paths)
privateKeyPath: .sc/profiles/github/id_rsa
publicKeyPath: .sc/profiles/github/id_rsa.pub
projectName: simple-container-api

# Example with explicit key content (typical for CI/CD)
projectName: simple-container-api
privateKey: |
  -----BEGIN RSA PRIVATE KEY-----
  MIIEpAIBAAKCAQEA1234567890abcdef...
  -----END RSA PRIVATE KEY-----
publicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ1234567890abcdef... user@host"
```

### `sc secrets list`

Display all available secrets in the current project.

```shell
# List all secrets
sc secrets list

# List secrets with verbose output
sc secrets list --verbose

# List secrets using specific profile
sc secrets list --profile github
```

**Example output:**
```
pkg/secrets/database-password.txt
pkg/secrets/api-keys.json
pkg/secrets/ssl-cert.pem
.sc/stacks/mystack/secrets.yaml
config/production.env
```

### `sc secrets add`

Add new secrets to the encrypted store.

```shell
# Add a single secret file
sc secrets add database-password.txt

# Add multiple files
sc secrets add api-keys.json ssl-cert.pem

# Add multiple files at once
sc secrets add config-dev.env config-prod.env

# Add with verbose output
sc secrets add --verbose database-password.txt

# Add using specific profile
sc secrets add --profile github database-password.txt
```

**What it does:**

- Encrypts the specified files/directories
- Adds encrypted versions to Git tracking
- Removes plaintext versions from Git (adds to .gitignore)
- Updates secrets index

### `sc secrets reveal`

Decrypt and reveal encrypted secrets for local use.

```shell
# Reveal all secrets
sc secrets reveal

# Reveal all secrets (no support for specific files)
sc secrets reveal

# Force reveal all secrets
sc secrets reveal --force

# Reveal with verbose output
sc secrets reveal --verbose
```

**What it does:**

- Decrypts encrypted secret files using your private key
- Creates plaintext versions for local development
- Updates .gitignore to prevent accidental commits

### `sc secrets allow`

Grant access to secrets by adding team members' public keys.

```shell
# Allow access for a team member (using public key content)
sc secrets allow "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... username@host"

# Allow multiple users (using public key content)
sc secrets allow "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... user1@host" "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... user2@host"

# Allow with verbose output
sc secrets allow --verbose "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... username@host"

# Allow using specific profile
sc secrets allow --profile github "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... username@host"

# Allow by reading from file (practical example)
sc secrets allow "$(cat ~/.ssh/id_rsa.pub)"
sc secrets allow "$(cat /path/to/teammate.pub)"
```

**What it does:**

- Adds public key to authorized keys list in `.sc/secrets.yaml`
- Re-encrypts all secrets with the new key included
- Updates team access permissions
- Requires the actual public key content as argument, not a file path

### `sc secrets disallow`

Revoke access to secrets by removing team members' public keys.

```shell
# Revoke access for a team member (using public key content)
sc secrets disallow "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... username@host"

# Revoke multiple users (using public key content)
sc secrets disallow "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... user1@host" "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... user2@host"

# Revoke with verbose output
sc secrets disallow --verbose "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... username@host"

# Revoke using specific profile
sc secrets disallow --profile github "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... username@host"

# Revoke by reading from file (practical example)
sc secrets disallow "$(cat /path/to/former-teammate.pub)"
```

**What it does:**

- Removes public key from authorized keys list in `.sc/secrets.yaml`
- Re-encrypts all secrets without the removed key
- Revokes access permissions
- Requires the actual public key content as argument, not a file path

### `sc secrets hide`

Hide (encrypt) repository secrets.

```shell
# Hide all secrets in the repository
sc secrets hide

# Force re-encrypt all secrets (useful after key changes)
sc secrets hide --force

# Hide with verbose output
sc secrets hide --verbose

# Hide using specific profile
sc secrets hide --profile github
```

**What it does:**

- Encrypts all secret files in the repository
- Updates encrypted versions with current authorized keys
- Removes plaintext versions and updates .gitignore
- Use `--force` to re-encrypt even if secrets appear up-to-date

### `sc secrets reveal`

Reveal (decrypt) repository secrets for local use.

```shell
# Reveal all secrets
sc secrets reveal

# Force decrypt all secrets
sc secrets reveal --force

# Reveal with verbose output
sc secrets reveal --verbose

# Reveal using specific profile
sc secrets reveal --profile github
```

**What it does:**

- Decrypts all encrypted secret files using your private key
- Creates plaintext versions for local development
- Updates .gitignore to prevent accidental commits of plaintext secrets
- Use `--force` to decrypt even if plaintext versions already exist

### `sc secrets delete`

Delete repository secrets.

```shell
# Delete specific secret files
sc secrets delete database-password.txt

# Delete multiple secrets
sc secrets delete api-keys.json ssl-cert.pem

# Delete with verbose output
sc secrets delete --verbose config-file.env

# Delete using specific profile
sc secrets delete --profile github secret-file.txt
```

**What it does:**

- Removes specified secret files from the encrypted store
- Cleans up both encrypted and plaintext versions
- Updates the secrets registry

### `sc secrets allowed-keys`

List public keys allowed to decrypt secrets.

```shell
# List all allowed public keys
sc secrets allowed-keys

# List with verbose output
sc secrets allowed-keys --verbose

# List using specific profile
sc secrets allowed-keys --profile github
```

**What it does:**

- Displays all public keys registered in `.sc/secrets.yaml`
- Shows which keys can decrypt the repository secrets
- Useful for auditing team access


## Team Collaboration Workflow

### Setting Up Team Access

1. **Project Administrator** initializes secrets:
```shell
sc secrets init
sc secrets add initial-secrets.env
git add . && git commit -m "Initialize secrets management"
git push
```

2. **Team Members** join:
```shell
# Generate SSH key if needed
sc secrets init -g

# Share public key with administrator
cp ~/.ssh/id_rsa.pub team-member-name.pub
# Send team-member-name.pub to administrator
```

3. **Administrator** grants access:
```shell
# Add team member's public key (using public key content)
sc secrets allow "$(cat team-member-name.pub)"
# Or directly paste the public key content:
# sc secrets allow "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... team-member@host"

git add . && git commit -m "Add team member access"
git push
```

4. **Team Members** access secrets:
```shell
git pull
sc secrets reveal
```

### Adding New Secrets

```shell
# Add new secret file
echo "secret-value" > new-secret.txt
sc secrets add new-secret.txt

# Commit changes
git add . && git commit -m "Add new secret"
git push
```

### Rotating Secrets

```shell
# Update secret content
echo "new-secret-value" > database-password.txt

# Re-encrypt with updated content
sc secrets add database-password.txt

# Commit changes
git add . && git commit -m "Rotate database password"
git push
```

### Removing Team Member Access

```shell
# Revoke access and re-encrypt all secrets (using public key content)
sc secrets disallow "$(cat former-team-member.pub)"
# Or directly use the public key content:
# sc secrets disallow "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... former-team-member@host"

# Commit security changes
git add . && git commit -m "Revoke access for former team member"
git push
```

## Best Practices

### Security Best Practices

1. **Never commit private keys**:
   ```shell
   # Ensure .gitignore includes
   echo "*.pem" >> .gitignore
   echo ".sc/cfg.*.yaml" >> .gitignore
   echo "id_rsa*" >> .gitignore
   ```

2. **Regular key rotation**:
   ```shell
   # Periodically rotate SSH keys
   sc secrets init -g
   sc secrets hide --force
   ```

3. **Audit access regularly**:
   ```shell
   sc secrets allowed-keys --verbose
   # Review authorized keys list
   ```

### Development Workflow

1. **Start development session**:
   ```shell
   git pull
   sc secrets reveal
   # Now use decrypted secrets for development
   ```

2. **End development session**:
   ```shell
   # Remove plaintext secrets manually for security
   rm -f database-password.txt api-keys.json
   # Or use sc secrets hide to encrypt them
   sc secrets hide
   ```

3. **Adding new secrets during development**:
   ```shell
   # Create secret file
   echo "new-api-key" > api-key.txt
   
   # Add to encrypted store
   sc secrets add api-key.txt
   
   # Commit
   git add . && git commit -m "Add new API key"
   ```

### CI/CD Integration

#### Setting Up CI/CD Secrets Access

For automated deployments, you can create separate key pairs for CI/CD environments:

```shell
# Generate CI/CD specific key pair
sc secrets init --profile github --generate

# This creates:
# .sc/cfg.github.yaml - CI/CD configuration
# .sc/profiles/github/id_rsa - CI/CD private key
# .sc/profiles/github/id_rsa.pub - CI/CD public key
```

**Add CI/CD public key to team access:**
```shell
# Administrator adds CI/CD public key to the chain
# Note: Use the actual public key content, not the file path
sc secrets allow "$(cat .sc/profiles/github/id_rsa.pub)"
git add . && git commit -m "Add CI/CD access"
git push
```

**Important:** When using `sc secrets allow`, you must provide the actual SSH public key content, not the file path. The generated public key needs to be added to the encryption chain so that secrets can be decrypted using the corresponding private key.

**In your CI/CD pipeline (e.g., GitHub Actions):**

**Method 1: Using SIMPLE_CONTAINER_CONFIG environment variable (Recommended)**
```yaml
# .github/workflows/deploy.yml
- name: Setup and deploy with secrets
  env:
    # Reference the entire .sc/cfg.github.yaml content from GitHub Actions secret
    SIMPLE_CONTAINER_CONFIG: ${{ secrets.SC_CONFIG }}
  run: |
    # Reveal secrets (uses SIMPLE_CONTAINER_CONFIG)
    sc secrets reveal
    
    # Deploy with secrets available
    sc deploy -s myservice -e staging
```

**Setup Instructions:**
1. Create your CI/CD configuration locally:
   ```shell
   # Generate CI/CD specific key pair
   sc secrets init --profile github --generate
   ```

2. Copy the entire content of `.sc/cfg.github.yaml` to a GitHub Actions secret named `SC_CONFIG`:
   ```yaml
   # Content to store in GitHub Actions secret SC_CONFIG:
   projectName: simple-container-api
   privateKey: |
     -----BEGIN RSA PRIVATE KEY-----
     MIIEpAIBAAKCAQEA...
     -----END RSA PRIVATE KEY-----
   publicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... user@host"
   ```

3. Add the CI/CD public key to your project:
   ```shell
   # Administrator adds CI/CD public key to the chain
   sc secrets allow "$(cat .sc/profiles/github/id_rsa.pub)"
   git add . && git commit -m "Add CI/CD access"
   git push
   ```

**Alternative approaches:**

**Method 2: Using profile-based configuration**
```yaml
# .github/workflows/deploy.yml
- name: Setup CI/CD secrets
  run: |
    # Copy CI/CD private key from secrets
    mkdir -p .sc/profiles/github
    echo "${{ secrets.SC_GITHUB_PRIVATE_KEY }}" > .sc/profiles/github/id_rsa
    chmod 600 .sc/profiles/github/id_rsa
    
    # Reveal secrets using CI/CD profile
    sc secrets reveal --profile github

- name: Deploy with secrets
  run: |
    # Now secrets are available for deployment
    sc deploy -s myservice -e staging --profile github
```

**For deployment configuration, secrets can be injected via environment variables:**
```yaml
# In your deployment configuration
env:
  DATABASE_URL: ${secret:database-url}
  API_KEY: ${secret:api-key}
```

**Important:** For this to work, secret values must be stored in `.sc/stacks/<parent-stack>/secrets.yaml` under the `values` section:
```yaml
# .sc/stacks/your-parent-stack/secrets.yaml
values:
  database-url: "postgresql://<USER>:<PASS>@<host>:5432/db"
  api-key: "your-secret-api-key-here"
```

Simple Container automatically decrypts and injects secrets during deployment by reading from the parent stack's secrets.yaml file.

## Troubleshooting

### Common Issues

**"Permission denied" errors**:
```shell
# Check SSH key permissions
chmod 600 ~/.ssh/id_rsa
chmod 644 ~/.ssh/id_rsa.pub

# Check allowed keys
sc secrets allowed-keys
```

**"Cannot decrypt secrets"**:
```shell
# Check if your key is allowed
sc secrets allowed-keys

# Re-initialize if needed
sc secrets init

# Contact administrator to grant access
```

**"Secrets out of sync"**:
```shell
# Pull latest changes
git pull

# Re-reveal secrets
sc secrets reveal
```

### Getting Help

```shell
# Get help for any command
sc secrets --help
sc secrets init --help
sc secrets add --help
sc secrets allow --help

# Check version and configuration
sc --version
sc secrets allowed-keys --verbose

# Check allowed keys for specific profile
sc secrets allowed-keys --profile github --verbose
```

## Per-scope secrets (`secrets.<scope>.yaml`)

The whole-file store (`.sc/secrets.yaml`) is decrypted by a single master key
(`SIMPLE_CONTAINER_CONFIG`) — every CI context that needs *any* secret can read *all* of
them. **Per-scope secrets** let you carve out a named subset (a "scope") sealed to its own
recipient set, so a narrow, attacker-reachable context — typically a `pull_request`-triggered
scan job — can hold a key that decrypts only that scope.

Scoped values live in committed, encrypted files alongside a stack's config:

```
.sc/
  scopes.yaml                    # scope -> recipient keys (governance; CODEOWNERS-gate this)
  stacks/<stack>/
    secrets.yaml                 # legacy whole-file store (unchanged)
    secrets.pr.yaml              # scope "pr": committed encrypted, structure diffable, values opaque
```

A recipient is either an **SSH public key** (`ssh-ed25519` / `ssh-rsa`) or an **AWS KMS key**
(`awskms://<key>?region=<region>`). Each value is encrypted once under a random data key bound
to its `(stack, scope, key)`; that data key is then wrapped for every recipient. A ciphertext
therefore cannot be moved to another stack, scope, or key, and every recipient decrypts the
same value. Old `sc` binaries never read these files (they fail closed on the newer schema).

- **SSH recipient** — the private key must be provided to decrypt (a file, a CI secret, or the
  ambient config). Good for humans and for the interim CI key.
- **KMS recipient** — the data key is wrapped with `kms:Encrypt` and unwrapped with
  `kms:Decrypt`, using the caller's **ambient AWS credentials**. In CI those come from an
  **OIDC-federated role**, so *no private key is stored anywhere* — this is the recommended
  end state for CI (see "KMS recipients" below).

### Commands

```bash
# Declare a scope and its recipients (edit .sc/scopes.yaml behind a CODEOWNERS gate).
# A recipient is an SSH public key OR an awskms:// URL.
sc secrets scope allow    --scope pr <ssh-pubkey>                     # add an SSH recipient + reseal
sc secrets scope allow    --scope pr 'awskms://alias/sc-pr?region=us-east-1'  # add a KMS recipient + reseal
sc secrets scope disallow --scope pr <recipient>     # remove a recipient + reseal (then rotate values!)

# Manage values in a scope.
sc secrets scope set    --scope pr -s <stack> KEY <value>   # or omit value / pass '-' to read stdin
sc secrets scope get    --scope pr -s <stack> KEY
sc secrets scope list   --scope pr -s <stack>
sc secrets scope delete --scope pr -s <stack> KEY

# Guardrails.
sc secrets scope lint     # CI gate: encryption shape, recipient drift, scope/stack binding, duplicate keys
sc secrets scope doctor   # which scopes the current key can open
```

Commit only the encrypted `secrets.<scope>.yaml` and `scopes.yaml`; never the legacy
plaintext `stacks/*/secrets.yaml`.

### Using a scope key in CI

Give the job the scope's private key and it decrypts only that scope — no
`SIMPLE_CONTAINER_CONFIG` required. `get` (and deploy-time `${secret:}` resolution) look up
the decryption key in order: `--key-file`, then env `SC_KEY_<SCOPE>` (e.g. `SC_KEY_PR`) or the
generic `SC_SCOPE_KEY`, then the ambient `SIMPLE_CONTAINER_CONFIG`.

```yaml
# a pull_request scan job — holds ONLY the pr-scope key
env:
  SC_KEY_PR: ${{ secrets.SC_KEY_PR }}   # ssh-ed25519 private key, recipient of the "pr" scope only
steps:
  - run: DD_KEY=$(sc secrets scope get --scope pr -s integrail defectdojo-api-key)
```

At deploy time, `${secret:KEY}` and `sc stack secret-get` transparently include every scope
the job's key can open, merged over the whole-file store (the legacy store wins on conflict;
`sc secrets scope lint` rejects a key that appears in two scopes or in both a scope and the
legacy store). A key that is **not** a recipient of a scope cannot decrypt it — the
`pull_request` clamp is cryptographic, not a config flag.

> **Rotation:** `disallow` re-encrypts current files but does NOT rewrite git history — that
> recipient can still read previously committed versions. Always rotate the scope's values
> after removing a recipient.

### Runbook: a leaked scope key

`disallow` stops a removed recipient from reading *future* commits, but the values it could
already read must be treated as compromised — git history is immutable. To revoke a leaked
scope key (SSH or KMS):

```bash
# 1. Remove the leaked recipient from the scope (reseals current files to the rest).
sc secrets scope disallow --scope pr <leaked-ssh-pubkey | awskms://…>

# 2. ROTATE every value in the scope at its source, then re-set it — the old ciphertext in
#    git history is still readable by the leaked key, so the values themselves must change.
sc secrets scope list --scope pr -s <stack>          # enumerate what to rotate
#   for each KEY: rotate upstream (new API token, etc.), then:
sc secrets scope set  --scope pr -s <stack> KEY <new-value>

# 3. Commit the resealed scope file(s) + scopes.yaml.
git add .sc/scopes.yaml .sc/stacks/<stack>/secrets.*.yaml && git commit -S -s -m "rotate pr scope after key revocation"
```

Then, depending on the recipient kind:

- **SSH scope key (`SC_KEY_PR`)** — delete/replace the GitHub secret; the old private key can
  no longer decrypt the rotated values (and is no longer a recipient going forward).
- **KMS recipient** — the "key" is the IAM permission to call `kms:Decrypt`, not a stored
  secret. Revoke it by removing `kms:Decrypt` from the compromised role/principal (or, if the
  KMS key itself is suspect, schedule key deletion / rotate to a new key and re-`allow` it).
  Because decryption requires assuming the OIDC role, tightening the role's trust policy or
  IAM immediately cuts off access — no secret to invalidate.

Prefer at least one **SSH break-glass recipient** on every scope so you can always reseal in
step 1 even if the KMS path is unavailable, and keep `sc secrets scope lint` a **required** CI
check so the collision/duplicate guarantees are enforced, not just advisory.

### KMS recipients (keyless CI, no stored key)

A stored `SC_KEY_PR` is a smaller master key, but it is still a long-lived private key sitting
in a CI secret. A **KMS recipient** removes it entirely: the scope's data keys are wrapped for
an AWS KMS key, and a CI job unwraps them with an **OIDC-federated IAM role** — so the job
proves its identity to AWS per-run and holds no secret key at all.

The recipient string is `sc`'s canonical KMS URL — the same form the Pulumi state secrets
provider uses:

```
awskms://<key-id | alias/<name> | arn:aws:kms:...>?region=<region>
```

**Add the KMS recipient** (needs `kms:Encrypt` on the key for the reseal). Keep at least one
SSH break-glass recipient so you can always reseal/recover if the KMS path is unavailable:

```bash
# scopes.yaml already has an SSH break-glass recipient; add the KMS key beside it.
# Prefer the key ARN over an alias (see "Secure-usage rules" — an alias can be repointed).
sc secrets scope allow --scope pr 'awskms://arn:aws:kms:us-east-1:123456789012:key/abcd-1234?region=us-east-1'
sc secrets scope set   --scope pr -s integrail defectdojo-api-key 'dd-…'   # sealed to BOTH recipients
```

**The resulting scope file** carries one opaque KMS wrap per value keyed by the KMS URL,
alongside the SSH wrap — no plaintext, fully diffable structure:

```yaml
schemaVersion: 1
stack: integrail
scope: pr
recipients:
  - awskms://arn:aws:kms:us-east-1:123456789012:key/abcd-1234?region=us-east-1
  - ssh-ed25519 AAAA…break-glass
values:
  defectdojo-api-key:
    ciphertext: <AEAD value under a random data key>
    wraps:
      "awskms://arn:aws:kms:us-east-1:123456789012:key/abcd-1234?region=us-east-1": [<data key wrapped by KMS>]
      "SHA256:…break-glass": [<data key sealed to the SSH key>]
```

**Consume it in CI with OIDC — no `SC_KEY_PR`, no `SIMPLE_CONTAINER_CONFIG`:**

```yaml
# a pull_request scan job — federates to AWS, then decrypts only the "pr" scope
permissions:
  id-token: write            # required for OIDC
  contents: read
steps:
  - uses: aws-actions/configure-aws-credentials@v5
    with:
      role-to-assume: arn:aws:iam::<account>:role/ci-oidc-pr-scan   # kms:Decrypt on the pr key only
      aws-region: us-east-1
  - run: DD_KEY=$(sc secrets scope get --scope pr -s integrail defectdojo-api-key)
```

`get`, `doctor`, and deploy-time `${secret:}` resolution all try, in order: an explicit
`--key-file`, the `SC_KEY_<SCOPE>` / `SC_SCOPE_KEY` env keys, the ambient
`SIMPLE_CONTAINER_CONFIG`, and finally a KMS Decrypt for any `awskms://` recipient using the
ambient AWS credentials. A value with no KMS recipient never triggers an AWS call.

**Secure-usage rules for KMS recipients**

- **Prefer a key ARN as the recipient, not an alias.** `awskms://alias/…` is convenient, but an
  alias only pins the key *by name* — an account admin who repoints the alias changes which key
  the recipient resolves to. The **key ARN** (`awskms://arn:aws:kms:<region>:<acct>:key/<id>`)
  is the durable security boundary; use it for anything sensitive and treat aliases as a
  convenience for low-risk scopes. `sc` pins the `KeyId` on every decrypt, so a value wrapped
  under a *different* key than the recipient names is rejected as a hard integrity error.
- **Scope the IAM role tightly — pin stack AND scope.** Allow `kms:Decrypt` on *only* the
  scope's key, and constrain the encryption context. `sc` sets it to
  `{sc:domain, sc:stack, sc:scope, sc:key}`, so condition on both `sc:stack` and `sc:scope`:

  ```json
  "Condition": { "StringEquals": {
    "kms:EncryptionContext:sc:scope": "pr",
    "kms:EncryptionContext:sc:stack": "integrail"
  }}
  ```

  Conditioning on `sc:scope` alone lets one role decrypt *every* stack's `pr` values wrapped
  under that key — fine if one key serves one trust level and stacks are mutually trusted, but
  add `sc:stack` when stacks must not read each other. KMS enforces the context server-side, so
  a value transplanted to another stack/scope/key fails to decrypt.
- **Never give the PR-scan role `kms:Decrypt` on a deploy/prod key.** The whole point is that
  a compromised PR job cannot reach deploy-grade secrets. Use a separate key (and role) per
  trust level; a deploy scope's key stays out of the PR role's policy.
- **The clamp is cryptographic.** A role that is not a recipient of `secrets.prod.yaml` (no
  `kms:Decrypt` on its key) cannot open it, regardless of anything in a PR's config.
- **The stack/scope/key are NOT secret.** They ride as the KMS EncryptionContext and are logged
  in CloudTrail (desirable for audit) — do not put a secret-ish string in a stack, scope, or
  key *name* expecting it to stay private.
- **How failures are classified.** A KMS `InvalidCiphertext` (mangled blob or broken
  stack/scope/key binding) or `IncorrectKey` (the wrap was made under a different key than the
  recipient names — a transplant) is a hard **integrity** error that fails the deploy as tamper.
  `AccessDenied`, a disabled key, or no ambient credentials is a least-privilege **skip** (the
  scope is simply not yours; a *missing* required `${secret:}` then fails closed downstream). A
  transient KMS fault (throttle after SDK retries, `KMSInternal`, timeout) fails the deploy as
  **unavailable** (retry) — never silently and never mislabeled as tamper.
- **Rotation still applies.** Removing a KMS recipient re-encrypts current files but does not
  rewrite history — rotate the values, and additionally rotate/disable the KMS key material if
  the key itself is the concern.

## Summary

Simple Container's secrets management provides a secure, Git-native way to handle sensitive data in your projects. Key benefits:

- **Secure**: SSH-RSA encryption with team-based access control
- **Simple**: Easy-to-use commands for all secret operations
- **Collaborative**: Git-based workflow for team secret sharing
- **Integrated**: Seamless integration with Simple Container deployments
- **Scoped**: Per-scope recipients (SSH keys or KMS-via-OIDC) so a narrow CI context decrypts only what it needs — with the KMS path storing no key at all

Use the commands outlined in this guide to implement robust secrets management in your Simple Container projects.
