# Milestone 1: HashiCorp Vault Integration (All Stacks)

## 🎯 Goal

Deliver first-class, context-aware integration between Simple Container (sc) and HashiCorp Vault so every stack can resolve `${secret:...}` values from Vault by default, with zero breaking changes and minimal configuration. Target parity with stable Vault capabilities as of Oct 2025 (KV v1/v2, Namespaces, AppRole/Kubernetes/OIDC/JWT auth, response wrapping, HCP Vault compatibility, TLS hardening).

---

## 📦 Scope (M1)

- Context-aware static secret resolution from Vault (KV v2) for all stacks
- Dynamic secrets support for deploy/CI-time usage via Vault Database and AWS engines
- Global secrets manager configuration in `server.yaml` with secure defaults
- Optional stack-level context/configuration in `client.yaml`
- Backward-compatible resolver: Vault → fallback shared paths → `secrets.yaml`
- Auth support: Kubernetes, AppRole, OIDC/JWT, Token (dev only)
- Vault Namespaces (Enterprise/HCP) and TLS hardening
- Minimal CLI enhancements for connectivity, validation, and dynamic retrieval
- Caching: in-memory for static secrets; dynamic secrets are never cached to disk and may be memory-cached only within lease TTL (or not cached at all)
- Observability: structured logs and basic metrics with lease metadata for dynamic secrets

Out of scope for M1 (planned next):
- AWS Secrets Manager, GCP Secret Manager, Azure Key Vault adapters
- Dynamic secret renewal for long-running services (recommend Vault Agent/CSI; to be addressed in a later milestone)
- SSH key registry automation

---

## ✅ Functional Requirements

1) Static secrets (KV v2)
- Resolve `${secret:<key>}` using context-derived paths
- Default path template: `{{.Organization}}/{{.ClientStack}}/{{.Environment}}/{{.Key}}`
- Optional version pinning: `${secret:<key>@v=3}`
- Fallback search order (configurable):
  1. `{{.Organization}}/{{.ClientStack}}/{{.Environment}}/{{.Key}}`
  2. `{{.Organization}}/shared/{{.Environment}}/{{.Key}}`
  3. `shared/{{.Environment}}/{{.Key}}`
  4. `secrets.yaml` fallback

2) Dynamic secrets (deploy/CI-time)
- Supported engines in M1: `database` (e.g., PostgreSQL/MySQL) and `aws` (STS-backed)
- Explicit reference syntax:
  - Database: `${vault:database/creds/<role>#username}`, `${vault:database/creds/<role>#password}`
  - AWS: `${vault:aws/creds/<role>#access_key}`, `${vault:aws/creds/<role>#secret_key}`, `${vault:aws/creds/<role>#security_token}`
- Resolution occurs at deploy/command time; no background renewal in M1
- Dynamic secrets are not cached to disk; in-memory only within lease TTL or not cached
- Lease metadata (lease_id, lease_duration, renewable) captured for observability

3) Server configuration (server.yaml)
- Define one or more secret managers; M1 supports `type: vault`
- Support address, namespace, KV version, mount, path templates, auth methods, TLS
- Timeouts, retries, and circuit breaker defaults
- Cache defaults: memory enabled for static; disk disabled; dynamic_no_cache policy enabled

4) Stack configuration (client.yaml)
- Optional `secrets.context` overrides: `organization`, `clientStack`, `environment`, `parentStack`
- Optional `secrets.pathOverrides` per key
- Optional `secrets.required` schema with validation rules
- Use `${secret:...}` for static KV; `${vault:...}` for dynamic engine-backed secrets

5) Auth methods
- Kubernetes service account JWT (recommended for k8s)
- AppRole (role_id + secret_id; response-wrapping support)
- OIDC/JWT (e.g., GitHub Actions OIDC, cloud workload identities)
- Token (dev-only)

6) Compatibility and fallback
- If Vault is unreachable or key not found, continue search order and fallback to `secrets.yaml`
- Clear, structured error messages including attempted context paths

7) CLI enhancements (minimal, M1)
- `sc secrets manager test vault` to verify connectivity and auth
- `sc secrets validate --stack <name> [--environment <env>]` resolves all referenced secrets, reports missing/invalid
- `sc secrets reveal --debug` shows source, context path, and lease metadata when dynamic
- `sc secrets dynamic get <vault-path>#<field> --stack <s> --environment <e> [--write-env <file>] [--print-lease]`

