# Simple Container Cloud API - Database Design

## Overview

The Simple Container Cloud API uses MongoDB as its primary database to store multi-tenant configuration data, user management, and Simple Container stack definitions. The design leverages MongoDB's flexible document model to store complex configurations while maintaining strong consistency through transactions.

## Database Schema Design

### Core Collections

#### 1. Organizations Collection

Represents companies/customers using the Simple Container Cloud API.

```javascript
// organizations
{
  _id: ObjectId,
  name: String,              // Organization name
  slug: String,              // URL-friendly identifier (unique)
  description: String,       // Organization description
  
  // Subscription & Billing
  subscription: {
    plan: String,            // "free", "pro", "enterprise"
    status: String,          // "active", "suspended", "cancelled"
    limits: {
      max_users: Number,
      max_projects: Number,
      max_parent_stacks: Number,
      max_client_stacks: Number
    }
  },
  
  // Settings
  settings: {
    default_cloud_providers: [String],  // ["aws", "gcp"]
    require_mfa: Boolean,
    audit_retention_days: Number
  },
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  created_by: ObjectId,      // Reference to users collection
  
  // Indexes for performance
  // Index: { slug: 1 } (unique)
  // Index: { created_at: 1 }
}
```

#### 2. Users Collection

Individual user accounts with authentication and role information.

```javascript
// users
{
  _id: ObjectId,
  
  // Identity
  email: String,             // Primary identifier (unique)
  name: String,              // Full name
  avatar_url: String,        // Profile picture URL
  
  // Authentication
  google_id: String,         // Google OAuth ID
  auth_providers: [{
    provider: String,        // "google", "github", etc.
    provider_id: String,     // External ID
    connected_at: Date
  }],
  
  // Organization Membership
  organizations: [{
    organization_id: ObjectId,
    role: String,            // "admin", "infrastructure_manager", "developer"
    permissions: [String],   // Custom permissions array
    joined_at: Date,
    invited_by: ObjectId
  }],
  
  // Cloud Provider Service Accounts
  cloud_accounts: [{
    provider: String,        // "aws", "gcp"
    account_id: String,      // Cloud provider account/project ID
    service_account_email: String,  // For GCP
    service_account_key_id: String,
    iam_role_arn: String,    // For AWS
    created_at: Date,
    last_validated: Date,
    status: String           // "active", "invalid", "expired"
  }],
  
  // Preferences
  preferences: {
    default_organization: ObjectId,
    theme: String,           // "light", "dark"
    timezone: String,
    notifications: {
      email_deployments: Boolean,
      email_failures: Boolean,
      browser_notifications: Boolean
    }
  },
  
  // Security
  last_login: Date,
  mfa_enabled: Boolean,
  mfa_secret: String,        // Encrypted TOTP secret
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  status: String,            // "active", "suspended", "deactivated"
  
  // Indexes for performance
  // Index: { email: 1 } (unique)
  // Index: { google_id: 1 } (unique, sparse)
  // Index: { "organizations.organization_id": 1 }
}
```

#### 3. Projects Collection

Logical groupings of parent and client stacks within organizations.

```javascript
// projects
{
  _id: ObjectId,
  organization_id: ObjectId,
  
  // Project Identity
  name: String,              // Project name
  slug: String,              // URL-friendly identifier (unique within org)
  description: String,       // Project description
  
  // Git Integration
  git_repository: {
    url: String,             // Git repository URL
    branch: String,          // Default branch
    path: String,            // Path within repository
    credentials_id: ObjectId, // Reference to stored git credentials
    auto_sync: Boolean,      // Automatic sync enabled
    last_sync: Date
  },
  
  // Access Control
  owners: [ObjectId],        // User IDs with owner access
  collaborators: [{
    user_id: ObjectId,
    role: String,            // "read", "write", "admin"
    permissions: [String],   // Custom permissions
    added_at: Date,
    added_by: ObjectId
  }],
  
  // Project Settings
  settings: {
    environments: [String],  // ["development", "staging", "production"]
    default_cloud_provider: String,
    auto_deploy: Boolean,
    require_approval: Boolean
  },
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  created_by: ObjectId,
  status: String,            // "active", "archived", "deleted"
  
  // Indexes for performance
  // Index: { organization_id: 1, slug: 1 } (unique)
  // Index: { organization_id: 1, created_at: -1 }
  // Index: { owners: 1 }
}
```

