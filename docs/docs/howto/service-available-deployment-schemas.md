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

# **1️⃣ `cloud-compose`: Multi-Container Microservices**
✅ Use **`cloud-compose`** for **microservices that require multiple containers, databases, message queues, or networking**.
✅ Requires a **Dockerfile** and a **`docker-compose.yaml`** file.
✅ Works with **Kubernetes, ECS Fargate, Cloud Run, etc.**

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

### **🔹 Required Files**
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

### **Deploying to Staging**
```sh
sc deploy -s myservice -e staging
```

---

# **2️⃣ `single-image`: Single-Container Applications**
✅ Use **`single-image`** for **single-container services like AWS Lambda or Cloud Run**.
✅ Only requires a **Dockerfile** (no `docker-compose.yaml` needed).
✅ Supports **cloud-specific configurations** like AWS Lambda settings.

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

### **🔹 Required Files**
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

# **3️⃣ `static`: Deploying Static Websites**
✅ Use **`static`** for **deploying static websites** (e.g., React, Vue, Angular).
✅ Requires a **pre-built directory with static files**.
✅ Supports **public cloud storage, CDN caching, and custom domains**.

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

### **🔹 Required Files**
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

# **4️⃣ Summary**
| Deployment Type     | Use Case                      | Required Files                      | Example Platforms         |
|---------------------|-------------------------------|-------------------------------------|---------------------------|
| **`cloud-compose`** | Multi-container microservices | `Dockerfile`, `docker-compose.yaml` | Kubernetes, ECS Fargate   |
| **`single-image`**  | Single-container applications | `Dockerfile`                        | AWS Lambda, Cloud Run     |
| **`static`**        | Static websites               | `bundleDir` with HTML/CSS/JS        | AWS S3, GCP Cloud Storage |
