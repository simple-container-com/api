---
title: Kubernetes
description: This guide is for DevOps teams who want to configure a parent stack for deploying infrastructure to a self-managed Kubernetes cluster
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Guide: Configuring a Parent Stack for Deploying Infrastructure to a Kubernetes Cluster with Simple Container**

This guide is for **DevOps teams** who want to configure a **parent stack (`server.yaml`)** for deploying infrastructure
**to a self-managed Kubernetes cluster** using **Simple Container**.

With this setup, developers can deploy microservices without needing to manage the Kubernetes infrastructure themselves.

---

# **1️⃣ Prerequisites**

Before configuring the parent stack, ensure that:

✅ A **Kubernetes cluster** is running and accessible.
✅ You have a **`kubeconfig` file** for authentication.
✅ Simple Container is installed:

   ```sh
   curl -s "https://dist.simple-container.com/sc.sh" | bash
   ```

---

# **2️⃣ Setting Up Secrets for Kubernetes Cluster Authentication**

In **self-managed Kubernetes clusters**, `sc` needs a `kubeconfig` file for authentication.

## **Step 1: Define `secrets.yaml`**

Create the **`.sc/stacks/devops/secrets.yaml`** file to store Kubernetes credentials:

```yaml
---
# File: "myproject/.sc/stacks/devops/secrets.yaml"
schemaVersion: 1.0

auth:
  kubernetes:
    type: kubernetes  # Authentication provider type
    config:
      kubeconfig: |-
        ---
        apiVersion: v1
        clusters:
        - cluster:
            insecure-skip-tls-verify: true
            server: https://1.2.3.4:6443  # Kubernetes API server endpoint
          name: my-k8s-cluster
        contexts:
        - context:
            cluster: my-k8s-cluster
            user: admin@my-k8s-cluster
          name: my-k8s-cluster
        current-context: my-k8s-cluster
        kind: Config
        preferences: {}
        users:
        - name: admin@my-k8s-cluster
          user:
            client-certificate-data: LS0tLS1CRUdtLS0tRU5EIENFUlRJRklDQVRFLS0tLS0KAASJD...
            client-key-data: LS0zJlYTlhaEZ3PT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo=

values:
  docker-registry-username: user
  docker-registry-password: password
  pass-phrase: some-secret-passphrase
```

### **🔹 What This Does**

✅ Stores **Kubernetes authentication (`kubeconfig`)**.
✅ Saves **Docker registry credentials** for pulling images.

---

# **3️⃣ Configuring Infra Provisioning (`server.yaml`)**

Now, define **`.sc/stacks/devops/server.yaml`** to provision infrastructure inside Kubernetes.

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
      type: fs  # Store state locally (alternative: S3, GCS)
      config:
        path: file:///${user:home}/.sc/pulumi/state
    secrets-provider:
      type: passphrase
      config:
        passPhrase: "${secret:pass-phrase}"

# Deployment templates for Kubernetes workloads
templates:
  stack-per-app-k8s:
    type: kubernetes-cloudrun  # Deployment type for services
    config:
      kubeconfig: "${auth:kubernetes}"
      dockerRegistryURL: docker.myproject.com
      dockerRegistryUsername: "${secret:docker-registry-username}"
      dockerRegistryPassword: "${secret:docker-registry-password}"
      caddyResource: caddy  # Routing for services
      useSSL: false  # Disable SSL (can be enabled later)

# Infrastructure resources provisioned inside Kubernetes
resources:
  registrar:
    inherit: common  # No DNS management required

  resources:
    production:
      template: stack-per-app-k8s
      resources:
        caddy:
          type: kubernetes-caddy
          config:
            kubeconfig: "${auth:kubernetes}"
            enable: true
            namespace: caddy
            image: docker.io/simplecontainer/caddy:latest
            replicas: 2
            usePrefixes: true  # Routes services using `/service1`, `/service2`
            serviceType: ClusterIP  # Internal routing
            provisionIngress: true
            useSSL: false
        postgres:
          type: kubernetes-helm-postgres-operator
          config:
            kubeconfig: "${auth:kubernetes}"
        rabbitmq:
          type: kubernetes-helm-rabbitmq-operator
          config:
            kubeconfig: "${auth:kubernetes}"
        redis:
          type: kubernetes-helm-redis-operator
          config:
            kubeconfig: "${auth:kubernetes}"
        mongodb:
          type: kubernetes-helm-mongodb-operator
          config:
            kubeconfig: "${auth:kubernetes}"
```

### **🔹 What This Does**

✅ **Configures Pulumi for state management** (`fs` for local storage).
✅ **Defines deployment templates** (`kubernetes-cloudrun`).
✅ **Provisions Kubernetes resources**:

- **Caddy** → Handles ingress and routing.
- **PostgreSQL, RabbitMQ, Redis, MongoDB** → Deployed using **Helm operators**.

---

# **4️⃣ Provisioning the Kubernetes Parent Stack**

Once `server.yaml` is configured, **provision** the Kubernetes infrastructure:

```sh
sc provision -s devops
```

### **What This Does**

✅ Connects to **Kubernetes using `kubeconfig`**.
✅ Deploys **Caddy, PostgreSQL, RabbitMQ, Redis, MongoDB** inside Kubernetes.
✅ Configures **persistent storage and networking**.

---

# **5️⃣ Deploying Microservices to Kubernetes**

Once the infrastructure is provisioned, developers can deploy their microservices.

## **Step 1: Define `client.yaml` for a Microservice**

```yaml
---
# File: "myproject/.sc/stacks/myservice/client.yaml"

schemaVersion: 1.0

stacks:
  production:
    type: cloud-compose
    parent: myproject/devops
    config:
      domain: ${env:MY_SERVICE_DOMAIN}
      prefix: ${env:MY_SERVICE_PREFIX}
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - postgres
      runs:
        - myservice
      env:
        DATABASE_HOST: "${resource:postgres.host}"
        DATABASE_NAME: "${resource:postgres.database}"
        DATABASE_USER: "${resource:postgres.user}"
      secrets:
        DATABASE_PASSWORD: "${resource:postgres.password}"
```

## **Step 2: Deploy the Service**

```sh
sc deploy -s myservice -e production
```

✅ The service is **automatically deployed to Kubernetes** using the defined settings.

---

# **6️⃣ Summary**

| Step                | Command                                | Purpose                                             |
|---------------------|----------------------------------------|-----------------------------------------------------|
| **Define Secrets**  | `secrets.yaml`                         | Stores Kubernetes credentials (`kubeconfig`)        |
| **Configure Infra** | `server.yaml`                          | Defines Kubernetes resources (DBs, queues, ingress) |
| **Provision Infra** | `sc provision -s devops`               | Deploys Kubernetes resources                        |
| **Define Service**  | `client.yaml`                          | Describes a microservice deployment                 |
| **Deploy Service**  | `sc deploy -s myservice -e production` | Deploys a microservice to Kubernetes                |