#### 4. Parent Stacks Collection

Infrastructure definitions (server.yaml) managed by DevOps/Infrastructure teams.

```javascript
// parent_stacks
{
  _id: ObjectId,
  organization_id: ObjectId,
  project_id: ObjectId,
  
  // Stack Identity
  name: String,              // Stack name (unique within project)
  display_name: String,      // Human-readable name
  description: String,       // Stack description
  
  // Simple Container Configuration
  // This stores the complete server.yaml content
  server_config: {
    schemaVersion: String,   // "1.0"
    provisioner: {
      type: String,          // "pulumi"
      config: Object         // Provisioner configuration
    },
    secrets: Object,         // Secrets configuration
    cicd: Object,           // CI/CD configuration
    templates: Object,      // Deployment templates
    resources: Object,      // Resource definitions
    variables: Object       // Variables configuration
  },
  
  // Deployment Information
  environments: [{
    name: String,           // "production", "staging", etc.
    status: String,         // "deployed", "deploying", "failed", "not_deployed"
    last_deployed: Date,
    deployed_by: ObjectId,
    deployment_id: String,  // Reference to deployment logs
    resource_count: Number,
    estimated_cost: Number,
    
    // Cloud Provider Information
    cloud_resources: [{
      resource_id: String,   // Cloud provider resource ID
      resource_type: String, // "aws-s3-bucket", "gcp-gke-autopilot-cluster"
      resource_name: String,
      status: String,        // "healthy", "unhealthy", "unknown"
      created_at: Date,
      last_checked: Date,
      metadata: Object       // Provider-specific metadata
    }]
  }],
  
  // Access Control (Infrastructure Manager Role Required)
  owners: [ObjectId],        // Users with full access
  editors: [ObjectId],       // Users who can modify
  viewers: [ObjectId],       // Users who can view
  
  // Version Control
  version: Number,           // Incremental version number
  git_commit: String,        // Last git commit hash
  change_history: [{
    version: Number,
    changed_by: ObjectId,
    changed_at: Date,
    change_summary: String,
    git_commit: String
  }],
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  created_by: ObjectId,
  last_modified_by: ObjectId,
  
  // Indexes for performance
  // Index: { organization_id: 1, project_id: 1, name: 1 } (unique)
  // Index: { organization_id: 1, created_at: -1 }
  // Index: { owners: 1 }
  // Index: { "environments.status": 1 }
}
```

#### 5. Client Stacks Collection

Application configurations (client.yaml) managed by developers.

```javascript
// client_stacks
{
  _id: ObjectId,
  organization_id: ObjectId,
  project_id: ObjectId,
  
  // Stack Identity
  name: String,              // Stack name (unique within project)
  display_name: String,      // Human-readable name
  description: String,       // Stack description
  
  // Parent Stack Relationship
  parent_stack_id: ObjectId, // Reference to parent_stacks
  parent_environment: String, // Which parent environment to use
  
  // Simple Container Configuration
  // This stores the complete client.yaml content
  client_config: {
    schemaVersion: String,   // "1.0"
    defaults: Object,        // Default values and YAML anchors
    stacks: Object          // Stack configurations by environment
  },
  
  // Docker Compose Configuration
  docker_compose: Object,    // docker-compose.yaml content
  dockerfile_content: String, // Dockerfile content
  
  // Deployment Information
  environments: [{
    name: String,           // "production", "staging", etc.
    status: String,         // "deployed", "deploying", "failed", "not_deployed"
    last_deployed: Date,
    deployed_by: ObjectId,
    deployment_id: String,
    
    // Application-specific metrics
    replicas: {
      desired: Number,
      running: Number,
      ready: Number
    },
    
    // Service endpoints
    endpoints: [{
      name: String,         // Service name
      url: String,          // Public URL
      internal_url: String, // Internal URL
      health_status: String // "healthy", "unhealthy", "unknown"
    }],
    
    // Resource consumption
    resources_used: [{
      resource_name: String, // Parent resource name
      resource_type: String,
      connection_status: String, // "connected", "error"
      last_used: Date
    }]
  }],
  
  // Git Integration
  git_repository: {
    url: String,             // Application git repository
    branch: String,          // Deployment branch
    commit: String,          // Last deployed commit
    dockerfile_path: String, // Path to Dockerfile
    compose_path: String     // Path to docker-compose.yaml
  },
  
  // Access Control (Developer Role Sufficient)
  owners: [ObjectId],        // Users with full access
  collaborators: [{
    user_id: ObjectId,
    role: String,            // "read", "write"
    added_at: Date
  }],
  
  // Version Control
  version: Number,
  change_history: [{
    version: Number,
    changed_by: ObjectId,
    changed_at: Date,
    change_summary: String,
    config_diff: String     // JSON diff of configuration changes
  }],
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  created_by: ObjectId,
  last_modified_by: ObjectId,
  
  // Indexes for performance
  // Index: { organization_id: 1, project_id: 1, name: 1 } (unique)
  // Index: { parent_stack_id: 1 }
  // Index: { organization_id: 1, created_at: -1 }
  // Index: { owners: 1 }
}
```

