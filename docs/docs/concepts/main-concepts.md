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

## **Architecture Overview**

```mermaid
graph TB
    subgraph PS ["🏗️ PARENT STACK (DevOps Owned)"]
        SY["📄 server.yaml<br/>resources:<br/>  production:<br/>    postgres-db: ...<br/>    redis-cache: ...<br/>    s3-storage: ...<br/>provisioner: ...<br/>templates: ..."]
        IR["Infrastructure Resources<br/>• Databases (RDS, MongoDB)<br/>• Storage (S3, GCS)<br/>• Clusters (EKS, GKE)<br/>• Networking, Security"]
        PSN["👥 Managed by: DevOps Team<br/>🔄 Updated: Infrastructure changes"]
    end
    
    subgraph SS ["🚀 SERVICE STACK (Developer Owned)"]
        CY["📄 client.yaml<br/>parent: org/infrastructure<br/>config:<br/>  uses: [postgres-db, redis]<br/>  runs: [web-app]<br/>  env:<br/>    DB_URL: \${resource:...}"]
        DC["📄 docker-compose.yaml<br/>services:<br/>  web-app: ...<br/>  worker: ..."]
        AS["Application Services<br/>• Microservices<br/>• Web Applications<br/>• Background Jobs<br/>• APIs & Services"]
        SSN["👩‍💻 Managed by: Developers<br/>🔄 Updated: New services/features"]
    end
    
    subgraph DF ["📋 DEPLOYMENT FLOW"]
        S1["1️⃣ DevOps defines<br/>infrastructure<br/>(server.yaml)"]
        S2["2️⃣ Developers define<br/>services<br/>(client.yaml)"]
        S3["3️⃣ Simple Container<br/>orchestrates<br/>deployment"]
        
        S1 --> S2
        S2 --> S3
    end
    
    PS --> SS
    SS -.-> PS
    
    classDef parentStack fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef serviceStack fill:#e8f5e8,stroke:#2e7d32,stroke-width:2px
    classDef deployFlow fill:#fff8e1,stroke:#f57f17,stroke-width:2px
    
    class PS,SY,IR,PSN parentStack
    class SS,CY,DC,AS,SSN serviceStack
    class DF,S1,S2,S3 deployFlow
```

**📈 SCALING BENEFITS:**

- 🚀 **500x faster** customer onboarding (5 min vs 2-3 days)
- 📊 **90% configuration reduction** (500 vs 5000+ lines)
- 👥 **5x operational efficiency** (1 DevOps per 100+ customers)
- 💰 **70% cost reduction** through resource sharing
- ⚡ **Zero-downtime** service deployments

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

- **Cloud infrastructure** (Kubernetes clusters, AWS ECS clusters, databases, storage, networking)
- **Secrets management** (via Kubernetes Secrets, AWS Secrets Manager, or Google Secret Manager)
- **Centralized state management** (so infrastructure is consistent across environments)
- **Provisioning of shared resources** (databases, message queues, API gateways)

### **Who Manages the Parent Stack?**
➡️ **DevOps teams** define and maintain the parent stack.

### **When is the Parent Stack Modified?**
➡️ Only when **adding new infrastructure resources** (e.g., a new database, message queue, or cloud provider).

---

# **2️⃣ What is the Service Stack?**

The **service stack** represents an **individual microservice** that a **developer wants to deploy**. It consumes infrastructure from the **parent stack** but does not modify it.

- **Developers only configure their microservice's deployment settings**
- **Microservices automatically connect to infrastructure provisioned by the parent stack**
- **No need to request DevOps intervention for every new service**

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

# **4️⃣ Why This Separation Matters: Scaling Advantages**

## **Multi-Dimensional Resource Allocation**

**Traditional Approach - Manual Resource Management:**
```yaml
# Each customer needs separate infrastructure definition
resource "aws_ecs_cluster" "customer_a" {
  name = "customer-a-cluster"
}

resource "aws_rds_instance" "customer_a_db" {
  identifier = "customer-a-database"
  engine     = "postgres"
  # ... complex configuration
}
```

**Simple Container - Flexible Resource Sharing:**
```yaml
# server.yaml - Define resource pools once
resources:
  production:
    resources:
      # Shared resources for standard customers
      mongodb-shared-us:
        type: mongodb-atlas
        config:
          clusterName: shared-us
          instanceSize: M30
          
      # Dedicated resources for enterprise
      mongodb-enterprise-1:
        type: mongodb-atlas
        config:
          clusterName: enterprise-1
          instanceSize: M80

# client.yaml - Customers choose resources flexibly
stacks:
  standard-customer-1:
    uses: [mongodb-shared-us]  # Shared resource
    
  enterprise:
    uses: [mongodb-enterprise-1]  # Dedicated resource
```

