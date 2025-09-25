# Vector Database Example

This example shows how to deploy a high-performance vector database with Network Load Balancer and auto-scaling configuration.

## Configuration

- **Type**: ECS cloud-compose deployment
- **Size**: 1024 CPU, 2048MB memory
- **Scaling**: Min 1, Max 3 instances with 70% CPU threshold
- **Load Balancer**: Network Load Balancer (NLB) for high performance
- **Domain**: Not proxied through Cloudflare for direct access

## Key Features

- **Network Load Balancer**: `loadBalancerType: "nlb"` for high-performance vector operations
- **Direct Access**: `domainProxied: false` bypasses CDN for database connections
- **Auto-scaling**: Configured for vector database workloads
- **Multi-Environment**: Staging and production deployments

## Environments

- **Staging**: `staging-vectordb.example.com`
- **Production**: `vectordb.example.com`

## Performance Considerations

- NLB provides lower latency for database operations
- Direct domain access avoids CDN caching issues
- CPU threshold at 70% allows for burst workloads
- Minimum 1 replica for cost optimization, scales up to 3

## Usage

1. Ensure your Docker Compose includes vector database service
2. Configure NLB for optimal performance
3. Deploy to staging for performance testing
4. Promote to production when performance is validated

## Parent Stack Requirements

This example requires a parent stack that provides:
- ECS deployment capabilities with NLB support
- Domain management with proxy control
- Performance monitoring for scaling decisions
