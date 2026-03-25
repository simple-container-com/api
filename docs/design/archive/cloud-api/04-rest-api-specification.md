# Simple Container Cloud API - REST API Specification

## Overview

The Simple Container Cloud API provides a comprehensive RESTful interface for managing multi-tenant Simple Container deployments. The API follows OpenAPI 3.0 specifications and implements standard HTTP methods with JSON request/response bodies.

## API Design Principles

### Base URL Structure
```
https://api.simple-container.com/api/v1
```

### Versioning Strategy
- **URL Path Versioning**: `/api/v1/`, `/api/v2/`
- **Backward Compatibility**: Maintained within major versions
- **Deprecation Policy**: 6-month notice with migration guides

### Resource Naming Conventions
- **Plural Nouns**: `/organizations`, `/parent-stacks`, `/client-stacks`
- **Hierarchical Structure**: `/organizations/{org_id}/projects/{project_id}/parent-stacks`
- **Kebab Case**: Multi-word resources use kebab-case (`parent-stacks`, not `parentStacks`)

### HTTP Status Codes
- **200 OK**: Successful GET, PUT, PATCH operations
- **201 Created**: Successful POST operations
- **204 No Content**: Successful DELETE operations
- **400 Bad Request**: Invalid request format or parameters
- **401 Unauthorized**: Authentication required or invalid
- **403 Forbidden**: Authenticated but insufficient permissions
- **404 Not Found**: Resource not found
- **409 Conflict**: Resource conflict (e.g., duplicate name)
- **422 Unprocessable Entity**: Valid format but business logic error
- **429 Too Many Requests**: Rate limit exceeded
- **500 Internal Server Error**: Server-side error

## Authentication

### Bearer Token Authentication
All API endpoints require authentication via JWT Bearer tokens in the Authorization header.

```http
Authorization: Bearer <jwt_token>
```

### Token Structure
```json
{
  "user_id": "user_123",
  "email": "user@example.com",
  "org_id": "org_456", 
  "role": "infrastructure_manager",
  "permissions": ["parent_stacks.*", "resources.*"],
  "exp": 1640995200
}
```

## Global Response Format

### Success Response
```json
{
  "success": true,
  "data": {
    // Response payload
  },
  "meta": {
    "timestamp": "2024-01-15T10:30:00Z",
    "request_id": "req_789abc123",
    "api_version": "v1"
  }
}
```

### Error Response
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid stack configuration",
    "details": {
      "field": "server_config.resources",
      "reason": "Missing required resource definition"
    }
  },
  "meta": {
    "timestamp": "2024-01-15T10:30:00Z",
    "request_id": "req_789abc123",
    "api_version": "v1"
  }
}
```

### Pagination Response
```json
{
  "success": true,
  "data": [
    // Array of resources
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 156,
    "total_pages": 8,
    "has_next": true,
    "has_prev": false
  },
  "meta": {
    "timestamp": "2024-01-15T10:30:00Z",
    "request_id": "req_789abc123"
  }
}
```

## Core API Endpoints

### 1. Authentication & User Management

#### Login with Google OAuth
```http
POST /api/v1/auth/google
Content-Type: application/json

{
  "authorization_code": "4/0AX4XfWh...",
  "redirect_uri": "https://app.simple-container.com/auth/callback"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 3600,
    "token_type": "Bearer",
    "user": {
      "id": "user_123",
      "email": "john@example.com",
      "name": "John Doe",
      "avatar_url": "https://...",
      "organizations": [
        {
          "id": "org_456",
          "name": "Acme Corp",
          "role": "infrastructure_manager",
          "permissions": ["parent_stacks.*", "resources.*"]
        }
      ]
    }
  }
}
```

#### Refresh Token
```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

#### Get Current User
```http
GET /api/v1/user
Authorization: Bearer <token>
```

#### Update User Profile
```http
PATCH /api/v1/user
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "John Smith",
  "preferences": {
    "theme": "dark",
    "timezone": "America/New_York",
    "notifications": {
      "email_deployments": true,
      "email_failures": true
    }
  }
}
```

### 2. Organization Management

#### List Organizations
```http
GET /api/v1/organizations
Authorization: Bearer <token>
```

#### Get Organization Details
```http
GET /api/v1/organizations/{org_id}
Authorization: Bearer <token>
```

#### Update Organization
```http
PUT /api/v1/organizations/{org_id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Acme Corporation",
  "description": "Software development company",
  "settings": {
    "default_cloud_providers": ["aws", "gcp"],
    "require_mfa": true,
    "audit_retention_days": 365
  }
}
```

