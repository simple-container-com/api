# Resource Adoption Examples

This directory contains practical examples of adopting existing cloud infrastructure into Simple Container.

## 📋 **Available Examples**

### **Multi-Environment Adoption**
Complete example showing how to adopt existing resources across production, staging, and development environments.

**Includes:**
- MongoDB Atlas clusters (3 environments)
- GCP Cloud SQL Postgres instances (3 environments)  
- GCP Redis Memorystore instances (3 environments)
- GKE Autopilot clusters (3 environments)

**Files:**
- `server.yaml` - Parent stack with resource adoption configuration
- `secrets.yaml` - Template for required secrets and authentication
- `client.yaml` - Example client service using adopted resources
- `docker-compose.yaml` - Service configuration for local development

### **Key Features Demonstrated**

✅ **Zero Downtime Adoption** - Import existing resources without modification
✅ **Multi-Environment Support** - Consistent configuration across environments  
✅ **Resource Compatibility** - Adopted resources work identically to provisioned ones
✅ **Secrets Management** - Secure handling of adoption credentials
✅ **Client Integration** - Services automatically connect to adopted resources

## 🚀 **Quick Start**

1. **Copy the example files** to your project
2. **Update resource identifiers** with your actual resource names/IDs
3. **Configure secrets** with your cloud credentials
4. **Deploy the parent stack** to import resources
5. **Deploy client services** using the adopted resources

```bash
# Copy example to your project
cp -r examples/resource-adoption/* your-project/.sc/

# Configure your actual resource identifiers
vim .sc/stacks/infrastructure/server.yaml

# Add your secrets and credentials  
vim .sc/stacks/infrastructure/secrets.yaml

# Deploy and import existing resources
sc provision -s infrastructure

# Deploy a service using adopted resources
sc deploy -s your-service -e prod
```

## 📖 **Learn More**

- **[Resource Adoption Guide](../../guides/resource-adoption.md)** - Complete documentation
- **[Supported Resources](../../reference/supported-resources.md)** - Full resource reference
- **[Secrets Management](../../guides/secrets-management.md)** - Security best practices

## 💡 **Use Cases**

### **Enterprise Migration**
- Adopt existing production infrastructure
- Gradually migrate services to Simple Container
- Maintain existing resource investments

### **Multi-Cloud Strategy**  
- Adopt resources from different cloud providers
- Unified management across AWS, GCP, Azure
- Consistent deployment workflows

### **Team Onboarding**
- Import existing team infrastructure
- Enable self-service deployments
- Reduce operational complexity

---

**Ready to adopt your infrastructure?** Start with the [Resource Adoption Guide](../../guides/resource-adoption.md)! 🚀
