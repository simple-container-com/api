---
title: Motivation
description: Description of why Simple Container is a useful tool for development of services
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Why Simple Container?**

In today's fast-paced software development environment, organizations must deploy and manage **dozens or even hundreds of microservices** efficiently. Traditional **CI/CD pipelines** often require **significant DevOps effort**, complex infrastructure provisioning, and manual management of microservices. **Simple Container** offers a **game-changing approach** that simplifies deployment, automates infrastructure management, and accelerates software delivery.

This page explores why organizations should adopt **`sc`** for their **CI/CD pipelines**, highlighting its **ease of use, automation capabilities, cloud-agnostic nature, and security features**.

As organizations embrace **microservices architectures**, they face a fundamental challenge: **how to efficiently manage, deploy, and scale multiple services without creating complexity for developers and DevOps teams**. Traditional methods of provisioning and deploying microservices often involve **manual infrastructure management, complex CI/CD pipelines, and extensive collaboration between developers and DevOps**.

Simple Container is a **powerful yet lightweight tool** designed to streamline **microservices deployment and delivery**. It provides a **developer-friendly approach** while keeping **infrastructure management centralized and isolated from service deployment**. By bridging the gap between **DevOps and development teams**, `sc` enables organizations to **move faster, reduce bottlenecks, and simplify their microservices workflow**.

For the detailed use-cases of what you can do with Simple Container, please read [Howto](/howto/use-cases/) section.

---

## **1️⃣ The Challenges of Microservices Deployment**

Microservices offer **flexibility, scalability, and modularity**, but **deploying and maintaining them is complex**. In a traditional setup:

- **Developers must coordinate with DevOps** teams to configure cloud resources (databases, networking, secrets, storage).
- **Infrastructure provisioning requires expertise in Infrastructure as Code (IaC) tools** like Terraform or Pulumi.
- **Deployments are tightly coupled with CI/CD pipelines**, requiring manual configuration for every new service.
- **Secrets management and environment-specific configurations require additional setup**, often leading to security risks.

As organizations scale, these challenges become **major bottlenecks**, slowing down **innovation and delivery cycles**.

---

## **2️⃣ What is Simple Container?**

Simple Container is a **cloud-agnostic deployment tool** designed specifically for **microservices**. It provides:

✅ **A unified way to deploy services without DevOps intervention.**

✅ **A declarative configuration approach using simple YAML files.**

✅ **Built-in secrets management and cloud resource integration.**

✅ **Seamless support for Kubernetes, AWS ECS Fargate, and Google Cloud Run.**

At its core, `sc` allows **developers to focus on coding**, while **DevOps teams manage infrastructure separately**. This **clear separation of concerns** eliminates the need for developers to understand cloud provisioning and allows **DevOps to standardize infrastructure across all services**.

---

## **3️⃣ Where Simple Container Fits in Microservices Development**

### **🔹 Empowering Developers with Hassle-Free Deployments**
With `sc`, developers no longer need to:

- **Request infrastructure changes from DevOps.**
- **Manually configure networking, storage, or database connections.**
- **Write complex deployment scripts.**

Instead, they can **deploy new microservices seamlessly**, using simple configurations that **automatically integrate with the existing infrastructure**.

This **reduces friction between development and operations**, enabling teams to launch new features **faster and more reliably**.

### **🔹 A Single Source of Truth for Infrastructure**
While developers focus on building services, **DevOps teams retain full control over infrastructure**. In `sc`:

- Cloud resources such as **databases, message queues, storage, and networking** are centrally defined.
- **Secrets and environment-specific configurations are managed securely.**
- **All infrastructure is provisioned via a single, structured configuration, reducing inconsistencies.**

This **eliminates duplication, reduces security risks, and ensures a consistent setup across multiple services**.

### **🔹 Automating CI/CD Without Extra Complexity**
Traditional CI/CD pipelines require:

- **Setting up deployment scripts for every service.**
- **Managing multiple cloud provider integrations.**
- **Manually configuring rollbacks and service scaling.**

With `sc`, these processes are **built-in**. Deployments happen **without manually modifying CI/CD pipelines**, and rollback mechanisms ensure **safer deployments with minimal downtime**.

---

## **4️⃣ Where Simple Container Fits in Microservices Delivery**

### **🔹 Cloud-Agnostic Deployment for Enterprise Flexibility**
Organizations today operate in **multi-cloud and hybrid environments**. Some workloads run **on Kubernetes clusters**, while others leverage **managed services like AWS ECS Fargate or Google Cloud Run**.

Simple Container enables **seamless deployment across all these platforms** without requiring developers to modify service configurations. This **future-proofs deployments**, allowing teams to switch cloud providers **without rewriting infrastructure code**.

### **🔹 Security & Compliance Without Extra Tools**
Security is a **critical concern** in microservices delivery. Managing **secrets and environment-specific credentials** across multiple services is challenging.

`sc` offers **built-in secrets management**, ensuring that credentials are:

- **Stored securely in cloud-native secret managers (AWS Secrets Manager, GCP Secret Manager, Kubernetes Secrets).**
- **Automatically injected into services at runtime.**
- **Never exposed in plaintext or CI/CD logs.**

This **reduces security risks while keeping deployments efficient**.

### **🔹 Scalability & Reliability for Growing Organizations**
As organizations scale, microservices need to:

- **Spin up new instances dynamically based on demand.**
- **Auto-scale without manual intervention.**
- **Ensure high availability and fault tolerance.**

Simple Container integrates **seamlessly with cloud-native scaling solutions**, ensuring that services remain **resilient, highly available, and cost-efficient**.

---

## **5️⃣ Why Organizations Should Adopt `sc`**

### **✅ Faster Microservice Deployment**

- Developers can **self-deploy services** without waiting for DevOps.
- No need for **custom Terraform/Pulumi configurations** for every microservice.

### **✅ Reduced DevOps Overhead**

- DevOps focuses **only on infrastructure**, reducing operational burden.
- Centralized management **eliminates duplication** and ensures consistency.

### **✅ Built-in Security & Secrets Management**

- Secrets are **automatically handled**, reducing the risk of leaks.
- Cloud-native integrations ensure **compliance with security policies**.

### **✅ Cloud-Agnostic & Future-Proof**

- Works with **Kubernetes, AWS, and Google Cloud** seamlessly.
- Migrating workloads **requires no changes to service configurations**.

### **✅ CI/CD Without Complexity**

- **No need for external deployment tools**—CI/CD is built into `sc`.
- **Automated rollbacks** ensure stability in production.

---

## **Conclusion**

Simple Container is a **revolutionary tool** for organizations adopting **microservices architectures**. By **simplifying deployments, reducing operational complexity, and ensuring cloud-agnostic flexibility**, `sc` empowers **both developers and DevOps teams** to work more efficiently.

For any organization scaling microservices, `sc` presents a **compelling alternative** to traditional CI/CD pipelines, offering **automation, security, and ease of use**—all in a **developer-friendly format**.

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