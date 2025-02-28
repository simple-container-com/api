---
title: Comparision to other tools
description: This document provides a detailed comparison between **Simple Container**, **Pulumi**, and **Terraform** to help DevOps teams and developers choose the best tool for managing cloud infrastructure and microservice deployments.
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Comparison: Simple Container vs Pulumi vs Terraform**

This document provides a **detailed comparison** between **Simple Container**, **Pulumi**, and **Terraform** to help DevOps teams and developers choose the best tool for managing cloud infrastructure and microservice deployments.

---

# **1️⃣ Overview of the Tools**

| Tool                 | Purpose                                                            | Primary Users            | Configuration Language                 |
|----------------------|--------------------------------------------------------------------|--------------------------|----------------------------------------|
| **Simple Container** | **CI/CD & infrastructure deployment for microservices**            | Developers & DevOps      | YAML                                   |
| **Pulumi**           | **Infrastructure as Code (IaC) & Cloud Infrastructure Management** | DevOps & Cloud Engineers | TypeScript, Python, Go, C#             |
| **Terraform**        | **Infrastructure as Code (IaC) & Cloud Provisioning**              | DevOps & Cloud Engineers | HCL (HashiCorp Configuration Language) |

---

# **2️⃣ Key Differences**

## **🔹 Purpose & Focus**

| Feature           | Simple Container                                | Pulumi                                                            | Terraform                                                            |
|-------------------|-------------------------------------------------|-------------------------------------------------------------------|----------------------------------------------------------------------|
| **Primary Focus** | **Microservices Deployment & CI/CD Pipelines**  | **Full Infrastructure as Code (IaC) with imperative programming** | **Full Infrastructure as Code (IaC) with declarative configuration** |
| **Best For**      | **Developers & DevOps** deploying microservices | **DevOps & Cloud Engineers** managing infrastructure with code    | **DevOps & Cloud Engineers** managing cloud infrastructure           |
| **Use Cases**     | **CI/CD, Microservices, Secrets Management**    | **Cloud-native deployments, Kubernetes, AWS, GCP, Azure**         | **Cloud Infrastructure, Networking, Security, Compute**              |

✅ **SC is focused on microservice deployment, while Pulumi and Terraform are full-fledged IaC tools.**

---

## **🔹 Configuration Language**

| Feature        | Simple Container                     | Pulumi                                | Terraform                                  |
|----------------|--------------------------------------|---------------------------------------|--------------------------------------------|
| **Language**   | **YAML** (declarative)               | **TypeScript, Python, Go, C#**        | **HCL (HashiCorp Configuration Language)** |
| **Complexity** | **Simple (high-level abstractions)** | **Moderate (imperative programming)** | **Complex (declarative DSL)**              |

✅ **SC uses YAML for simplicity, whereas Pulumi and Terraform require programming or a DSL.**

---

## **🔹 Infrastructure Management**

| Feature                         | Simple Container                              | Pulumi                                         | Terraform                                            |
|---------------------------------|-----------------------------------------------|------------------------------------------------|------------------------------------------------------|
| **Infrastructure Provisioning** | ✅ **Yes (but high-level, via `server.yaml`)** | ✅ **Yes (fine-grained IaC)**                   | ✅ **Yes (fine-grained IaC)**                         |
| **Supports Multi-Cloud**        | ✅ **Yes**                                     | ✅ **Yes**                                      | ✅ **Yes**                                            |
| **State Management**            | ✅ **Built-in (S3, GCS, FS)**                  | ✅ **Built-in via Pulumi Cloud, S3, GCS, etc.** | ✅ **State stored in Terraform Cloud, S3, GCS, etc.** |

✅ **SC abstracts away infrastructure details, while Pulumi and Terraform provide full control.**

---

## **🔹 Microservice Deployment**

