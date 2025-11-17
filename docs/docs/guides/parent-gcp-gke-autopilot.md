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
          updateMode: "Auto"  # Off, Initial, Auto, InPlaceOrRecreate
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
              updateMode: "Auto"  # Recommended for ingress controllers (recreates pods)
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

| Mode                  | Description                             | Use Case                       |
|-----------------------|-----------------------------------------|--------------------------------|
| **Off**               | Only provides recommendations           | Testing and analysis           |
| **Initial**           | Sets resources only at pod creation     | Conservative approach          |
| **Auto**              | Updates by recreating pods              | Recommended for stateless apps |
| **InPlaceOrRecreate** | Updates resources in-place or recreates | Advanced use (preview feature) |

## **VPA Best Practices for GKE Autopilot**

✅ **Use `Auto` mode** for ingress controllers like Caddy to ensure proper resource scaling

✅ **Set appropriate `minAllowed`** to prevent resource starvation

✅ **Set reasonable `maxAllowed`** to control costs

✅ **Monitor VPA recommendations** before enabling automatic updates

✅ **Combine with manual resource limits** for fine-grained control

---

# **6️⃣ Advanced Configuration: Kubernetes CloudExtras**

Beyond VPA, Simple Container supports comprehensive Kubernetes configuration through `cloudExtras`. This section covers all available options for fine-tuning your GKE Autopilot deployments.

## **Complete CloudExtras Reference**

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
      
      # Comprehensive Kubernetes configuration
      cloudExtras:
        # Node selection and placement
        nodeSelector:
          workload-group: "high-memory"
          environment: "staging"
          
        # Pod disruption budget for high availability
        disruptionBudget:
          minAvailable: 2          # Keep at least 2 pods running
          # maxUnavailable: 1      # Alternative: max 1 pod down
          
        # Rolling update strategy
        rollingUpdate:
          maxSurge: 2              # Add up to 2 extra pods during update
          maxUnavailable: 1        # Max 1 pod unavailable during update
          
        # Pod affinity and anti-affinity rules
        affinity:
          nodePool: "high-memory-pool"     # Target specific node pool
          exclusiveNodePool: true          # Only run on this node pool
          computeClass: "Performance"      # GKE compute class
          
        # Pod tolerations for tainted nodes
        tolerations:
          - key: "workload-group"
            operator: "Equal"
            value: "high-memory"
            effect: "NoSchedule"
          - key: "environment"
            operator: "Equal"
            value: "staging"
            effect: "NoExecute"
            
        # Vertical Pod Autoscaler (covered in detail above)
        vpa:
          enabled: true
          updateMode: "Auto"
          minAllowed:
            cpu: "100m"
            memory: "128Mi"
          maxAllowed:
            cpu: "4"
            memory: "8Gi"
          controlledResources: ["cpu", "memory"]
          
        # Global readiness probe configuration
        readinessProbe:
          httpGet:
            path: "/health"
            port: 8080
          initialDelaySeconds: 10
          timeoutSeconds: 5
          periodSeconds: 15
          failureThreshold: 5
          successThreshold: 1
          
        # Global liveness probe configuration
        livenessProbe:
          httpGet:
            path: "/health"
            port: 8080
          initialDelaySeconds: 30
          timeoutSeconds: 10
          periodSeconds: 30
          failureThreshold: 3
