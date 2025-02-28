---
title: Main use-cases for Simple-Container.com
description: What Simple Container can do for you?
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# Main use-cases for Simple Container

Simple Container is designed to **simplify microservices deployment and infrastructure management** while maintaining cloud flexibility.
This guide outlines the **main use cases** for `sc`, highlighting how it fits into modern software development and DevOps workflows.

## Separation of DevOps and Developer parties

Simple Container allows DevOps of the company to easily set up the basics of the infrastructure using the chosen
cloud provider (be it an AWS or GCP cloud or even a hosted Kubernetes cluster), prepare main secrets and account
configurations ahead of time.
DevOps should simply prepare a basic configuration of their resources and then invoke a single command `sc provision`
which will take care of the initial provision steps, and will create all resources a company needs.

For developers Simple Container provides the high-level abstraction allowing to easily set up their CI/CD pipeline and
deploy their services into the provisioned cloud. With Simple Container adding a new microservice a company needs takes
a couple of very simple steps making the whole process a self-service operation without much help needed from DevOps.


## **1️⃣ Deploying Microservices Without DevOps Involvement**
✅ **Use Case:** Developers need to deploy new microservices **quickly and independently** without requiring DevOps to configure infrastructure.
✅ **Benefit:** **Developers only provide a simple configuration**, while `sc` automatically integrates it with the existing cloud infrastructure.
✅ **Ideal for:** Organizations with many microservices, where **DevOps cannot manually provision every service**.

---

## **2️⃣ Managing Infrastructure Centrally Without Complexity**
✅ **Use Case:** DevOps teams need a **single source of truth** for infrastructure such as **databases, storage, networking, and secrets**.
✅ **Benefit:** Infrastructure is **centrally defined** in `sc`, while developers remain isolated from infrastructure complexities.
✅ **Ideal for:** Organizations using **Terraform/Pulumi but want to simplify microservices provisioning**.

---

## **3️⃣ Deploying Services Across Multiple Cloud Providers**
✅ **Use Case:** Organizations want to **deploy services across AWS, GCP, and Kubernetes** without rewriting cloud-specific configurations.
✅ **Benefit:** `sc` abstracts cloud infrastructure, allowing services to run **seamlessly on different platforms**.
✅ **Ideal for:** Companies that need **multi-cloud or hybrid cloud deployments**.

---

## **4️⃣ Simplifying CI/CD with Built-in Deployment Automation**
✅ **Use Case:** Teams want to **automate deployments** without managing complex CI/CD pipeline scripts.
✅ **Benefit:** `sc` provides **built-in deployment commands**, eliminating the need for **manual Helm charts, Terraform pipelines, or custom scripts**.
✅ **Ideal for:** Teams looking to **reduce deployment complexity and improve release speed**.

---

## **5️⃣ Secure Secrets Management Without External Tools**
✅ **Use Case:** Microservices require **environment-specific credentials (e.g., database passwords, API keys)** stored securely.
✅ **Benefit:** `sc` **automatically stores secrets in cloud-native secret managers** and injects them securely into deployed services.
✅ **Ideal for:** Organizations that previously relied on **manual secret injection or external tools like Vault**.

---

## **6️⃣ Deploying Static Websites with Cloud Storage & CDN**
✅ **Use Case:** Teams need to **host static websites (React, Vue, Angular, documentation sites)** on cloud storage with CDN integration.
✅ **Benefit:** `sc` provisions **S3, GCP Cloud Storage, or other cloud storage solutions**, handling **domain setup and caching**.
✅ **Ideal for:** Companies looking for a **fast and automated static site deployment**.

---

## **7️⃣ Migrating Services Between Cloud Providers Without Rewriting Configurations**
✅ **Use Case:** Organizations need to **move workloads from AWS to GCP, or from Kubernetes to ECS** while minimizing downtime.
✅ **Benefit:** `sc` allows teams to **update infrastructure configurations without modifying microservices deployment settings**.
✅ **Ideal for:** Businesses that need **cloud migration flexibility**.

---

## **8️⃣ Scaling Microservices with Cloud-Native Auto-Scaling**
✅ **Use Case:** Services must **scale dynamically based on demand** without manual intervention.
✅ **Benefit:** `sc` integrates with **Kubernetes auto-scaling, AWS ECS scaling, and serverless scaling mechanisms**, ensuring optimal resource usage.
✅ **Ideal for:** Organizations handling **high-traffic applications** that require **automatic scaling**.

---

## **9️⃣ Managing Microservices Dependencies Without Manual Configuration**
✅ **Use Case:** A service needs **databases, messaging queues, or cloud storage** but shouldn’t require developers to configure them manually.
✅ **Benefit:** `sc` **automatically connects services to provisioned resources** such as databases and queues, reducing developer effort.
✅ **Ideal for:** Teams working with **complex microservice architectures**.

---

## **10️⃣ Standardizing Microservices Deployment Across Teams**
✅ **Use Case:** Different teams deploy services using **various methods (Terraform, Helm, custom scripts), leading to inconsistency**.
✅ **Benefit:** `sc` ensures **consistent deployment standards across all microservices**, improving maintainability.
✅ **Ideal for:** Enterprises with **multiple teams managing microservices independently**.

---

# **Conclusion**

Simple Container **solves key challenges in microservices deployment** by providing:
✅ **Developer autonomy for microservice deployment**
✅ **Centralized and flexible infrastructure management**
✅ **Multi-cloud portability**
✅ **CI/CD simplification with built-in automation**
✅ **Secure secrets handling without external tools**

Organizations adopting `sc` benefit from **faster deployments, reduced DevOps workload, and better scalability**.
