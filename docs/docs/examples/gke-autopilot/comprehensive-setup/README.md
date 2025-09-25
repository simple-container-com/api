# GKE Autopilot Comprehensive Setup Example

This example shows how to deploy a complete GCP setup with GKE Autopilot, all major GCP services, and comprehensive resource configurations based on production usage patterns.

## Configuration

- **Provisioner**: Pulumi with GCP bucket state storage and GCP KMS secrets
- **Templates**: Static website and GKE Autopilot with resource references
- **Resources**: MongoDB Atlas, Redis, GKE cluster, Artifact Registry, Pub/Sub
- **Domain**: Cloudflare integration for DNS management

## Key Features

- **Complete GCP Setup**: All major GCP services configured
- **GKE Autopilot**: Kubernetes cluster with specific version and Caddy integration
- **Advanced Pub/Sub**: Multiple topics/subscriptions with dead letter policies
- **Redis Configuration**: Custom memory policies and regional deployment
- **MongoDB Atlas**: GCP provider integration with Western Europe region
- **Artifact Registry**: Docker registry with immutable tags configuration
- **Template-Based**: GKE template with resource references

## GCP Services Included

### Core Infrastructure
- **GKE Autopilot Cluster**: v1.27.16 with Caddy reverse proxy (2 replicas)
- **Artifact Registry**: Docker registry in europe-west3
- **GCP Redis**: 2GB memory with custom eviction policy
- **GCP Pub/Sub**: Multiple topics and subscriptions with advanced configuration

### External Services
- **MongoDB Atlas**: M0 instance with GCP provider
- **Cloudflare**: DNS management for example.com

### State Management
- **GCP Bucket**: State storage in europe-west3
- **GCP KMS**: Secrets encryption with global key

## Pub/Sub Configuration

Advanced Pub/Sub setup with:
- **Dead Letter Policies**: Message retention and exactly-once delivery
- **Multiple Topics**: Image generation and data processing workers
- **Subscriptions**: 600s ack deadline, 24h message retention
- **Labels**: Environment and type labeling for organization

## Template Structure

The GKE Autopilot template references specific resources:
```yaml
gkeClusterResource: gke-autopilot-res
artifactRegistryResource: artifact-registry-res
```

## Usage

1. Configure GCP authentication with proper project ID
2. Set up Cloudflare API token for DNS management
3. Configure MongoDB Atlas credentials
4. Deploy the parent stack first
5. Use the templates for service deployments

## Authentication Requirements

- **GCP**: Service account with comprehensive permissions
- **Cloudflare**: API token for DNS management
- **MongoDB Atlas**: Public/private key pair

## Regional Configuration

All resources deployed in **europe-west3** for:
- Low latency within Europe
- GDPR compliance
- Cost optimization

## Scaling Considerations

- **GKE Autopilot**: Auto-scaling based on workload demands
- **Redis**: 2GB memory suitable for medium workloads
- **MongoDB Atlas**: M0 free tier (upgrade to M10+ for production)
- **Pub/Sub**: Unlimited scaling with pay-per-use model
