# GKE Version Management Guide

This guide helps you manage GKE (Google Kubernetes Engine) versions correctly in Simple Container to avoid deployment failures.

## ⚠️ Critical Issue: Outdated GKE Versions

**Common Error**:
```
Error: Master version unsupported
```

**Root Cause**: GCP regularly deprecates old GKE versions. Hardcoded versions in your `server.yaml` become invalid over time.

## Quick Fix

### 1. Check Current Available Versions

```bash
# Check available versions for your region
gcloud container get-server-config --location=YOUR_REGION \
  --format="table(channels.channel,channels.validVersions[])" \
  --flatten="channels" --filter="channels.channel=STABLE"

# Get default stable version
gcloud container get-server-config --location=YOUR_REGION \
  --format="value(channels[0].defaultVersion)" \
  --filter="channels.channel=STABLE"
```

### 2. Update Your server.yaml

**Before** (Outdated):
```yaml
gke-cluster:
  type: gcp-gke-autopilot-cluster
  config:
    gkeMinVersion: 1.27.16-gke.1296000  # ❌ Deprecated
    location: europe-west3
```

**After** (Current):
```yaml
gke-cluster:
  type: gcp-gke-autopilot-cluster
  config:
    gkeMinVersion: 1.33.4-gke.1245000  # ✅ Check: gcloud container get-server-config --location=europe-west3
    location: europe-west3
```

## Regional Considerations

Different GCP regions support different GKE versions and update at different times:

### Major Regions and Characteristics:

- **us-central1** (Iowa): Often gets updates first, highest availability
- **europe-west3** (Frankfurt): Good for EU compliance, stable updates
- **asia-southeast1** (Singapore): Good for APAC, may lag slightly
- **europe-north1** (Finland): Cost-effective EU option
- **us-west1** (Oregon): West Coast US, good performance

### Check Multiple Regions:

```bash
# Compare versions across regions
for region in us-central1 europe-west3 asia-southeast1; do
  echo "=== $region ==="
  gcloud container get-server-config --location=$region \
    --format="value(channels[0].defaultVersion)" \
    --filter="channels.channel=STABLE" 2>/dev/null || echo "Region not available"
done
```

## Version Selection Strategies

### 1. **Specific Version** (Most Reliable)
```yaml
gkeMinVersion: "1.33.4-gke.1245000"
```
- ✅ **Pros**: Predictable, consistent deployments
- ❌ **Cons**: Requires regular updates

### 2. **Major Version** (Flexible)
```yaml
gkeMinVersion: "1.33"
```
- ✅ **Pros**: Auto-selects latest patch within major version
- ❌ **Cons**: May get unexpected updates

### 3. **Latest Stable** (Dynamic)
```yaml
# Use latest stable (not recommended for production)
# Omit gkeMinVersion entirely
```
- ✅ **Pros**: Always current
- ❌ **Cons**: May break on GCP updates

## Troubleshooting Workflow

### Step 1: Identify the Issue
```bash
# If deployment fails with version error
sc provision --preview -s your-stack
```

### Step 2: Check Current Versions
```bash
# For your specific region
gcloud container get-server-config --location=YOUR_REGION \
  --format="value(channels[0].defaultVersion)" \
  --filter="channels.channel=STABLE"
```

### Step 3: Update Configuration
1. Edit your `server.yaml`
2. Update `gkeMinVersion` with current version
3. Add comment with verification command

### Step 4: Verify and Deploy
```bash
# Test with preview first
sc provision --preview -s your-stack

# Deploy if preview succeeds
sc provision -s your-stack
```

## Best Practices

### 1. **Version Comments**
Always add comments showing how to verify current versions:
```yaml
gkeMinVersion: "1.33.4-gke.1245000"  # Check: gcloud container get-server-config --location=europe-west3
```

### 2. **Regular Updates**
- **Monthly**: Check for new stable versions
- **Before Major Deployments**: Always verify current versions
- **After GCP Announcements**: Update when GCP announces deprecations

### 3. **Documentation**
Keep a record of version updates in your project:
```markdown
## Version History
- 2025-10-18: Updated to 1.33.4-gke.1245000 (previous: 1.27.16-gke.1296000)
- 2025-09-15: Updated to 1.27.16-gke.1296000
```

### 4. **Multi-Region Considerations**
If deploying to multiple regions, verify versions are available in all target regions:
```bash
# Check all your target regions
regions=("us-central1" "europe-west3" "asia-southeast1")
for region in "${regions[@]}"; do
  echo "Checking $region..."
  gcloud container get-server-config --location=$region \
    --format="value(channels[0].defaultVersion)" \
    --filter="channels.channel=STABLE"
done
```

### 5. **Automation**
Consider creating a script to check and update versions:
```bash
#!/bin/bash
# update-gke-versions.sh
REGION="europe-west3"
CURRENT_VERSION=$(gcloud container get-server-config --location=$REGION \
  --format="value(channels[0].defaultVersion)" --filter="channels.channel=STABLE")

echo "Current stable version for $REGION: $CURRENT_VERSION"
echo "Update your server.yaml files with this version"
```

## Common Errors and Solutions

### Error: "Master version unsupported"
**Solution**: Update `gkeMinVersion` to current stable version

### Error: "Invalid location"  
**Solution**: Verify region name with `gcloud compute regions list`

### Error: "Version not available in region"
**Solution**: Choose a different region or use region-specific version

### Error: "Insufficient permissions"
**Solution**: Ensure GCP credentials have Container Engine Admin role

## Version Lifecycle

### GCP Version Support Timeline:
1. **Alpha** → **Beta** → **Stable** → **Default** → **Deprecated** → **Unsupported**
2. **Typical Lifecycle**: ~6-12 months from stable to deprecated
3. **Deprecation Notice**: GCP provides 3+ months advance notice

### Recommended Approach:
- **Production**: Use stable versions that are 1-2 versions behind latest
- **Development**: Use latest stable for testing new features
- **Critical Systems**: Pin to specific versions and update on planned schedule

## Resources

- [GKE Release Notes](https://cloud.google.com/kubernetes-engine/docs/release-notes)
- [GKE Versioning and Upgrades](https://cloud.google.com/kubernetes-engine/docs/concepts/cluster-upgrades)
- [gcloud container get-server-config Reference](https://cloud.google.com/sdk/gcloud/reference/container/get-server-config)

---

**💡 Pro Tip**: Bookmark this page and check GKE versions monthly to avoid deployment surprises!
