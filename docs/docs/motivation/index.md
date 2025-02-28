---
title: Motivation
description: Description of why Simple Container is a useful tool for development of services
platform: platform
product: simple-container
category: devguide
subcategory: learning
guides: tutorials
date: '2024-06-12'
---

# **Why Simple Container?**

In today's fast-paced software development environment, organizations must deploy and manage **dozens or even hundreds of microservices** efficiently. Traditional **CI/CD pipelines** often require **significant DevOps effort**, complex infrastructure provisioning, and manual management of microservices. **Simple Container** offers a **game-changing approach** that simplifies deployment, automates infrastructure management, and accelerates software delivery.

This page explores why organizations should adopt **`sc`** for their **CI/CD pipelines**, highlighting its **ease of use, automation capabilities, cloud-agnostic nature, and security features**.

As organizations embrace **microservices architectures**, they face a fundamental challenge: **how to efficiently manage, deploy, and scale multiple services without creating complexity for developers and DevOps teams**. Traditional methods of provisioning and deploying microservices often involve **manual infrastructure management, complex CI/CD pipelines, and extensive collaboration between developers and DevOps**.

Simple Container is a **powerful yet lightweight tool** designed to streamline **microservices deployment and delivery**. It provides a **developer-friendly approach** while keeping **infrastructure management centralized and isolated from service deployment**. By bridging the gap between **DevOps and development teams**, `sc` enables organizations to **move faster, reduce bottlenecks, and simplify their microservices workflow**.

For the detailed use-cases of what you can do with Simple Container, please read [Howto](/howto/use-cases/) section.

---

## **1️⃣ The Challenges of Microservices Deployment**

Microservices offer **flexibility, scalability, and modularity**, but **deploying and maintaining them is complex**. In a traditional setup:

- **Developers must coordinate with DevOps** teams to configure cloud resources (databases, networking, secrets, storage).
- **Infrastructure provisioning requires expertise in Infrastructure as Code (IaC) tools** like Terraform or Pulumi.
- **Deployments are tightly coupled with CI/CD pipelines**, requiring manual configuration for every new service.
- **Secrets management and environment-specific configurations require additional setup**, often leading to security risks.

As organizations scale, these challenges become **major bottlenecks**, slowing down **innovation and delivery cycles**.

---

## **2️⃣ What is Simple Container?**

Simple Container is a **cloud-agnostic deployment tool** designed specifically for **microservices**. It provides:

✅ **A unified way to deploy services without DevOps intervention.**

✅ **A declarative configuration approach using simple YAML files.**

✅ **Built-in secrets management and cloud resource integration.**

✅ **Seamless support for Kubernetes, AWS ECS Fargate, and Google Cloud Run.**

At its core, `sc` allows **developers to focus on coding**, while **DevOps teams manage infrastructure separately**. This **clear separation of concerns** eliminates the need for developers to understand cloud provisioning and allows **DevOps to standardize infrastructure across all services**.

---

## **3️⃣ Where Simple Container Fits in Microservices Development**

### **🔹 Empowering Developers with Hassle-Free Deployments**
With `sc`, developers no longer need to:

- **Request infrastructure changes from DevOps.**
- **Manually configure networking, storage, or database connections.**
- **Write complex deployment scripts.**

Instead, they can **deploy new microservices seamlessly**, using simple configurations that **automatically integrate with the existing infrastructure**.

This **reduces friction between development and operations**, enabling teams to launch new features **faster and more reliably**.

### **🔹 A Single Source of Truth for Infrastructure**
While developers focus on building services, **DevOps teams retain full control over infrastructure**. In `sc`:

- Cloud resources such as **databases, message queues, storage, and networking** are centrally defined.
- **Secrets and environment-specific configurations are managed securely.**
- **All infrastructure is provisioned via a single, structured configuration, reducing inconsistencies.**

This **eliminates duplication, reduces security risks, and ensures a consistent setup across multiple services**.

### **🔹 Automating CI/CD Without Extra Complexity**
Traditional CI/CD pipelines require:

- **Setting up deployment scripts for every service.**
- **Managing multiple cloud provider integrations.**
- **Manually configuring rollbacks and service scaling.**

With `sc`, these processes are **built-in**. Deployments happen **without manually modifying CI/CD pipelines**, and rollback mechanisms ensure **safer deployments with minimal downtime**.

---

## **4️⃣ Where Simple Container Fits in Microservices Delivery**

### **🔹 Cloud-Agnostic Deployment for Enterprise Flexibility**
Organizations today operate in **multi-cloud and hybrid environments**. Some workloads run **on Kubernetes clusters**, while others leverage **managed services like AWS ECS Fargate or Google Cloud Run**.

Simple Container enables **seamless deployment across all these platforms** without requiring developers to modify service configurations. This **future-proofs deployments**, allowing teams to switch cloud providers **without rewriting infrastructure code**.

### **🔹 Security & Compliance Without Extra Tools**
Security is a **critical concern** in microservices delivery. Managing **secrets and environment-specific credentials** across multiple services is challenging.

`sc` offers **built-in secrets management**, ensuring that credentials are:

- **Stored securely in cloud-native secret managers (AWS Secrets Manager, GCP Secret Manager, Kubernetes Secrets).**
- **Automatically injected into services at runtime.**
- **Never exposed in plaintext or CI/CD logs.**

This **reduces security risks while keeping deployments efficient**.

### **🔹 Scalability & Reliability for Growing Organizations**
As organizations scale, microservices need to:

- **Spin up new instances dynamically based on demand.**
- **Auto-scale without manual intervention.**
- **Ensure high availability and fault tolerance.**

Simple Container integrates **seamlessly with cloud-native scaling solutions**, ensuring that services remain **resilient, highly available, and cost-efficient**.

---

## **5️⃣ Why Organizations Should Adopt `sc`**

### **✅ Faster Microservice Deployment**

- Developers can **self-deploy services** without waiting for DevOps.
- No need for **custom Terraform/Pulumi configurations** for every microservice.

### **✅ Reduced DevOps Overhead**

- DevOps focuses **only on infrastructure**, reducing operational burden.
- Centralized management **eliminates duplication** and ensures consistency.

### **✅ Built-in Security & Secrets Management**

- Secrets are **automatically handled**, reducing the risk of leaks.
- Cloud-native integrations ensure **compliance with security policies**.

### **✅ Cloud-Agnostic & Future-Proof**

- Works with **Kubernetes, AWS, and Google Cloud** seamlessly.
- Migrating workloads **requires no changes to service configurations**.

### **✅ CI/CD Without Complexity**

- **No need for external deployment tools**—CI/CD is built into `sc`.
- **Automated rollbacks** ensure stability in production.

---

## **Conclusion**

Simple Container is a **revolutionary tool** for organizations adopting **microservices architectures**. By **simplifying deployments, reducing operational complexity, and ensuring cloud-agnostic flexibility**, `sc` empowers **both developers and DevOps teams** to work more efficiently.

For any organization scaling microservices, `sc` presents a **compelling alternative** to traditional CI/CD pipelines, offering **automation, security, and ease of use**—all in a **developer-friendly format**.