#### List Organization Members
```http
GET /api/v1/organizations/{org_id}/members
Authorization: Bearer <token>
```

#### Invite User to Organization
```http
POST /api/v1/organizations/{org_id}/members
Authorization: Bearer <token>
Content-Type: application/json

{
  "email": "newuser@example.com",
  "role": "developer",
  "permissions": ["client_stacks.*"]
}
```

### 3. Project Management

#### List Projects
```http
GET /api/v1/organizations/{org_id}/projects
Authorization: Bearer <token>
Query Parameters:
  - page (optional): Page number (default: 1)
  - per_page (optional): Items per page (default: 20, max: 100)
  - status (optional): Filter by status ("active", "archived")
```

#### Create Project
```http
POST /api/v1/organizations/{org_id}/projects
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "E-commerce Platform",
  "slug": "ecommerce-platform",
  "description": "Main e-commerce application and infrastructure",
  "settings": {
    "environments": ["development", "staging", "production"],
    "default_cloud_provider": "gcp",
    "auto_deploy": false,
    "require_approval": true
  },
  "git_repository": {
    "url": "https://github.com/company/ecommerce-platform",
    "branch": "main",
    "path": ".sc",
    "auto_sync": true
  }
}
```

#### Get Project Details
```http
GET /api/v1/organizations/{org_id}/projects/{project_id}
Authorization: Bearer <token>
```

#### Update Project
```http
PUT /api/v1/organizations/{org_id}/projects/{project_id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "E-commerce Platform Updated",
  "description": "Updated description",
  "settings": {
    "environments": ["development", "staging", "production", "preview"],
    "require_approval": false
  }
}
```

#### Delete Project
```http
DELETE /api/v1/organizations/{org_id}/projects/{project_id}
Authorization: Bearer <token>
```

### 4. Parent Stack Management

#### List Parent Stacks
```http
GET /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks
Authorization: Bearer <token>
Query Parameters:
  - page (optional): Page number
  - per_page (optional): Items per page  
  - environment (optional): Filter by deployment environment
  - status (optional): Filter by deployment status
```

#### Create Parent Stack
```http
POST /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "infrastructure",
  "display_name": "Main Infrastructure Stack",
  "description": "Shared infrastructure for all environments",
  "server_config": {
    "schemaVersion": "1.0",
    "provisioner": {
      "type": "pulumi",
      "config": {
        "state-storage": {
          "type": "gcp-bucket",
          "config": {
            "credentials": "${auth:gcloud}",
            "projectId": "${auth:gcloud.projectId}",
            "name": "infrastructure-state",
            "location": "us-central1"
          }
        }
      }
    },
    "templates": {
      "gke-stack": {
        "type": "gcp-gke-autopilot",
        "config": {
          "projectId": "${auth:gcloud.projectId}",
          "credentials": "${auth:gcloud}",
          "gkeClusterResource": "main-cluster",
          "artifactRegistryResource": "main-registry"
        }
      }
    },
    "resources": {
      "resources": {
        "production": {
          "template": "gke-stack",
          "resources": {
            "main-cluster": {
              "type": "gcp-gke-autopilot-cluster",
              "config": {
                "projectId": "${auth:gcloud.projectId}",
                "credentials": "${auth:gcloud}",
                "location": "us-central1",
                "gkeMinVersion": "1.33.4-gke.1245000"
              }
            },
            "main-registry": {
              "type": "gcp-artifact-registry", 
              "config": {
                "projectId": "${auth:gcloud.projectId}",
                "credentials": "${auth:gcloud}",
                "location": "us-central1"
              }
            }
          }
        }
      }
    }
  }
}
```

#### Get Parent Stack
```http
GET /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks/{stack_id}
Authorization: Bearer <token>
```

#### Update Parent Stack Configuration
```http
PUT /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks/{stack_id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "display_name": "Updated Infrastructure Stack",
  "description": "Updated description",
  "server_config": {
    // Complete updated server.yaml configuration
  }
}
```

#### Delete Parent Stack
```http
DELETE /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks/{stack_id}
Authorization: Bearer <token>
Query Parameters:
  - force (optional): Force deletion even if client stacks depend on it
```

### 5. Client Stack Management

#### List Client Stacks
```http
GET /api/v1/organizations/{org_id}/projects/{project_id}/client-stacks
Authorization: Bearer <token>
Query Parameters:
  - parent_stack_id (optional): Filter by parent stack
  - environment (optional): Filter by deployment environment
  - status (optional): Filter by deployment status
```

