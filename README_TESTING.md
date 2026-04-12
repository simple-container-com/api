# Testing Guide for Container Security Features

This document describes how to set up and run integration and E2E tests for the container security features, specifically image signing with cosign.

## Table of Contents

- [Overview](#overview)
- [Test Types](#test-types)
- [Prerequisites](#prerequisites)
- [Test Setup](#test-setup)
- [Running Tests](#running-tests)
- [Troubleshooting](#troubleshooting)
- [CI/CD Integration](#cicd-integration)

## Overview

The container security implementation includes three levels of testing:

1. **Unit Tests** - Fast, isolated tests with mocks (no external dependencies)
2. **Integration Tests** - Tests that execute real `cosign` commands
3. **E2E Tests** - Full workflow tests with real Docker registries

## Test Types

### Unit Tests

Location: `pkg/security/*_test.go`

- Run without any external tools
- Use mocks and test doubles
- Fast execution (< 1 second)
- Run on every commit

```bash
# Run all unit tests
welder run test

# Run specific package unit tests
go test ./pkg/security/...
go test ./pkg/security/signing/...
```

### Integration Tests

Location: `pkg/security/signing/integration_test.go`, `pkg/security/executor_integration_test.go`

- Execute real `cosign` commands
- Require `cosign` installation
- Test key generation, signing, verification
- Skip gracefully if `cosign` not installed
- Medium execution time (5-30 seconds)

```bash
# Run integration tests
go test -tags=integration ./pkg/security/signing/
go test -tags=integration ./pkg/security/
```

### E2E Tests

Location: `pkg/security/signing/e2e_test.go`

- Full workflow: build → push → sign → verify → retrieve
- Require `cosign`, `docker`, and registry access
- Use ephemeral registry (ttl.sh) or local registry
- Longer execution time (30-120 seconds)

```bash
# Run E2E tests
go test -tags=e2e ./pkg/security/signing/

# Run E2E tests with verbose output
go test -v -tags=e2e ./pkg/security/signing/
```

## Prerequisites

### Required Tools

#### 1. Cosign (Required for integration and E2E tests)

**Installation:**

```bash
# macOS (Homebrew)
brew install cosign

# Linux (download binary)
COSIGN_VERSION=v3.0.2
wget "https://github.com/sigstore/cosign/releases/download/${COSIGN_VERSION}/cosign-linux-amd64"
sudo mv cosign-linux-amd64 /usr/local/bin/cosign
sudo chmod +x /usr/local/bin/cosign

# Verify installation
cosign version
```

**Minimum version:** v3.0.2 or later

**Installation URL:** https://docs.sigstore.dev/cosign/installation/

#### 2. Docker (Required for E2E tests)

```bash
# Verify Docker is running
docker ps

# If not running, start Docker daemon
# macOS: Open Docker Desktop
# Linux: sudo systemctl start docker
```

### Optional Tools for Local Development

#### Local Docker Registry (for E2E tests)

```bash
# Start local registry on port 5000
docker run -d -p 5000:5000 --name registry registry:2

# Verify registry is running
docker ps | grep registry

# Stop and remove when done
docker stop registry
docker rm registry
```

## Test Setup

### 1. Generate Test Keys (for local testing)

```bash
# Create test keys directory
mkdir -p ~/.simple-container/test-keys

# Generate cosign key pair
cd ~/.simple-container/test-keys
cosign generate-key-pair

# Enter password when prompted (e.g., "test-password")
# This creates: cosign.key and cosign.pub
```

### 2. Set Environment Variables

For local development and testing:

```bash
# Optional: Set test OIDC token (for keyless signing tests)
export TEST_OIDC_TOKEN="your-test-oidc-token"

# For GitHub Actions keyless signing
export SIGSTORE_ID_TOKEN="${ACTIONS_ID_TOKEN_REQUEST_TOKEN}"
```

### 3. Configure Test Registry Access

The E2E tests use `ttl.sh` by default, which is a public ephemeral registry that doesn't require authentication. Images expire after 24 hours.

**Alternative: Use Docker Hub**

```bash
# Login to Docker Hub
docker login

# E2E tests will use your authenticated registry
```

**Alternative: Use Local Registry**

```bash
# Start local registry (see above)
docker run -d -p 5000:5000 --name registry registry:2

# E2E tests will automatically detect and use local registry
```

## Running Tests

### Quick Start

```bash
# Run all unit tests (fast, no prerequisites)
welder run test

# Run all tests including integration (requires cosign)
go test -tags=integration ./pkg/security/...

# Run all tests including E2E (requires cosign + docker)
go test -tags=e2e ./pkg/security/...

# Run everything together
go test -tags="integration,e2e" -v ./pkg/security/...
```

### Detailed Test Execution

#### Unit Tests Only

```bash
# All packages
go test ./pkg/security/...

# Specific package
go test ./pkg/security/signing/
go test ./pkg/security/tools/

# With coverage
go test -cover ./pkg/security/...

# With coverage report
go test -coverprofile=coverage.out ./pkg/security/...
go tool cover -html=coverage.out
```

#### Integration Tests

```bash
# Run integration tests
go test -tags=integration ./pkg/security/signing/

# Verbose output
go test -v -tags=integration ./pkg/security/signing/

# Specific test
go test -tags=integration -run TestKeyBasedSigningIntegration ./pkg/security/signing/

# With race detection
go test -race -tags=integration ./pkg/security/signing/
```

#### E2E Tests

```bash
# Run E2E tests (uses ttl.sh registry)
go test -tags=e2e ./pkg/security/signing/

# Verbose output with timing
go test -v -tags=e2e ./pkg/security/signing/

# Specific E2E test
go test -tags=e2e -run TestE2EKeyBasedWorkflow ./pkg/security/signing/

# With timeout (E2E tests can take longer)
go test -timeout 5m -tags=e2e ./pkg/security/signing/
```

#### All Tests Together

```bash
# Run unit + integration + E2E
go test -tags="integration,e2e" ./pkg/security/...

# Verbose with coverage
go test -v -cover -tags="integration,e2e" ./pkg/security/...
```

### Running Tests in Parallel

```bash
# Run tests in parallel (faster)
go test -parallel 4 ./pkg/security/...

# Integration tests in parallel
go test -parallel 2 -tags=integration ./pkg/security/...
```

## Troubleshooting

### Cosign Not Found

**Error:**
```
Skipping integration test: cosign not installed
```

**Solution:**
```bash
# Install cosign (see Prerequisites section)
# Verify installation
cosign version
which cosign
```

### Docker Not Running

**Error:**
```
Cannot connect to the Docker daemon
```

**Solution:**
```bash
# macOS: Open Docker Desktop
# Linux: Start Docker service
sudo systemctl start docker

# Verify
docker ps
```

### Registry Push Fails

**Error:**
```
Failed to push test image: unauthorized
```

**Solution:**
```bash
# Option 1: Use ttl.sh (no auth required)
# Tests use this by default

# Option 2: Login to Docker Hub
docker login

# Option 3: Use local registry
docker run -d -p 5000:5000 --name registry registry:2
```

### Test Timeout

**Error:**
```
test timed out after 2m0s
```

**Solution:**
```bash
# Increase timeout
go test -timeout 10m -tags=e2e ./pkg/security/signing/

# Or specify per test
go test -timeout 5m -tags=integration ./pkg/security/...
```

### Cosign Version Too Old

**Error:**
```
Warning: Cosign version may be below minimum (v3.0.2+)
```

**Solution:**
```bash
# Check version
cosign version

# Update to latest version
# macOS
brew upgrade cosign

# Linux
COSIGN_VERSION=v3.0.2
wget "https://github.com/sigstore/cosign/releases/download/${COSIGN_VERSION}/cosign-linux-amd64"
sudo mv cosign-linux-amd64 /usr/local/bin/cosign
sudo chmod +x /usr/local/bin/cosign
```

### OIDC Token Tests Skipped

**Note:** This is expected behavior unless running in GitHub Actions or with explicit OIDC token.

**To enable keyless signing tests:**
```bash
# Set test OIDC token
export TEST_OIDC_TOKEN="your-valid-oidc-jwt-token"

# Run tests
go test -tags=integration -run TestKeylessSigningIntegration ./pkg/security/signing/
```

## CI/CD Integration

### GitHub Actions

The integration and E2E tests are designed to run in GitHub Actions with keyless signing support.

**Example workflow:**

```yaml
name: Security Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    permissions:
      id-token: write  # Required for OIDC
      contents: read

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install cosign
        uses: sigstore/cosign-installer@v3
        with:
          cosign-release: 'v3.0.2'

      - name: Run unit tests
        run: welder run test

      - name: Run integration tests
        run: go test -tags=integration -v ./pkg/security/...

      - name: Run E2E tests
        run: go test -tags=e2e -v ./pkg/security/signing/
        env:
          SIGSTORE_ID_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Local Pre-commit Hook

Add to `.git/hooks/pre-commit`:

```bash
#!/bin/bash
set -e

echo "Running security unit tests..."
go test ./pkg/security/...

# Optional: Run integration tests if cosign is installed
if command -v cosign &> /dev/null; then
    echo "Running security integration tests..."
    go test -tags=integration ./pkg/security/...
fi

echo "All security tests passed!"
```

## Test Coverage Goals

- **Unit tests:** 90%+ coverage
- **Integration tests:** Cover all major cosign operations
- **E2E tests:** Cover full workflows (sign, verify, retrieve)

**Generate coverage report:**

```bash
# Unit test coverage
go test -coverprofile=coverage.out ./pkg/security/...
go tool cover -html=coverage.out

# Integration test coverage
go test -tags=integration -coverprofile=coverage-integration.out ./pkg/security/...
go tool cover -html=coverage-integration.out
```

## Security Considerations

### Test Keys

- **Never commit real private keys** to the repository
- Test keys are generated in temporary directories
- Private keys have 0600 permissions
- Test keys are automatically cleaned up after tests

### Test Registries

- E2E tests use `ttl.sh` (ephemeral, public registry)
- Images expire after 24 hours
- Don't push sensitive images to test registries
- Local registry option available for sensitive testing

### Fail-Open Testing

Tests validate that signing failures:
- Log appropriate warnings
- Don't crash the application
- Allow operations to continue when `Required: false`

## Additional Resources

- [Cosign Documentation](https://docs.sigstore.dev/cosign/)
- [Sigstore Project](https://www.sigstore.dev/)
- [Docker Registry Documentation](https://docs.docker.com/registry/)
- [ttl.sh Ephemeral Registry](https://ttl.sh/)

## Support

For issues or questions:
- Check [Troubleshooting](#troubleshooting) section above
- Review test output with `-v` flag for details
- Ensure all prerequisites are installed and up to date
- Check cosign version: `cosign version` (minimum v3.0.2)

## Quick Reference

```bash
# Unit tests (fast, no prerequisites)
welder run test

# Integration tests (requires cosign)
go test -tags=integration ./pkg/security/...

# E2E tests (requires cosign + docker)
go test -tags=e2e ./pkg/security/signing/

# All tests with verbose output
go test -v -tags="integration,e2e" ./pkg/security/...

# Check cosign installation
cosign version

# Check docker installation
docker version

# Generate test keys
cosign generate-key-pair
```
