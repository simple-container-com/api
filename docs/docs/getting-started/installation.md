---
title: Installing and Using Simple Container
description: This guide is for both DevOps teams and developers who want to install and start using sc
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Guide: Installing and Using Simple Container**

This guide is for **both DevOps teams and developers** who want to install and start using **Simple Container** for **cloud-native microservices deployment**.

---

# **What is Simple Container?**
Simple Container is a **cloud-agnostic CI/CD tool** that simplifies the deployment of **microservices and static websites** across platforms like **Kubernetes, AWS ECS, and Google Cloud**.

**Supports multiple cloud providers** (AWS, GCP, Yandex Cloud, Cloud.ru, Kubernetes) through a pluggable architecture.

**Easy configuration using `client.yaml` and `server.yaml`**.

**Secure secrets management** with public-key encryption (`ssh-rsa`, `ssh-ed25519`) and optional `awskms://` / `gcpkms://` recipients for scoped secrets.

**Automated infrastructure provisioning and deployments**.

**Agent-ready via MCP** — `sc assistant mcp` lets Claude Code, Cursor, and [Simple Forge](https://simple-forge.com) drive SC primitives (templates, resources, parent stacks, secrets) as first-class tools. See the [Agent-first operations](#agent-first-operations) section below and the [AI Assistant guide](../ai-assistant/index.md).

---

# **Installing `sc`**
## **Install `sc` on Linux/macOS**
To install `sc`, run:
```sh
curl -s "https://dist.simple-container.com/sc.sh" | bash
```

This automatically downloads and installs `sc` in `/usr/local/bin`.

**Verify the installation:**
```sh
sc --version
```

---

# **Initializing `sc`**
After installing `sc`, you need to **set up a public-key recipient** for secrets management.

## **Step 3: Set Up Encryption Keys**
Simple Container encrypts secrets to your team's **public keys**, so every secret
file in your repo is readable only to key holders. Both key types are first-class:

| Recipient key | Cipher | How to use |
|---|---|---|
| **`ssh-rsa`** | RSA-OAEP (SHA-256) | `~/.ssh/id_rsa` / `~/.ssh/id_rsa.pub` |
| **`ssh-ed25519`** | X25519 ECDH sealed box (HKDF-SHA256 + ChaCha20-Poly1305) | `~/.ssh/id_ed25519` / `~/.ssh/id_ed25519.pub` |

Scoped secrets additionally accept **`awskms://`** and **`gcpkms://`** URL
recipients when secret-level audit trails or hardware-backed key material is
required. See [Secrets Management](../guides/secrets-management.md#cipher-per-recipient-type)
for cipher internals, recipient management, and key rotation.

If you **already have an SSH key**, initialize `sc`:
```sh
sc secrets init
```
If you **need to generate a new SSH key**, use:
```sh
sc secrets init -g
```

This writes `.sc/cfg.default.yaml` and registers your public key in
`.sc/secrets.yaml` — the recipient registry every subsequent
`sc secrets add` encrypts to. For CI/CD, you'll also want a separate
`github` profile (see Step 6).

---

# **Setting Up the Parent Stack (For DevOps)**
The **DevOps team** must configure a **parent stack** (`server.yaml`) to define infrastructure and cloud resources.

## **Step 4: Create `secrets.yaml`**
```sh
mkdir -p .sc/stacks/devops
touch .sc/stacks/devops/secrets.yaml
```
Define **cloud authentication and secrets** in `secrets.yaml`:
```yaml
---
schemaVersion: 1.0

auth:
  aws:
    type: aws-token
    config:
      accessKey: "AKIAIOSFODNN7EXAMPLE"
      secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
      region: "us-west-2"

values:
  CLOUDFLARE_API_TOKEN: "abcdefgh123456789"
  MONGODB_ATLAS_PUBLIC_KEY: "public-key-123"
  MONGODB_ATLAS_PRIVATE_KEY: "private-key-456"
```

This file is in **plaintext right now** — it has not yet been registered
with `sc`, and nothing is stopping an accidental `git add .` from
committing it. Register it so SC manages its encryption, THEN encrypt it,
BEFORE touching git:

```sh
# 1. Register the file with SC — adds it to .sc/secrets.yaml's `files:`
#    list and adds a `.sc/stacks/devops/secrets.yaml` entry to .gitignore
#    so a bare `git add .` cannot catch the plaintext.
sc secrets add .sc/stacks/devops/secrets.yaml

# 2. Encrypt all registered files — rewrites each in-place to the
#    ciphertext form safe to commit alongside the rest of the repo.
sc secrets hide
```

!!! danger "Never commit a plaintext `secrets.yaml`"
    `sc secrets add` is NOT optional. SC's registry at `.sc/secrets.yaml`
    is the single source of truth for which files are secrets — a file
    that is not registered is not encrypted, not gitignored, and not
    noticed by `sc secrets hide` on your way to commit. The three plaintext
    tokens above are perfectly valid secret material; the moment you push,
    GitHub's secret scanner (and every other scanner) will flag them.

    See the [Secrets Management guide](../guides/secrets-management.md)
    for the full workflow — adding files, rotating recipients, scoped
    secrets, and recovering from a leak.

---

## **Step 5: Define the Infrastructure in `server.yaml`**
Now, define **infrastructure resources** inside `.sc/stacks/devops/server.yaml`:
```yaml
---
schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        credentials: "${auth:aws}"
        bucketName: myproject-sc-state
    secrets-provider:
      type: aws-kms
      config:
        credentials: "${auth:aws}"
        keyName: myproject-sc-kms-key

templates:
  stack-per-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"

resources:
  staging:
    template: stack-per-app
    resources:
      mongodb:
        type: mongodb-atlas
        config:
          admins: [ "admin" ]
          developers: [ "developer1" ]
          instanceSize: "M10"
          region: "US_WEST_2"
          cloudProvider: AWS
          privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
          publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
```

### **Step 6: Provision the Parent Stack**
Once `server.yaml` is configured, **provision the infrastructure**:
```sh
sc provision -s devops
```

This **creates AWS infrastructure and configures MongoDB Atlas**.

---

# **Setting Up Services (For Developers)**
Once the **parent stack is ready**, developers can configure **`client.yaml`** to deploy services.

## **Step 7: Create `client.yaml` for a Microservice**
```sh
mkdir -p .sc/stacks/myservice
touch .sc/stacks/myservice/client.yaml
```
Define the **service configuration** inside `.sc/stacks/myservice/client.yaml`:
```yaml
---
schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: myproject/devops
    config:
      domain: staging-myservice.myproject.com
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - mongodb
      runs:
        - myservice
      env:
        DATABASE_HOST: "${resource:mongodb.host}"
        DATABASE_NAME: "${resource:mongodb.database}"
        DATABASE_USER: "${resource:mongodb.user}"
      secrets:
        DATABASE_PASSWORD: "${resource:mongodb.password}"
```

---

# **Deploying a Microservice**
Once **`client.yaml`** is defined, deploy the service.

### **Step 8: Deploy the Service to Staging**
```sh
sc deploy -s myservice -e staging
```
This **builds, pushes, and deploys the service to AWS ECS Fargate**.

---

# **Managing Secrets with `sc`**

The lifecycle is **register → hide → commit → reveal → work → hide → commit**:

### **Register a Secret File**
```sh
sc secrets add .sc/stacks/devops/secrets.yaml
sc secrets add .env
```
Each `sc secrets add` call records the file in the registry at
`.sc/secrets.yaml` and adds a `.gitignore` entry. **Every file containing
credentials or secret values needs this** — the plaintext stack
`secrets.yaml`, local `.env` files, Kubernetes kubeconfigs, service-account
JSON key files, anything a scanner would flag.

### **Hide Secrets Before Committing**
```sh
sc secrets hide
```
Encrypts every registered file to the public keys of every recipient
listed in `.sc/secrets.yaml`. Run before every `git commit` that touches
secret material.

### **Reveal Secrets After Pulling Changes**
```sh
sc secrets reveal
```
Decrypts every registered file in place using your private key — the only
local step after a `git pull` that landed a secret rotation.

### **Add a Teammate**
```sh
sc secrets add-recipient <path-to-their-pubkey>
sc secrets hide                                   # re-encrypt to the new set
```
Also used to add a `github` profile key for CI/CD. See
[Secrets Management → Team Collaboration Workflow](../guides/secrets-management.md#team-collaboration-workflow)
for recipient types (SSH RSA, SSH ed25519, AWS KMS, GCP KMS), rotation
procedures, and the two-profile model (`default` for local, `github` for
GitHub Actions).

---

# **Agent-first operations**

Every step above is driveable by an AI coding agent (Claude Code, Cursor,
etc.) through `sc`'s built-in MCP server. The agent sees SC primitives —
templates, resources, parent stacks, secrets — as first-class tools rather
than guessing YAML. Two entry points, pick the one that matches your
workflow:

### **`sc assistant` — on-demand scaffolding + MCP in your own editor**
```sh
# In an app repo — generates client.yaml, docker-compose.yaml, Dockerfile
sc assistant dev

# In a platform repo — generates server.yaml + provisioner + secrets wiring
sc assistant devops

# As a persistent MCP server any agent client can connect to
sc assistant mcp
```
`sc assistant mcp` is the integration surface Claude Code, Cursor, and
[Forge](https://simple-forge.com) all consume. See the
[AI Assistant guide](../ai-assistant/index.md) for the tool schema and
mode internals.

### **[Simple Forge](https://simple-forge.com) — hosted agentic DevOps**
Forge is the hosted platform built on top of Simple Container: agents
that run end-to-end SDLC operations (bootstrap a parent stack, add a
service, deploy, debug, destroy) against your repos. The same `sc`
primitives you've just installed drive the Forge platform — your SC
configurations stay portable between a local terminal, a GitHub Action,
and a Forge-driven agent run. Forge agents follow published SDLC skills
such as `sc-init-secrets`, `sc-parent-setup`, `sc-service-deploy`, and
`sc-caveats`, which operationalise the same workflow this guide walks
through.

### **Local-agent discipline: never let an agent commit plaintext secrets**
Agent-first workflows amplify the Step 4 rule above. When an agent writes
or modifies a secret file:

1. The agent runs `sc secrets add <path>` **before** staging it.
2. The agent runs `sc secrets hide` **before** `git add` / `git commit`.
3. The agent **never** runs `git add -A` or `git add .` in a directory
   containing a plaintext secret file (there is a short race window
   between step 1 and step 2 where the file is on disk unencrypted but
   `.gitignore` protects it; a careless `git add -f` can still catch it).

The Forge platform's `sc-init-secrets` skill encodes this invariant, and
`sc` itself refuses to encrypt a file with no registered recipients — the
system is fail-closed by default.

---

# **Summary**
| Step                      | For                 | Command                                            | Purpose                              |
|---------------------------|---------------------|----------------------------------------------------|--------------------------------------|
| **Install `sc`**          | DevOps & Developers | `curl -s "https://dist.simple-container.com/sc.sh" | bash`                                | Installs Simple Container CLI |
| **Initialize Secrets**    | DevOps & Developers | `sc secrets init -g`                               | Generates SSH keys for secrets       |
| **Define Infrastructure** | DevOps              | `server.yaml`                                      | Configures cloud resources           |
| **Provision Infra**       | DevOps              | `sc provision -s devops`                           | Deploys AWS/GCP/Kubernetes resources |
| **Define a Service**      | Developers          | `client.yaml`                                      | Configures microservice deployment   |
| **Deploy a Service**      | Developers          | `sc deploy -s myservice -e staging`                | Deploys microservice to the cloud    |
| **Register a Secret File**| DevOps & Developers | `sc secrets add .sc/stacks/<stack>/secrets.yaml`   | Registers + gitignores               |
| **Encrypt Before Commit** | DevOps & Developers | `sc secrets hide`                                  | Rewrites to ciphertext safe to push  |
| **Decrypt After Pull**    | DevOps & Developers | `sc secrets reveal`                                | Restores plaintext locally           |
| **Agent-driven ops**      | Everyone            | `sc assistant mcp` / [simple-forge.com](https://simple-forge.com) | Drive SC primitives from a Claude Code / Cursor / Forge agent |