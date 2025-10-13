# Kubernetes Affinity Rules Examples

This directory contains comprehensive examples demonstrating Simple Container's **affinity rules support** for Kubernetes CloudRun templates. These examples show how to implement sophisticated pod scheduling and node pool isolation strategies.

## 🎯 **Overview**

Simple Container's affinity rules enable enterprise-grade workload placement strategies through the `cloudExtras.affinity` configuration block. This feature supports:

- **Node Pool Isolation**: Target specific node pools for workload segregation
- **Exclusive Scheduling**: Ensure pods only run on designated node pools  
- **Compute Class Optimization**: Specify performance characteristics
- **Advanced Kubernetes Affinity**: Full node/pod affinity and anti-affinity rules

## 📁 **Examples in This Directory**

### **1. Multi-Tier Node Isolation (`multi-tier-node-isolation/`)**
Real-world example based on enterprise GCP migration requirements:
- **Processing Services**: High-performance node pool isolation
- **Bot Services**: General-purpose node pool for Telegram bots
- **White Label Clients**: Scale-out node pool with cost optimization
- **Multi-tier Architecture**: Complete enterprise deployment pattern

### **2. High Availability Patterns (`high-availability/`)**
Advanced scheduling patterns for production workloads:
- **Zone Anti-Affinity**: Spread pods across availability zones
- **Node Anti-Affinity**: Distribute workloads across nodes
- **Pod Co-location**: Group related services together
- **Disaster Recovery**: Multi-region deployment strategies

### **3. Performance Optimization (`performance-optimization/`)**
Examples focused on performance and resource optimization:
- **CPU-Intensive Workloads**: Dedicated high-CPU node pools
- **Memory-Intensive Services**: High-memory node pool targeting
- **Storage-Optimized**: SSD-backed node pool selection
- **GPU Workloads**: GPU node pool affinity rules

## 🚀 **Quick Start**

### **Basic Node Pool Isolation**
```yaml
stacks:
  my-service:
    type: cloud-compose
    config:
      cloudExtras:
        affinity:
          nodePool: "high-performance"
          exclusiveNodePool: true
          computeClass: "Performance"
```

### **Advanced Affinity Rules**
```yaml
stacks:
  my-service:
    type: cloud-compose
    config:
      cloudExtras:
        affinity:
          nodePool: "processing"
          nodeAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              nodeSelectorTerms:
                - matchExpressions:
                    - key: "cloud.google.com/gke-nodepool"
                      operator: "In"
                      values: ["processing", "backup-processing"]
          podAntiAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              - labelSelector:
                  matchLabels:
                    appName: "my-service"
                topologyKey: "topology.kubernetes.io/zone"
```

## 📋 **Supported Affinity Properties**

### **Simple Container Properties**
- **`nodePool`**: Target node pool name (e.g., "processing", "bots")
- **`exclusiveNodePool`**: Boolean - enforce exclusive scheduling
- **`computeClass`**: Performance class ("Performance", "Scale-Out", "general-purpose")

### **Advanced Kubernetes Properties**
- **`nodeAffinity`**: Node selection rules and preferences
- **`podAffinity`**: Pod co-location rules
- **`podAntiAffinity`**: Pod separation and distribution rules

## 🔧 **Implementation Details**

### **GKE Integration**
Simple Container automatically maps affinity rules to GKE-specific labels:
- `nodePool` → `cloud.google.com/gke-nodepool`
- `computeClass` → `node.kubernetes.io/instance-type`

### **Data Flow**
1. **Configuration** → `cloudExtras.affinity` in client.yaml
2. **Processing** → Simple Container converts to Kubernetes affinity
3. **Deployment** → Applied to pod specifications
4. **Scheduling** → Kubernetes scheduler enforces rules

## 📚 **Use Cases**

### **Enterprise Scenarios**
- **Multi-tenant Applications**: Isolate customer workloads
- **Performance Tiers**: Separate high/low priority services
- **Cost Optimization**: Efficient node pool utilization
- **Compliance**: Regulatory workload separation

### **Technical Patterns**
- **Database Isolation**: Separate data processing workloads
- **Batch Processing**: Dedicated compute resources
- **Web Services**: Load balancer affinity
- **Microservices**: Service mesh optimization

## 🛠 **Prerequisites**

- Simple Container with Kubernetes CloudRun template support
- GKE cluster with multiple node pools (for node pool examples)
- Understanding of Kubernetes affinity concepts

## 🔗 **Related Documentation**

- [Simple Container Kubernetes Guide](../../guides/kubernetes-native/)
- [GKE Autopilot Examples](../gke-autopilot/)
- [Advanced Configurations](../advanced-configs/)
- [Template Placeholders](../../concepts/template-placeholders/)

## 📝 **Contributing**

When adding new affinity examples:
1. Create a dedicated subdirectory
2. Include complete client.yaml and server.yaml files
3. Add comprehensive README with use case explanation
4. Test with actual Kubernetes clusters
5. Document any cloud provider specific requirements

---

**Note**: These examples demonstrate production-ready configurations used in real-world deployments. Adapt the node pool names and compute classes to match your specific infrastructure setup.
