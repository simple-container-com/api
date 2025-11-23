# Resource Adoption Implementation Plan

## Overview

This document provides a detailed implementation plan for adding resource adoption support to Simple Container, based on analysis of the existing codebase.

## Current Code Architecture

### **1. MongoDB Atlas** (`pkg/clouds/pulumi/mongodb/`)

**Current Provisioning Flow**:
```
mongodb/cluster.go (Cluster function)
├── Creates MongoDB Atlas Project (if projectId not provided)
├── Creates MongoDB Atlas Cluster (NewCluster)
├── Configures backup schedule (optional)
├── Configures network access (private link or IP whitelist)
├── Creates database users (admins, developers)
└── Exports: projectId, clusterId, mongoUri, users
```

**Current Compute Processor** (`mongodb/compute_proc.go`):
```
ClusterComputeProcessor
├── Reads outputs from parent stack
├── Gets projectId and mongoUri from parent
├── Creates database user via Pulumi provider (createDatabaseUser)
│   └── mongodbatlas.NewDatabaseUser() - uses Pulumi MongoDB Atlas provider
├── Generates environment variables (MONGO_USER, MONGO_PASSWORD, MONGO_URI)
└── Returns connection details
```

**Registration** (`mongodb/init.go`):
```go
api.RegisterResources(map[string]api.ProvisionFunc{
    mongodb.ResourceTypeMongodbAtlas: Cluster,  // "mongodb-atlas"
})
api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
    mongodb.ResourceTypeMongodbAtlas: ClusterComputeProcessor,
})
```

### **2. GKE Autopilot** (`pkg/clouds/pulumi/gcp/`)

**Current Provisioning Flow** (`gke_autopilot.go`):
```
GkeAutopilot function
├── Creates GKE Autopilot Cluster (container.NewCluster)
├── Generates kubeconfig
├── Optional: Provisions Caddy with GCS storage
│   ├── Creates GCS bucket for ACME certificates
│   ├── Creates service account for Caddy
│   └── Deploys Caddy service
└── Exports: kubeconfig, Caddy config
```

**Current Compute Processor** (`gke_autopilot_compute_proc.go`):
```go
func GkeAutopilotComputeProcessor(...) (*api.ResourceOutput, error) {
    params.Log.Error(ctx.Context(), "not implemented for gke autopilot")
    return &api.ResourceOutput{Ref: nil}, nil
}
// ❌ NOT IMPLEMENTED
```

**Stack Provisioning** (`gke_autopilot_stack.go`):
```
GkeAutopilotStack function
├── Gets kubeconfig from parent stack
├── Gets registry URL from parent stack
├── Builds and pushes Docker images
├── Deploys SimpleContainer to cluster
└── Optionally configures DNS records
```

### **3. GCP Cloud SQL Postgres** (`pkg/clouds/pulumi/gcp/`)

**Current Provisioning Flow** (`postgres.go`):
```
Postgres function
├── Generates random root password
├── Creates Cloud SQL instance (sql.NewDatabaseInstance)
└── Exports: root password
```

**Current Compute Processor** (`compute_proc.go`):
```
PostgresComputeProcessor
├── Reads root password from parent stack
├── Gets kubeconfig from parent stack (via usersProvisionRuntime)
├── Creates Kubernetes provider
├── Creates database and user via Kubernetes Jobs
│   └── Uses kubernetes/compute_proc_postgres.go:NewPostgresInitDbUserJob
├── Generates environment variables
└── Returns connection details
```

**User Creation Architecture**:
- Uses `usersProvisionRuntime` config to determine where to run jobs
- Type "gke" → runs K8s Jobs in GKE cluster
- Jobs use Cloud SQL Proxy to connect securely

## Proposed Adoption Architecture

### **Design Principle: Separate Adoption Logic**

Create separate files for adoption logic to keep code clean and maintainable:

```
pkg/clouds/pulumi/mongodb/
├── cluster.go           (existing - provisioning)
├── adopt_cluster.go     (NEW - adoption)
├── compute_proc.go      (existing - needs enhancement)
└── init.go              (existing - needs update)

pkg/clouds/pulumi/gcp/
├── gke_autopilot.go            (existing - provisioning)
├── adopt_gke_autopilot.go      (NEW - adoption)
├── gke_autopilot_compute_proc.go (existing - needs implementation)
├── postgres.go                 (existing - provisioning)
├── adopt_postgres.go           (NEW - adoption)
├── compute_proc.go             (existing - needs enhancement)
└── init.go                     (existing - needs update)
```

