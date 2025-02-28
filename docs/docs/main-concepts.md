---
title: Main concepts
description: One of the key principles of Simple Container is the separation of concerns between infrastructure management and microservice deployment.
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Separation of Parent Stack and Service Stack in Simple Container**

## **Introduction**

One of the key principles of **Simple Container** is the **separation of concerns** between **infrastructure management** and **microservice deployment**.
This is achieved by **separating the "parent stack" (managed by DevOps) from the "service stack" (managed by developers)**.

This guide explains:
✅ **What the parent stack is and how it works**
✅ **What the service stack is and how it works**
✅ **How this separation benefits both DevOps and developers**

---

# **1️⃣ What is the Parent Stack?**

The **parent stack** is the **core infrastructure** required for microservices to run. It is **managed by DevOps** and provides:

✅ **Cloud infrastructure** (Kubernetes clusters, AWS ECS clusters, databases, storage, networking).
✅ **Secrets management** (via Kubernetes Secrets, AWS Secrets Manager, or Google Secret Manager).
✅ **Centralized state management** (so infrastructure is consistent across environments).
✅ **Provisioning of shared resources** (databases, message queues, API gateways).

### **Who Manages the Parent Stack?**
➡️ **DevOps teams** define and maintain the parent stack.

### **When is the Parent Stack Modified?**
➡️ Only when **adding new infrastructure resources** (e.g., a new database, message queue, or cloud provider).

---

# **2️⃣ What is the Service Stack?**

The **service stack** represents an **individual microservice** that a **developer wants to deploy**. It consumes infrastructure from the **parent stack** but does not modify it.

✅ **Developers only configure their microservice's deployment settings.**
✅ **Microservices automatically connect to infrastructure provisioned by the parent stack.**
✅ **No need to request DevOps intervention for every new service.**

### **Who Manages the Service Stack?**
➡️ **Developers** define and maintain their own service configurations.

### **When is the Service Stack Modified?**
➡️ Whenever a **new microservice is added** or an **existing service is updated**.

---

# **3️⃣ Key Differences Between Parent Stack and Service Stack**

| Feature                | Parent Stack (DevOps)                           | Service Stack (Developers)          |
|------------------------|-------------------------------------------------|-------------------------------------|
| **Purpose**            | Defines shared infrastructure                   | Defines microservice deployment     |
| **Managed By**         | DevOps                                          | Developers                          |
| **Configuration File** | `server.yaml`                                   | `client.yaml`                       |
| **Modified When**      | Infrastructure changes (new DB, queue, cluster) | New service added or updated        |
| **Includes**           | Databases, secrets, cloud resources             | Microservice dependencies & scaling |

---

# **4️⃣ Why This Separation Matters**

✅ **Developers focus on coding, not cloud infrastructure.**
✅ **DevOps standardizes infrastructure without worrying about microservices.**
✅ **Adding a new microservice is self-service—no need for DevOps approval.**
✅ **Security is maintained by isolating infrastructure from microservices.**

This separation **scales well** as organizations grow, preventing bottlenecks where **DevOps must manually configure every microservice**.

---

# **Conclusion**

The **separation of parent stack and service stack** in `sc` ensures:
✅ **Faster microservice deployment without DevOps bottlenecks**
✅ **A single source of truth for infrastructure managed by DevOps**
✅ **A simple onboarding process for developers, reducing complexity**

By adopting this separation, organizations can **scale their microservices architecture efficiently and securely**.