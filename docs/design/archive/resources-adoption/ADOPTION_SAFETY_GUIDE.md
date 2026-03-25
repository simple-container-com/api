# Simple Container Resource Adoption Safety Guide

## ⚠️ CRITICAL: Production Resource Protection

This guide documents the critical safety mechanisms implemented to prevent production resource deletion during adoption, following the **production MongoDB cluster deletion incident**.

## 🚨 The Problem

When adopting existing resources into Simple Container, Pulumi may mark resources as "REPLACED" instead of "IMPORTED", which triggers a **DELETE operation on production resources**.

### What Happened
- User adopted MongoDB cluster `"primary-dev-e5099b5"` in production environment
- Pulumi marked the adopted resource as "REPLACED" instead of "IMPORTED"  
- This triggered a DELETE operation on the production MongoDB cluster
- **Result: Production data loss**

## 🛡️ Protection Mechanisms Implemented

### 1. Resource Protection (`sdk.Protect(true)`)

All adoption functions now include `sdk.Protect(true)` to prevent Pulumi from deleting resources:

```go
opts := []sdk.ResourceOption{
    sdk.Provider(params.Provider),
    sdk.Import(sdk.ID(resourceId)),
    // CRITICAL: Protect adopted resources from deletion
    sdk.Protect(true),
}
```

### 2. Configuration Drift Protection (`sdk.IgnoreChanges`)

Comprehensive ignore patterns prevent configuration drift from triggering replacements:

```go
sdk.IgnoreChanges([]string{
    // Core configuration that might drift
    "diskSizeGb", "numShards", "cloudBackup",
    // Provider-specific settings
    "providerAutoScalingComputeEnabled",
    // Advanced settings managed outside Pulumi
    "advancedConfiguration", "labels", "tags",
})
```

### 3. Production Environment Warnings

Critical warnings alert users when adopting resources in production:

```go
if input.StackParams.Environment == "production" || input.StackParams.Environment == "prod" {
    params.Log.Warn(ctx.Context(), "🚨 CRITICAL: Adopting %s %q in PRODUCTION environment %q", resourceType, resourceName, environment)
    params.Log.Warn(ctx.Context(), "🛡️  PROTECTION: Resource will be protected from deletion with sdk.Protect(true)")
    params.Log.Warn(ctx.Context(), "⚠️  WARNING: Ensure resource configuration matches exactly to prevent replacements")
}
```

## 📋 Protected Resource Types

The following resource types now have comprehensive adoption protection:

### ✅ MongoDB Atlas
- **File**: `pkg/clouds/pulumi/mongodb/adopt_cluster.go`
- **Protection**: `sdk.Protect(true)` + comprehensive `IgnoreChanges`
- **Warnings**: Production environment alerts

### ✅ GCP Cloud SQL PostgreSQL  
- **File**: `pkg/clouds/pulumi/gcp/adopt_postgres.go`
- **Protection**: `sdk.Protect(true)` + comprehensive `IgnoreChanges`
- **Warnings**: Production environment alerts

### ✅ GCP Redis Memorystore
- **File**: `pkg/clouds/pulumi/gcp/adopt_redis.go`  
- **Protection**: `sdk.Protect(true)` + comprehensive `IgnoreChanges`
- **Warnings**: Production environment alerts

### ✅ GKE Autopilot Clusters
- **File**: `pkg/clouds/pulumi/gcp/adopt_gke_autopilot.go`
- **Protection**: `sdk.Protect(true)` + comprehensive `IgnoreChanges`
- **Warnings**: Production environment alerts

## 🔧 Utility Functions

### Adoption Protection Utility
**File**: `pkg/clouds/pulumi/adoption_protection.go`

```go
// Provides comprehensive protection for adopted resources
func AdoptionProtectionOptions(input api.ResourceInput, params pApi.ProvisionParams, ignoreChanges []string) []sdk.ResourceOption

// Logs critical safety warnings for production environments  
func LogAdoptionWarnings(input api.ResourceInput, params pApi.ProvisionParams, resourceType, resourceName string)

// Validates common adoption configuration requirements
func ValidateAdoptionConfig(adoptFlag bool, resourceName, descriptorName string) error
```

## 🚀 Best Practices for Safe Adoption

### 1. Always Test in Staging First
```yaml
# Test adoption in staging environment first
resources:
  staging:
    template: gke-autopilot-staging
    resources:
      mongodb:
        type: mongodb-atlas
        config:
          adopt: true
          clusterName: "staging-cluster-test"
```

### 2. Verify Configuration Matches Exactly
Before adoption, ensure your Simple Container configuration matches the existing resource exactly:

```bash
# Check existing MongoDB cluster configuration
atlas clusters describe staging-cluster-test --projectId PROJECT_ID

# Ensure server.yaml matches exactly:
# - instanceSize matches providerInstanceSizeName
# - region matches providerRegionName  
# - cloudProvider matches providerName
```

### 3. Use Dry Run for Validation
```bash
# Always run with --dry-run first
sc provision --dry-run

# Look for any "REPLACE" operations in output
# Should only see "IMPORT" operations for adopted resources
```

### 4. Monitor Pulumi Logs Carefully
Watch for these critical log messages:

```
✅ GOOD: "adopting existing MongoDB Atlas cluster"
✅ GOOD: "successfully adopted MongoDB Atlas cluster" 
❌ BAD: Any "REPLACE" operations in Pulumi output
❌ BAD: Any "DELETE" operations for adopted resources
```

## 🆘 Emergency Recovery

If a protected resource shows "REPLACE" in Pulumi preview:

### 1. STOP Immediately
```bash
# Cancel the operation immediately
Ctrl+C
```

### 2. Check Protection Status
```bash
# Verify resource is protected in Pulumi state
pulumi state show <resource-name>
# Should show: "protect": true
```

### 3. Remove from State (Safe)
```bash
# Remove from Pulumi state without deleting actual resource
pulumi state delete <resource-name>
```

### 4. Re-adopt with Correct Configuration
- Fix configuration mismatch
- Re-run adoption with corrected settings

## 🔍 Validation Checklist

Before adopting any production resource:

- [ ] **Configuration Match**: Server.yaml exactly matches existing resource
- [ ] **Staging Test**: Successfully adopted identical resource in staging
- [ ] **Dry Run Clean**: `--dry-run` shows only IMPORT operations
- [ ] **Protection Enabled**: Logs show "Resource will be protected from deletion"
- [ ] **Team Notification**: Production team aware of adoption process
- [ ] **Backup Verified**: Recent backups available for critical data

## 📞 Support

If you encounter any adoption issues:

1. **Stop the operation immediately**
2. **Check this guide** for troubleshooting steps
3. **Contact the Simple Container team** with:
   - Resource type being adopted
   - Environment (production/staging)
   - Pulumi logs showing the issue
   - Configuration files (server.yaml)

## 🔄 Future Enhancements

Planned improvements to adoption safety:

- [ ] **Pre-adoption validation**: Automatic configuration matching
- [ ] **Interactive confirmation**: Required approval for production adoptions
- [ ] **Backup verification**: Automatic backup checks before adoption
- [ ] **Rollback mechanism**: Automated rollback for failed adoptions

---

**Remember**: The protection mechanisms are your safety net, but careful configuration and testing are your first line of defense against production incidents.