## Implementation Plan by Resource

### **Phase 1: MongoDB Atlas Adoption**

#### **File: `pkg/clouds/pulumi/mongodb/adopt_cluster.go`** (NEW)

```go
package mongodb

import (
    "github.com/pkg/errors"
    "github.com/pulumi/pulumi-mongodbatlas/sdk/v3/go/mongodbatlas"
    sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
    "github.com/simple-container-com/api/pkg/api"
    "github.com/simple-container-com/api/pkg/clouds/mongodb"
    pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// AdoptedClusterConfig extends AtlasConfig with adoption-specific fields
type AdoptedClusterConfig struct {
    *mongodb.AtlasConfig
    Adopt       bool   `json:"adopt" yaml:"adopt"`
    ClusterName string `json:"clusterName" yaml:"clusterName"`
    // ProjectId already in AtlasConfig
}

// AdoptCluster imports an existing MongoDB Atlas cluster into Pulumi state
func AdoptCluster(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    atlasCfg, ok := input.Descriptor.Config.Config.(*mongodb.AtlasConfig)
    if !ok {
        return nil, errors.Errorf("failed to convert mongodb atlas config for %q", input.Descriptor.Type)
    }

    projectName := toProjectName(stack.Name, input)
    clusterName := toClusterName(stack.Name, input)
    
    params.Log.Info(ctx.Context(), "Adopting existing MongoDB Atlas cluster %q in project %q", 
        clusterName, atlasCfg.ProjectId)

    opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}

    // Lookup existing cluster (doesn't create, just references)
    cluster, err := mongodbatlas.LookupCluster(ctx, &mongodbatlas.LookupClusterArgs{
        Name:      clusterName,
        ProjectId: atlasCfg.ProjectId,
    }, sdk.Provider(params.Provider))
    if err != nil {
        return nil, errors.Wrapf(err, "failed to lookup adopted MongoDB cluster %q", clusterName)
    }

    // Import cluster into Pulumi state using pulumi.Import
    adoptedCluster, err := mongodbatlas.GetCluster(ctx, fmt.Sprintf("%s-cluster", clusterName), 
        sdk.ID(cluster.ClusterId), 
        &mongodbatlas.ClusterState{
            Name:      sdk.StringPtr(cluster.Name),
            ProjectId: sdk.StringPtr(cluster.ProjectId),
        }, 
        append(opts, sdk.Import(sdk.ID(cluster.ClusterId)))...)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to import adopted MongoDB cluster %q", clusterName)
    }

    // Export connection details (same as provisioned)
    ctx.Export(toProjectIdExport(projectName), sdk.String(atlasCfg.ProjectId))
    ctx.Export(toClusterIdExport(clusterName), sdk.String(cluster.ClusterId))
    ctx.Export(toMongoUriExport(clusterName), sdk.String(cluster.MongoUri))
    ctx.Export(toMongoUriWithOptionsExport(clusterName), sdk.String(cluster.MongoUriWithOptions))

    params.Log.Info(ctx.Context(), "✅ Successfully adopted MongoDB Atlas cluster %q", clusterName)

    return &api.ResourceOutput{Ref: adoptedCluster}, nil
}
```

#### **Modify: `pkg/clouds/pulumi/mongodb/cluster.go`**

Add adoption detection at the beginning of `Cluster` function:

```go
func Cluster(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    if input.Descriptor.Type != mongodb.ResourceTypeMongodbAtlas {
        return nil, errors.Errorf("unsupported mongodb-atlas type %q", input.Descriptor.Type)
    }

    atlasCfg, ok := input.Descriptor.Config.Config.(*mongodb.AtlasConfig)
    if !ok {
        return nil, errors.Errorf("failed to convert mongodb atlas config for %q", input.Descriptor.Type)
    }

    // NEW: Check if this is an adoption request
    if adopt, ok := input.Descriptor.Config.Config.(interface{ Adopt() bool }); ok && adopt.Adopt() {
        params.Log.Info(ctx.Context(), "Detected adopt=true, using adoption flow for MongoDB Atlas cluster")
        return AdoptCluster(ctx, stack, input, params)
    }

    // Continue with existing provisioning logic...
    out := &ClusterOutput{}
    // ...
}
```