---

## 📐 Non-Functional Requirements

- Security
  - TLS verification on by default; CA pinning supported
  - Never log secret values; redact paths and metadata as needed
  - Short timeouts, limited retries, circuit breaker per manager
  - Least-privilege Vault policies limited to target paths
  - No root tokens; AppRole/OIDC/Kubernetes recommended; token renewal where applicable
  - Encrypted cache (in-memory); optional disk cache must be encrypted with Vault Transit or local KMS
  - Response wrapping for sensitive bootstrap materials (e.g., AppRole secret_id)
- Performance
  - Single-digit millisecond cache hits; <300ms p95 remote fetch under normal latency
  - Batching and per-request context timeouts (default 30s) with jittered retries
- Reliability
  - Degrade gracefully to fallback sources
  - Health checks and lazy re-authentication
- Observability
  - Structured logs with correlation IDs
  - Basic metrics: cache hit/miss, resolution latency, error rates per manager

---

## 🧩 Configuration Design

### Server Configuration (server.yaml)

```yaml
# server.yaml
schemaVersion: 1.0

secrets:
  managers:
    - type: vault
      priority: 1
      config:
        address: https://vault.company.com
        namespace: platform/production           # Optional (Vault Enterprise/HCP)
        mount: secret                            # KV mount name (e.g., "secret"); adapter handles /data for v2
        kv_version: 2                            # 1 or 2; autodetect if omitted
        path_template: "{{.Organization}}/{{.ClientStack}}/{{.Environment}}/{{.Key}}"
        fallback_templates:
          - "{{.Organization}}/shared/{{.Environment}}/{{.Key}}"
          - "shared/{{.Environment}}/{{.Key}}"

        auth:
          type: kubernetes                       # kubernetes | approle | oidc | jwt | token
          role: simple-container-prod
          service_account: simple-container      # optional (auto-detect if omitted)
          token_path: /var/run/secrets/kubernetes.io/serviceaccount/token
          # AppRole example:
          # type: approle
          # role_id: ${env:VAULT_ROLE_ID}
          # secret_id: ${env:VAULT_SECRET_ID}      # or use response wrapping
          # wrapped_token: ${env:VAULT_SECRET_ID_WRAPPED}  # unwrap at runtime
          # OIDC/JWT example:
          # type: oidc
          # role: sc-deploy
          # jwt: ${env:CI_JOB_JWT}                 # workload identity / GitHub OIDC

        tls:
          verify: true
          ca_cert: /etc/ssl/certs/vault-ca.pem   # or ca_cert_data
          # client_cert: /etc/ssl/certs/app.pem
          # client_key: /etc/ssl/private/app.key

        request_timeout: 30s
        max_retries: 3
        retry_wait_min: 250ms
        retry_wait_max: 2s

  cache:
    memory:
      enabled: true
      default_ttl: 15m
      max_entries: 5000
    disk:
      enabled: false            # disabled by default; enable only with strong encryption

  security:
    audit_logging: true
    redact_paths: true

  policy:
    dynamic_no_cache: true      # do not cache dynamic engine secrets
```

Notes
- If `mount` points to KV v2 (e.g., `secret` with `kv_version: 2`), the adapter handles `/data` reads and `/metadata` versioning internally
- `namespace` is passed via `X-Vault-Namespace`
- Multiple managers can be declared but only Vault is supported in M1

### Stack Configuration (client.yaml)

