# Branch Preview Workflow — Architecture Design

**Date**: 2026-04-07
**Branch**: `feature/branch-builds-for-preview-versions`
**Status**: Draft

---

## 1. Overview

A new GitHub Actions workflow (`branch-preview.yaml`) that builds and publishes a fully testable preview release of Simple Container from any feature branch. Unlike the main `push.yaml` release, a preview build:

- Does **not** overwrite the `sc.sh` installer or the global `version` file at `dist.simple-container.com`
- Publishes versioned SC binaries downloadable by pinning `SIMPLE_CONTAINER_VERSION`
- Publishes a branch-specific `simplecontainer/github-actions:{version}` Docker image
- Creates a dedicated release commit (on a separate `release/{version}` branch) where all `action.yml` files reference the preview Docker image tag
- Pushes a git tag `v{version}` pointing to that release commit, making the SC GitHub Actions usable at that exact version via `@v{version}`

---

## 2. Version Format

Preview versions follow the CalVer pattern already used by `push.yaml`, extended with a commit suffix:

```
{YYYY}.{MM}.{DD}.{patch}-preview.{short_sha}
```

**Example**: `2026.04.07.3-preview.abc1234`

### Computation logic

The `prepare` job uses the same `reecetech/version-increment@2023.10.2` action as `push.yaml` to compute the next CalVer version, but with `use_api: "false"` so it only calculates the version without creating a real git tag (the actual tag is created later by `publish-git-tag` with the preview suffix):

```yaml
- name: Get next version
  uses: reecetech/version-increment@2023.10.2
  id: base-version
  with:
    scheme: "calver"
    increment: "patch"
    use_api: "false"   # compute only — no git tag created here

- name: Build preview version string
  id: version
  run: |
    SHORT_SHA=$(git rev-parse --short=7 HEAD)
    VERSION="${{ steps.base-version.outputs.version }}-preview.${SHORT_SHA}"
    echo "version=${VERSION}" >> $GITHUB_OUTPUT
    echo "short-sha=${SHORT_SHA}" >> $GITHUB_OUTPUT
```

This ensures:
- Version increment logic is identical to production (no custom counting scripts)
- Preview versions sort below the corresponding release version (`2026.04.07.3-preview.abc1234` < `2026.04.07.3` in pre-release ordering)
- The version is globally unique due to the `short_sha` suffix

---

## 3. Workflow File

**Location**: `.github/workflows/branch-preview.yaml`

### 3.1 Triggers

```yaml
on:
  workflow_dispatch:   # manual trigger from any branch
```

`workflow_dispatch` only — no automatic push triggers. Preview builds are opt-in to avoid noise on every branch push.

### 3.2 Concurrency

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true   # cancel old runs for same branch
```

### 3.3 Permissions

```yaml
permissions:
  contents: write   # needed to create and push git tags and release branch
```

---

## 4. Job Graph

```
                   prepare
                      │
                      ▼
                 build-setup
          ┌───────────┼───────────┐
          ▼           ▼           ▼
   build-platforms build-binaries test
          │               │        │
          │               └───┬────┘
          │                   ▼
          │             docker-build
          │                   │
          └──────┬────────────┘
                 │
    ┌────────────┴────────────┐
    ▼                         ▼
publish-sc-preview    publish-git-tag
    └────────────┬────────────┘
                 │
              finalize
```

### Parallelism rationale

| Job | Waits for | Reason |
|---|---|---|
| `build-platforms` | `build-setup` | Go tools + schemas needed |
| `build-binaries` | `build-setup` | Go tools needed |
| `test` | `build-setup` | Go tools needed |
| `docker-build` | `build-binaries`, `test` | Needs the binary artifact; tests must pass. Does **not** need `build-platforms` (SC binaries not used in Docker image) |
| `publish-sc-preview` | `build-platforms`, `test` | Needs versioned SC tarballs; tests must pass. Does **not** need `docker-build` |
| `publish-git-tag` | `docker-build` | Docker image must be published before the git tag references it. Does **not** need `build-platforms` or `publish-sc-preview` |
| `finalize` | `publish-sc-preview`, `publish-git-tag` | Gates notifications on both publish jobs completing |

`publish-sc-preview` and `publish-git-tag` run **in parallel** — they have no dependency on each other.

### Critical path

```
prepare → build-setup → build-binaries ─┐
                                         ├─ docker-build → publish-git-tag → finalize
                              test ──────┘
