---
title: AWS ECS Fargate
description: This guide is for DevOps teams who want to configure a parent stack for deploying infrastructure on AWS ECS Fargate and MongoDB Atlas
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Guide: Configuring a Parent Stack for AWS ECS Fargate & MongoDB Atlas with Simple Container**

This guide is for **DevOps teams** who want to configure a **parent stack (`server.yaml`)** for deploying infrastructure **on AWS ECS Fargate** and **MongoDB Atlas** using **Simple Container**.

With this setup, developers can **deploy microservices to ECS Fargate** while using **MongoDB Atlas as a managed database**.

---

# **1️⃣ Prerequisites**
Before configuring the parent stack, ensure that:

✅ You have an **AWS account** & IAM credentials with permissions to create ECS Fargate clusters.
✅ You have a **MongoDB Atlas account** with a valid API key.
✅ **Simple Container is installed**:
   ```sh
   curl -s "https://dist.simple-container.com/sc.sh" | bash
   ```

---

# **2️⃣ Setting Up AWS & MongoDB Atlas Secrets**

AWS and MongoDB Atlas credentials must be stored in **`secrets.yaml`**.

## **Step 1: Define `secrets.yaml`**
Create the **`.sc/stacks/devops/secrets.yaml`** file to store AWS credentials & MongoDB Atlas API keys:
```yaml
---
# File: "myproject/.sc/stacks/devops/secrets.yaml"

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

### **🔹 What This Does**
✅ Stores **AWS credentials** for programmatic access.
✅ Saves **MongoDB Atlas API keys** for provisioning databases.

---

# **3️⃣ Configuring Infrastructure Provisioning (`server.yaml`)**

Now, define `.sc/stacks/devops/server.yaml` to provision **ECS Fargate & MongoDB Atlas**.

## **Step 2: Define `server.yaml`**
```yaml
---
# File: "myproject/.sc/stacks/devops/server.yaml"
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

# Deployment templates for ECS Fargate-based workloads
templates:
  stack-per-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"

# Infrastructure resources provisioned inside AWS & MongoDB Atlas
resources:
  registrar:
    type: cloudflare  # Optional DNS management
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
            developers: [ "developer1", "developer2" ]
            instanceSize: "M10"
            orgId: "878cd82332ff12c2332d2234"
            region: "US_WEST_2"
            cloudProvider: AWS
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
            backup:
              every: 4h
              retention: 24h
```

### **🔹 What This Does**
✅ **Configures Pulumi** for managing **state in an S3 bucket**.
✅ **Uses AWS KMS to encrypt secrets**.
✅ **Defines an ECS Fargate template (`stack-per-app`)** for developers to deploy microservices.
✅ **Provisions MongoDB Atlas**, making it available for microservices.

---

# **4️⃣ Provisioning the AWS & MongoDB Atlas Parent Stack**
Once `server.yaml` is configured, **provision** the infrastructure:

```sh
sc provision -s devops
```

### **What This Does**
✅ Creates an **S3 bucket** for state storage.
✅ Deploys **MongoDB Atlas** with provisioned users.
✅ Configures **Cloudflare DNS (optional)**.
✅ Prepares **ECS Fargate infrastructure for microservices**.

---

# **5️⃣ Deploying Microservices to ECS Fargate**
Once the infrastructure is provisioned, developers can deploy their microservices.

## **Step 1: Define `client.yaml` for a Microservice**
```yaml
---
# File: "myproject/.sc/stacks/myservice/client.yaml"

schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: myproject/devops
    config:
      domain: ${env:MY_SERVICE_DOMAIN}
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - mongodb
      runs:
        - myservice
      size:
        cpu: 512
        memory: 1024
      scale:
        min: 1
        max: 5
        policy:
          cpu:
            max: 70
      env:
        DATABASE_HOST: "${resource:mongodb.host}"
        DATABASE_NAME: "${resource:mongodb.database}"
        DATABASE_USER: "${resource:mongodb.user}"
      secrets:
        DATABASE_PASSWORD: "${resource:mongodb.password}"
```

## **Step 2: Deploy the Service**
```sh
sc deploy -s myservice -e staging
```

✅ The service is **automatically deployed to ECS Fargate** using the defined settings.

---

# **6️⃣ Summary**
| Step                | Command                             | Purpose                                       |
|---------------------|-------------------------------------|-----------------------------------------------|
| **Define Secrets**  | `secrets.yaml`                      | Stores AWS & MongoDB Atlas credentials        |
| **Configure Infra** | `server.yaml`                       | Defines ECS Fargate & MongoDB Atlas resources |
| **Provision Infra** | `sc provision -s devops`            | Deploys AWS infrastructure                    |
| **Define Service**  | `client.yaml`                       | Describes a microservice deployment           |
| **Deploy Service**  | `sc deploy -s myservice -e staging` | Deploys a microservice to ECS Fargate         |