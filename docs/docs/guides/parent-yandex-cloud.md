---
title: Yandex Cloud
description: This guide is for DevOps teams who want to configure a parent stack for deploying serverless containers on Yandex Cloud with Simple Container
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2026-09-27'
---


# **Guide: Configuring a Parent Stack for Yandex Cloud with Simple Container**

This guide is for **DevOps teams** deploying services on **Yandex Cloud** (YC) with
**Simple Container**. The template it covers pushes a container image to **Container
Registry**, runs it as a **Serverless Container**, delivers secrets via **Lockbox
references**, provisions the state bucket and blobs on **Object Storage**, and (when
a `.ru` domain is declared) publishes the service through an **API Gateway** with a
CNAME in the **Cloud DNS** zone you own.

---

# **Prerequisites**

- **Simple Container is installed**:

  ```sh
  curl -s "https://dist.simple-container.com/sc.sh" | bash
  ```
- A **Yandex Cloud folder** (a cloud id and folder id) and a **service account**
  with `admin` on that folder. Simple Container provisions Container Registry,
  Serverless Containers, Lockbox, IAM bindings, DNS recordsets and API Gateways
  under this identity.
- A **service-account key** (`yc iam key create --service-account-id ... --output ...`)
  and a **static access key pair** for the S3-compatible API
  (`yc iam access-key create --service-account-id ...`). The service-account key
  authenticates every YC gRPC call; the static key pair signs the Object Storage
  and Container Registry API calls that use the S3 wire protocol.
- A **Pulumi state bucket** created by hand — `yc storage bucket create --name
  <name>`. The `yc-object-storage` state backend has no `ProvisionFunc` registered
  and a self-provisioning state backend is chicken-and-egg anyway; SC references
  the bucket with `provision: false`.

---

# **Setting Up Yandex Cloud Secrets**

## **Step 1: Define `secrets.yaml`**
Create the **`.sc/stacks/devops/secrets.yaml`** file to hold the YC credentials:

```yaml
---
# File: "myproject/.sc/stacks/devops/secrets.yaml"

schemaVersion: 1.0

auth:
  yc:
    type: yc-service-account
    cloudId: b1g...
    folderId: b1g...
    # The full contents of `yc iam key create --output` for the SA that will
    # provision the stack (folder-admin).
    serviceAccountKey: |
      {
        "id": "...",
        "service_account_id": "...",
        "created_at": "...",
        "key_algorithm": "RSA_2048",
        "public_key": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n",
        "private_key": "PLEASE DO NOT REMOVE THIS LINE! Yandex.Cloud SA Key ID <...>\n-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n"
      }
    # The static access key pair (from `yc iam access-key create --output`)
    # signs S3-compatible calls: Object Storage and Container Registry.
    accessKey: "YCAJE..."
    secretAccessKey: "..."
    region: ru-central1
    zone: ru-central1-a

values:
  # Passphrase used by the `passphrase` secrets-provider below to encrypt the
  # stack's Pulumi state. Any random string; treat it like a KMS key.
  state-passphrase: "..."
```

### **What This Does**

- `${auth:yc}` interpolation resolves to the credentials blob above — every
  YC resource that needs to authenticate accepts `credentials: "${auth:yc}"`.
- **The blob is opaque.** SC resolves `${auth:yc}` into the `Credentials` field
  of a config struct and never fans it out into sibling fields such as
  `folderId`. Any custom resource type must call `api.ConvertAuth(cfg,
  &cfg.AccountConfig)` itself or fail with `folderId must be set` — the
  built-in `yc-*` types already do.

> The `.sc/stacks/*/secrets.yaml` files are **gitignored**; the encrypted
> `.sc/secrets.yaml` registry is committed. Do not commit the plaintext.

---

# **Configuring Infrastructure Provisioning (`server.yaml`)**

## **Step 2: Define `server.yaml`**

