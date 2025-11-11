# GKE Autopilot Comprehensive Setup Example

This example shows how to deploy a complete GCP setup with GKE Autopilot, all major GCP services, and comprehensive resource configurations based on production usage patterns.

## Configuration

- **Provisioner**: Pulumi with GCP bucket state storage and GCP KMS secrets
- **Templates**: Static website and GKE Autopilot with resource references
- **Resources**: MongoDB Atlas, Redis, GKE cluster, Artifact Registry, Pub/Sub
- **Domain**: Cloudflare integration for DNS management

## Key Features

- **Complete GCP Setup**: All major GCP services configured
- **GKE Autopilot**: Kubernetes cluster with current stable version and Caddy integration
- **Advanced Pub/Sub**: Multiple topics/subscriptions with dead letter policies
- **Redis Configuration**: Custom memory policies and regional deployment
- **MongoDB Atlas**: GCP provider integration with Western Europe region
- **Artifact Registry**: Docker registry with immutable tags configuration
- **Template-Based**: GKE template with resource references

## GCP Services Included

### Core Infrastructure
- **GKE Autopilot Cluster**: Current stable version with Caddy reverse proxy (2 replicas)
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

## GKE Version Management

**⚠️ Important**: The GKE version in `server.yaml` must be updated regularly as GCP deprecates old versions.

### Check Current Available Versions

```bash
# Check available versions for your region
gcloud container get-server-config --location=europe-west3 \
  --format="table(channels.channel,channels.validVersions[])" \
  --flatten="channels" --filter="channels.channel=STABLE"

# Get default stable version
gcloud container get-server-config --location=europe-west3 \
  --format="value(channels[0].defaultVersion)" \
  --filter="channels.channel=STABLE"
```

### Regional Considerations

Different GCP regions support different GKE versions:

- **europe-west3** (Frankfurt): Used in this example
- **us-central1** (Iowa): Often has latest versions first  
- **asia-southeast1** (Singapore): May have different version availability

**Always check your target region** before deployment:

```bash
# Compare versions across regions
for region in us-central1 europe-west3 asia-southeast1; do
  echo "=== $region ==="
  gcloud container get-server-config --location=$region \
    --format="value(channels[0].defaultVersion)" \
    --filter="channels.channel=STABLE" 2>/dev/null || echo "Region not available"
done
```

### Troubleshooting Version Errors

If you encounter "Master version unsupported" errors:

1. **Check Current Versions**: Run the version checking commands above
2. **Update server.yaml**: Replace `gkeMinVersion` with current stable version
3. **Consider Regional Switch**: Some regions get updates faster
4. **Use Version Ranges**: Consider using major version only (e.g., "1.33") for flexibility

## Usage

1. **Check GKE Versions**: Verify current available versions for europe-west3
2. **Update server.yaml**: Replace `gkeMinVersion` with current stable version if needed
3. Configure GCP authentication with proper project ID
4. Set up Cloudflare API token for DNS management
5. Configure MongoDB Atlas credentials
6. Deploy the parent stack first
7. Use the templates for service deployments

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