#### **Modify: `pkg/clouds/mongodb/mongodb.go`**

Add `Adopt` field to `AtlasConfig`:

```go
type AtlasConfig struct {
    // Existing fields...
    Admins         []string                      `json:"admins" yaml:"admins"`
    Developers     []string                      `json:"developers" yaml:"developers"`
    InstanceSize   string                        `json:"instanceSize" yaml:"instanceSize"`
    OrgId          string                        `json:"orgId" yaml:"orgId"`
    ProjectId      string                        `json:"projectId" yaml:"projectId"`
    ProjectName    string                        `json:"projectName" yaml:"projectName"`
    Region         string                        `json:"region" yaml:"region"`
    PrivateKey     string                        `json:"privateKey" yaml:"privateKey"`
    PublicKey      string                        `json:"publicKey" yaml:"publicKey"`
    CloudProvider  string                        `json:"cloudProvider" yaml:"cloudProvider"`
    Backup         *AtlasBackup                  `json:"backup,omitempty" yaml:"backup,omitempty"`
    NetworkConfig  *AtlasNetworkConfig           `json:"networkConfig,omitempty" yaml:"networkConfig,omitempty"`
    ExtraProviders map[string]api.AuthDescriptor `json:"extraProviders,omitempty" yaml:"extraProviders,omitempty"`
    DiskSizeGB     *float64                      `json:"diskSizeGB,omitempty" yaml:"diskSizeGB,omitempty"`
    NumShards      *int                          `json:"numShards,omitempty" yaml:"numShards,omitempty"`
    
    // NEW: Adoption fields
    Adopt       bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
    ClusterName string `json:"clusterName,omitempty" yaml:"clusterName,omitempty"` // For adoption: explicit cluster name
}

// NEW: Method to check if this is an adoption request
func (r *AtlasConfig) IsAdoption() bool {
    return r.Adopt
}
```

#### **Modify: `pkg/clouds/pulumi/mongodb/compute_proc.go`**

**NO CHANGES NEEDED** - The compute processor already uses Pulumi MongoDB Atlas provider to create users, which works identically for adopted and provisioned clusters!

The existing `createDatabaseUser` function (line 434) already does:
```go
mongodbatlas.NewDatabaseUser(ctx, userObjectName, &mongodbatlas.DatabaseUserArgs{
    AuthDatabaseName: sdk.String("admin"),
    Password:         password.Result,
    ProjectId:        sdk.String(user.projectId),  // Works with adopted clusters
    Roles:            roles,
    Username:         sdk.String(user.username),
}, opts...)
```

This works because it only needs `projectId` and credentials, which are the same for adopted clusters.

---

### **Phase 2: GCP Cloud SQL Postgres Adoption**

#### **File: `pkg/clouds/pulumi/gcp/adopt_postgres.go`** (NEW)

