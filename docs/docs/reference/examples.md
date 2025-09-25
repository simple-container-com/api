---
title: Examples
description: This document provides some examples of where organizations can benefit from Simple Container usage
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Examples**

## **1️⃣ Simplifying Microservice Deployment for Developers**

One of the biggest challenges in a **microservices architecture** is ensuring that **developers can deploy new services independently** without requiring constant DevOps involvement.

### **How `sc` Helps**

✅ **Developers only need a `client.yaml` configuration** to deploy a new service.

✅ **Familiar tooling** like **Dockerfile** and **docker-compose** keeps onboarding simple.

✅ **No need for Terraform/Pulumi modifications**—DevOps manages infrastructure separately.

### **Example: Adding a New Microservice**
With traditional CI/CD pipelines, adding a microservice requires:

- **Infrastructure changes** (Terraform modules, Helm charts).
- **CI/CD pipeline configuration**.
- **Networking, secrets, and storage setup**.

With **`sc`**, a developer **only defines `client.yaml`**:

```yaml
---
schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: myorg/devops
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - postgres
      runs:
        - myservice
      env:
        POSTGRES_HOST: "${resource:postgres.host}"
        DATABASE_USER: "${resource:postgres.user}"
      secrets:
        DATABASE_PASSWORD: "${resource:postgres.password}"
```

✅ **One simple YAML file replaces hours of DevOps work.**
✅ **Developers focus on coding, not cloud infrastructure.**

---

## **2️⃣ Centralized Infrastructure Management for DevOps**

Traditional **microservice deployments** require DevOps teams to configure:

- **Cloud infrastructure** (AWS ECS, Kubernetes, networking).
- **Secrets management** (AWS Secrets Manager, Vault, Kubernetes Secrets).
- **State management** (Terraform or Pulumi state).

With `sc`, DevOps **only needs to define core infrastructure** in **`server.yaml`**:

```yaml
---
schemaVersion: 1.0

resources:
  staging:
    template: stack-per-app
    resources:
      postgres:
        type: gcp-cloudsql-postgres
        config:
          projectId: "${auth:gcloud.projectId}"
```

✅ **Once defined, this setup supports all microservices without modifications.**

✅ **Developers are isolated from infrastructure complexity.**

---

## **3️⃣ Built-in CI/CD, No Need for External Automation**

Most CI/CD tools **require custom scripts** for building, pushing, and deploying services. With `sc`:

✅ **CI/CD is built-in**—no need for **Jenkins, GitHub Actions, or manual Helm deployments**.

✅ **Deploy with a single command**:

```sh
sc deploy -s myservice -e staging
```
✅ **Automatic rollbacks** make it safer than traditional pipelines.

### **Comparison: CI/CD Complexity**
| Feature                 | Traditional CI/CD                          | Simple Container        |
|-------------------------|--------------------------------------------|--------------------------------|
| **Pipeline Complexity** | Requires scripting (Bash, Terraform, Helm) | **Just use `sc deploy`**       |
| **Secret Injection**    | Needs Vault, AWS Secrets Manager           | **Built-in (`sc secrets`)**    |
| **Rollback Mechanism**  | Manual process                             | **Automated rollback support** |

---

## **4️⃣ Cloud-Agnostic & Easy Migration**

Organizations **often need to migrate workloads** between **AWS, GCP, and Kubernetes clusters**. With Terraform or Pulumi, migrations **require rewriting infrastructure code**.

With `sc`, migrations **only require modifying `server.yaml`**, while `client.yaml` remains **unchanged**.

✅ **Move workloads from AWS to GCP without changing service configurations.**

✅ **Supports AWS ECS, Kubernetes, and Google Cloud Run natively.**

🔹 **Example: Migrating from AWS to GCP**
- **Terraform/Pulumi:** Requires modifying state storage, networking, IAM policies.
- **SC:** Just update `server.yaml`, **no changes at the service level**.

```yaml
---
# Before (AWS)
resources:
  staging:
    template: stack-per-app
    resources:
      postgres:
        type: aws-rds-postgres
        config:
          instanceSize: "db.t3.micro"

# After (GCP)
resources:
  staging:
    template: stack-per-app
    resources:
      postgres:
        type: gcp-cloudsql-postgres
        config:
          instanceSize: "db-f1-micro"
```
✅ **Migrate entire workloads in minutes, not weeks.**

---

## **5️⃣ Secure Secrets Management Built-In**

Managing secrets securely is **a major challenge in CI/CD**. Most organizations rely on **Vault, AWS Secrets Manager, or Kubernetes Secrets**, requiring **manual configuration**.

### **How `sc` Handles Secrets Automatically**

✅ **Secrets are securely stored in the cloud provider's native secret manager**.

✅ **No need for manual secret injection—SC provisions and injects secrets automatically.**

| Cloud Provider | Secrets Storage       |
|----------------|-----------------------|
| **AWS**        | AWS Secrets Manager   |
| **GCP**        | Google Secret Manager |
| **Kubernetes** | Kubernetes Secrets    |

Example **secret injection in `client.yaml`**:
```yaml
secrets:
  DATABASE_PASSWORD: "${resource:postgres.password}"
```
✅ **Automatically stored in AWS/GCP/Kubernetes Secrets—fully managed by `sc`.**

---

## **6️⃣ Faster Time to Market with Less Overhead**

By adopting `sc`, organizations gain:

✅ **Faster onboarding**—developers deploy services with a simple YAML config.

✅ **Less DevOps overhead**—DevOps teams focus on core infrastructure, not microservices.

✅ **Reduced CI/CD complexity**—built-in deployment automation eliminates external tooling.

### **Comparison: Developer Workflow**
| Task                       | Traditional Pipeline                 | SC-Powered Pipeline                          |
|----------------------------|--------------------------------------|----------------------------------------------|
| **Add a new microservice** | Modify Terraform/Pulumi, Helm charts | Add `client.yaml`, deploy instantly          |
| **Manage secrets**         | Requires Vault, AWS Secrets Manager  | **Built-in (`sc secrets`)**                  |
| **Deploy a service**       | Manual CI/CD setup                   | **`sc deploy -s myservice -e staging`**      |
| **Migrate across clouds**  | Requires rewriting Terraform/Pulumi  | **Update `server.yaml`, no service changes** |

---

## **Conclusion**

Organizations adopting **Simple Container (`sc`)** for their **CI/CD pipelines** gain:

✅ **Faster deployments** with minimal configuration.

✅ **Cloud-agnostic flexibility** without rewriting infrastructure.

✅ **Reduced DevOps effort**—developers manage deployments independently.

✅ **Built-in security and secrets management** without external tools.

By **simplifying microservice deployment**, **reducing overhead**, and **automating infrastructure management**, `sc` **transforms CI/CD pipelines into a developer-friendly, efficient workflow**.