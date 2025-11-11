# Blockchain Service Example

This example shows how to deploy a blockchain integration service with cross-service dependencies, smart contract integration, and testnet configuration.

## Configuration

- **Type**: ECS cloud-compose deployment
- **Size**: 1024 CPU, 2048MB memory
- **Version**: "10" (high version number for mature service)
- **Database**: MongoDB with cross-service dependency
- **Blockchain**: BNB Chain with Sepolia Linea testnet RPC

## Key Features

- **Cross-Service Dependencies**: References another service's MongoDB resource
- **Dependency Pattern**: `${dependency:backend-service.mongodb.uri}`
- **Blockchain Integration**: Multiple smart contract addresses
- **Testnet Configuration**: Sepolia Linea testnet RPC endpoint
- **External APIs**: Brevo email, Claimr, Telegram bot integration
- **Smart Contracts**: NFT, Token, and Staking contract addresses

## Environments

- **Staging**: `staging-blockchain.example.com`
- **Production**: `blockchain.example.com`

## Cross-Service Dependencies

This service depends on the `backend-service` for MongoDB access:
```yaml
dependencies:
  - name: backend-service
    owner: backend-service/backend-service
    resource: mongodb
```

## Blockchain Configuration

- **Network**: BNB Chain (Sepolia Linea testnet)
- **RPC Endpoint**: `https://rpc.sepolia.linea.build`
- **Contracts**: NFT, Token, Staking contracts
- **Staking**: BNB staking integration

## Usage

1. Deploy the `backend-service` first (dependency requirement)
2. Configure smart contract addresses for your testnet
3. Set up Telegram bot and email service credentials
4. Deploy to staging for blockchain testing
5. Promote to production with mainnet contracts

## Parent Stack Requirements

This example requires a parent stack that provides:
- ECS deployment capabilities
- Cross-service dependency resolution
- Secrets management for contract addresses
- Domain management