| Feature                                   | Simple Container                | Pulumi    | Terraform                                  |
|-------------------------------------------|---------------------------------|-----------|--------------------------------------------|
| **Deploy to Kubernetes (GKE, EKS, AKS)**  | ✅ **Yes (via `cloud-compose`)** | ✅ **Yes** | ❌ **Limited (needs Helm or K8S provider)** |
| **Deploy to AWS ECS Fargate**             | ✅ **Yes**                       | ✅ **Yes** | ✅ **Yes**                                  |
| **Deploy to AWS Lambda / GCP Cloud Run**  | ✅ **Yes (via `single-image`)**  | ✅ **Yes** | ✅ **Yes**                                  |
| **Deploy Static Websites (S3, GCS, CDN)** | ✅ **Yes (via `static`)**        | ✅ **Yes** | ✅ **Yes**                                  |

✅ **SC is **built for deployment**, while Pulumi and Terraform require manual setup for CI/CD.**

---

## **🔹 Secrets Management**

| Feature                         | Simple Container                         | Pulumi                                  | Terraform                                               |
|---------------------------------|------------------------------------------|-----------------------------------------|---------------------------------------------------------|
| **Built-in Secrets Management** | ✅ **Yes (via SSH-based encryption)**     | ✅ **Yes (via Pulumi Secrets Provider)** | ❌ **No (requires Vault, SOPS, or AWS Secrets Manager)** |
| **Secure Secret Sharing**       | ✅ **Yes**                                | ✅ **Yes**                               | ❌ **No (manual setup required)**                        |
| **Encryption Support**          | ✅ **Yes (AWS KMS, GCP KMS, passphrase)** | ✅ **Yes (KMS, GCP, HashiCorp Vault)**   | ❌ **No (requires external tools)**                      |

✅ **SC has built-in secrets management, whereas Terraform needs external tools.**

---

## **🔹 CI/CD Automation**

| Feature                       | Simple Container            | Pulumi                                      | Terraform                                   |
|-------------------------------|-----------------------------|---------------------------------------------|---------------------------------------------|
| **Built-in CI/CD Deployment** | ✅ **Yes (via `sc deploy`)** | ❌ **No (requires separate CI/CD pipeline)** | ❌ **No (requires separate CI/CD pipeline)** |
| **Easy Rollbacks**            | ✅ **Yes**                   | ❌ **No (manual rollback required)**         | ❌ **No (manual rollback required)**         |

✅ **SC is designed for CI/CD, while Pulumi and Terraform rely on external automation.**

---

# **3️⃣ When to Choose Which Tool?**
✅ **Use Simple Container if:**

- You need **easy CI/CD for microservices**.
- You want **a simple YAML-based approach** for cloud deployments.
- You require **built-in secrets management**.
- You need **fast microservice deployments to Kubernetes, ECS, or Lambda**.

✅ **Use Pulumi if:**

- You need **full Infrastructure as Code (IaC) with a programming language**.
- You want **strong integration with AWS, GCP, Azure**.
- You prefer **fine-grained control over infrastructure**.

✅ **Use Terraform if:**

- You need **full Infrastructure as Code (IaC) with a declarative approach**.
- You want **multi-cloud infrastructure provisioning**.
- You prefer **HCL over programming languages**.

---

# **4️⃣ Summary Table**

| Feature                               | Simple Container            | Pulumi                     | Terraform                        |
|---------------------------------------|-----------------------------|----------------------------|----------------------------------|
| **Best Use Case**                     | Microservice deployment     | Full IaC with programming  | Full IaC with declarative config |
| **Configuration Language**            | YAML                        | TypeScript, Python, Go, C# | HCL                              |
| **Infrastructure Provisioning**       | Limited (via `server.yaml`) | ✅ Yes                      | ✅ Yes                            |
| **Cloud Provider Support**            | AWS, GCP, Kubernetes        | AWS, GCP, Azure, K8s       | AWS, GCP, Azure, K8s             |
| **Secrets Management**                | ✅ Built-in                  | ✅ Built-in                 | ❌ External required              |
| **CI/CD Built-in**                    | ✅ Yes                       | ❌ No                       | ❌ No                             |
| **State Management**                  | ✅ Yes                       | ✅ Yes                      | ✅ Yes                            |
| **Automated Microservice Deployment** | ✅ Yes                       | ❌ No                       | ❌ No                             |

---

# **5️⃣ Conclusion**