```yaml
# .sc/stacks/myservice/client.yaml
schemaVersion: 1.0

stacks:
  - name: myservice
    deployment:
      type: cloud-compose
      dockerComposeFile: ./docker-compose.yaml

    # Optional: secrets configuration for this stack
    secrets:
      # Context overrides (normally auto-derived)
      context:
        organization: yourorg              # default: derived from parent stack
        clientStack: myservice             # default: current stack name
        environment: production            # default: -e flag
        parentStack: yourorg/infrastructure

      # Optional: per-key path overrides or alternative keys
      pathOverrides:
        DATABASE_URL: "{{.Organization}}/{{.ClientStack}}/{{.Environment}}/db-url"
        JWT_SECRET:   "{{.Organization}}/shared/{{.Environment}}/jwt-secret"

      # Optional: required secret policy with hints and defaults for local dev
      required:
        - key: DATABASE_URL
          required: true
          onMissing: error                 # error | warn | ignore
        - key: STRIPE_API_KEY
          required: true
          onMissing: error
        - key: LOG_LEVEL
          required: false
          default: info

    env:
      # Dynamic database credentials (deploy/CI-time)
      - name: DATABASE_URL
        value: "postgres://${vault:database/creds/app-role#username}:${vault:database/creds/app-role#password}@${secret:db-host}:5432/app"
      # Dynamic AWS credentials (deploy/CI-time)
      - name: AWS_ACCESS_KEY_ID
        value: "${vault:aws/creds/deploy-role#access_key}"
      - name: AWS_SECRET_ACCESS_KEY
        value: "${vault:aws/creds/deploy-role#secret_key}"
      - name: AWS_SESSION_TOKEN
        value: "${vault:aws/creds/deploy-role#security_token}"
      # Static KV secrets with optional version pinning
      - name: JWT_SECRET
        value: "${secret:jwt-secret@v=3}"
```

Notes
- Existing `${secret:...}` references continue to work; overrides are optional
- Context defaults are derived from deploy flags and parent stack; overrides support special cases

---

## 🔁 Resolution Flow (M1)

1) Build context from deployment and configuration
- Organization (from parent stack), ClientStack (stack name), Environment (flag), ParentStack

2) For static KV: try Vault with primary path template
- If not found, try fallback templates in order
- On permission error, report clearly and continue to next fallback template

3) For dynamic engine refs: fetch credentials at deploy/command time
- Do not cache to disk; optionally keep in-memory within lease TTL
- Emit lease metadata for observability; no renewal in M1

4) If all Vault attempts fail for static KV, use `secrets.yaml` as fallback

5) Cache successful static KV results (respect configured TTL)

6) Emit structured logs and metrics for each attempt (include lease metadata for dynamic)

---

## ⚡ Dynamic Secrets Design (M1)

- Supported engines: `database` and `aws`
- Syntax:
  - Database: `${vault:database/creds/<role>#username}`, `${vault:database/creds/<role)#password}`
  - AWS: `${vault:aws/creds/<role>#access_key}`, `${vault:aws/creds/<role>#secret_key}`, `${vault:aws/creds/<role>#security_token}`
- Semantics:
  - Resolved at deploy/command time; not intended for long-running automatic renewal
  - No disk caching; memory-cached only within lease TTL (or not cached)
  - Lease metadata surfaced for audit/observability
- Long-running services:
  - Recommend Vault Agent Injector or CSI driver for automatic renewal/injection
  - sc will document annotations/patterns but will not manage renewals in M1

---

## 🧭 How To: Configure Dynamic Secrets

### Database (PostgreSQL example)

```bash
# Enable and configure the database secrets engine
vault secrets enable database

# Configure connection (replace placeholders)
vault write database/config/app \
  plugin_name=postgresql-database-plugin \
  allowed_roles="app" \
  connection_url="postgresql://{{username}}:{{password}}@db.example.com:5432/postgres?sslmode=require" \
  username="${DB_ADMIN_USER}" \
  password="${DB_ADMIN_PASS}"

# Role to generate dynamic creds
vault write database/roles/app \
  db_name=app \
  creation_statements="CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; GRANT ALL PRIVILEGES ON DATABASE app TO \"{{name}}\";" \
  default_ttl=1h \
  max_ttl=24h

# sc usage (deploy-time)
# ${vault:database/creds/app#username}, ${vault:database/creds/app#password}
```

### AWS (STS-backed creds)

```bash
# Enable and configure the AWS secrets engine
vault secrets enable aws

# Configure root or assumed role (least privilege recommended)
vault write aws/config/root \
  access_key=${AWS_ACCESS_KEY_ID} \
  secret_key=${AWS_SECRET_ACCESS_KEY} \
  region=us-east-1

# Define a role that maps to an IAM policy/role
vault write aws/roles/deploy-role \
  credential_type=assumed_role \
  role_arn=arn:aws:iam::123456789012:role/ScDeployRole \
  ttl=1h \
  max_ttl=24h

# sc usage (deploy-time)
# ${vault:aws/creds/deploy-role#access_key}, #secret_key, #security_token
```