```

`build-platforms → publish-sc-preview` runs alongside this path and feeds into `finalize`.

---

## 5. Job Specifications

### 5.1 `prepare`

**Runner**: `ubuntu-latest`
**Outputs**: `version`, `short-sha`

Steps:
1. `actions/checkout@v4` (default depth is sufficient; `reecetech` uses the GitHub API via `use_api: false` to inspect tags)
2. `reecetech/version-increment@2023.10.2` with `use_api: "false"` — computes next CalVer patch without creating a tag
3. Append commit suffix to form the full preview version

```yaml
- uses: actions/checkout@v4
- name: Get next version
  uses: reecetech/version-increment@2023.10.2
  id: base-version
  with:
    scheme: "calver"
    increment: "patch"
    use_api: "false"
- name: Build preview version string
  id: version
  run: |
    SHORT_SHA=$(git rev-parse --short=7 HEAD)
    VERSION="${{ steps.base-version.outputs.version }}-preview.${SHORT_SHA}"
    echo "version=${VERSION}" >> $GITHUB_OUTPUT
    echo "short-sha=${SHORT_SHA}" >> $GITHUB_OUTPUT
```

---

### 5.2 `build-setup`

**Runner**: `blacksmith-8vcpu-ubuntu-2204`
**Needs**: `prepare`
**Outputs**: `cicd-bot-telegram-token`, `cicd-bot-telegram-chat-id`

Identical to `branch.yaml`'s `build-setup` job:
- Install SC (latest release for secrets access)
- Reveal secrets
- `welder run rebuild` (rebuilds SC binary from branch source, `SKIP_EMBEDDINGS=true`)
- Run `clean`, `tools`, `generate-schemas`, `fmt`
- Extract Telegram secrets
- Upload `bin-tools` artifact

> **Why `welder run rebuild` uses `latest` SC**: The rebuild step installs a fresh SC binary from the branch source and injects it into `./bin/sc`. From that point forward, the branch's own SC binary is used.

---

### 5.3 `build-platforms`

**Runner**: `blacksmith-8vcpu-ubuntu-2204`
**Needs**: `[prepare, build-setup]`
**Matrix**: `linux/amd64`, `darwin/arm64`, `darwin/amd64`

Same as `push.yaml` — builds versioned SC binaries with `VERSION` injected via ldflags:

```bash
go build \
  -ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=${VERSION}" \
  -o dist/${GOOS}-${GOARCH}/sc ./cmd/sc

tar -czf .sc/stacks/dist/bundle/sc-${GOOS}-${GOARCH}.tar.gz -C dist/${GOOS}-${GOARCH} sc
cp .sc/stacks/dist/bundle/sc-${GOOS}-${GOARCH}.tar.gz \
   .sc/stacks/dist/bundle/sc-${GOOS}-${GOARCH}-v${VERSION}.tar.gz
```

Uploads artifact: `sc-{os}-{arch}` containing only the versioned tarball (`sc-{os}-{arch}-v{version}.tar.gz`).

---

### 5.4 `build-binaries`

**Runner**: `blacksmith-8vcpu-ubuntu-2204`
**Needs**: `[prepare, build-setup]` — parallel with `build-platforms` and `test`
**Matrix**: `github-actions` target only (cloud-helpers is not needed for preview)

```bash
go build \
  -a -installsuffix cgo \
  -ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=${VERSION}" \
  -o dist/github-actions ./cmd/github-actions