```go
package gcp

import (
    "fmt"
    "github.com/pkg/errors"
    "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/sql"
    "github.com/pulumi/pulumi-random/sdk/v4/go/random"
    sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
    "github.com/simple-container-com/api/pkg/api"
    "github.com/simple-container-com/api/pkg/clouds/gcloud"
    pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// AdoptPostgres imports an existing Cloud SQL Postgres instance into Pulumi state
func AdoptPostgres(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    pgCfg, ok := input.Descriptor.Config.Config.(*gcloud.PostgresGcpCloudsqlConfig)
    if !ok {
        return nil, errors.Errorf("failed to convert postgresql config for %q", input.Descriptor.Type)
    }

    postgresName := toPostgresName(input, input.Descriptor.Name)
    
    params.Log.Info(ctx.Context(), "Adopting existing Cloud SQL Postgres instance %q in project %q", 
        postgresName, pgCfg.Project)

    // Lookup existing instance
    existingInstance, err := sql.LookupDatabaseInstance(ctx, &sql.LookupDatabaseInstanceArgs{
        Name:    postgresName,
        Project: &pgCfg.Project,
    })
    if err != nil {
        return nil, errors.Wrapf(err, "failed to lookup adopted Postgres instance %q", postgresName)
    }

    // Import instance into Pulumi state
    pgInstance, err := sql.GetDatabaseInstance(ctx, postgresName,
        sdk.ID(existingInstance.Id),
        &sql.DatabaseInstanceState{
            Name:            sdk.StringPtr(existingInstance.Name),
            Region:          sdk.StringPtr(existingInstance.Region),
            DatabaseVersion: sdk.StringPtr(existingInstance.DatabaseVersion),
        },
        sdk.Provider(params.Provider),
        sdk.Import(sdk.ID(existingInstance.Id)))
    if err != nil {
        return nil, errors.Wrapf(err, "failed to import adopted Postgres instance %q", postgresName)
    }

    // For adopted resources, root password comes from secrets.yaml, not generated
    rootPasswordExport := toPostgresRootPasswordExport(postgresName)
    
    // Export a placeholder that tells compute processor to read from secrets.yaml
    ctx.Export(rootPasswordExport, sdk.String("${secret:POSTGRES_ROOT_PASSWORD}"))
    
    // Export additional connection details
    ctx.Export(toPostgresRootUsernameExport(postgresName), sdk.String("postgres"))
    ctx.Export(toPostgresInstanceNameExport(postgresName), sdk.String(existingInstance.Name))
    ctx.Export(toPostgresConnectionNameExport(postgresName), sdk.String(existingInstance.ConnectionName))

    params.Log.Info(ctx.Context(), "✅ Successfully adopted Cloud SQL Postgres instance %q", postgresName)

    return &api.ResourceOutput{Ref: pgInstance}, nil
}

func toPostgresRootUsernameExport(resName string) string {
    return fmt.Sprintf("%s-root-username", resName)
}

func toPostgresInstanceNameExport(resName string) string {
    return fmt.Sprintf("%s-instance-name", resName)
}

func toPostgresConnectionNameExport(resName string) string {
    return fmt.Sprintf("%s-connection-name", resName)
}
```

#### **Modify: `pkg/clouds/pulumi/gcp/postgres.go`**

Add adoption detection:

```go
func Postgres(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    if input.Descriptor.Type != gcloud.ResourceTypePostgresGcpCloudsql {
        return nil, errors.Errorf("unsupported postgres type %q", input.Descriptor.Type)
    }

    pgCfg, ok := input.Descriptor.Config.Config.(*gcloud.PostgresGcpCloudsqlConfig)
    if !ok {
        return nil, errors.Errorf("failed to convert postgresql config for %q", input.Descriptor.Type)
    }

    // NEW: Check if this is an adoption request
    if pgCfg.Adopt {
        params.Log.Info(ctx.Context(), "Detected adopt=true, using adoption flow for Cloud SQL Postgres")
        return AdoptPostgres(ctx, stack, input, params)
    }

    // Continue with existing provisioning logic...
}
```

#### **Modify: `pkg/clouds/gcloud/postgres.go`**

Add adoption fields:

```go
type PostgresGcpCloudsqlConfig struct {
    Credentials           `json:",inline" yaml:",inline"`
    Version               string                  `json:"version" yaml:"version"`
    Project               string                  `json:"project" yaml:"project"`
    Tier                  *string                 `json:"tier" yaml:"tier"`
    Region                *string                 `json:"region" yaml:"region"`
    MaxConnections        *int                    `json:"maxConnections" yaml:"maxConnections"`
    DeletionProtection    *bool                   `json:"deletionProtection" yaml:"deletionProtection"`
    QueryInsightsEnabled  *bool                   `json:"queryInsightsEnabled" yaml:"queryInsightsEnabled"`
    QueryStringLength     *int                    `json:"queryStringLength" yaml:"queryStringLength"`
    UsersProvisionRuntime *ProvisionRuntimeConfig `json:"usersProvisionRuntime" yaml:"usersProvisionRuntime"`
    
    // NEW: Adoption fields
    Adopt          bool   `json:"adopt,omitempty" yaml:"adopt,omitempty"`
    InstanceName   string `json:"instanceName,omitempty" yaml:"instanceName,omitempty"`
    ConnectionName string `json:"connectionName,omitempty" yaml:"connectionName,omitempty"`
}
```

#### **Modify: `pkg/clouds/pulumi/gcp/compute_proc.go`**

Update `PostgresComputeProcessor` to handle root password from secrets.yaml for adopted resources:

