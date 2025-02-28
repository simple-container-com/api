---
title: Migrating from Terraform or Pulumi
description: This tutorial guides DevOps teams and developers on how to migrate from Terraform or Pulumi to Simple Container
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Tutorial: Migrating from Terraform or Pulumi to Simple Container**

This tutorial guides **DevOps teams and developers** on how to migrate from **Terraform or Pulumi** to **Simple Container** for microservice deployment and infrastructure management.

✅ **Why Migrate to `sc`?**
- **Simplifies microservice deployment** (CI/CD, scaling, rollbacks, and secrets included).
- **Reduces complexity** (high-level YAML configuration instead of Terraform HCL or Pulumi code).
- **Cloud-agnostic** (migrate across AWS, GCP, and Kubernetes **without modifying service configurations**).

---

# **1️⃣ Understanding the Migration Approach**

| Feature                       | Terraform / Pulumi                      | Simple Container                         |
|-------------------------------|-----------------------------------------|-------------------------------------------------|
| **Infrastructure Management** | Declarative IaC (HCL, Python, Go, etc.) | **High-level YAML (`server.yaml`)**             |
| **Microservice Deployment**   | Requires external CI/CD                 | **Built-in (`client.yaml`)**                    |
| **Secrets Management**        | Requires external tools (Vault, SSM)    | **Built-in (`sc secrets`)**                     |
| **Networking & Routing**      | Requires manual configuration           | **Simplified with `caddy` and ingress support** |

✅ **SC abstracts infrastructure complexity while keeping cloud provider flexibility**.

---

# **2️⃣ Step-by-Step Migration Guide**
This guide covers **migrating an AWS ECS Fargate and MongoDB Atlas setup** from Terraform/Pulumi to **Simple Container**.

---

## **🔹 Step 1: Extract Infrastructure Configuration**
Identify Terraform or Pulumi resources you need to migrate.

### **Example Terraform Setup**
```hcl
resource "aws_s3_bucket" "state" {
  bucket = "myproject-sc-state"
}

resource "aws_ecs_cluster" "cluster" {
  name = "my-ecs-cluster"
}

resource "aws_rds_instance" "database" {
  engine         = "postgres"
  instance_class = "db.t3.micro"
  allocated_storage = 20
}
```

### **Example Pulumi Setup (TypeScript)**
```typescript
import * as aws from "@pulumi/aws";

const stateBucket = new aws.s3.Bucket("state");

const cluster = new aws.ecs.Cluster("ecs-cluster");

const db = new aws.rds.Instance("database", {
  engine: "postgres",
  instanceClass: "db.t3.micro",
  allocatedStorage: 20,
});
```

✅ **We will migrate these resources to `server.yaml`.**

---

## **🔹 Step 2: Create `secrets.yaml`**
Define **cloud authentication and credentials** in **SC’s secrets file**.

```sh
mkdir -p .sc/stacks/devops
touch .sc/stacks/devops/secrets.yaml
```

### **`secrets.yaml` for AWS & MongoDB Atlas**
```yaml
---
# File: ".sc/stacks/devops/secrets.yaml"

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

✅ **SC securely manages AWS and MongoDB credentials**.

---

## **🔹 Step 3: Define `server.yaml` for Infrastructure**
Instead of using **Terraform** or **Pulumi**, SC defines **infrastructure in `server.yaml`**.

```sh
touch .sc/stacks/devops/server.yaml
```

### **`server.yaml` for AWS ECS & MongoDB Atlas**
```yaml
---
# File: ".sc/stacks/devops/server.yaml"

schemaVersion: 1.0

# Provisioning state management
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

# Deployment templates for ECS Fargate workloads
templates:
  stack-per-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"

# Infrastructure resources provisioned inside AWS & MongoDB Atlas
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "89cc23bd273c76d6767f6566c54621c2"
      zoneName: "myproject.com"

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

✅ **SC simplifies infrastructure by grouping resources logically in `server.yaml`**.

---

## **🔹 Step 4: Provision the Infrastructure**
Instead of running **Pulumi or Terraform**, use:
```sh
sc provision -s devops
```
✅ **This provisions AWS ECS, MongoDB Atlas, and networking automatically**.

---

## **🔹 Step 5: Define `client.yaml` for Microservices Deployment**
Configure **SC’s microservice deployment** instead of Terraform’s ECS task definitions.

```sh
mkdir -p .sc/stacks/myservice
touch .sc/stacks/myservice/client.yaml
```

### **`client.yaml` for Deploying a Microservice**
```yaml
---
# File: ".sc/stacks/myservice/client.yaml"

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

✅ **SC automatically maps microservices to infrastructure resources**.

---

## **🔹 Step 6: Deploy the Microservice**
Instead of manually defining ECS tasks in Terraform or Pulumi, use:
```sh
sc deploy -s myservice -e staging
```
✅ **SC automates the CI/CD process**.

---

# **3️⃣ Summary of Migration**
| Task                               | Terraform / Pulumi                | Simple Container             |
|------------------------------------|-----------------------------------|-------------------------------------|
| **Define Secrets**                 | AWS Secrets Manager, Vault        | `secrets.yaml`                      |
| **Define Infrastructure**          | Terraform / Pulumi files          | `server.yaml`                       |
| **Provision Infra**                | `terraform apply` or `pulumi up`  | `sc provision -s devops`            |
| **Define Microservice Deployment** | ECS Task Definitions, Helm Charts | `client.yaml`                       |
| **Deploy Microservice**            | CI/CD + Terraform                 | `sc deploy -s myservice -e staging` |

✅ **SC simplifies infrastructure and deployment** while keeping cloud flexibility.