#### 6. Stack Secrets Collection

Encrypted secrets (secrets.yaml) with proper access control.

```javascript
// stack_secrets
{
  _id: ObjectId,
  organization_id: ObjectId,
  
  // Associated Stack
  stack_id: ObjectId,        // References parent_stacks or client_stacks
  stack_type: String,        // "parent" or "client"
  environment: String,       // Environment these secrets apply to
  
  // Encrypted Secret Data
  // This stores the complete secrets.yaml content, encrypted
  encrypted_secrets: {
    schemaVersion: String,   // "1.0"
    auth: Object,           // Authentication configurations (encrypted)
    values: Object          // Secret values (encrypted)
  },
  
  // Encryption Information
  encryption: {
    algorithm: String,       // "AES-256-GCM"
    key_version: Number,     // For key rotation
    encrypted_at: Date,
    encrypted_by: ObjectId
  },
  
  // Access Control (Strict - Only Infrastructure Managers + Stack Owners)
  accessible_by: [ObjectId], // User IDs who can decrypt these secrets
  access_history: [{
    user_id: ObjectId,
    accessed_at: Date,
    action: String,          // "read", "write", "rotate"
    ip_address: String
  }],
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  created_by: ObjectId,
  
  // Indexes for performance
  // Index: { stack_id: 1, environment: 1 } (unique)
  // Index: { accessible_by: 1 }
  // Index: { organization_id: 1 }
}
```

#### 7. Cloud Accounts Collection

Cloud provider service accounts and their management.

```javascript
// cloud_accounts
{
  _id: ObjectId,
  organization_id: ObjectId,
  user_id: ObjectId,         // User who owns this account
  
  // Cloud Provider Information
  provider: String,          // "aws", "gcp"
  account_id: String,        // Cloud account/project ID
  region: String,            // Default region
  
  // Service Account Details
  service_account: {
    // For GCP
    email: String,           // Service account email
    project_id: String,      // GCP project ID
    key_id: String,          // Service account key ID
    
    // For AWS
    user_name: String,       // IAM user name
    access_key_id: String,   // AWS access key ID
    role_arn: String,        // Assumed role ARN
    
    // Common
    created_at: Date,
    last_rotated: Date,
    expires_at: Date
  },
  
  // Permissions & Roles
  permissions: {
    iam_roles: [String],     // Assigned IAM roles/policies
    custom_permissions: [String],
    permission_boundary: String,
    last_validated: Date,
    validation_status: String // "valid", "invalid", "pending"
  },
  
  // Usage Tracking
  usage: {
    last_used: Date,
    operations_count: Number,
    estimated_cost: Number,
    resource_count: Number
  },
  
  // Status & Health
  status: String,            // "active", "suspended", "deleted", "error"
  health_check: {
    last_checked: Date,
    status: String,          // "healthy", "unhealthy", "unknown"
    error_message: String
  },
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  
  // Indexes for performance
  // Index: { organization_id: 1, user_id: 1, provider: 1 }
  // Index: { user_id: 1 }
  // Index: { status: 1 }
}
```