## **Core Scaling Benefits:**

- **Developers focus on coding, not cloud infrastructure** - **15 minutes** to first deployment vs **2-3 days**
- **DevOps standardizes infrastructure without worrying about microservices** - **Template updates apply to all customers**
- **Adding a new microservice is self-service** - **5 minutes** vs **1-2 days** DevOps bottleneck
- **Security is maintained by isolating infrastructure from microservices** - **Automatic namespace isolation**
- **Resource Pool Management** - Define resources once, allocate flexibly
- **Cost Optimization** - Share resources among compatible customers
- **Easy Migration** - Move customers between resource pools by changing `uses` directive

This separation **scales exceptionally well** as organizations grow, preventing bottlenecks where **DevOps must manually configure every microservice**.

---

# **5️⃣ Quantified Scaling Benefits**

## **Operational Scalability Metrics**

| Metric                        | Traditional Approach          | Simple Container            | Improvement        |
|-------------------------------|-------------------------------|-----------------------------|--------------------|
| **DevOps to Customer Ratio**  | 1:10-20 customers             | 1:100+ customers            | **5x efficiency**  |
| **Customer Onboarding Time**  | 2-3 days                      | 5 minutes                   | **500x faster**    |
| **Configuration Lines**       | 5000+ lines for 100 customers | 500 lines for 100 customers | **90% reduction**  |
| **Infrastructure Drift Risk** | High (manual management)      | Low (template-based)        | **Reduced errors** |

## **Development Velocity Impact**

**Traditional Approach:**

- **Time to First Deployment**: 2-3 days (infrastructure setup)
- **Developer Onboarding**: 2-4 weeks (Kubernetes/AWS training)
- **Feature Development**: Blocked by infrastructure changes

**Simple Container:**

- **Time to First Deployment**: 15 minutes (configuration only)
- **Developer Onboarding**: 1-2 hours (simple YAML configuration)
- **Feature Development**: Independent of infrastructure

## **Cost Optimization Results**

**Simple Container achieves:**

- **70% cost reduction** through intelligent resource sharing
- **80% staff reduction** in operational overhead
- **1 DevOps engineer per 100+ customers** vs 1 per 10-20 traditional
- **Automatic right-sizing** and scaling optimization

---

# **6️⃣ Real-World Scaling Scenarios**

## **Scenario 1: Adding 100 New Customers**

**Traditional Kubernetes/ECS:**
```bash
# For each of 100 customers, DevOps must:
1. Create namespace/cluster
2. Define deployment YAML (50+ lines each)
3. Configure ingress and SSL certificates
4. Set up monitoring and logging
5. Create secrets manually

# Result: 5000+ lines of configuration
# Time: 2-3 days per customer = 200-300 days
```

**Simple Container:**
```yaml
# For each of 100 customers, developers add:
customer-001:
  parentEnv: production
  config:
    domain: customer001.myapp.com
    secrets:
      CUSTOMER_SETTINGS: ${env:CUSTOMER_001_SETTINGS}

# Result: 5 lines per customer = 500 lines total
# Time: 5 minutes per customer = 8.3 hours total
```

## **Scenario 2: Performance Tier Migration**

**Traditional Approach:**

- Manual infrastructure rebuild
- Data migration downtime
- Complex rollback procedures
- High risk of errors

**Simple Container:**
```yaml
# Before: Customer on shared resources
customer-enterprise:
  uses: [mongodb-shared-us]
  
# After: Customer on dedicated resources (one line change!)
customer-enterprise:
  uses: [mongodb-enterprise-dedicated]
  
# Automatic migration, zero downtime, easy rollback
```

---

# **Conclusion**

The **separation of parent stack and service stack** in Simple Container ensures:

- **500x faster customer onboarding** (5 minutes vs 2-3 days)
- **90% reduction in configuration complexity** (500 vs 5000+ lines)
- **5x operational efficiency** (1 DevOps per 100+ vs 10-20 customers)
- **70% cost reduction** through intelligent resource sharing
- **Zero downtime migrations** with one-line configuration changes
- **Developer self-service** without infrastructure expertise requirements

By adopting this separation, organizations can **scale from startup to enterprise without operational complexity growth**, transforming container orchestration from a complex infrastructure challenge into a simple configuration management task.