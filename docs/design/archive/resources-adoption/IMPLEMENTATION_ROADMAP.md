# Resource Adoption Implementation Roadmap

## 🎯 **Implementation Ready Status**

✅ **Schema Validation Complete** - All configurations validated against actual Simple Container schemas  
✅ **Export Compatibility Verified** - Pulumi export patterns confirmed compatible  
✅ **Compute Processor Analysis Done** - 90% reuse confirmed, minimal changes needed  
✅ **Real Customer Configuration Ready** - Complete 3-environment setup documented  
✅ **Architecture Validated** - Single parent stack, single state bucket approach confirmed  

## 📋 **Short-Term Implementation Plan (2-3 Weeks)**

### **Phase 1: MongoDB Atlas Adoption (Days 1-3)** 🥇
**Priority: HIGH** | **Risk: LOW** | **Effort: 2-3 days**

#### **Why Start Here**
- ✅ **Zero compute processor changes** needed
- ✅ **Already uses Pulumi provider** (`mongodbatlas.NewDatabaseUser`)
- ✅ **Simplest adoption case** - proves the concept
- ✅ **High confidence** - validated against actual schemas

#### **Implementation Tasks**
```
Day 1: Core Adoption Logic
□ Add `adopt` and `clusterName` fields to AtlasConfig struct
□ Implement adopt_cluster.go with sdk.Import() pattern
□ Use identical export naming functions (toProjectIdExport, toClusterIdExport)
□ Register adoption function in init.go

Day 2: Integration & Testing
□ Test adoption with real MongoDB Atlas cluster
□ Verify exports match provisioned cluster exports
□ Test compute processor works unchanged
□ Validate service receives correct environment variables

Day 3: Documentation & Polish
□ Update JSON schemas with welder
□ Add adoption examples to official docs
□ Create migration guide for existing MongoDB clusters
```

#### **Success Criteria**
- [ ] `adopt: true` MongoDB cluster imported into Pulumi state
- [ ] Compute processor creates database users via Atlas API
- [ ] Client services receive identical environment variables
- [ ] No changes to existing provisioning flow

---

### **Phase 2: GCP Cloud SQL Postgres Adoption (Days 4-7)** 🥈
**Priority: HIGH** | **Risk: LOW-MEDIUM** | **Effort: 3-4 days**

#### **Why Second**
- ✅ **Minor compute processor changes** needed
- ✅ **Well-understood pattern** - K8s Jobs for user creation
- ✅ **Schema validated** - all fields confirmed
- ⚠️ **Secrets.yaml integration** - read root password from config

#### **Implementation Tasks**
```
Day 4: Schema & Core Logic
□ Add `adopt`, `instanceName`, `connectionName` fields to PostgresGcpCloudsqlConfig
□ Implement adopt_postgres.go with Cloud SQL import
□ Use identical export naming (toPostgresRootPasswordExport)

Day 5: Compute Processor Enhancement
□ Modify compute processor to read root credentials from secrets.yaml
□ Test K8s Job creation with adopted instance
□ Verify database and user creation works

Day 6-7: Integration & Testing
□ Test with real Cloud SQL instance
□ Verify connection from GKE cluster
□ Test service deployment and database access
□ Update schemas and documentation
```

#### **Success Criteria**
- [ ] Cloud SQL instance imported without modification
- [ ] K8s Jobs create databases and users successfully
- [ ] Services connect to adopted Postgres instances
- [ ] Root credentials read from configuration

---

### **Phase 3: GCP Redis Adoption (Days 8-9)** 🥉
**Priority: MEDIUM** | **Risk: LOW** | **Effort: 1-2 days**

#### **Why Third**
- ✅ **Simple connection string injection**
- ✅ **No user creation needed** (shared AUTH token)
- ✅ **Minimal compute processor changes**

#### **Implementation Tasks**
```
Day 8: Implementation
□ Add `adopt` and `instanceId` fields to RedisConfig
□ Implement adopt_redis.go
□ Update compute processor for connection string injection

Day 9: Testing & Documentation
□ Test with real Redis instances
□ Verify connection from services
□ Update schemas and docs
```

---

### **Phase 4: GKE Autopilot Adoption (Days 10-14)** 🎯
**Priority: HIGH** | **Risk: MEDIUM** | **Effort: 4-5 days**

#### **Why Last**
- ⚠️ **Most complex** - compute processor needs implementation
- ⚠️ **Caddy handling** - detection and patching logic needed
- ⚠️ **Template integration** - GKE is used as template base

