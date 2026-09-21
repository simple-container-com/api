---
title: Deployment types
description: This guide is for developers who want to deploy their services with sc
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---


# **Guide for Developers: Configuring All Deployment Types in Simple Container**

Simple Container supports **three deployment types**:

| Deployment Type     | Use Case                      | Example Platforms         |
|---------------------|-------------------------------|---------------------------|
| **`cloud-compose`** | Multi-container microservices | Kubernetes, ECS Fargate   |
| **`single-image`**  | Single-container applications | AWS Lambda, Cloud Run     |
| **`static`**        | Static websites               | AWS S3, GCP Cloud Storage |

This guide explains how to configure each deployment type using **`client.yaml`**.

---

# **`cloud-compose`: Multi-Container Microservices**

Use **`cloud-compose`** for **microservices that require multiple containers, databases, message queues, or networking**.

Requires a **Dockerfile** and a **`docker-compose.yaml`** file.

Works with **Kubernetes, ECS Fargate, Cloud Run, etc.**

## **Example `client.yaml` for `cloud-compose`**
```yaml
---
# File: "myproject/.sc/stacks/myservice/client.yaml"

schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: myproject/devops
    config:
      domain: staging-myservice.myproject.com
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - mongodb  # Uses a MongoDB database from `server.yaml`
      runs:
        - myservice  # Deploys the "myservice" container
      size:
        cpu: 512  # 0.5 vCPU
        memory: 1024  # 1GB RAM
      scale:
        min: 1
        max: 5
        policy:
          cpu:
            max: 70  # Scale up if CPU usage exceeds 70%
      env:
        DATABASE_HOST: "${resource:mongodb.host}"
        DATABASE_NAME: "${resource:mongodb.database}"
        DATABASE_USER: "${resource:mongodb.user}"
      secrets:
        DATABASE_PASSWORD: "${resource:mongodb.password}"
```

### **Required Files**
- **`Dockerfile`** → Defines how the service is built.
- **`docker-compose.yaml`** → Defines how the service runs.

### **Example `docker-compose.yaml`**
```yaml
version: '3.8'
services:
  myservice:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    environment:
      NODE_ENV: production
      DATABASE_URL: ${DATABASE_HOST}
```

### **Exposing UDP ports (e.g. WebRTC / LiveKit)**

By default a `cloud-compose` service is created as a `ClusterIP` Service and
exposed through the shared Caddy ingress, which reverse-proxies **HTTP/TCP**
traffic only. Protocols that Caddy cannot proxy — most notably **UDP** — must be
exposed directly by turning the service's own Kubernetes Service into a
`LoadBalancer`.

Two things are required:

1. **Declare the UDP port in `docker-compose.yaml`** using the compose
   `/udp` suffix. The port protocol is now preserved end-to-end and emitted on
   both the container port and the Service port. Ports without a suffix stay
   `TCP`, exactly as before.
2. **Set `serviceType: LoadBalancer`** under `cloudExtras`. This makes the
   service's own Service a `LoadBalancer` with its own external IP, instead of
   the default `ClusterIP` fronted by Caddy.

```yaml
# docker-compose.yaml — a LiveKit SFU media server
services:
  livekit:
    image: livekit/livekit-server:latest
    ports:
      - "7880:7880"      # signaling (wss)  → TCP
      - "7881:7881"      # ICE/TCP          → TCP
      - "7882:7882/udp"  # RTC media        → UDP
```

```yaml
# client.yaml
stacks:
  production:
    type: cloud-compose
    parent: myproject/devops
    config:
      dockerComposeFile: ./docker-compose.yaml
      runs:
        - livekit
      cloudExtras:
        serviceType: LoadBalancer
```

**Mixed TCP + UDP on one Service.** When a service exposes both TCP and UDP
ports, `sc` emits a **single mixed-protocol `LoadBalancer` Service** carrying all
ports (rather than splitting into separate TCP and UDP Services). This keeps a
single external IP for the whole service, which is what UDP media servers need
in order to advertise one address to clients. Mixed-protocol Services are GA in
Kubernetes since 1.26 (the `MixedProtocolLBService` feature) and are supported
by recent GKE — your cluster must be on a version that supports them.

> **Announcing the external IP is the application's job.** `sc` provisions the
> `LoadBalancer` and the UDP port, but a media server such as LiveKit must be
> told its public address to put into ICE candidates (e.g. LiveKit's
> `rtc.use_external_ip` / `rtc.node_ip`). Wiring that address into your app's
> configuration is out of scope for `sc`.

### **Deploying to Staging**
```sh
sc deploy -s myservice -e staging
```

---

# **`single-image`: Single-Container Applications**

Use **`single-image`** for **single-container services like AWS Lambda or Cloud Run**.

Only requires a **Dockerfile** (no `docker-compose.yaml` needed).

Supports **cloud-specific configurations** like AWS Lambda settings.

## **Example `client.yaml` for `single-image`**
```yaml
---
# File: "myproject/.sc/stacks/myservice/client.yaml"

schemaVersion: 1.0

stacks:
  staging:
    type: single-image
    template: lambda-eu  # AWS Lambda deployment
    parent: myproject/devops
    config:
      domain: staging-myservice.myproject.com
      image:
        dockerfile: ${git:root}/Dockerfile
      timeout: 180  # AWS Lambda execution timeout
      maxMemory: 2048  # 2GB RAM
      staticEgressIP: true  # Ensures outbound requests use a static IP
      cloudExtras:
        lambdaRoutingType: function-url
        lambdaInvokeMode: RESPONSE_STREAM
      uses:
        - mongodb
      env:
        NODE_ENV: production
      secrets:
        MONGO_URI: "${resource:mongodb.uri}"
```

### **Required Files**
- **`Dockerfile`** → Defines how the service is packaged into a container.

### **Example `Dockerfile`**
```Dockerfile
FROM node:18
WORKDIR /app
COPY . .
RUN npm install
CMD ["node", "server.js"]
```

### **Deploying to AWS Lambda**
```sh
sc deploy -s myservice -e staging
```

---

# **`static`: Deploying Static Websites**

Use **`static`** for **deploying static websites** (e.g., React, Vue, Angular).

Requires a **pre-built directory with static files**.

Supports **public cloud storage, CDN caching, and custom domains**.

## **Example `client.yaml` for `static` Deployment**
```yaml
---
# File: "myproject/.sc/stacks/landing-page/client.yaml"

schemaVersion: 1.0

stacks:
  prod:
    type: static
    parent: myproject/devops
    config:
      bundleDir: ${git:root}/public  # Directory containing built static files
      domain: simple-container.com  # Public domain
      indexDocument: index.html  # Default page served
      errorDocument: index.html  # Handles client-side routing (React, Vue.js)
      location: EUROPE-CENTRAL2
```

### **Required Files**
- **A built directory (`public/`)** → Contains `index.html`, `assets/`, etc.

### **Building a Static Site (Example for React)**
```sh
npm install
npm run build
```
This creates a `public/` directory.

### **Deploying the Static Site**
```sh
sc deploy -s landing-page -e prod
```

---

# **Summary**
| Deployment Type     | Use Case                      | Required Files                      | Example Platforms         |
|---------------------|-------------------------------|-------------------------------------|---------------------------|
| **`cloud-compose`** | Multi-container microservices | `Dockerfile`, `docker-compose.yaml` | Kubernetes, ECS Fargate   |
| **`single-image`**  | Single-container applications | `Dockerfile`                        | AWS Lambda, Cloud Run     |
| **`static`**        | Static websites               | `bundleDir` with HTML/CSS/JS        | AWS S3, GCP Cloud Storage |