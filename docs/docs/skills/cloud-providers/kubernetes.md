---
title: Kubernetes Setup
description: Kubernetes-specific setup guide for Simple Container
platform: platform
product: simple-container
category: skills
subcategory: cloud-provider
date: '2026-03-29'
---

# Kubernetes Setup Skill

This skill guides you through setting up Kubernetes credentials and resources for Simple Container. Follow these steps to configure Kubernetes authentication and create required resources.

## Prerequisites

- Kubernetes cluster (self-hosted, EKS, GKE, AKS)
- kubectl CLI installed (`kubectl version`)
- SC CLI installed (see [Installation](../installation.md))

## Steps

### Step 1: Install kubectl

If you haven't already, install kubectl:

```bash
# Install kubectl (Linux)
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl
sudo mv kubectl /usr/local/bin/

# Verify installation
kubectl version --client
```

### Step 2: Configure kubectl

#### Option A: From Cloud Provider

**AWS EKS:**
```bash
# Update kubeconfig for EKS
aws eks update-kubeconfig --name my-cluster --region us-east-1
```

**GCP GKE:**
```bash
# Get credentials for GKE
gcloud container clusters get-credentials my-cluster --region us-central1
```

**Azure AKS:**
```bash
# Get credentials for AKS
az aks get-credentials --name my-cluster --resource-group my-group
```

#### Option B: From Custom Cluster

```bash
# If you have kubeconfig file
export KUBECONFIG=/path/to/kubeconfig

# Or copy to default location
mkdir -p ~/.kube
cp /path/to/kubeconfig ~/.kube/config
```

### Step 3: Verify Cluster Access

```bash
# Check cluster connectivity
kubectl cluster-info

# List nodes
kubectl get nodes

# List namespaces
kubectl get namespaces
```

### Step 4: Create Project Namespace

Create a namespace for your project:

```bash
# Create namespace
kubectl create namespace myproject

# Set default namespace (optional)
kubectl config set-context --current --namespace=myproject
```

### Step 5: Set Up Container Registry

Configure access to your container registry:

**Docker Hub:**
```bash
# Login to Docker Hub
echo "${DOCKER_HUB_PASSWORD}" | docker login -u "${DOCKER_HUB_USERNAME}" --password-stdin
```

**AWS ECR:**
```bash
# Get ECR login command
aws ecr get-login-password --region us-east-1 | kubectl create secret docker-registry ecr-secret \
  --docker-server=123456789012.dkr.ecr.us-east-1.amazonaws.com \
  --docker-username=AWS \
  --docker-password="$(aws ecr get-login-password --region us-east-1)"
```

**GCP Artifact Registry:**
```bash
# Get GCR login command
gcloud auth print-access-token | kubectl create secret docker-registry gcr-secret \
  --docker-server=https://us-central1-docker.pkg.dev \
  --docker-username=oauth2accesstoken \
  --docker-password="$(gcloud auth print-access-token)"
```

### Step 6: Create Secrets

Create Kubernetes secrets for your application:

```bash
# Create generic secret
kubectl create secret generic database-credentials \
  --from-literal=username=admin \
  --from-literal=password="${DB_PASSWORD}" \
  --namespace=myproject

# Create TLS secret
kubectl create secret tls my-tls-secret \
  --cert=path/to/cert.crt \
  --key=path/to/cert.key \
  --namespace=myproject
```

### Step 7: Verify kubeconfig

Export kubeconfig for SC:

```bash
# Export kubeconfig to file
kubectl config view --flatten > kubeconfig.yaml

# Or get base64 encoded
cat kubeconfig.yaml | base64 -w 0
```

### Step 8: Set Environment Variables

For SC to use your Kubernetes credentials, set these environment variables:

```bash
# Kubernetes context
export KUBECTL_CONTEXT="my-cluster"

# Or kubeconfig path
export KUBECONFIG_PATH="/path/to/kubeconfig"

# Or base64 encoded kubeconfig
export KUBECONFIG_BASE64="$(cat kubeconfig.yaml | base64 -w 0)"
```

## Environment Variables for Kubernetes

| Variable | Description | Required |
|----------|-------------|----------|
| `KUBECTL_CONTEXT` | Kubernetes context name | Yes* |
| `KUBECONFIG_PATH` | Path to kubeconfig file | Yes* |
| `KUBECONFIG_BASE64` | Base64 encoded kubeconfig | Yes* |

*Either `KUBECTL_CONTEXT` with default kubeconfig, or `KUBECONFIG_PATH`, or `KUBECONFIG_BASE64` is required.

## Example: Full Kubernetes server.yaml

```yaml
schemaVersion: 1.0

project: myproject
name: devops

provider:
  name: kubernetes
  context: my-cluster
  namespace: myproject

auth:
  - name: k8s-main
    provider: kubernetes
    config:
      kubeconfig: ${KUBECONFIG_PATH}

resources:
  - name: postgres-db
    type: kubernetes:postgres
    config:
      storage: 10Gi
      className: standard

  - name: s3-compatible
    type: kubernetes:minio
    config:
      storage: 20Gi
```

## Kubernetes Resources in SC

SC supports these Kubernetes resource types:

| Resource Type | Description |
|---------------|-------------|
| `kubernetes:postgres` | PostgreSQL database |
| `kubernetes:mysql` | MySQL database |
| `kubernetes:redis` | Redis cache |
| `kubernetes:minio` | S3-compatible storage |
| `kubernetes:mongodb` | MongoDB |
| `kubernetes:rabbitmq` | RabbitMQ |

## Common Issues

### "Unable to connect to the server"

Check cluster connectivity:
```bash
kubectl cluster-info
```

### "Unauthorized"

Your credentials don't have permission. Check:
```bash
# Check current context
kubectl config current-context

# List contexts
kubectl config get-contexts
```

### "Namespace not found"

Create the namespace:
```bash
kubectl create namespace myproject
```

### "ImagePullBackOff"

Check your container registry credentials:
```bash
# List registry secrets
kubectl get secrets --namespace=myproject
```

## Next Steps

After Kubernetes setup:

1. [DevOps Setup](../devops-setup.md) - Create server.yaml with K8s resources
2. [Service Setup](../service-setup.md) - Configure your service deployment