#### 8. Resources Collection

Discovered and managed cloud resources.

```javascript
// resources
{
  _id: ObjectId,
  organization_id: ObjectId,
  
  // Resource Identity
  cloud_provider: String,    // "aws", "gcp"
  cloud_account_id: String,  // Cloud account/project ID
  resource_id: String,       // Cloud provider resource ID (unique per account)
  resource_type: String,     // "aws-s3-bucket", "gcp-gke-autopilot-cluster"
  resource_name: String,     // Human-readable name
  region: String,           // Cloud region
  
  // Simple Container Integration
  managed_by_sc: Boolean,    // Whether SC manages this resource
  parent_stack_id: ObjectId, // If managed by SC, which parent stack
  stack_environment: String, // Which environment
  
  // Resource Details
  configuration: Object,     // Cloud-specific configuration
  tags: Object,             // Resource tags/labels
  metadata: Object,         // Additional metadata
  
  // Status & Monitoring
  status: String,           // "healthy", "unhealthy", "unknown", "deleted"
  health_check: {
    last_checked: Date,
    status_details: String,
    metrics: Object
  },
  
  // Cost Information
  cost: {
    estimated_monthly: Number,
    last_calculated: Date,
    currency: String
  },
  
  // Discovery Information
  discovered_at: Date,
  discovered_by: String,    // "sc_discovery", "manual_import", "provisioning"
  last_synced: Date,
  
  // Relationships
  dependencies: [ObjectId], // Other resources this depends on
  dependents: [ObjectId],   // Other resources that depend on this
  
  // Metadata
  created_at: Date,
  updated_at: Date,
  
  // Indexes for performance
  // Index: { organization_id: 1, cloud_provider: 1, resource_id: 1 } (unique)
  // Index: { parent_stack_id: 1 }
  // Index: { managed_by_sc: 1 }
  // Index: { status: 1 }
}
```

#### 9. Audit Logs Collection

Comprehensive activity and change tracking.

```javascript
// audit_logs
{
  _id: ObjectId,
  organization_id: ObjectId,
  
  // Event Information
  event_type: String,        // "stack_created", "resource_provisioned", "user_login"
  event_category: String,    // "authentication", "stack_management", "resource_management"
  
  // Actor Information
  actor: {
    user_id: ObjectId,
    user_email: String,
    user_name: String,
    ip_address: String,
    user_agent: String
  },
  
  // Target Information
  target: {
    resource_type: String,   // "parent_stack", "client_stack", "user", "resource"
    resource_id: ObjectId,
    resource_name: String
  },
  
  // Change Details
  changes: {
    action: String,          // "create", "update", "delete", "provision", "deploy"
    before: Object,          // Previous state (for updates)
    after: Object,           // New state (for creates/updates)
    diff: String            // Human-readable change summary
  },
  
  // Request Context
  request: {
    method: String,          // HTTP method
    endpoint: String,        // API endpoint
    request_id: String,      // Correlation ID
    duration_ms: Number
  },
  
  // Result Information
  result: String,           // "success", "failure", "partial"
  error_message: String,    // If result is failure
  
  // Metadata
  timestamp: Date,          // Event timestamp
  severity: String,         // "info", "warning", "error"
  
  // Indexes for performance
  // Index: { organization_id: 1, timestamp: -1 }
  // Index: { event_type: 1, timestamp: -1 }
  // Index: { "actor.user_id": 1, timestamp: -1 }
  // Index: { "target.resource_id": 1, timestamp: -1 }
}
```

#### 10. Sessions Collection

User session management (also cached in Redis).