```

## **CloudExtras Field Reference**

| Field              | Type                | Description                    | GKE Autopilot Support      |
|--------------------|---------------------|--------------------------------|----------------------------|
| `nodeSelector`     | `map[string]string` | Node selection labels          | ✅ Custom labels supported  |
| `disruptionBudget` | `object`            | Pod disruption budget for HA   | ✅ Full support             |
| `rollingUpdate`    | `object`            | Rolling update strategy        | ✅ Full support             |
| `affinity`         | `object`            | Pod affinity and anti-affinity | ✅ With workload separation |
| `tolerations`      | `[]object`          | Pod tolerations for taints     | ✅ Custom tolerations       |
| `vpa`              | `object`            | Vertical Pod Autoscaler        | ✅ Native GKE support       |
| `readinessProbe`   | `object`            | Global readiness probe         | ✅ Full support             |
| `livenessProbe`    | `object`            | Global liveness probe          | ✅ Full support             |

## **Node Selection and Workload Separation**

GKE Autopilot supports custom node selection for workload separation:

```yaml
cloudExtras:
  # Custom node selector labels
  nodeSelector:
    workload-group: "compute-intensive"
    cost-optimization: "spot-instances"
    
  # Affinity rules for advanced placement
  affinity:
    nodePool: "compute-pool"           # Target specific node pool
    exclusiveNodePool: true            # Exclusive placement
    computeClass: "Performance"        # GKE compute class
    
  # Tolerations for custom taints
  tolerations:
    - key: "workload-group"
      operator: "Equal"
      value: "compute-intensive"
      effect: "NoSchedule"
```

**How it works:**
- GKE Autopilot automatically creates nodes with your custom labels
- Pods are scheduled only on nodes matching the `nodeSelector`
- Tolerations allow pods to run on tainted nodes

## **High Availability Configuration**

Configure pod disruption budgets and rolling updates for production workloads:

```yaml
cloudExtras:
  # Ensure minimum availability during disruptions
  disruptionBudget:
    minAvailable: 3              # Keep at least 3 pods running
    # maxUnavailable: 1          # Alternative: max 1 pod down
    
  # Control rolling update behavior
  rollingUpdate:
    maxSurge: "50%"              # Add 50% more pods during update
    maxUnavailable: "25%"        # Max 25% pods unavailable
```

## **Health Probe Configuration**

Configure global health probes for all containers:

```yaml
cloudExtras:
  # Readiness probe - when pod is ready to receive traffic
  readinessProbe:
    httpGet:
      path: "/api/ready"
      port: 8080
    initialDelaySeconds: 15      # Wait 15s before first check
    timeoutSeconds: 5            # 5s timeout per check
    periodSeconds: 10            # Check every 10s
    failureThreshold: 3          # 3 failures = not ready
    successThreshold: 1          # 1 success = ready
    
  # Liveness probe - when to restart pod
  livenessProbe:
    httpGet:
      path: "/api/health"
      port: 8080
    initialDelaySeconds: 60      # Wait 60s before first check
    timeoutSeconds: 10           # 10s timeout per check
    periodSeconds: 30            # Check every 30s
    failureThreshold: 3          # 3 failures = restart pod
```

### **Probe Types**

```yaml
# HTTP probe (most common)
readinessProbe:
  httpGet:
    path: "/health"
    port: 8080
    
# TCP probe (for non-HTTP services)
livenessProbe:
  tcpSocket:
    port: 5432
    
# Command probe (custom health check)
readinessProbe:
  exec:
    command:
      - "/bin/sh"
      - "-c"
      - "pg_isready -U postgres"
```

## **Environment-Specific Configuration**

Different environments can have different CloudExtras configurations:

```yaml
# Production - High availability focus
stacks:
  production:
    config:
      cloudExtras:
        disruptionBudget:
          minAvailable: 3
        vpa:
          updateMode: "Auto"
        affinity:
          exclusiveNodePool: true
          
# Staging - Cost optimization focus  
  staging:
    config:
      cloudExtras:
        disruptionBudget:
          minAvailable: 1
        vpa:
          updateMode: "Initial"
        nodeSelector:
          cost-optimization: "spot"
```

---

# **7️⃣ Summary**
| Step                | Command                             | Purpose                                 |
|---------------------|-------------------------------------|-----------------------------------------|
| **Define Secrets**  | `secrets.yaml`                      | Stores GCP credentials                  |
| **Configure Infra** | `server.yaml`                       | Defines GKE Autopilot & GCP resources   |
| **Provision Infra** | `sc provision -s devops`            | Deploys GCP infrastructure              |
| **Define Service**  | `client.yaml`                       | Describes a microservice deployment     |
| **Deploy Service**  | `sc deploy -s myservice -e staging` | Deploys a microservice to GKE Autopilot |