```go
func PostgresComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    // ... existing code ...

    rootPasswordExport := toPostgresRootPasswordExport(postgresName)
    rootPassword, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s%s-cproc-rootpass", postgresName, suffix), fullParentReference, rootPasswordExport, true)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to get root password from parent stack for %q", postgresName)
    }
    
    // NEW: Check if root password is a placeholder for secrets.yaml
    if strings.HasPrefix(rootPassword, "${secret:") {
        // This is an adopted resource, read actual password from secrets
        secretName := strings.TrimSuffix(strings.TrimPrefix(rootPassword, "${secret:"), "}")
        // The actual resolution happens in the Kubernetes Job via secret injection
        params.Log.Info(ctx.Context(), "Using root credentials from secrets.yaml for adopted Postgres instance")
    }

    // Rest of the logic remains the same - K8s Jobs work identically for adopted resources
    // ...
}
```

---

### **Phase 3: GKE Autopilot Adoption**

#### **File: `pkg/clouds/pulumi/gcp/adopt_gke_autopilot.go`** (NEW)

```go
package gcp

import (
    "fmt"
    "github.com/pkg/errors"
    "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
    sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
    "github.com/simple-container-com/api/pkg/api"
    "github.com/simple-container-com/api/pkg/clouds/gcloud"
    "github.com/simple-container-com/api/pkg/clouds/k8s"
    pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
    "github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
)

// AdoptGkeAutopilot imports an existing GKE Autopilot cluster into Pulumi state
func AdoptGkeAutopilot(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    gkeInput, ok := input.Descriptor.Config.Config.(*gcloud.GkeAutopilotResource)
    if !ok {
        return nil, errors.Errorf("failed to convert gke autopilot config for %q", input.Descriptor.Type)
    }

    clusterName := kubernetes.ToClusterName(input, input.Descriptor.Name)
    location := gkeInput.Location
    
    params.Log.Info(ctx.Context(), "Adopting existing GKE Autopilot cluster %q in location %q", 
        clusterName, location)

    // Lookup existing cluster
    existingCluster, err := container.LookupCluster(ctx, &container.LookupClusterArgs{
        Name:     clusterName,
        Location: &location,
        Project:  &gkeInput.ProjectId,
    })
    if err != nil {
        return nil, errors.Wrapf(err, "failed to lookup adopted GKE cluster %q", clusterName)
    }

    // Import cluster into Pulumi state
    cluster, err := container.GetCluster(ctx, clusterName,
        sdk.ID(existingCluster.Id),
        &container.ClusterState{
            Name:     sdk.StringPtr(existingCluster.Name),
            Location: sdk.StringPtr(existingCluster.Location),
            Project:  sdk.StringPtr(existingCluster.Project),
        },
        sdk.Provider(params.Provider),
        sdk.Import(sdk.ID(existingCluster.Id)))
    if err != nil {
        return nil, errors.Wrapf(err, "failed to import adopted GKE cluster %q", clusterName)
    }

    // Generate kubeconfig (same as provisioned)
    kubeconfig := generateKubeconfig(cluster, gkeInput)
    ctx.Export(toKubeconfigExport(clusterName), kubeconfig)

    out := GkeAutopilotOut{Cluster: cluster}

    // Handle Caddy for adopted cluster
    if gkeInput.Caddy != nil {
        caddyHandling := gkeInput.Caddy.AdoptionHandling
        
        if caddyHandling != nil && caddyHandling.SkipDeployment {
            params.Log.Info(ctx.Context(), "Skipping Caddy deployment for adopted cluster (skipDeployment=true)")
        } else if caddyHandling != nil && caddyHandling.PatchExisting {
            params.Log.Info(ctx.Context(), "Will patch existing Caddy deployment in adopted cluster")
            // Caddy patching happens in gke_autopilot_stack.go during service deployment
        } else {
            params.Log.Info(ctx.Context(), "Deploying new Caddy instance to adopted cluster")
            // Deploy Caddy as if it's a new cluster
            // ... same Caddy deployment logic as provisioned clusters
        }
    }

    params.Log.Info(ctx.Context(), "✅ Successfully adopted GKE Autopilot cluster %q", clusterName)

    return &api.ResourceOutput{Ref: out}, nil
}
```

