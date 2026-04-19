# CLI Integration: Advanced Secrets Management System

## 🖥️ Overview

The Advanced Secrets Management System integrates seamlessly with Simple Container's existing CLI commands while adding new capabilities for managing external secrets managers and SSH key registries. All existing workflows continue to work unchanged, with enhanced functionality available through configuration.

## 🔄 Backward Compatibility

### Existing Commands Work Unchanged

All current Simple Container secrets commands continue to function exactly as before:

```bash
# ✅ All existing commands work unchanged
sc secrets init
sc secrets list
sc secrets add .sc/stacks/my-app/secrets.yaml
sc secrets reveal
sc secrets hide
sc secrets allow "$(cat ~/.ssh/id_rsa.pub)"
sc secrets disallow "$(cat ~/.ssh/id_rsa.pub)"
sc secrets allowed-keys
```

### Transparent Secret Resolution

When external secrets managers are configured, secret resolution becomes transparent:

```bash
# Same command, but now checks:
# 1. Vault (if configured)
# 2. AWS Secrets Manager (if configured) 
# 3. secrets.yaml (fallback)
sc secrets reveal
```

## 🆕 New CLI Capabilities

### Secrets Manager Management

```bash
# Configure external secrets managers
sc secrets manager add vault \
  --address https://vault.company.com \
  --auth-method kubernetes \
  --role simple-container-prod

sc secrets manager add aws-secrets-manager \
  --region us-east-1 \
  --prefix simple-container/

sc secrets manager list
sc secrets manager test vault
sc secrets manager remove aws-secrets-manager
```

### SSH Key Registry Management

```bash
# Configure SSH key sources
sc secrets ssh-keys add-source github-org \
  --org your-company \
  --teams platform,devops \
  --api-token-secret github-api-token

sc secrets ssh-keys add-source file \
  --path /etc/simple-container/authorized_keys \
  --watch

sc secrets ssh-keys add-source url \
  --url https://keys.company.com/api/authorized_keys \
  --header "Authorization: Bearer ${secret:keys-api-token}"

# Manage SSH key sources
sc secrets ssh-keys list-sources
sc secrets ssh-keys refresh
sc secrets ssh-keys test-source github-org
sc secrets ssh-keys remove-source file-source-1
```

### Enhanced Secret Operations

```bash
# List secrets from all sources (context-aware)
sc secrets list --source vault --stack production-api --environment production
sc secrets list --source aws-secrets-manager --stack production-api
sc secrets list --all-sources

# Get secret with source information and context
sc secrets get database-password --show-source --stack production-api --environment production

# Set secret in external manager with context
sc secrets set database-password "new-secure-password" --manager vault --stack production-api --environment production

# Test secret resolution hierarchy with context
sc secrets test-resolution database-password --stack production-api --environment production
```

## 🔧 Enhanced Commands

### Enhanced `sc secrets reveal`

```bash
# Basic usage (unchanged)
sc secrets reveal

# New verbose mode shows source information with context
sc secrets reveal --verbose
# Output:
# 🔍 Resolving secrets for production-api/production...
# ✅ database-password (from vault:yourorg/production-api/production/database-password)
# ✅ jwt-secret (from aws-secrets-manager:yourorg/production-api/production/jwt-secret)
# ✅ stripe-api-key (from secrets.yaml)
# 
# All secrets resolved successfully!

# Show resolution details with context paths
sc secrets reveal --debug
# Output:
# 🔍 Resolving secret: database-password
#   ↳ Context: yourorg/production-api/production
#   ↳ Trying vault at secret/yourorg/production-api/production/database-password... ✅ Found
# 🔍 Resolving secret: jwt-secret  
#   ↳ Context: yourorg/production-api/production
#   ↳ Trying vault at secret/yourorg/production-api/production/jwt-secret... ❌ Not found
#   ↳ Trying aws-secrets-manager at yourorg/production-api/production/jwt-secret... ✅ Found
# 🔍 Resolving secret: shared-config
#   ↳ Context: yourorg/production-api/production
#   ↳ Trying vault at secret/yourorg/production-api/production/shared-config... ❌ Not found
#   ↳ Trying vault at secret/yourorg/shared/production/shared-config... ✅ Found
```