```yaml
---
# File: "myproject/.sc/stacks/devops/server.yaml"
schemaVersion: 1.0

# Provisioning state management
provisioner:
  type: pulumi
  config:
    # Pulumi state in a YC Object Storage bucket you created by hand.
    # `provision: false` is required — see Prerequisites.
    state-storage:
      type: yc-object-storage
      config:
        credentials: "${auth:yc}"
        bucketName: myproject-sc-yc-state
        provision: false
    # `yc-kms` has no ProvisionFunc registered in the current build, so
    # `passphrase` is the practical choice for the initial deploy.
    secrets-provider:
      type: passphrase
      config:
        passPhrase: "${secret:state-passphrase}"

# Deployment template — one YC Serverless Container per service.
templates:
  yc-container:
    type: yc-serverless-container
    config:
      credentials: "${auth:yc}"
      # `registryId` and `serviceAccountId` are deliberately left unset:
      # SC then creates both itself (one Container Registry per stack, one
      # per-container service account with the default roles listed below).

resources:
  # Multi-registrar DNS. Declaring `registrars:` (plural) rather than the
  # legacy `registrar:` block accepts several DNS providers side-by-side;
  # each service's `domain:` is routed to the entry whose zone is the
  # longest suffix match, on a label boundary.
  registrars:
    yandex:
      type: yc-dns
      config:
        credentials: "${auth:yc}"
        # The YC *resource* name of the DNS zone, not the DNS name of the
        # zone. `simple-forge.ru.` is served by a resource named
        # `simple-forge-ru`. Set `zoneId:` directly to skip the lookup.
        zoneName: simple-forge-ru
        # The wildcard certificate covering this zone. Adopted, never
        # issued — one managed cert per zone shares the single
        # `_acme-challenge.<zone>` CNAME target; a second contends with
        # its renewal and fails silently up to 90 days later.
        certificateId: fpqu...

  resources:
    staging:
      template: yc-container
      resources:
        blobs:
          # A YC Object Storage bucket shared by all services deploying
          # into this env under the template above. SC provisions it, its
          # service account, its folder-role binding and a static S3 key
          # pair — a `${resource:blobs.accessKey}` on a client stack
          # references the key pair rather than embedding it.
          type: yc-bucket
          config:
            credentials: "${auth:yc}"
            name: myproject-blobs
            # Optional. `true` lets `sc destroy` empty the bucket during
            # teardown; without it a bucket with objects blocks the
            # destroy.
            forceDestroy: false
```

### **What This Does**

- Provisions Pulumi state in YC Object Storage using the S3-compatible API.
  The state URL SC emits is `s3://<bucket>?endpoint=https://storage.yandexcloud.net&region=<region>&s3ForcePathStyle=true` — those three
  parameters are all `gocloud` accepts on that backend; any additional query
  parameter is rejected at login time.
- Declares the `yc-container` template every service will point at. The
  provisioning code creates one Container Registry per stack (whose name is
  `<image-name>-registry`) and one service account per container (whose folder
  roles default to `container-registry.images.puller` +
  `lockbox.payloadViewer`, and can be extended via `cloudExtras.roles`).
- Wires the `yc-dns` registrar for the shared zone. Records under `NS`, `SOA`
  and anything below `_acme-challenge.` are refused at conversion time —
  overwriting any of those breaks the zone or a renewal.
- Declares one `yc-bucket` resource per environment; the S3 credentials for
  that bucket are exported so any client stack listing `uses: [blobs]` picks
  them up via `${resource:blobs.*}` placeholders.

### The edge for a `.ru` domain is an **API Gateway**, not a CDN

The `yc-dns` registrar publishes each service through an **API Gateway**
resource that terminates TLS from the adopted certificate and forwards the
target's own `Host` header to the container URL. The CDN alternative was
rejected before this design landed for two reasons: one CDN resource per
container is required (a wildcard CDN cannot fan out to multiple container
IDs — the hostname is the identity), and YC CDN keys the cache on URI
alone, so byte-identical responses on distinct hostnames leak across
services. API Gateway has neither issue and no per-resource flat fee.

## **Step 3: Provision the Parent Stack**
```sh
sc provision -s devops
```

> **`sc provision` has no `-e` flag** in the current build — it provisions
> every environment the stack declares. Use `sc deploy -e <env>` for the
> per-env action.

### **What This Does**

Reads `server.yaml`, resolves `${auth:yc}` and `${secret:…}` placeholders,
then hands the plan to Pulumi, which creates the Container Registry,
service accounts, folder-role bindings, buckets and DNS-facing resources
inside YC.

---

# **Deploying Services to Yandex Cloud**

## **Step 1: Define `client.yaml` for a service**