---

## 🔐 Security Best Practices (adopt in M1)

- Enforce TLS verification and pin CA where possible
- Use Kubernetes, AppRole, or OIDC/JWT auth; avoid static tokens in production
- Apply least-privilege Vault policies scoped to path templates (stack/env)
- Use short-lived tokens and enable token renewal; auto-reauth on expiry
- Prefer response-wrapping for AppRole secret_id delivery
- Redact secret values in logs; scrub keys that look like secrets
- Protect caches with process and filesystem hardening; keep disk cache disabled by default
- Do not cache dynamic engine secrets to disk; prefer no caching or TTL-bound in-memory cache only
- Add circuit breaker and bounded retries to avoid cascading failures
- Capture audit logs (Vault side) for all reads; correlate with sc request IDs
- Avoid enabling list on KV metadata paths in production unless required; prefer direct key access to reduce enumeration risk

---

## 🔒 Access Control: Policies and Roles

Objective
- Enforce least privilege and strict stack isolation so a stack cannot read other stacks’ secrets while enabling safe dynamic secrets usage.

Path Strategy (KV v2)
- Mount: `secret` (kv v2)
- Per-stack path: `secret/data/{org}/{stack}/{env}/*`
- Shared-per-env path: `secret/data/{org}/shared/{env}/*`
- Optional list endpoints (KV v2 metadata): `secret/metadata/{org}/{stack}/{env}`, `secret/metadata/{org}/shared/{env}`

Policy Strategy
- One policy per stack+env, plus optional shared policy; deny everything else by default
- Grant read-only on exact paths; do not grant write/update/delete/sudo

Example Policies (HCL)

```hcl
# Read-only access to a single stack+env (KV v2)
path "secret/data/yourorg/myservice/production/*" {
  capabilities = ["read"]
}

# Optional: allow listing metadata for tooling visibility (use sparingly)
path "secret/metadata/yourorg/myservice/production" {
  capabilities = ["list"]
}

# Optional shared env policy (attach only if needed)
path "secret/data/yourorg/shared/production/*" {
  capabilities = ["read"]
}

path "secret/metadata/yourorg/shared/production" {
  capabilities = ["list"]
}
```

Dynamic Secrets Policies
- Clients only need `read` on engine `creds` endpoints. Engine configuration remains admin-only.

```hcl
# Database engine dynamic credentials for this stack+env
path "database/creds/myservice-prod" {
  capabilities = ["read"]
}

# AWS engine dynamic credentials for this stack+env
path "aws/creds/myservice-deploy-prod" {
  capabilities = ["read"]
}

# Do NOT grant access to engine admin/config paths
# e.g., database/config/*, database/roles/*, aws/config/*, aws/roles/* remain restricted
```

Auth Role Mapping
- Bind identities to the minimal policy set required for each stack+env.

Kubernetes (recommended for workloads on K8s)

```hcl
# Vault role example (conceptual; created via API)
role "sc-yourorg-myservice-production" {
  bound_service_account_names      = ["simple-container"]
  bound_service_account_namespaces = ["prod-namespace"]
  policies = [
    "sc-yourorg-myservice-production-kv",
    "sc-yourorg-shared-production-kv",      # only if needed
    "sc-yourorg-myservice-production-db",   # only if dynamic DB used
    "sc-yourorg-myservice-production-aws"   # only if dynamic AWS used
  ]
  ttl = "1h"
}
```

AppRole (VMs/containers without K8s SA)
- Create an AppRole per stack+env with the same policy set; deliver `secret_id` using response wrapping; restrict CIDRs if possible.

OIDC/JWT (CI and federated identities)
- Create a role per stack+env with claim constraints (issuer, audience, repo/project) and attach the same policy set.

Renewal & Rotation
- Static KV: rotate values out-of-band; consumers read latest or pinned version.
- Dynamic: leases include `lease_id`, `lease_duration`, `renewable`. Renewal extends the lease and does not require write on engine paths. Use Vault Agent/CSI for long-running services; CI/deploy flows typically do not renew.