### Enhanced `sc secrets list`

```bash
# Enhanced output with source information
sc secrets list
# Output:
# 📋 Available secrets:
# 
# From vault (3 secrets):
#   • database-password
#   • redis-password  
#   • jwt-secret
#   
# From aws-secrets-manager (2 secrets):
#   • stripe-api-key
#   • sendgrid-api-key
#   
# From secrets.yaml (1 secret):
#   • legacy-secret
#
# Total: 6 secrets across 3 sources

# Filter by source
sc secrets list --source vault
sc secrets list --source secrets.yaml

# Show detailed information
sc secrets list --detailed
# Output includes metadata, creation dates, versions
```

### Enhanced `sc secrets allowed-keys`

```bash
# Enhanced output with SSH key sources
sc secrets allowed-keys
# Output:
# 🔑 Authorized SSH Keys:
#
# From github-org:your-company (5 keys):
#   • SHA256:abc123... john@company.com (github)
#   • SHA256:def456... jane@company.com (github)
#   • SHA256:ghi789... mike@company.com (github)
#
# From file:/etc/simple-container/authorized_keys (2 keys):
#   • SHA256:jkl012... admin@server (local)
#   • SHA256:mno345... backup@server (local)
#
# From url:https://keys.company.com/api/authorized_keys (1 key):
#   • SHA256:pqr678... service@company.com (api)
#
# Total: 8 keys from 3 sources
# Last refresh: 2024-01-15 14:30:00 UTC

# Force refresh from all sources
sc secrets allowed-keys --refresh

# Show detailed key information
sc secrets allowed-keys --detailed
```

## 🚀 Deployment Integration

### Context-Aware Deployment

During deployment, Simple Container automatically provides context information to secrets managers based on the deployment parameters:

```bash
# Context is automatically derived from deployment parameters
sc deploy -s production-api -e production

# Context information used:
# - ClientStack: "production-api" (from -s flag)
# - Environment: "production" (from -e flag) 
# - ParentStack: "yourorg/infrastructure" (from client.yaml)
# - Organization: "yourorg" (extracted from parent stack)
```

### Enhanced `sc deploy` with Context-Aware Resolution

```bash
# Same command, enhanced secret resolution with context
sc deploy -s production-api -e production

# During deployment, secrets are resolved with context:
# 1. Try external managers with context paths
# 2. Try shared secret paths for common configurations
# 3. Fallback to secrets.yaml
# 4. Clear error messages with context information

# Deployment output shows context-aware resolution
# 🔍 Resolving secrets for production-api/production...
# ✅ DATABASE_URL (from vault:yourorg/production-api/production/database-password)
# ✅ STRIPE_API_KEY (from aws-secrets-manager:yourorg/production-api/production/stripe-api-key)  
# ✅ SHARED_JWT_SECRET (from vault:yourorg/shared/production/jwt-secret)
# ✅ LEGACY_CONFIG (from secrets.yaml)
# 🚀 Starting deployment...
```

### Pre-deployment Secret Validation

```bash
# Validate all required secrets are available
sc secrets validate --stack production-api

# Output:
# 🔍 Validating secrets for stack: production-api
# ✅ DATABASE_URL (required) - found in vault
# ✅ STRIPE_API_KEY (required) - found in aws-secrets-manager
# ❌ SENDGRID_API_KEY (required) - not found in any source
# ⚠️  OPTIONAL_API_KEY (optional) - not found
#
# ❌ Validation failed: 1 required secret missing

# Fix mode suggests solutions
sc secrets validate --stack production-api --fix
# Output:
# ❌ SENDGRID_API_KEY missing. Suggestions:
#   • Add to vault: sc secrets set SENDGRID_API_KEY "your-key" --manager vault
#   • Add to secrets.yaml: sc secrets add .sc/stacks/production-api/secrets.yaml
```

## 🔧 Configuration Commands

### Server Configuration Management