```

Uploads artifact: `github-actions-binary`.

---

### 5.5 `test`

**Runner**: `blacksmith-8vcpu-ubuntu-2204`
**Needs**: `[prepare, build-setup]` — parallel with `build-platforms` and `build-binaries`

```bash
go test ./...
```

---

### 5.6 `docker-build`

**Runner**: `blacksmith-8vcpu-ubuntu-2204`
**Needs**: `[prepare, build-setup, build-binaries, test]`

Does **not** need `build-platforms` — SC binaries are not used in the Docker image. Starts as soon as `build-binaries` and `test` complete, in parallel with `build-platforms` tail work.

Builds and pushes **only** the preview-versioned `github-actions` image. No `latest`, `staging`, or other tags are written.

```yaml
tags: simplecontainer/github-actions:${{ needs.prepare.outputs.version }}
```

Steps:
1. Download `github-actions-binary` artifact → `dist/`
2. Install SC (latest), reveal secrets
3. Docker Buildx setup
4. Docker Hub login via `sc stack secret-get -s dist dockerhub-cicd-token`
5. Build and push `github-actions.Dockerfile` with single preview tag

---

### 5.7 `publish-sc-preview`

**Runner**: `blacksmith-8vcpu-ubuntu-2204`
**Needs**: `[prepare, build-setup, build-platforms, test]`

Does **not** need `docker-build` — SC binary publishing is independent of the Docker image build. Runs in parallel with `publish-git-tag`.

Uploads versioned SC tarballs to `dist.simple-container.com` **without** modifying `sc.sh` or the `version` file.

Steps:
1. Download all `sc-*` platform artifacts → `artifacts/`
2. Download `bin-tools` artifact, fix permissions
3. Install SC (latest), reveal secrets
4. Assemble the dist bundle — **versioned tarballs only**:

```bash
mkdir -p .sc/stacks/dist/bundle
cp artifacts/sc-*/*-v${VERSION}.tar.gz .sc/stacks/dist/bundle/

# IMPORTANT: do NOT copy sc.sh or write a version file
# This prevents overwriting the latest pointer for anyone running sc.sh without pinning a version
```

5. Deploy via SC:

```bash
bash <(curl -Ls "https://welder.simple-container.com/welder.sh") deploy -e prod --timestamps
```

> **Contract**: The `dist` stack deploy must be idempotent and additive — uploading files to the CDN bucket without deleting files that are not in the bundle. If the current stack implementation does a full sync (potentially deleting `sc.sh`), the deploy step must be adjusted to use a targeted upload or the stack must support a `--no-delete` mode. This is a prerequisite risk to validate during implementation.

After this step, users can install a preview version with:

```bash
SIMPLE_CONTAINER_VERSION=2026.04.07.3-preview.abc1234 \
  curl -s "https://dist.simple-container.com/sc.sh" | bash
```

---

### 5.8 `publish-git-tag`

**Runner**: `blacksmith-8vcpu-ubuntu-2204`
**Needs**: `[prepare, docker-build]`

Does **not** need `build-platforms` or `publish-sc-preview` — the git tag only references the Docker image, which must already exist. Runs in parallel with `publish-sc-preview`.

This job creates a dedicated release commit with updated `action.yml` files and pushes a git tag. It does **not** modify the working branch.

Steps:

#### Step 1 — Checkout and configure git

```bash
git remote set-url origin https://${{ secrets.GITHUB_TOKEN }}@github.com/simple-container-com/api.git
git fetch --tags
```

Uses `fregante/setup-git-user@v2` for bot identity.

#### Step 2 — Create release branch from current HEAD

```bash
RELEASE_BRANCH="release/${VERSION}"
git checkout -b "${RELEASE_BRANCH}"
```

#### Step 3 — Update all action.yml docker image references

Replace `docker://simplecontainer/github-actions:staging` → `docker://simplecontainer/github-actions:{version}` in all four action files:

```bash
find .github/actions -name "action.yml" | while read f; do
  sed -i "s|docker://simplecontainer/github-actions:staging|docker://simplecontainer/github-actions:${VERSION}|g" "$f"
done
```

**Files affected**:
- `.github/actions/cancel-stack/action.yml`
- `.github/actions/deploy-client-stack/action.yml`
- `.github/actions/destroy/action.yml`
- `.github/actions/provision-parent-stack/action.yml`

#### Step 4 — Commit and tag

```bash
git add .github/actions/*/action.yml
git commit -m "chore: release preview v${VERSION} - update github-actions image tag"
git tag "v${VERSION}"
git push origin "v${VERSION}"
# Optionally push the release branch for traceability:
git push origin "${RELEASE_BRANCH}"
```

> The tag `v{version}` now points to a commit where all SC GitHub Actions reference the exact preview Docker image. Users of this repo can reference actions at that tag:
>
> ```yaml
> uses: simple-container-com/api/.github/actions/deploy-client-stack@v2026.04.07.3-preview.abc1234
> ```
>
> This will pull the Docker image `simplecontainer/github-actions:2026.04.07.3-preview.abc1234`.

---

### 5.9 `finalize`

**Runner**: `ubuntu-latest`
**Needs**: `[prepare, build-setup, publish-sc-preview, publish-git-tag]`
**Condition**: `always()` — runs regardless of upstream success/failure to send Telegram notifications

#### Build summary

On success, writes a GitHub Actions job summary (`$GITHUB_STEP_SUMMARY`) with copy-paste instructions:

```yaml
- name: Write build summary
  if: ${{ !contains(needs.*.result, 'failure') }}
  env:
    VERSION: ${{ needs.prepare.outputs.version }}
  run: |
    cat >> $GITHUB_STEP_SUMMARY << EOF
    ## Preview build v${VERSION}

    ### Use SC GitHub Actions at this version

    Reference any SC action with the \`@v${VERSION}\` tag:

    \`\`\`yaml
    - uses: simple-container-com/api/.github/actions/deploy-client-stack@v${VERSION}
      with:
        stack-name: my-stack
        sc-config: \${{ secrets.SC_CONFIG }}

    - uses: simple-container-com/api/.github/actions/provision-parent-stack@v${VERSION}
      with:
        stack-name: my-stack
        sc-config: \${{ secrets.SC_CONFIG }}

    - uses: simple-container-com/api/.github/actions/destroy@v${VERSION}
      with:
        stack-name: my-stack
        sc-config: \${{ secrets.SC_CONFIG }}
    \`\`\`

    This uses Docker image: \`simplecontainer/github-actions:${VERSION}\`

    ### Install this SC version

    \`\`\`bash
    SIMPLE_CONTAINER_VERSION=${VERSION} curl -s "https://dist.simple-container.com/sc.sh" | bash
    \`\`\`

    > **Note**: this is a preview build from branch \`$GITHUB_REF_NAME\`. It will not be picked up by anyone running \`sc.sh\` without the version pin.
    EOF
```