#### Create Client Stack
```http
POST /api/v1/organizations/{org_id}/projects/{project_id}/client-stacks
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "web-app",
  "display_name": "E-commerce Web Application",
  "description": "Frontend web application for e-commerce platform",
  "parent_stack_id": "parent_stack_456",
  "parent_environment": "production",
  "client_config": {
    "schemaVersion": "1.0",
    "stacks": {
      "production": {
        "type": "cloud-compose",
        "parent": "infrastructure",
        "config": {
          "uses": ["main-cluster", "main-registry"],
          "runs": ["web-app"],
          "domain": "app.example.com",
          "env": {
            "NODE_ENV": "production",
            "API_URL": "https://api.example.com"
          }
        }
      }
    }
  },
  "docker_compose": {
    "version": "3.8",
    "services": {
      "web-app": {
        "build": ".",
        "ports": ["8080:8080"],
        "environment": {
          "NODE_ENV": "${NODE_ENV}"
        },
        "labels": {
          "simple-container.com/ingress": "true",
          "simple-container.com/ingress/port": "8080"
        }
      }
    }
  },
  "dockerfile_content": "FROM node:18-alpine\nWORKDIR /app\nCOPY package*.json ./\nRUN npm install\nCOPY . .\nEXPOSE 8080\nCMD [\"npm\", \"start\"]",
  "git_repository": {
    "url": "https://github.com/company/web-app",
    "branch": "main",
    "dockerfile_path": "Dockerfile",
    "compose_path": "docker-compose.yaml"
  }
}
```

#### Get Client Stack
```http
GET /api/v1/organizations/{org_id}/projects/{project_id}/client-stacks/{stack_id}
Authorization: Bearer <token>
```

#### Update Client Stack
```http
PUT /api/v1/organizations/{org_id}/projects/{project_id}/client-stacks/{stack_id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "display_name": "Updated Web Application",
  "client_config": {
    // Updated client.yaml configuration
  },
  "docker_compose": {
    // Updated docker-compose.yaml
  }
}
```

### 6. Stack Operations

#### Provision Parent Stack
```http
POST /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks/{stack_id}/provision
Authorization: Bearer <token>
Content-Type: application/json

{
  "environment": "production",
  "skip_preview": false,
  "timeout_minutes": 30
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "operation_id": "op_789def456",
    "status": "pending",
    "started_at": "2024-01-15T10:30:00Z",
    "estimated_duration": "15 minutes",
    "github_repository": "organization/infrastructure-stack",
    "workflow_dispatch": {
      "dispatched_at": "2024-01-15T10:30:05Z",
      "event_type": "provision-infrastructure",
      "workflow_run_url": "https://github.com/organization/infrastructure-stack/actions/runs/123456"
    },
    "progress": {
      "phase": "workflow_dispatched",
      "step": "waiting_for_github_workflow",
      "completion_percent": 0,
      "message": "GitHub Actions workflow dispatched, waiting for execution to start..."
    }
  }
}
```

#### Deploy Client Stack
```http
POST /api/v1/organizations/{org_id}/projects/{project_id}/client-stacks/{stack_id}/deploy
Authorization: Bearer <token>
Content-Type: application/json

{
  "environment": "production",
  "git_commit": "a1b2c3d4e5f6",
  "rollback_on_failure": true,
  "timeout_minutes": 15
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "operation_id": "op_456def789",
    "status": "pending",
    "started_at": "2024-01-15T10:30:00Z",
    "github_repository": "developer/web-application",
    "workflow_dispatch": {
      "dispatched_at": "2024-01-15T10:30:05Z",
      "event_type": "deploy-service",
      "workflow_run_url": "https://github.com/developer/web-application/actions/runs/789012"
    }
  }
}
```

#### Get Operation Status
```http
GET /api/v1/operations/{operation_id}
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "operation_id": "op_789def456",
    "type": "provision_parent",
    "status": "completed",
    "started_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:45:00Z",
    "duration": "15m23s",
    "result": "success",
    "github_workflow": {
      "repository": "organization/infrastructure-stack",
      "workflow_run_id": "123456",
      "workflow_run_url": "https://github.com/organization/infrastructure-stack/actions/runs/123456",
      "status": "completed",
      "conclusion": "success"
    },
    "progress": {
      "phase": "completed",
      "completion_percent": 100,
      "message": "Infrastructure provisioned successfully via GitHub Actions"
    },
    "logs": [
      {
        "timestamp": "2024-01-15T10:30:05Z",
        "level": "info",
        "source": "github_workflow",
        "message": "GitHub Actions workflow started"
      },
      {
        "timestamp": "2024-01-15T10:35:12Z",
        "level": "info",
        "source": "sc_engine", 
        "message": "GKE cluster provisioned successfully"
      }
    ],
    "resources_created": [
      {
        "resource_id": "gke_cluster_abc123",
        "resource_type": "gcp-gke-autopilot-cluster",
        "resource_name": "main-cluster",
        "status": "healthy"
      }
    ]
  }
}
```