#### **Implementation Tasks**
```
Day 10-11: Core Adoption
□ Add `adopt` and `clusterName` fields to GkeAutopilotResource
□ Implement adopt_gke_autopilot.go
□ Handle kubeconfig export for adopted clusters

Day 12-13: Compute Processor Implementation
□ Implement GKE Autopilot compute processor (currently stub)
□ Add Caddy detection and patching logic
□ Test workload deployment to adopted clusters

Day 14: Integration Testing
□ End-to-end testing with real GKE clusters
□ Verify Caddy adoption handling works
□ Test service deployment and ingress
```

#### **Success Criteria**
- [ ] GKE cluster imported without modification
- [ ] Existing Caddy deployments detected and patched
- [ ] Services deploy successfully to adopted clusters
- [ ] Ingress and networking work correctly

---

## 🔧 **Implementation Architecture**

### **File Structure Pattern**
```
pkg/clouds/pulumi/[provider]/
├── cluster.go           # Existing provisioning
├── adopt_cluster.go     # NEW - Adoption logic
├── compute_proc.go      # Existing (minimal changes)
└── init.go             # Register adoption functions
```

### **Adoption Function Pattern**
```go
// adopt_cluster.go
func AdoptCluster(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
    config := input.Config.(mongodb.AtlasConfig)
    
    if !config.Adopt {
        return nil, fmt.Errorf("adopt flag not set")
    }
    
    // Import existing resource
    cluster, err := sdk.Import(ctx, config.ClusterName, mongodbatlas.Cluster{}, ...)
    
    // Use IDENTICAL export naming as provisioning
    projectName := toProjectName(stack.Name, input)
    clusterName := toClusterName(stack.Name, input)
    
    ctx.Export(toProjectIdExport(projectName), cluster.ProjectId)
    ctx.Export(toClusterIdExport(clusterName), cluster.ClusterId)
    // ... identical exports
    
    return &api.ResourceOutput{}, nil
}
```

### **Registration Pattern**
```go
// init.go
func init() {
    api.RegisterResources(map[string]pApi.ProvisionFunc{
        "mongodb-atlas": func(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
            config := input.Config.(mongodb.AtlasConfig)
            if config.Adopt {
                return AdoptCluster(ctx, stack, input, params)  // NEW
            }
            return Cluster(ctx, stack, input, params)           // Existing
        },
    })
}
```

---

## 🧪 **Testing Strategy**

### **Phase 1 Testing (MongoDB)**
```bash
# 1. Create test MongoDB Atlas cluster manually
# 2. Configure adoption in server.yaml
# 3. Run provision and verify import
sc provision -s infrastructure
# 4. Deploy test service and verify database access
sc deploy -s test-service -e prod
```

### **Integration Testing**
- [ ] Test with REAL_CUSTOMER_ADOPTION.md configuration
- [ ] Verify all 12 resources adopted correctly
- [ ] Test service deployments to all environments
- [ ] Validate environment variable injection

### **Regression Testing**
- [ ] Ensure existing provisioning flows unchanged
- [ ] Verify non-adoption resources still work
- [ ] Test mixed adoption/provisioning scenarios

---

## 📊 **Success Metrics**

### **Technical Metrics**
- [ ] All 4 resource types support adoption
- [ ] 100% compute processor compatibility maintained
- [ ] Zero breaking changes to existing flows
- [ ] Schema compliance maintained

### **Customer Metrics**
- [ ] Real customer infrastructure adopted successfully
- [ ] Service deployments work across all environments
- [ ] Migration completed without downtime
- [ ] Documentation enables self-service adoption

---

## 🚀 **Getting Started**

### **Immediate Next Steps**
1. **Create GitHub Issues** - One per phase
2. **Set up Development Branch** - `feature/resource-adoption`
3. **Begin Phase 1** - MongoDB Atlas adoption
4. **Daily Standups** - Track progress and blockers

### **Development Environment**
```bash
# 1. Checkout feature branch
git checkout -b feature/resource-adoption

# 2. Set up test MongoDB Atlas cluster
# 3. Configure test environment with adoption flags
# 4. Begin implementation following the roadmap
```

---

## 📋 **Risk Mitigation**

### **Technical Risks**
| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Export key mismatch | Low | High | Use identical naming functions |
| Compute processor breaks | Low | High | Start with MongoDB (no changes) |
| Schema validation fails | Low | Medium | All schemas pre-validated |
| Pulumi import issues | Medium | Medium | Test with simple resources first |

### **Timeline Risks**
- **Buffer Days**: Built into each phase
- **Parallel Work**: Documentation can be done alongside implementation
- **Rollback Plan**: Feature flags allow disabling adoption per resource

---

**Status**: Ready to begin implementation  
**Next Action**: Create GitHub issues and begin Phase 1 (MongoDB Atlas)  
**Timeline**: 2-3 weeks for complete implementation  
**Confidence**: HIGH (based on comprehensive validation)
