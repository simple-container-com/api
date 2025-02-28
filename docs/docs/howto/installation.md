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

# **1️⃣ What is Simple Container?**
Simple Container is a **cloud-agnostic CI/CD tool** that simplifies the deployment of **microservices and static websites** across platforms like **Kubernetes, AWS ECS, and Google Cloud**.

✅ **Supports multiple cloud providers** (AWS, GCP, Kubernetes).
✅ **Easy configuration using `client.yaml` and `server.yaml`**.
✅ **Secure secrets management** with SSH-based encryption.
✅ **Automated infrastructure provisioning and deployments**.

---

# **2️⃣ Installing `sc`**
## **🔹 Step 1: Install `sc` on Linux/macOS**
To install `sc`, run:
```sh
curl -s "https://dist.simple-container.com/sc.sh" | bash
```
✅ This automatically downloads and installs `sc` in `/usr/local/bin`.

**Verify the installation:**
```sh
sc --version
```

---

## **🔹 Step 2: Install `sc` on Windows**
For Windows users:
1. Download the latest release from [Simple Container Downloads](https://dist.simple-container.com/).
2. Extract and add the binary to the system `PATH`.
3. Verify installation:
   ```sh
   sc --version
   ```

---

# **3️⃣ Initializing `sc`**
After installing `sc`, you need to **set up SSH authentication** for secrets management.

## **🔹 Step 3: Set Up SSH Key for Secrets**
If you **already have an SSH key**, initialize `sc`:
```sh
sc secrets init
```
If you **need to generate a new SSH key**, use:
```sh
sc secrets init -g
```

✅ This ensures that **secrets** can be securely encrypted and managed.

---

# **4️⃣ Setting Up the Parent Stack (For DevOps)**
The **DevOps team** must configure a **parent stack** (`server.yaml`) to define infrastructure and cloud resources.

## **🔹 Step 4: Create `secrets.yaml`**
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

✅ This securely **stores AWS credentials and API tokens**.

---

## **🔹 Step 5: Define the Infrastructure in `server.yaml`**
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

### **🔹 Step 6: Provision the Parent Stack**
Once `server.yaml` is configured, **provision the infrastructure**:
```sh
sc provision -s devops
```

✅ This **creates AWS infrastructure and configures MongoDB Atlas**.

---

# **5️⃣ Setting Up Services (For Developers)**
Once the **parent stack is ready**, developers can configure **`client.yaml`** to deploy services.

## **🔹 Step 7: Create `client.yaml` for a Microservice**
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

# **6️⃣ Deploying a Microservice**
Once **`client.yaml`** is defined, deploy the service.

### **🔹 Step 8: Deploy the Service to Staging**
```sh
sc deploy -s myservice -e staging
```
✅ This **builds, pushes, and deploys the service to AWS ECS Fargate**.

---

# **7️⃣ Managing Secrets with `sc`**
### **🔹 Add a Secret File**
```sh
sc secrets add .env
```
✅ Encrypts `.env` before committing to Git.

### **🔹 Hide Secrets Before Committing**
```sh
sc secrets hide
```
✅ Ensures **no secrets are leaked in Git**.

### **🔹 Reveal Secrets After Pulling Changes**
```sh
sc secrets reveal
```
✅ **Decrypts and restores** secret files locally.

---

# **8️⃣ Summary**
| Step                      | For                 | Command                                            | Purpose                              |
|---------------------------|---------------------|----------------------------------------------------|--------------------------------------|
| **Install `sc`**          | DevOps & Developers | `curl -s "https://dist.simple-container.com/sc.sh" | bash`                                | Installs Simple Container CLI |
| **Initialize Secrets**    | DevOps & Developers | `sc secrets init -g`                               | Generates SSH keys for secrets       |
| **Define Infrastructure** | DevOps              | `server.yaml`                                      | Configures cloud resources           |
| **Provision Infra**       | DevOps              | `sc provision -s devops`                           | Deploys AWS/GCP/Kubernetes resources |
| **Define a Service**      | Developers          | `client.yaml`                                      | Configures microservice deployment   |
| **Deploy a Service**      | Developers          | `sc deploy -s myservice -e staging`                | Deploys microservice to the cloud    |
| **Manage Secrets**        | DevOps              | `sc secrets add .env`                              | Encrypts a secret file               |