#### Cancel Operation
```http
POST /api/v1/operations/{operation_id}/cancel
Authorization: Bearer <token>
```

### 7. Resource Discovery & Management

#### Discover Cloud Resources
```http
POST /api/v1/organizations/{org_id}/resources/discover
Authorization: Bearer <token>
Content-Type: application/json

{
  "cloud_provider": "gcp",
  "regions": ["us-central1", "us-east1"],
  "resource_types": ["gcp-gke-autopilot-cluster", "gcp-bucket", "gcp-cloudsql-postgres"],
  "filters": {
    "tags": {
      "environment": "production"
    }
  }
}
```

#### List Discovered Resources
```http
GET /api/v1/organizations/{org_id}/resources
Authorization: Bearer <token>
Query Parameters:
  - cloud_provider (optional): Filter by provider
  - resource_type (optional): Filter by type
  - managed_by_sc (optional): Filter by SC management status
  - region (optional): Filter by region
```

#### Adopt Existing Resource
```http
POST /api/v1/organizations/{org_id}/resources/{resource_id}/adopt
Authorization: Bearer <token>
Content-Type: application/json

{
  "parent_stack_id": "parent_stack_456",
  "stack_environment": "production",
  "configuration_overrides": {
    // Any configuration adjustments needed for adoption
  }
}
```

### 8. Secrets Management

#### List Stack Secrets (Metadata Only)
```http
GET /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks/{stack_id}/secrets
Authorization: Bearer <token>
Query Parameters:
  - environment (optional): Filter by environment
```

#### Update Stack Secrets
```http
PUT /api/v1/organizations/{org_id}/projects/{project_id}/parent-stacks/{stack_id}/secrets/{environment}
Authorization: Bearer <token>
Content-Type: application/json

{
  "encrypted_secrets": {
    "schemaVersion": "1.0",
    "auth": {
      "gcloud": {
        "type": "gcp-service-account",
        "config": {
          "projectId": "my-gcp-project",
          "credentials": "encrypted_service_account_json"
        }
      }
    },
    "values": {
      "DATABASE_PASSWORD": "encrypted_password_value",
      "API_KEY": "encrypted_api_key_value"
    }
  }
}
```

### 9. GitHub Integration Management

#### Authorize GitHub Repository
```http
POST /api/v1/organizations/{org_id}/github/repositories/authorize
Authorization: Bearer <token>
Content-Type: application/json

{
  "repository_owner": "organization",
  "repository_name": "infrastructure-stack",
  "purpose": "infrastructure",
  "permissions": ["contents:write", "actions:write"]
}
```

#### List Authorized Repositories
```http
GET /api/v1/organizations/{org_id}/github/repositories
Authorization: Bearer <token>
Query Parameters:
  - purpose (optional): Filter by purpose (infrastructure/deployment)
```

#### Get Workflow Status
```http
GET /api/v1/github/repositories/{repo_owner}/{repo_name}/workflows/{workflow_run_id}
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "workflow_run_id": "123456",
    "status": "completed",
    "conclusion": "success",
    "started_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:45:00Z",
    "html_url": "https://github.com/organization/infrastructure-stack/actions/runs/123456",
    "workflow_name": "Provision Infrastructure",
    "event": "repository_dispatch"
  }
}
```

#### Download Stack Configuration (For Workflows)
```http
GET /api/v1/workflows/stacks/{stack_id}/config
Authorization: Bearer <workflow_token>
Query Parameters:
  - environment: Target environment
  - stack_type: parent or client
```

**Response:**
```json
{
  "success": true,
  "data": {
    "stack_name": "infrastructure",
    "environment": "production",
    "server_config": {
      "schemaVersion": "1.0",
      "provisioner": {
        "type": "pulumi"
      },
      "resources": {
        // Complete server.yaml configuration
      }
    },
    "secrets": {
      // Decrypted secrets for the environment
    }
  }
}
```