```javascript
// sessions
{
  _id: ObjectId,
  
  // Session Identity
  session_id: String,        // Random session identifier (unique)
  user_id: ObjectId,
  organization_id: ObjectId,
  
  // Authentication Tokens
  access_token: String,      // JWT access token (encrypted)
  refresh_token: String,     // JWT refresh token (encrypted)
  
  // Session Details
  created_at: Date,
  expires_at: Date,
  last_accessed: Date,
  
  // Client Information
  ip_address: String,
  user_agent: String,
  device_fingerprint: String,
  
  // Status
  status: String,           // "active", "expired", "revoked"
  revoked_at: Date,
  revoked_by: ObjectId,
  revoked_reason: String,
  
  // Indexes for performance
  // Index: { session_id: 1 } (unique)
  // Index: { user_id: 1, status: 1 }
  // Index: { expires_at: 1 } (TTL index for automatic cleanup)
}
```

## Indexing Strategy

### Primary Indexes

```javascript
// Performance-critical indexes
db.organizations.createIndex({ "slug": 1 }, { unique: true })
db.users.createIndex({ "email": 1 }, { unique: true })
db.users.createIndex({ "organizations.organization_id": 1 })

db.projects.createIndex({ "organization_id": 1, "slug": 1 }, { unique: true })

db.parent_stacks.createIndex({ 
  "organization_id": 1, 
  "project_id": 1, 
  "name": 1 
}, { unique: true })

db.client_stacks.createIndex({ 
  "organization_id": 1, 
  "project_id": 1, 
  "name": 1 
}, { unique: true })
db.client_stacks.createIndex({ "parent_stack_id": 1 })

db.stack_secrets.createIndex({ "stack_id": 1, "environment": 1 }, { unique: true })

db.resources.createIndex({ 
  "organization_id": 1, 
  "cloud_provider": 1, 
  "resource_id": 1 
}, { unique: true })

db.audit_logs.createIndex({ "organization_id": 1, "timestamp": -1 })
db.sessions.createIndex({ "session_id": 1 }, { unique: true })
db.sessions.createIndex({ "expires_at": 1 }, { expireAfterSeconds: 0 })
```

## Data Relationships

### Organization Hierarchy
```
Organization
├── Users (many-to-many via organizations array in users)
├── Projects (one-to-many)
│   ├── Parent Stacks (one-to-many)
│   │   ├── Stack Secrets (one-to-many)
│   │   └── Resources (one-to-many)
│   └── Client Stacks (one-to-many)
│       └── Stack Secrets (one-to-many)
├── Cloud Accounts (one-to-many)
└── Audit Logs (one-to-many)
```

### Stack Relationships
```
Parent Stack
├── Client Stacks (one-to-many via parent_stack_id)
├── Resources (one-to-many via parent_stack_id)
└── Stack Secrets (one-to-many via stack_id)

Client Stack
├── Parent Stack (many-to-one via parent_stack_id)
└── Stack Secrets (one-to-many via stack_id)
```

## Transaction Patterns

### Multi-Document Operations

MongoDB transactions ensure data consistency for operations that span multiple collections:

```javascript
// Example: Creating a new parent stack with initial secrets
session.withTransaction(async () => {
  // 1. Create parent stack
  const stack = await db.parent_stacks.insertOne({...}, { session })
  
  // 2. Create associated secrets
  await db.stack_secrets.insertOne({
    stack_id: stack.insertedId,
    stack_type: "parent",
    ...
  }, { session })
  
  // 3. Log the action
  await db.audit_logs.insertOne({
    event_type: "parent_stack_created",
    target: { resource_id: stack.insertedId },
    ...
  }, { session })
})
```

## Security Considerations

### Data Encryption

- **Secrets Encryption**: All secret data encrypted using AES-256-GCM
- **PII Encryption**: Sensitive user data encrypted at rest
- **Database Encryption**: MongoDB encryption at rest enabled
- **Connection Encryption**: TLS 1.3 for all database connections

### Access Control

- **Database Authentication**: Strong authentication with role-based access
- **Connection Limits**: Connection pooling with limits
- **Query Monitoring**: Slow query logging and monitoring
- **Backup Encryption**: All backups encrypted with separate keys

### Compliance

- **GDPR Compliance**: User data deletion and export capabilities
- **SOC 2 Type II**: Audit trail and access controls
- **Data Residency**: Configurable data location based on organization requirements

This database design provides a robust foundation for the Simple Container Cloud API, maintaining full compatibility with existing Simple Container configurations while adding the necessary multi-tenancy and access control features.
