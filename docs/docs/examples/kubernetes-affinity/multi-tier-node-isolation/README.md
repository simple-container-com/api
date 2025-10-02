# Multi-Tier Node Pool Isolation Example

This example demonstrates **node pool isolation** using Simple Container's affinity rules, based on real-world enterprise migration requirements. It shows how to implement multi-tier architecture with dedicated node pools for different service types.

## 🎯 **Use Case**

A fintech company needed to migrate from dedicated servers to GCP with cost optimization and performance isolation:

- **Processing Services**: High-performance node pool for Django/Celery workloads
- **Bot Services**: General-purpose node pool for Telegram bots  
- **White Label Clients**: Scale-out node pool for cost-effective client isolation
- **Shared Resources**: Cloud SQL PostgreSQL and Redis Memorystore

## 💰 **Cost Benefits**

- **82% cost reduction** in Phase 1 ($485/month savings)
- **Pod-based billing** with GKE Autopilot
- **Efficient resource utilization** through node pool isolation
- **White Label scaling**: $3-15/month per client vs $200/month previously

## 🏗 **Architecture Overview**

```
GKE Autopilot Cluster
├── Processing Node Pool (Performance)
│   ├── Django API (2-8 replicas)
│   └── Celery Workers (2-8 replicas)
├── Bots Node Pool (General-Purpose)  
│   ├── Telegram Bot (1-6 replicas)
│   └── Support Bot (1-6 replicas)
├── White Label Node Pool (Scale-Out)
│   ├── Client A Bot (0-3 replicas)
│   ├── Client B Bot (0-3 replicas)
│   └── ... (per client isolation)
└── Shared Resources
    ├── Cloud SQL PostgreSQL
    └── Redis Memorystore
```

## 📁 **Files in This Example**

- **`server.yaml`** - Parent stack with GKE cluster and shared resources
- **`client.yaml`** - Service stacks with affinity rules
- **`secrets.yaml`** - Authentication configuration
- **`docker-compose.yaml`** - Application containers

## 🚀 **Key Features**

- **Node Pool Isolation**: Each service type runs on dedicated node pools
- **Exclusive Scheduling**: `exclusiveNodePool: true` prevents cross-contamination
- **Compute Class Optimization**: Performance, general-purpose, and scale-out classes
- **Auto-scaling**: HPA configuration with min/max replicas
- **Cost Optimization**: Efficient resource allocation per workload type

## 🔧 **Affinity Rules Explained**

### **Processing Services**
```yaml
cloudExtras:
  affinity:
    nodePool: "processing"
    exclusiveNodePool: true
    computeClass: "Performance"
```
- Runs only on high-performance nodes
- Isolated from other workloads
- Optimized for CPU/memory intensive tasks

### **Bot Services**
```yaml
cloudExtras:
  affinity:
    nodePool: "bots"
    exclusiveNodePool: true
    computeClass: "general-purpose"
```
- Balanced CPU/memory allocation
- Separate from processing workloads
- Cost-effective for I/O bound tasks

### **White Label Clients**
```yaml
cloudExtras:
  affinity:
    nodePool: "whitelabel"
    exclusiveNodePool: true
    computeClass: "Scale-Out"
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                appType: "simple-container"
            topologyKey: "kubernetes.io/hostname"
```
- Cost-optimized node pool
- Anti-affinity spreads clients across nodes
- Prevents single points of failure

## 📊 **Scaling Configuration**

| Service Type | Min Replicas | Max Replicas | Node Pool | Compute Class |
|-------------|-------------|-------------|-----------|---------------|
| Processing API | 2 | 8 | processing | Performance |
| Celery Workers | 2 | 8 | processing | Performance |
| Telegram Bot | 1 | 6 | bots | general-purpose |
| Support Bot | 1 | 6 | bots | general-purpose |
| White Label (per client) | 0 | 3 | whitelabel | Scale-Out |

## 🛠 **Prerequisites**

### **GKE Cluster Setup**
```bash
# Create GKE Autopilot cluster with multiple node pools
gcloud container clusters create spacepay-cluster \
  --enable-autoscaling \
  --enable-autopilot \
  --region=us-central1
```

### **Node Pool Configuration**
The example assumes these node pools exist:
- **processing**: High-CPU/memory nodes (e.g., n1-highmem-4)
- **bots**: Balanced nodes (e.g., n1-standard-2)  
- **whitelabel**: Cost-optimized nodes (e.g., e2-small)

## 📋 **Deployment Steps**

### **1. Deploy Parent Stack**
```bash
# Deploy infrastructure (GKE cluster + shared resources)
sc deploy --stack infrastructure --env production
```

### **2. Configure Secrets**
```bash
# Add GCP credentials
sc secrets add gcp-credentials --file service-account.json

# Add database credentials  
sc secrets add postgres-password --value "secure-password"
```

### **3. Deploy Services**
```bash
# Deploy processing services
sc deploy --stack processing --env production

# Deploy bot services
sc deploy --stack telegram-bots --env production

# Deploy white label clients
sc deploy --stack whitelabel-client-a --env production
```

## 🔍 **Monitoring & Verification**

### **Verify Node Pool Assignment**
```bash
# Check pod placement
kubectl get pods -o wide --all-namespaces

# Verify node pool labels
kubectl get nodes --show-labels | grep gke-nodepool
```

### **Monitor Resource Usage**
```bash
# Check HPA status
kubectl get hpa --all-namespaces

# Monitor node utilization
kubectl top nodes
```

## 🎛 **Customization Options**

### **Adjust Node Pool Names**
Update the `nodePool` values to match your cluster:
```yaml
cloudExtras:
  affinity:
    nodePool: "your-custom-pool-name"
```

### **Modify Compute Classes**
Change `computeClass` based on your node types:
```yaml
cloudExtras:
  affinity:
    computeClass: "n1-standard-4"  # Use actual instance type
```

### **Add Advanced Affinity Rules**
Extend with custom Kubernetes affinity:
```yaml
cloudExtras:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: "custom-label"
                operator: "In"
                values: ["custom-value"]
```

## 🚨 **Troubleshooting**

### **Pods Stuck in Pending**
- Check node pool capacity: `kubectl describe nodes`
- Verify node pool labels match affinity rules
- Ensure cluster autoscaling is enabled

### **Affinity Rules Not Applied**
- Validate YAML syntax in `cloudExtras.affinity`
- Check Simple Container logs for conversion errors
- Verify Kubernetes version supports affinity features

### **Cost Higher Than Expected**
- Monitor actual vs requested resources
- Check for over-provisioning in HPA settings
- Review node pool utilization metrics

## 📚 **Related Examples**

- [High Availability Patterns](../high-availability/) - Zone anti-affinity
- [Performance Optimization](../performance-optimization/) - Resource-specific scheduling
- [GKE Autopilot Examples](../../gke-autopilot/) - GKE-specific configurations

---

**Production Ready**: This example is based on actual enterprise migration requirements and has been validated in production environments.