#### Report Workflow Progress (From Workflows)
```http
POST /api/v1/workflows/operations/{operation_id}/progress
Authorization: Bearer <workflow_token>
Content-Type: application/json

{
  "phase": "provisioning",
  "current_step": "creating_gke_cluster",
  "completion_percent": 45,
  "message": "Creating GKE Autopilot cluster in us-central1",
  "workflow_run_id": "123456"
}
```

### 10. Cloud Account Management

#### List Cloud Accounts
```http
GET /api/v1/organizations/{org_id}/cloud-accounts
Authorization: Bearer <token>
```

#### Get Cloud Account Status
```http
GET /api/v1/organizations/{org_id}/cloud-accounts/{account_id}
Authorization: Bearer <token>
```

#### Refresh Cloud Account Credentials
```http
POST /api/v1/organizations/{org_id}/cloud-accounts/{account_id}/refresh
Authorization: Bearer <token>
```

### 10. Audit & Monitoring

#### Get Audit Logs
```http
GET /api/v1/organizations/{org_id}/audit-logs
Authorization: Bearer <token>
Query Parameters:
  - event_type (optional): Filter by event type
  - actor_id (optional): Filter by user ID
  - start_date (optional): Filter from date (ISO 8601)
  - end_date (optional): Filter to date (ISO 8601)
  - page, per_page: Pagination
```

#### Get Organization Usage Statistics
```http
GET /api/v1/organizations/{org_id}/usage
Authorization: Bearer <token>
Query Parameters:
  - period (optional): "day", "week", "month" (default: "month")
  - start_date (optional): Custom period start
  - end_date (optional): Custom period end
```

**Response:**
```json
{
  "success": true,
  "data": {
    "period": {
      "start": "2024-01-01T00:00:00Z",
      "end": "2024-01-31T23:59:59Z"
    },
    "metrics": {
      "api_requests": 45230,
      "provisioning_operations": 123,
      "deployment_operations": 456,
      "active_parent_stacks": 5,
      "active_client_stacks": 23,
      "cloud_resources_managed": 87,
      "estimated_monthly_cost": 1250.75
    },
    "breakdown": {
      "by_user": [
        {
          "user_id": "user_123",
          "user_name": "John Doe",
          "api_requests": 12340,
          "operations": 45
        }
      ],
      "by_cloud_provider": {
        "gcp": {
          "resources": 52,
          "estimated_cost": 890.25
        },
        "aws": {
          "resources": 35,
          "estimated_cost": 360.50
        }
      }
    }
  }
}
```

## Error Codes

### Authentication Errors
- `AUTH_REQUIRED`: Authentication token required
- `AUTH_INVALID`: Invalid or expired token
- `AUTH_INSUFFICIENT`: Insufficient permissions

### Validation Errors  
- `VALIDATION_ERROR`: General validation error
- `INVALID_FORMAT`: Invalid request format
- `REQUIRED_FIELD`: Required field missing
- `INVALID_VALUE`: Invalid field value

### Business Logic Errors
- `RESOURCE_NOT_FOUND`: Resource not found
- `RESOURCE_CONFLICT`: Resource name conflict
- `DEPENDENCY_ERROR`: Resource dependency conflict
- `OPERATION_IN_PROGRESS`: Conflicting operation in progress

### System Errors
- `INTERNAL_ERROR`: Server-side error
- `SERVICE_UNAVAILABLE`: Service temporarily unavailable
- `RATE_LIMIT_EXCEEDED`: Too many requests

## WebSocket API for Real-Time Updates

### Connection
```javascript
const ws = new WebSocket('wss://api.simple-container.com/ws?token=<jwt_token>');
```

### Event Types
```json
// Operation progress updates
{
  "type": "operation_progress",
  "data": {
    "operation_id": "op_789def456",
    "status": "running",
    "progress": {
      "completion_percent": 45,
      "current_step": "configuring_networking",
      "message": "Setting up VPC networking..."
    }
  }
}

// Stack status changes
{
  "type": "stack_status_change",
  "data": {
    "stack_id": "stack_123",
    "stack_type": "parent",
    "environment": "production",
    "old_status": "deploying", 
    "new_status": "deployed"
  }
}

// Resource health updates
{
  "type": "resource_health_update",
  "data": {
    "resource_id": "resource_456",
    "resource_type": "gcp-gke-autopilot-cluster",
    "old_status": "unknown",
    "new_status": "healthy",
    "timestamp": "2024-01-15T10:30:00Z"
  }
}
```

This comprehensive REST API specification provides all the necessary endpoints for managing Simple Container deployments through a web interface while maintaining compatibility with the existing CLI-based workflow.
