---
title: Deloying a new microservice
description: This guide is for developers who want to deploy their services with sc to their existing organization
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Guide: Dploying a New Microservice in Organization Using Simple Container**

As a **developer at Organization**, you can **deploy a new service (`billing`)** with **Simple Container** **without requiring DevOps involvement**.

✅ **MongoDB and PostgreSQL are already available** in the organization's infrastructure.

✅ **You only need to define `client.yaml`** and use familiar tools like **Dockerfile & docker-compose**.

✅ **Deployment is automated** using `sc deploy`.

---

# **1️⃣ Steps to Deploy the `billing` Service**
Follow these steps to deploy the **`billing`** service:

### **🔹 Step 1: Create the Service Directory**
```sh
mkdir -p .sc/stacks/billing
```

---

### **🔹 Step 2: Define `client.yaml`**
Create and edit **`.sc/stacks/billing/client.yaml`**:
```yaml
---
# File: ".sc/stacks/billing/client.yaml"

schemaVersion: 1.0

stacks:
  staging:
    type: cloud-compose
    parent: simple-container/devops  # Reference to the organization's infrastructure
    config:
      dockerComposeFile: ./docker-compose.yaml
      uses:
        - mongodb  # Use the existing MongoDB instance
        - postgres  # Use the existing PostgreSQL instance
      runs:
        - billing  # Name of the service in docker-compose.yaml
      env:
        DATABASE_TYPE: "postgres"
        MONGO_URI: "${resource:mongodb.uri}"  # Inject MongoDB connection string
        POSTGRES_HOST: "${resource:postgres.host}"
        POSTGRES_DB: "${resource:postgres.database}"
        POSTGRES_USER: "${resource:postgres.user}"
      secrets:
        POSTGRES_PASSWORD: "${resource:postgres.password}"  # Securely inject PostgreSQL password
```

✅ **This defines how `billing` connects to existing infrastructure**.

✅ **No changes required from DevOps** since resources (`mongodb`, `postgres`) are already available.

---

### **🔹 Step 3: Define `docker-compose.yaml`**
Create a **Docker Compose file** for running the service locally **and deploying it**.

```yaml
version: "3.8"

services:
  billing:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    environment:
      DATABASE_TYPE: postgres
      MONGO_URI: ${MONGO_URI}
      POSTGRES_HOST: ${POSTGRES_HOST}
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
```

✅ **Ensures the service runs identically in local and cloud environments**.

---

### **🔹 Step 4: Deploy the Service**
Run the following command to deploy **`billing`** to **staging**:
```sh
sc deploy -s billing -e staging
```

To deploy to **production**, use:
```sh
sc deploy -s billing -e production
```

✅ **SC automatically builds, pushes, and deploys the service to Organization's cloud infrastructure**.

✅ **Secrets (e.g., `POSTGRES_PASSWORD`) are securely injected**.

---

### **🔹 Step 5: Verify Deployment**
Verify your service deployment:

- **Check your cloud provider console** (AWS ECS, GKE, etc.) to confirm the service is running
- **Visit your service URL** to test functionality
- **Check cloud provider logs** (CloudWatch for AWS, Cloud Logging for GCP) for any issues
- **Monitor resource usage** through your cloud provider's monitoring tools

✅ **You can now monitor and debug your service using your cloud provider's native tools**.

---

# **2️⃣ Summary**
| Step                             | Command                               | Purpose                             |
|----------------------------------|---------------------------------------|-------------------------------------|
| **Create the service directory** | `mkdir -p .sc/stacks/billing`         | Sets up `billing` service stack     |
| **Define `client.yaml`**         | Edit `.sc/stacks/billing/client.yaml` | Configures service deployment       |
| **Define `docker-compose.yaml`** | Edit `docker-compose.yaml`            | Ensures local and cloud consistency |
| **Deploy to Staging**            | `sc deploy -s billing -e staging`     | Deploys the new service             |
| **Deploy to Production**         | `sc deploy -s billing -e production`  | Deploys to production               |
| **Verify Deployment**            | Check cloud provider console          | Monitors service health             |
| **View Logs**                    | Use cloud provider logging tools      | Debugs issues                       |