Telegram notification pattern is identical to `branch.yaml`, with the preview version included in the status message.

---

## 6. Data Flow Diagram

```
Branch HEAD
    │
    ├─── build SC binaries ──────────────────────► dist.simple-container.com
    │      (VERSION injected)                        sc-linux-amd64-v{version}.tar.gz
    │                                                sc-darwin-arm64-v{version}.tar.gz
    │                                                sc-darwin-amd64-v{version}.tar.gz
    │                                                (sc.sh NOT updated)
    │
    ├─── build github-actions binary
    │       └─── docker build ──────────────────► Docker Hub
    │              (github-actions.Dockerfile)      simplecontainer/github-actions:{version}
    │                                               (latest/staging NOT updated)
    │
    └─── release branch ─────────────────────────► GitHub Repo
           action.yml files updated                 tag: v{version}
           to use :{version} docker tag             branch: release/{version}
```

---

## 7. What Is NOT Done (Preserving `latest`)

| Artifact | Preview behavior | Production behavior |
|---|---|---|
| `sc.sh` | **NOT updated** — stays at last released version | Updated with new version number |
| `dist.simple-container.com/version` | **NOT updated** | Updated to new version |
| `simplecontainer/github-actions:latest` | **NOT pushed** | Pushed |
| `simplecontainer/github-actions:staging` | **NOT pushed** | N/A |
| `action.yml` on working branch | **NOT modified** — changes go to `release/{version}` | N/A |

---

## 8. Security Considerations

- `GITHUB_TOKEN` with `contents: write` is used for git push. The `release/{version}` branch and tag push are the only write operations.
- Docker Hub credentials come from SC secrets (same pattern as production builds).
- The preview tag uses a content-addressed suffix (commit SHA) preventing tag collisions or silent overwrites.
- Preview Docker images do not receive the `latest` tag, so they are not accidentally pulled by users without explicit pinning.

---

## 9. Prerequisites and Risks

| Item | Risk | Mitigation |
|---|---|---|
| `welder deploy` full-sync behavior | Could delete `sc.sh` if bundle doesn't include it | Audit `dist` stack Pulumi code; use `--no-delete` flag or targeted file upload if needed |
| `reecetech/version-increment` tag creation | Would create a real CalVer tag for a preview build | Not used; version computed manually via `git tag -l` count |
| `GITHUB_TOKEN` push permissions | Branch protection on `main` may block push to `release/*` | Release branch is a new branch, not protected; git tag push should be allowed |
| Parallel preview runs same day | Two concurrent runs could compute the same `patch` number | Acceptable — `short_sha` still makes version unique; worst case is a tag conflict that surfaces immediately |

---

## 10. Example End-to-End Usage

After a preview build of `feature/my-feature` completes:

**Install preview SC CLI**:
```bash
SIMPLE_CONTAINER_VERSION=2026.04.07.3-preview.abc1234 \
  curl -s "https://dist.simple-container.com/sc.sh" | bash
```

**Use preview SC GitHub Actions**:
```yaml
steps:
  - uses: simple-container-com/api/.github/actions/deploy-client-stack@v2026.04.07.3-preview.abc1234
    with:
      stack-name: my-stack
      sc-config: ${{ secrets.SC_CONFIG }}
```
This references Docker image `simplecontainer/github-actions:2026.04.07.3-preview.abc1234` directly.

**Verify published Docker image**:
```bash
docker pull simplecontainer/github-actions:2026.04.07.3-preview.abc1234
```