```yaml
---
# File: "myproject/.sc/stacks/myservice/client.yaml"

schemaVersion: 1.0

stacks:
  staging:
    type: single-image
    template: yc-container
    parent: myproject/devops
    config:
      # Routed to the parent's `yc-dns` registrar by zone suffix. Building
      # an API Gateway in front of the container is what makes TLS + a
      # `Host`-preserving edge work on a `.ru` domain.
      domain: myservice.example.ru
      image:
        dockerfile: ${git:root}/Dockerfile
      # YC container memory is capped at the free-tier limit (128 MB) up
      # to 2 GB paid, and must be a multiple of 128; SC rejects any other
      # value rather than silently rounding.
      maxMemory: 128
      # Per YC quotas: 30 s free tier / up to 3600 s paid.
      timeout: 30
      uses:
        - blobs   # picks up ${resource:blobs.*} from the parent
      env:
        LOG_LEVEL: info
      # Values here are placed in a per-container Lockbox secret, and the
      # container revision receives Lockbox *references* rather than the
      # inlined values.
      secrets:
        SOME_TOKEN: "${secret:MY_SERVICE_TOKEN}"
```

### **Advanced: YC CloudExtras**

The `cloudExtras` block on `client.yaml` carries YC-specific extras — most
prominently `schedules`, which produce YC Timer Triggers pointing back at
the container.

```yaml
stacks:
  staging:
    type: single-image
    template: yc-container
    parent: myproject/devops
    config:
      # ... basic configuration above
      cloudExtras:
        # Extra IAM roles granted to the container's service account on
        # top of `container-registry.images.puller` and
        # `lockbox.payloadViewer`.
        roles:
          - storage.viewer
          - lockbox.payloadViewer

        # Cron schedules — one YC Timer Trigger per entry, named
        # <container>-<schedule.name>. The trigger POSTs a YC-shaped
        # envelope to the container's ROOT path (`/`) and ignores the
        # `path` in its own payload — go-aws-lambda-sdk >= 2026.9.1
        # unwraps this envelope for you; a stdlib service can unwrap it
        # by hand.
        schedules:
          - name: cleanup
            # 6-field YC cron (min hour day-of-month month day-of-week
            # year), interpreted in UTC. Exactly one of day-of-month /
            # day-of-week must be `?`. A 5-field crontab line is
            # rejected. A minute is the minimum granularity.
            expression: "0 3 ? * * *"
            # Sent as the `details.payload` inside the YC trigger
            # envelope. Max 4096 characters. Not a URL query — YC does
            # not parse this string.
            request: '{"path":"/cleanup"}'
            # 1..5. Default 0 (no retry). A failed invocation is retried
            # this many times at `retryInterval` before being dropped
            # (or DLQ'd, if configured).
            retryAttempts: 2
            # 10s..60s. Must carry a unit — a bare integer is rejected
            # early with a fix-in-the-message error.
            retryInterval: 10s
            # Optional: send failed invocations to a Message Queue.
            # deadLetterQueue:
            #   queueId: ${resource:dlq.id}
```

#### **CloudExtras Field Reference**

| Field              | Type       | Description                                                | Example                                    |
|--------------------|------------|------------------------------------------------------------|--------------------------------------------|
| `roles`            | `[]string` | Extra folder-role bindings for the container's SA          | `["storage.viewer"]`                       |
| `schedules`        | `[]object` | Timer Triggers, one per entry                              | See below                                  |

**Schedules Object Fields:**

| Field           | Type     | Description                                                           | Required |
|-----------------|----------|-----------------------------------------------------------------------|----------|
| `name`          | `string` | Unique per container; produces `<container>-<name>` (YC ≤ 63 chars)   | Yes      |
| `expression`    | `string` | 6-field YC cron, UTC, exactly one of DOM/DOW is `?`                   | Yes      |
| `request`       | `string` | Payload string (≤ 4096 chars) delivered as `details.payload`          | Optional |
| `retryAttempts` | `int`    | 1..5                                                                  | Optional |
| `retryInterval` | `string` | 10s..60s, must carry a unit                                           | Optional |
| `deadLetterQueue.queueId` | `string` | YC Message Queue id for failed messages                       | Optional |

#### **Schedule Examples**

```yaml
# Every minute (useful for a smoke or a heartbeat).
- name: heartbeat
  expression: "* * * * ? *"
  request: '{"path":"/heartbeat"}'

# Daily at 03:00 UTC.
- name: nightly
  expression: "0 3 ? * * *"
  request: '{"path":"/cron/nightly"}'

# Every 15 minutes.
- name: sweep
  expression: "0/15 * ? * * *"
  request: '{"path":"/cron/sweep"}'
  retryAttempts: 3
  retryInterval: 30s
```

An AWS-style `cron(...)` wrapper is accepted and stripped, so a schedule
block can be shared byte-identical between an AWS and a YC client stack:
```yaml
- name: nightly
  expression: "cron(0 3 ? * * *)"
```

