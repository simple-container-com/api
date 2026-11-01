# **Simple Container**

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/simple-container-com/api/badge)](https://scorecard.dev/viewer/?uri=github.com/simple-container-com/api)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/12886/badge)](https://www.bestpractices.dev/projects/12886)
[![OpenSSF Baseline](https://www.bestpractices.dev/projects/12886/baseline)](https://www.bestpractices.dev/projects/12886)
[![CodeQL](https://github.com/simple-container-com/api/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/simple-container-com/api/actions/workflows/codeql.yml)
[![Semgrep](https://github.com/simple-container-com/api/actions/workflows/semgrep.yml/badge.svg?branch=main)](https://github.com/simple-container-com/api/actions/workflows/semgrep.yml)
[![SLSA Build L3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev/spec/v1.0/levels#build-l3)
[![Sigstore signed](https://img.shields.io/badge/sigstore-signed-blue?logo=sigstore)](docs/SECURITY.md)
[![Build](https://github.com/simple-container-com/api/actions/workflows/push.yaml/badge.svg?branch=main)](https://github.com/simple-container-com/api/actions/workflows/push.yaml)
[![govulncheck](https://github.com/simple-container-com/api/actions/workflows/govulncheck.yml/badge.svg?branch=main)](https://github.com/simple-container-com/api/actions/workflows/govulncheck.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/simple-container-com/api.svg)](https://pkg.go.dev/github.com/simple-container-com/api)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

**Simple Container (`sc`)** is a **cloud-agnostic deployment tool** designed to simplify **microservices deployment, 
infrastructure provisioning, and CI/CD automation**. 

It allows **developers to deploy services effortlessly**, while **DevOps teams manage infrastructure centrally**, 
ensuring **scalability, security, and reliability across multi-cloud environments**.

---

## **Features**

1. **Infrastructure as Code (IaC) Simplified** – Define infrastructure with **lightweight YAML configurations**, no need for Terraform or Pulumi.
2. **Effortless Microservice Deployment** – Deploy services with **one command**, using familiar tools like `Dockerfile` and `docker-compose.yaml`.
3. **Built-in Secrets Management** – Securely store and inject credentials using **AWS Secrets Manager, GCP Secret Manager, or Kubernetes Secrets**.
4. **Multi-Cloud & Kubernetes Support** – Seamlessly deploy to **AWS ECS, Google Cloud Run, Kubernetes clusters**, and more.
5. **Built-in CI/CD & Automated Rollbacks** – Deploy, monitor, and rollback services **without external CI/CD pipelines**.
6. **Comprehensive CloudWatch Monitoring** – Built-in alerts for ECS services and Application Load Balancers with **Email, Slack, Discord, and Telegram** notifications.

---

## **Installation**

To install `sc`, run:
```sh
curl -s "https://dist.simple-container.com/sc.sh" | bash
```
Verify installation:
```sh
sc --version
```

---

## **Getting Started**

1. **Initialize a project**
   ```sh
   sc init
   ```

2. **Configure infrastructure (for DevOps)**
    - Define cloud resources in `.sc/stacks/devops/server.yaml`.

3. **Define a microservice (for developers)**
    - Add a `client.yaml` file in `.sc/stacks/<service-name>/client.yaml`.

4. **Deploy a service**
   ```sh
   sc deploy -s myservice -e staging
   ```

---

## **Documentation**

Check out the [full documentation](https://docs.simple-container.com) for detailed guides, examples, and best practices.

---

## **Plugins**

Cloud integrations ship as independently released plugin binaries that `sc`
downloads and talks to over a local gRPC bus. Everything about an operator's
`client.yaml` stays the same; `server.yaml` names which plugin release to use.

| Plugin | Repository | Owns |
|---|---|---|
| `aws` | [simple-container-com/sc-plugin-aws](https://github.com/simple-container-com/sc-plugin-aws) | `aws` provider, `aws-lambda` / `ecs-fargate` / `aws-static-website` templates, `s3-bucket` / `ecr-repository` / `aws-rds-postgres` / `aws-rds-mysql` / `aws-cloudtrail-security-alerts` resources, `aws-kms` secrets-provider, `s3-bucket` state-store, `cloud-helpers` Lambda |
| `cloudflare` | [simple-container-com/sc-plugin-cloudflare](https://github.com/simple-container-com/sc-plugin-cloudflare) | `cloudflare` DNS registrar, Workers overrides, Pages Direct Upload |
| `cloudru` | [simple-container-com/sc-plugin-cloudru](https://github.com/simple-container-com/sc-plugin-cloudru) | `cloudru` provider, `cloudru-container`, `cloudru-postgres`, `cloudru-evolution-vm` |
| `mongodb` | [simple-container-com/sc-plugin-mongodb](https://github.com/simple-container-com/sc-plugin-mongodb) | `mongodb-atlas` provider + ComputeProcessor for `uses:` env fan-out, PreDestroy database-drop hook |
| `yandex` | [simple-container-com/sc-plugin-yandex](https://github.com/simple-container-com/sc-plugin-yandex) | `yandex` provider, `yc-dns`, `yc-serverless-container`, `yc-object-storage`, `yc-kms`, `yc-lockbox`, `yc-service-account`, `yandex-vm` |
| `echo` (template) | [simple-container-com/sc-plugin-echo](https://github.com/simple-container-com/sc-plugin-echo) | Template every new plugin starts from — `gh repo create --template` and swap in your cloud |

Everything else — GCP, Kubernetes, `k3s`, `vm` — is still built into `sc`.

- **Operator guide**: [docs/docs/guides/upgrading-to-plugins.md](docs/docs/guides/upgrading-to-plugins.md)
  — nothing is required of you (plugins auto-resolve to the latest release), but
  pinning the `plugins:` block in `server.yaml` is strongly recommended.
- **Author guide**: [docs/docs/guides/plugin-authoring.md](docs/docs/guides/plugin-authoring.md)
  — step-by-step for a new cloud plugin, starting from `sc-plugin-echo`.
- **Runtime library**: [pkg/plugin/runtime](pkg/plugin/runtime) —
  `NewContext`, `Settle`, `HarvestExports`, `ResourceInputForComputeProcess`,
  `DecodeInputs`. Every production plugin builds on this; do not re-copy the
  equivalents from an older plugin.
- **Bus protocol**: [pkg/plugin/README.md](pkg/plugin/README.md) and
  [pkg/plugin/proto/README.md](pkg/plugin/proto/README.md) — RPC surface,
  dispatch wiring, extraction checklist.
- **Design**: [sc-plugin-architecture](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-10-24/sc-plugin-architecture)
  (full rationale + two-gRPC conversation shape).
- **Trap catalogue**: [plugin-extraction-playbook](https://github.com/simple-container-com/forge/tree/main/docs/design/2026-11-01/plugin-extraction-playbook/)
  — fifteen live-caught traps as of 2026-11-01, with fix patterns and §4
  lint-pass checks for the next extraction.

---

## **Contributing**

We welcome contributions! Please see our [CONTRIBUTING.md](docs/CONTRIBUTING.md) for guidelines on how to get involved, and our [CODE_OF_CONDUCT.md](docs/CODE_OF_CONDUCT.md) for the community standards every participant is expected to uphold.

---

## **Roadmap**

See [ROADMAP.md](docs/ROADMAP.md) for current themes, release cadence, and where planned work surfaces in the [Issues](https://github.com/simple-container-com/api/issues) tracker.

---

## **License**

Simple Container is open-source and licensed under the [MIT License](LICENSE).

---

## **Community & Support**

1. **Feature Requests & Issues** – [GitHub Issues](https://github.com/simple-container-com/api/issues)
2. **Discussions & Updates** – [Join our Discord](https://discord.gg/simple-container)
3. **Enterprise Support** – Contact us at **support@simple-container.com**
4. **News and announcements** - [Follow us on X](https://x.com/simp_container)

Start deploying microservices effortlessly with **Simple Container** today!