```bash
# View current secrets configuration
sc config show secrets

# Output:
# 📋 Secrets Configuration:
#
# Managers (2 configured):
#   1. vault (priority 1)
#      • Address: https://vault.company.com
#      • Auth: kubernetes (role: simple-container-prod)
#      • Status: ✅ Connected
#
#   2. aws-secrets-manager (priority 2)  
#      • Region: us-east-1
#      • Prefix: simple-container/
#      • Status: ✅ Connected
#
# SSH Key Sources (3 configured):
#   • github-org:your-company (✅ Active, 15 keys)
#   • file:/etc/simple-container/authorized_keys (✅ Active, 2 keys)
#   • url:https://keys.company.com/api/authorized_keys (❌ Unreachable)
#
# Cache Settings:
#   • Memory: 50MB max, 15min TTL
#   • Disk: 200MB max, encrypted

# Test configuration
sc config test secrets

# Health check all components  
sc secrets health
```

### Migration Tools

```bash
# Migrate secrets from secrets.yaml to external manager
sc secrets migrate \
  --from secrets.yaml \
  --to vault \
  --stack production-api \
  --dry-run

# Output:
# 🔄 Migration Plan (DRY RUN):
# 
# Will migrate 5 secrets from secrets.yaml to vault:
#   • database-password → secret/simple-container/production-api/database-password
#   • jwt-secret → secret/simple-container/production-api/jwt-secret
#   • stripe-api-key → secret/simple-container/production-api/stripe-api-key
#   • sendgrid-api-key → secret/simple-container/production-api/sendgrid-api-key
#   • redis-password → secret/simple-container/production-api/redis-password
#
# After migration:
#   • secrets.yaml will be backed up to .sc/stacks/production-api/secrets.yaml.backup
#   • client.yaml references will remain unchanged
#   • Secret resolution will use vault first, then fall back to secrets.yaml

# Execute migration
sc secrets migrate \
  --from secrets.yaml \
  --to vault \
  --stack production-api \
  --backup

# Rollback migration if needed
sc secrets migrate rollback \
  --stack production-api \
  --backup-file .sc/stacks/production-api/secrets.yaml.backup.20240115-143000
```

## 🔍 Diagnostic Commands

### Secret Resolution Debugging

```bash
# Debug why a secret isn't resolving
sc secrets debug database-password

# Output:
# 🔍 Debugging secret resolution: database-password
#
# Resolution hierarchy:
#   1. vault (priority 1)
#      • Checking path: secret/simple-container/database-password
#      • Status: ❌ Secret not found
#      • Details: Path exists but no 'database-password' key
#
#   2. aws-secrets-manager (priority 2)  
#      • Checking name: simple-container/database-password
#      • Status: ❌ Connection failed
#      • Details: Access denied (check IAM permissions)
#
#   3. secrets.yaml (fallback)
#      • Checking file: .sc/stacks/production-api/secrets.yaml
#      • Status: ✅ Found
#      • Details: Value available, encrypted
#
# Result: Secret found in secrets.yaml (fallback)
#
# Recommendations:
#   • Fix AWS IAM permissions to enable aws-secrets-manager
#   • Consider moving secret to vault for better security
#   • Check vault path configuration

# Test connectivity to secrets managers
sc secrets test-connectivity

# Output:
# 🔗 Testing secrets manager connectivity:
#
# vault:
#   • Connection: ✅ Connected to https://vault.company.com
#   • Authentication: ✅ Token valid (expires in 23h 45m)
#   • Permissions: ✅ Can read/write to secret/simple-container/*
#
# aws-secrets-manager:
#   • Connection: ❌ Access denied
#   • IAM Role: arn:aws:iam::123456789012:role/simple-container-role
#   • Missing Permissions: secretsmanager:GetSecretValue
#   • Recommendation: Add SecretsManagerReadWrite policy
```

### SSH Key Registry Debugging

