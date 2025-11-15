---
title: GCP & GKE Autopilot
description: This guide is for DevOps teams who want to configure a parent stack for deploying infrastructure on Google Cloud Platform (GCP) using GKE Autopilot
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Guide: Configuring a Parent Stack for GCP & GKE Autopilot with Simple Container**

This guide is for **DevOps teams** who want to configure a **parent stack (`server.yaml`)** for deploying infrastructure **on Google Cloud Platform (GCP) using GKE Autopilot** with **Simple Container**.

With this setup, developers can **deploy microservices to GKE Autopilot** while leveraging **GCP-native services like CloudSQL, Redis, and Pub/Sub**.

---

# **1️⃣ Prerequisites**
Before configuring the parent stack, ensure that:

✅ You have a **GCP account** and a **GCP project**.

✅ You have a **GCP service account with proper IAM permissions** to create GKE clusters and other resources.

✅ **Simple Container** is installed:

   ```sh
   curl -s "https://dist.simple-container.com/sc.sh" | bash
   ```

---

# **2️⃣ Setting Up GCP Authentication & Secrets**

## **Step 1: Define `secrets.yaml`**
Create the **`.sc/stacks/devops/secrets.yaml`** file to store GCP credentials:
```yaml
---
# File: "myproject/.sc/stacks/devops/secrets.yaml"
schemaVersion: 1.0

auth:
  gcloud:
    type: gcp-service-account
    config:
      projectId: "my-gcp-project-id"
      credentials: |-
        {
          "type": "service_account",
          "project_id": "my-gcp-project-id",
          "private_key_id": "60bb42f229bc21f6d303b5967b6cd59265cb316d",
          "private_key": "-----BEGIN PRIVATE KEY-----\nBLABLABLA\n-----END PRIVATE KEY-----\n",
          "client_email": "deploy-bot@my-gcp-project-id.iam.gserviceaccount.com",
          "client_id": "2387492479284792742398427",
          "auth_uri": "https://accounts.google.com/o/oauth2/auth",
          "token_uri": "https://oauth2.googleapis.com/token",
          "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
          "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/deploy-bot%40my-gcp-project-id.iam.gserviceaccount.com"
        }

values:
  CLOUDFLARE_API_TOKEN: "abcdefgh123456789"
  MONGODB_ATLAS_PUBLIC_KEY: "public-key-123"
  MONGODB_ATLAS_PRIVATE_KEY: "private-key-456"
```

### **🔹 What This Does**

✅ **Stores GCP service account credentials** (`gcloud`).

✅ **Saves API tokens for DNS management**.

---

# **3️⃣ Configuring Infrastructure Provisioning (`server.yaml`)**

Now, define **`.sc/stacks/devops/server.yaml`** to provision **GKE Autopilot, CloudSQL, Redis, and Pub/Sub**.

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
      type: gcp-bucket
      config:
        credentials: "${auth:gcloud}"
        projectId: "${auth:gcloud.projectId}"
        bucketName: myproject-sc-state
        location: europe-west3
    secrets-provider:
      type: gcp-kms
      config:
        projectId: "${auth:gcloud.projectId}"
        keyName: myproject-sc-kms-key
        keyLocation: global
        credentials: "${auth:gcloud}"

# Deployment templates for GKE Autopilot workloads
templates:
  stack-per-app-gke:
    type: gcp-gke-autopilot
    config:
      projectId: "${auth:gcloud.projectId}"
      credentials: "${auth:gcloud}"
      gkeClusterResource: gke-autopilot-res
      artifactRegistryResource: artifact-registry-res

# Infrastructure resources provisioned inside GCP
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${secret:CLOUDFLARE_API_TOKEN}"
      accountId: "89cc23bd273c76d6767f6566c54621c2"
      zoneName: "myproject.com"

  resources:
    staging:
      template: stack-per-app-gke
      resources:
        mongodb:
          type: mongodb-atlas
          config:
            admins: [ "admin" ]
            developers: [ "developer1" ]
            instanceSize: "M10"
            orgId: "878cd82332ff12c2332d2234"
            region: "EU_CENTRAL_1"
            cloudProvider: GCP
            privateKey: "${secret:MONGODB_ATLAS_PRIVATE_KEY}"
            publicKey: "${secret:MONGODB_ATLAS_PUBLIC_KEY}"
        redis:
          type: gcp-redis
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            memorySizeGb: 2
            region: europe-west3
        gke-autopilot-res:
          type: gcp-gke-autopilot-cluster
          config:
            gkeMinVersion: "1.33.4-gke.1245000"  # Check: gcloud container get-server-config --location=europe-west3
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: europe-west3
        artifact-registry-res:
          type: gcp-artifact-registry
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            location: europe-west3
        pubsub:
          type: gcp-pubsub
          config:
            projectId: "${auth:gcloud.projectId}"
            credentials: "${auth:gcloud}"
            subscriptions:
              - name: workers.image-generator.sub
                topic: workers.image-generator