Least-Privilege Summary
- Static KV: `read` (and optionally `list`) on `secret/data/{org}/{stack}/{env}/*` (+ shared where explicitly required).
- Dynamic: `read` on `database/creds/{role}` and/or `aws/creds/{role}` only; no access to engine configuration endpoints.
- No cross-stack access unless explicitly granted for a single key or via shared namespaces.

---

## 🧪 Testing & Validation (M1)

- Unit tests for path rendering, fallback order, and error mapping
- Integration tests against dev Vault (KV v2, Namespaces)
- E2E tests: `sc secrets validate` on sample stacks; `sc deploy` dry-run
- Negative tests: permission denied, not found, network faults, token expiry
- Performance tests: cache hit/miss latency budgets

---

## 🚀 CLI Enhancements (M1)

```bash
# Verify Vault connectivity and auth
sc secrets manager test vault

# Validate all secret refs for a stack and env
sc secrets validate --stack myservice --environment production

# Show resolution details with context
sc secrets reveal --debug

# Retrieve a dynamic secret for CI and write to an env file
sc secrets dynamic get database/creds/app#password --stack myservice -e production --write-env .env.dynamic --print-lease
```

---

## ✨ Stretch and Bleeding-Edge (opt-in, if capacity allows)

- Vault Agent/Sidecar template support for file-based secret injection
- Response-wrapped bootstrap flows (unwrap at runtime) for AppRole
- Transit-backed cache encryption (envelope encryption)
- OIDC federation for CI (e.g., GitHub Actions → Vault role)
- Workload identity/SPIFFE integration to Vault auth
- HCP Vault multi-region compatibility testing
- Pre-warming cache during `sc deploy` planning

---

## 📅 Implementation Plan (4–6 weeks)

- Week 1: Adapter, auth methods, KV v2, path templates, config schema
- Week 2: Fallback search, caching, retries/circuit breaker, TLS hardening
- Week 3: CLI tests/validate commands, structured logs/metrics
- Week 4: Integration/E2E tests, docs, examples, opt-in disk cache (if transit configured)
- Weeks 5–6 (buffer): Beta rollout, feedback hardening, stretch items if time

---

## ✅ Acceptance Criteria

- Static (KV v2) context paths resolve across at least two stacks and two environments
- Optional version pinning works: `${secret:key@v=N}`
- Dynamic secrets for `database` and `aws` engines function at deploy/CI time; lease metadata displayed
- No disk caching of dynamic secrets; static caching honors TTL; TLS verification enabled by default
- Supports Kubernetes and AppRole auth; OIDC/JWT validated in CI
- Namespace propagation verified (if configured)
- Clear fallback to `secrets.yaml` with structured errors
- `sc secrets manager test`, `sc secrets validate`, and `sc secrets reveal --debug` demonstrate expected behavior
 - Policy isolation verified: stack tokens cannot read other stacks or engine admin endpoints; listing disabled unless explicitly enabled
 - Vault audit logs confirm only allowed paths are accessed

---

## ❓ Open Questions

- Do we need per-service custom fallback order beyond global defaults?
- Should we allow per-key TTL hints to influence cache retention?
- Minimum supported Vault version and client library pinning
- Disk cache default: keep disabled until transit configured?

---

## 📎 Example Vault Policies (reference)

```hcl
# Allow read-only access to stack/env scoped paths (KV v2 data)
path "secret/data/yourorg/myservice/production/*" {
  capabilities = ["read"]
}

# Optional: allow listing metadata (KV v2 metadata)
path "secret/metadata/yourorg/myservice/production" {
  capabilities = ["list"]
}

# Optional shared env access
path "secret/data/yourorg/shared/production/*" {
  capabilities = ["read"]
}

path "secret/metadata/yourorg/shared/production" {
  capabilities = ["list"]
}
```

---

## 🔭 Roadmap (post-M1)

- M2: AWS Secrets Manager adapter (feature parity), advanced CLI UX
- M3: GCP Secret Manager adapter
- M4: Azure Key Vault adapter
- M5: Dynamic secrets (DB/Cloud), lease renewal, secretless patterns
- M6: SSH key registry automation and sync