#### **Schedule change → trigger REPLACE, not update**

SC replaces the underlying Timer Trigger whenever its schedule shape
(cron expression, container ref or DLQ) changes, rather than issuing YC's
`UpdateTrigger`. This is deliberate: an in-place update via YC's API
leaves the trigger `ACTIVE` with the new spec but does not reliably re-arm
YC's scheduler when the next fire is more than a short window away. A
`+- replace` on the trigger in Pulumi's plan is expected on any cron
change and always lands the new spec on a fresh scheduler slot.

## **Step 2: Deploy the Service**
```sh
sc deploy -s myservice -e staging
```

SC builds the image, pushes it to the parent stack's Container Registry,
creates the container's Lockbox secret + version, provisions the container
revision that references them, wires the API Gateway custom domain to the
adopted certificate, and lands a CNAME in the DNS zone.

---

# **Verifying the Deploy**

```sh
# From outside YC:
curl -sS -o - https://myservice.example.ru/            # 200 + your service's response
curl -sS -o /dev/null -w "%{http_code}\n" -X POST \
  https://myservice.example.ru/                        # non-GET methods also forward
dig +short myservice.example.ru CNAME                  # <apigw-id>.<subdomain>.apigw.yandexcloud.net.
```

The container also carries a set of `SIMPLE_CONTAINER_*` env vars SC
injects on every deploy:

| Var                            | Value                                              |
|--------------------------------|----------------------------------------------------|
| `SIMPLE_CONTAINER_CLOUD`       | `yandex`                                           |
| `SIMPLE_CONTAINER_ENV`         | your env name, e.g. `staging`                      |
| `SIMPLE_CONTAINER_RESOURCE_TYPE` | `yc-serverless-container`                        |
| `SIMPLE_CONTAINER_STACK`       | `<client-stack>--<env>`                             |
| `SIMPLE_CONTAINER_VERSION`     | CalVer `YYYY.MM.DD-<sha>` in CI, `latest` locally  |

---

# **Tearing It Down**

```sh
sc destroy -s myservice -e staging          # client stack
sc destroy -s devops --parent               # parent stack
```

**What `sc destroy` removes**: the container, all revisions, its Lockbox
secret + version, its service account and folder-role bindings, the Timer
Triggers, the API Gateway, the DNS recordset that points at it, the
Container Registry and the bucket declared under the parent.

**What is intentionally preserved**:

- The **DNS zone** itself. `yc-dns` looks it up (data source) and never
  creates it — a second public zone for the same domain would be one
  nobody's nameservers point at.
- The **adopted certificate** referenced by `certificateId`. It is a plain
  string argument on the API Gateway, not a Pulumi resource, so destroy
  leaves it in place.
- The **Pulumi state bucket** (`provision: false`).

**Known destroy hazard**: a `ContainerRegistry` with images still pushed
into it refuses to delete (`Registry ... is not empty, you must delete
all images first`). The upstream `yandex_container_registry` TF resource
has no `force_destroy` knob. Empty the registry by hand before re-running
destroy:

```sh
for img in $(yc container image list --registry-id <id> --format json \
             | jq -r '.[].id'); do
  yc container image delete --id "$img"
done
sc destroy -s myservice -e staging
```

(A follow-up will add a pre-destroy hook to SC that empties the registry
via the YC `containerregistry.v1` API before Pulumi tries to delete it.)

**Lockbox** secrets enter YC's usual `PENDING_DELETE` state on destroy —
this is a YC platform behaviour, not an SC bug, and there is no cost
implication for zero-entry secrets.

---

# **Summary**

For a `.ru` service on YC, Simple Container:

- Bridges the Pulumi YC provider from `simple-container-com/pulumi-yandex`
  so a `sc deploy` runs inline Automation-API against YC even from a
  network where the upstream Terraform provider registries are geo-blocked.
- Provisions Container Registry, Serverless Containers, Lockbox and IAM
  bindings with the same client/parent shape as the AWS ECS Fargate and
  GCP Cloud Run templates.
- Publishes the service through an API Gateway backed by an adopted
  wildcard certificate, with a CNAME in the DNS zone SC looks up (never
  creates).
- Replaces the Timer Trigger on any schedule change, so a `sc deploy`
  that changes a cron always lands on a fresh YC scheduler slot.
- Cleanly tears down with `sc destroy` — except when a `ContainerRegistry`
  still holds images (see hazard above).
