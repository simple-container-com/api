# Billing System Example

This example shows how to deploy a billing system with multi-environment configuration using YAML anchors and parent environment inheritance.

## Configuration

- **Type**: Lambda single-image deployment
- **Template**: Uses `lambda-eu` template from parent stack
- **Timeout**: 300 seconds (5 minutes) for billing operations
- **Memory**: 512MB for processing billing data
- **Database**: MongoDB integration for billing records
- **Multi-Environment**: staging, test, beta, prod with YAML anchors

## Key Features

- **Multi-Environment with YAML Anchors**: Efficient configuration reuse across environments
- **Parent Environment Inheritance**: `beta` uses `parentEnv: prod` for production-like testing
- **Long Timeout**: 5 minutes for complex billing calculations and external API calls
- **Domain Pattern**: `{env}-billing.example.com` structure for easy identification
- **MongoDB Integration**: Billing records and transaction storage

## Environments

- **Staging**: `staging-billing.example.com` - Development testing
- **Test**: `test-billing.example.com` - QA testing
- **Beta**: `beta-billing.example.com` - Production-like testing (inherits from prod)
- **Production**: `billing.example.com` - Live billing operations

## Parent Environment Pattern

The beta environment uses `parentEnv: prod` to inherit production resources while maintaining separate configuration:
```yaml
beta:
  <<: *stack
  parentEnv: prod  # Inherits prod resources
  config:
    domain: beta-billing.example.com
```

## Use Cases

- **Invoice Generation**: Process customer invoices with complex calculations
- **Payment Processing**: Handle payment transactions and reconciliation
- **Subscription Management**: Manage recurring billing cycles
- **Tax Calculations**: Complex tax calculations for multiple jurisdictions
- **External API Integration**: Payment gateways, tax services, accounting systems

## Usage

1. Ensure your parent stack provides MongoDB resource for billing data
2. Configure API keys for payment gateway integration
3. Deploy to staging and test environments first
4. Use beta environment for production-like testing
5. Promote to production when billing operations are validated

## Parent Stack Requirements

This example requires a parent stack that provides:
- `lambda-eu` template with extended timeout support
- `mongodb` resource for billing data storage
- Multi-environment support with parent environment inheritance
- Secrets management for payment gateway API keys

## Security Considerations

- **PCI Compliance**: Ensure proper handling of payment data
- **Encryption**: All billing data encrypted at rest and in transit
- **Access Control**: Restricted access to billing functions
- **Audit Logging**: Complete audit trail for all billing operations