- **Use Simple Container** (`sc`) for **fast microservice deployments with built-in CI/CD**.
- **Use Pulumi** if you need **fine-grained control over cloud resources with imperative programming**.
- **Use Terraform** if you need **declarative IaC for provisioning and managing cloud infrastructure**.

# **Migrating from Terraform or Pulumi to Simple Container: Key Benefits**

Migrating from **Terraform or Pulumi** to **Simple Container** offers a **simplified approach to microservice deployment** 
while maintaining **cloud provider flexibility**. 

Unlike Terraform and Pulumi, which require **manual infrastructure provisioning and CI/CD setup**, `sc` **automates deployments** and abstracts infrastructure complexity.

---

# **1️⃣ Key Benefits of Migrating to `sc`**

## **🔹 1. Developers Can Add Microservices Easily (Without DevOps Involvement)**

✅ **No need to manually configure cloud resources for each microservice**.

✅ **No Terraform modules or Pulumi scripts required for every new service**.

✅ **Developers only define a `client.yaml`** alongside familiar tools like **Dockerfile & docker-compose**.

**🔹 Example: Adding a New Microservice (`myservice`)**
Instead of modifying Terraform or Pulumi configurations, **developers only create a simple `client.yaml`**:

```yaml
---
# File: ".sc/stacks/myservice/client.yaml"

schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: myproject/devops
    config:
      dockerComposeFile: ./docker-compose.yaml  # Familiar tooling
      uses:
        - mongodb  # No need to define database again (DevOps-managed)
      runs:
        - myservice
      env:
        DATABASE_HOST: "${resource:mongodb.host}"
        DATABASE_USER: "${resource:mongodb.user}"
      secrets:
        DATABASE_PASSWORD: "${resource:mongodb.password}"
```

✅ **With Terraform/Pulumi, adding a microservice means modifying infrastructure code. With `sc`, developers just create `client.yaml`.**

**🔹 Deployment Command (No Separate CI/CD Setup Required)**
```sh
sc deploy -s myservice -e staging
```
✅ **No need to define ECS tasks or Kubernetes manifests—`sc` handles everything.**

---

## **🔹 2. DevOps Focuses Only on Core Infrastructure (No Need to Manage Each Microservice)**

✅ **Single source of truth for infrastructure (`server.yaml`)**.

✅ **No need to update Terraform or Pulumi for every new microservice**.

✅ **Developers are isolated from cloud infrastructure complexities**.

**🔹 Example: DevOps Defines Infrastructure Once (`server.yaml`)**
```yaml
---
# File: ".sc/stacks/devops/server.yaml"

schemaVersion: 1.0

provisioner:
  type: pulumi
  config:
    state-storage:
      type: s3-bucket
      config:
        credentials: "${auth:aws}"
        bucketName: myproject-sc-state

resources:
  staging:
    template: stack-per-app
    resources:
      mongodb:
        type: mongodb-atlas
        config:
          admins: [ "admin" ]
          instanceSize: "M10"
          region: "US_WEST_2"
          cloudProvider: AWS
          privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
          publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
```
✅ **Once this is set up, DevOps never needs to modify it for new microservices.**

---

# **2️⃣ Why `sc` Is Better for Microservices Deployment?**
| Feature                             | Terraform / Pulumi                        | Simple Container                       |
|-------------------------------------|-------------------------------------------|----------------------------------------|
| **Adding a New Microservice**       | ❌ Requires Terraform/Pulumi modifications | ✅ Developers only create `client.yaml` |
| **Cloud Infrastructure Management** | ✅ Yes                                     | ✅ Yes (but DevOps-only)                |
| **Microservice Deployment**         | ❌ Requires CI/CD setup                    | ✅ Built-in (`sc deploy`)               |
| **Secrets Management**              | ❌ External tools required                 | ✅ Built-in (`sc secrets`)              |
| **Multi-Cloud Support**             | ✅ Yes                                     | ✅ Yes                                  |

---

# **3️⃣ Summary**

✅ **Developers can add microservices easily** without modifying infrastructure code.

✅ **DevOps only manages core cloud resources** in a centralized `server.yaml`.

✅ **Secrets and deployments are automated**, reducing manual work.

✅ **No Terraform/Pulumi modifications needed for each new service**.