```

### **🔹 What This Does**

✅ **Configures Pulumi** for managing **state in a Google Cloud Storage bucket**.

✅ **Uses GCP KMS to encrypt secrets**.

✅ **Defines a GKE Autopilot template** (`stack-per-app-gke`) for deploying workloads.

✅ **Provisions MongoDB Atlas, Redis, Pub/Sub, and Artifact Registry** to support microservices.

---

# **4️⃣ Provisioning the GCP & GKE Autopilot Parent Stack**
Once `server.yaml` is configured, **provision** the infrastructure:

```sh
sc provision -s devops
```

### **What This Does**

✅ Creates a **Google Cloud Storage bucket** for state storage.

✅ Deploys **MongoDB Atlas, Redis, and Pub/Sub** in GCP.

✅ Configures **GKE Autopilot for running microservices**.

---

# **5️⃣ Deploying Microservices to GKE Autopilot**
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

✅ The service is **automatically deployed to GKE Autopilot** using the defined settings.

---

# **6️⃣ Advanced Configuration: Vertical Pod Autoscaler (VPA)**

GKE Autopilot supports **Vertical Pod Autoscaler (VPA)** for automatic resource optimization. Simple Container provides built-in VPA support for both **application deployments** and **Caddy ingress controllers**.

## **VPA for Application Deployments**

Add VPA configuration to your `client.yaml` using `cloudExtras`:

```yaml
# File: "myproject/.sc/stacks/myservice/client.yaml"
stacks:
  staging:
    type: cloud-compose
    parent: myproject/devops
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses: [mongodb]
      runs: [myservice]
      # VPA Configuration for automatic resource optimization
      cloudExtras:
        vpa:
          enabled: true
          updateMode: "Auto"  # Off, Initial, Recreation, Auto
          minAllowed:
            cpu: "100m"
            memory: "128Mi"
          maxAllowed:
            cpu: "2"
            memory: "4Gi"
          controlledResources: ["cpu", "memory"]
```

## **VPA for Caddy Ingress Controller**

Configure VPA for the Caddy ingress controller in your `server.yaml`:

```yaml
# File: "myproject/.sc/stacks/devops/server.yaml"
resources:
  staging:
    resources:
      gke-cluster:
        type: gcp-gke-autopilot-cluster
        config:
          projectId: "${auth:gcloud.projectId}"
          credentials: "${auth:gcloud}"
          location: "us-central1"
          gkeMinVersion: "1.33.4-gke.1245000"
          # Caddy configuration as part of GKE Autopilot cluster
          caddy:
            enable: true
            namespace: caddy
            replicas: 2
            # VPA Configuration for Caddy ingress controller
            vpa:
              enabled: true
              updateMode: "Recreation"  # Recommended for ingress controllers
              minAllowed:
                cpu: "50m"
                memory: "64Mi"
              maxAllowed:
                cpu: "1"
                memory: "1Gi"
            # Optional: Manual resource limits alongside VPA
            resources:
              limits:
                cpu: "500m"
                memory: "512Mi"
              requests:
                cpu: "100m"
                memory: "128Mi"
```

## **VPA Update Modes**

| Mode | Description | Use Case |
|------|-------------|----------|
| **Off** | Only provides recommendations | Testing and analysis |
| **Initial** | Sets resources only at pod creation | Conservative approach |
| **Recreation** | Updates by recreating pods | Recommended for stateless apps |
| **Auto** | Updates resources in-place | Advanced use (may cause brief interruptions) |

## **VPA Best Practices for GKE Autopilot**

✅ **Use `Recreation` mode** for ingress controllers like Caddy to avoid service interruptions

✅ **Set appropriate `minAllowed`** to prevent resource starvation

✅ **Set reasonable `maxAllowed`** to control costs

✅ **Monitor VPA recommendations** before enabling automatic updates

✅ **Combine with manual resource limits** for fine-grained control

---

# **7️⃣ Summary**
| Step                | Command                             | Purpose                                 |
|---------------------|-------------------------------------|-----------------------------------------|
| **Define Secrets**  | `secrets.yaml`                      | Stores GCP credentials                  |
| **Configure Infra** | `server.yaml`                       | Defines GKE Autopilot & GCP resources   |
| **Provision Infra** | `sc provision -s devops`            | Deploys GCP infrastructure              |
| **Define Service**  | `client.yaml`                       | Describes a microservice deployment     |
| **Deploy Service**  | `sc deploy -s myservice -e staging` | Deploys a microservice to GKE Autopilot |