```bash
# Debug SSH key source issues
sc secrets ssh-keys debug

# Output:  
# 🔍 SSH Key Sources Debug:
#
# github-org:your-company:
#   • API Connection: ✅ Connected to https://api.github.com
#   • Authentication: ✅ Token valid
#   • Organization Access: ✅ Can read your-company
#   • Team Access: ✅ Can read teams: platform, devops
#   • Rate Limiting: ✅ 4,950/5,000 requests remaining
#   • Last Refresh: 2024-01-15 14:25:00 UTC (5 min ago)
#   • Keys Retrieved: 15
#
# file:/etc/simple-container/authorized_keys:
#   • File Access: ✅ File readable
#   • File Watcher: ✅ Active
#   • Last Modified: 2024-01-15 09:30:00 UTC
#   • Keys Parsed: 2 valid, 0 invalid
#
# url:https://keys.company.com/api/authorized_keys:
#   • HTTP Connection: ❌ Connection timeout
#   • SSL Certificate: ✅ Valid
#   • Authentication: ⚠️  Untested (connection failed)
#   • Recommendation: Check network connectivity and firewall rules

# Test individual SSH key source
sc secrets ssh-keys test github-org
```

## 📊 Monitoring and Observability

### Built-in Metrics Commands

```bash
# Show secrets management statistics
sc secrets stats

# Output:
# 📊 Secrets Management Statistics:
#
# Resolution Performance (last 24h):
#   • Total Requests: 1,247
#   • Cache Hit Rate: 78.3%
#   • Average Response Time: 45ms
#   • P95 Response Time: 120ms
#
# Success Rates:
#   • vault: 98.2% (1,225/1,247)
#   • aws-secrets-manager: 94.1% (22/22 fallback requests)  
#   • secrets.yaml: 100% (0/0 fallback requests)
#
# SSH Key Registry:
#   • Total Keys: 17
#   • Last Refresh: 2024-01-15 14:25:00 UTC
#   • Refresh Success Rate: 100%
#   • Average Refresh Time: 3.2s

# Export metrics for monitoring systems
sc secrets stats --format prometheus
sc secrets stats --format json
```

### Health Monitoring

```bash
# Continuous health monitoring
sc secrets monitor

# Output (updates every 10 seconds):
# 🔄 Secrets Management Health Monitor
#
# [14:30:00] ✅ All systems healthy
# [14:30:10] ✅ All systems healthy  
# [14:30:20] ⚠️  AWS Secrets Manager slow response (2.3s)
# [14:30:30] ❌ Vault connection failed (retrying...)
# [14:30:40] ✅ Vault reconnected - all systems healthy

# Set up alerting  
sc secrets monitor --alert-webhook https://alerts.company.com/webhook
```

## 🔒 Security Commands

### Audit and Compliance

```bash
# Show secrets audit log
sc secrets audit

# Output:
# 📋 Secrets Access Audit (last 7 days):
#
# 2024-01-15 14:25:33 | user@company.com | READ | database-password | vault | SUCCESS
# 2024-01-15 14:20:15 | user@company.com | READ | jwt-secret | aws-secrets-manager | SUCCESS
# 2024-01-15 14:15:42 | service-account | READ | stripe-api-key | vault | FAILED (not found)
# 2024-01-15 14:15:42 | service-account | READ | stripe-api-key | aws-secrets-manager | SUCCESS
# 2024-01-15 13:45:12 | admin@company.com | WRITE | new-api-key | vault | SUCCESS
#
# Summary: 145 accesses, 98.6% success rate

# Export audit log
sc secrets audit --format json --since 30d > secrets-audit.json

# Security scan
sc secrets security-scan

# Output:
# 🔍 Security Scan Results:
#
# ✅ Encryption: All secrets encrypted at rest
# ✅ Transport: All connections use TLS 1.2+
# ✅ Authentication: All managers use strong authentication
# ⚠️  Permissions: AWS role has broader permissions than needed
# ✅ Key Management: SSH keys regularly rotated
# ❌ Compliance: Missing audit log retention policy
#
# Recommendations:
# • Restrict AWS IAM permissions to specific secret paths
# • Configure audit log retention (suggested: 2 years)
```

---

**Status**: This CLI integration design ensures seamless adoption of the Advanced Secrets Management System while maintaining full backward compatibility with existing Simple Container workflows and adding powerful new capabilities for enterprise secret management.
