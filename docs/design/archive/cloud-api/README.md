# Simple Container Cloud API

This directory contains the comprehensive design documentation for the Simple Container Cloud API - a REST API service that provides web-based management capabilities for Simple Container infrastructure and application deployments.

## Overview

The Simple Container Cloud API transforms the CLI-based Simple Container experience into a web-accessible, multi-tenant service that enables:

- **Infrastructure Managers** to create and manage parent stacks (shared infrastructure)
- **Developers** to deploy and manage client stacks (applications that consume infrastructure)
- **Organizations** to manage multiple users with role-based access control
- **Cloud Integration** with automated service account provisioning and management

## Design Documents

### Core Architecture
- [**System Architecture**](./01-system-architecture.md) - Overall service design, technology stack, and component interactions
- [**Database Design**](./02-database-design.md) - MongoDB schema for multi-tenant data storage and RBAC
- [**Authentication & RBAC**](./03-authentication-rbac.md) - User authentication, authorization, and role-based access control

### API Specifications  
- [**REST API Specification**](./04-rest-api-specification.md) - Complete API endpoints, request/response schemas, and operation flows
- [**Stack Management APIs**](./05-stack-management-apis.md) - Detailed stack lifecycle management operations

### Integration & Deployment
- [**Cloud Integrations**](./06-cloud-integrations.md) - AWS/GCP service account automation and resource provisioning
- [**Security & Compliance**](./07-security-compliance.md) - Security model, data protection, and compliance considerations
- [**Deployment Architecture**](./08-deployment-architecture.md) - Service deployment patterns and infrastructure requirements

## Key Features

### Multi-Tenant Architecture
- **Organizations** - Companies with multiple users and projects
- **Users** - Individual team members with specific roles and permissions
- **Projects** - Logical groupings of parent and client stacks
- **RBAC** - Fine-grained permissions for infrastructure vs application management

### Simple Container Integration
- **Parent Stack Management** - Web interface for DevOps teams to define infrastructure templates and resources
- **Client Stack Management** - Developer-friendly interface for application deployment and configuration
- **Real-time Status** - Live monitoring of provisioning and deployment operations
- **Resource Discovery** - Automatic detection and cataloging of existing cloud resources

### Cloud Provider Integration
- **Automated Provisioning** - Service account creation and IAM configuration upon user authentication
- **Multi-Cloud Support** - AWS and GCP integration with extensible architecture for additional providers
- **Resource Adoption** - Discovery and management of existing cloud infrastructure

## Development Phases

### Phase 1: Core Service (MVP)
- Basic authentication with Google OAuth
- MongoDB database setup with core schemas
- Parent stack CRUD operations
- Client stack CRUD operations
- Basic RBAC (infrastructure managers vs developers)

### Phase 2: Cloud Integration
- Automated GCP service account provisioning
- AWS IAM integration
- Resource discovery and adoption
- Real-time provisioning status

### Phase 3: Advanced Features
- Advanced RBAC with custom roles
- Multi-organization support
- Audit logging and compliance
- Advanced monitoring and alerting

## Getting Started

1. Review the [System Architecture](./01-system-architecture.md) for overall design understanding
2. Examine the [Database Design](./02-database-design.md) for data modeling
3. Study the [REST API Specification](./04-rest-api-specification.md) for implementation details
4. Follow the implementation guidelines in each design document

## Technology Stack

- **Backend**: Go (Gin framework)
- **Database**: MongoDB with transaction support
- **Authentication**: Google OAuth 2.0, JWT tokens
- **Cloud SDKs**: AWS SDK, Google Cloud SDK
- **Simple Container**: Integration with existing CLI and provisioning engine
- **Deployment**: Docker containers, Kubernetes-ready