#### **Modify: `pkg/clouds/gcloud/gke_autopilot.go`**

Add adoption fields:

```go
type GkeAutopilotResource struct {
    Credentials   `json:",inline" yaml:",inline"`
    GkeMinVersion string           `json:"gkeMinVersion" yaml:"gkeMinVersion"`
    Location      string           `json:"location" yaml:"location"`
    Zone          string           `json:"zone" yaml:"zone"`
    Timeouts      *Timeouts        `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`
    Caddy         *k8s.CaddyConfig `json:"caddy,omitempty" yaml:"caddy,omitempty"`
    
    // NEW: Adoption fields
    Adopt bool `json:"adopt,omitempty" yaml:"adopt,omitempty"`
}
```

#### **Modify: `pkg/clouds/k8s/types.go`** (add Caddy adoption handling)

```go
type CaddyConfig struct {
    // ... existing fields ...
    
    // NEW: Adoption handling for existing Caddy deployments
    AdoptionHandling *CaddyAdoptionHandling `json:"adoptionHandling,omitempty" yaml:"adoptionHandling,omitempty"`
}

type CaddyAdoptionHandling struct {
    SkipDeployment  bool   `json:"skipDeployment,omitempty" yaml:"skipDeployment,omitempty"`
    PatchExisting   bool   `json:"patchExisting,omitempty" yaml:"patchExisting,omitempty"`
    DeploymentName  string `json:"deploymentName,omitempty" yaml:"deploymentName,omitempty"`
}
```

---

## Implementation Checklist

### **Phase 1: MongoDB Atlas** (Estimated: 2-3 days)
- [ ] Create `pkg/clouds/pulumi/mongodb/adopt_cluster.go`
- [ ] Add `Adopt` and `ClusterName` fields to `AtlasConfig`
- [ ] Modify `Cluster()` function to detect and route to adoption
- [ ] Test adoption with existing MongoDB Atlas cluster
- [ ] Verify compute processor works with adopted clusters
- [ ] Update documentation

### **Phase 2: GCP Cloud SQL Postgres** (Estimated: 3-4 days)
- [ ] Create `pkg/clouds/pulumi/gcp/adopt_postgres.go`
- [ ] Add adoption fields to `PostgresGcpCloudsqlConfig`
- [ ] Modify `Postgres()` function to detect and route to adoption
- [ ] Update `PostgresComputeProcessor` to handle secrets.yaml passwords
- [ ] Test K8s Jobs work with adopted Postgres instances
- [ ] Update documentation

### **Phase 3: GKE Autopilot** (Estimated: 4-5 days)
- [ ] Create `pkg/clouds/pulumi/gcp/adopt_gke_autopilot.go`
- [ ] Add adoption fields to `GkeAutopilotResource`
- [ ] Add `CaddyAdoptionHandling` types
- [ ] Modify `GkeAutopilot()` function to detect and route to adoption
- [ ] Implement Caddy detection and patching logic
- [ ] Implement `GkeAutopilotComputeProcessor` (currently stub)
- [ ] Test adoption with existing GKE cluster
- [ ] Update documentation

### **Phase 4: Integration Testing** (Estimated: 3-4 days)
- [ ] End-to-end test: Adopt GKE + Postgres + MongoDB
- [ ] Deploy service using all adopted resources
- [ ] Verify user creation in adopted databases
- [ ] Verify Caddy patching works correctly
- [ ] Performance testing
- [ ] Documentation review

## Total Estimated Time: 12-16 days

## Key Design Decisions

1. **Separate Adoption Files**: Keep adoption logic in dedicated files (`adopt_*.go`) for maintainability
2. **Natural Adoption Flow**: `sc provision` detects `adopt: true` and routes to adoption logic automatically - no separate import command needed
3. **Pulumi Import Pattern**: Use `sdk.Import()` to add existing resources to Pulumi state during provision
4. **Unified Compute Processors**: Compute processors work identically for adopted and provisioned resources
5. **Secrets Management**: Root credentials for adopted resources come from `secrets.yaml`, not generated
6. **Backward Compatibility**: All existing provisioning code remains unchanged, adoption is additive

## Next Steps

1. Review this plan with the team
2. Create GitHub issues for each phase
3. Start with Phase 1 (MongoDB Atlas) as proof of concept
4. Iterate based on learnings from Phase 1
