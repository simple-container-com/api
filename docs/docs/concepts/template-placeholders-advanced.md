# Advanced Template Placeholders & Compute Processors

Simple Container provides an advanced template placeholder system that automatically injects environment variables and secrets when resources are used in your deployments. This is powered by **compute processors** that understand how to connect your services to the resources they depend on.

## Overview

When you reference resources in your `client.yaml` using the `uses` or `dependencies` sections, Simple Container's compute processors automatically:

1. **Inject environment variables** with connection details, credentials, and configuration
2. **Inject secrets** for sensitive data like passwords and API keys
3. **Resolve template placeholders** like `${resource:name.property}` and `${dependency:name.property}`

This eliminates the need to manually configure connection strings, credentials, and other resource-specific details.

## Environment Variable Precedence

Simple Container follows a specific precedence order for environment variables:

1. **docker-compose.yaml** - Base values for local development
2. **client.yaml `env` section** - Overrides docker-compose values for deployment
3. **client.yaml `secrets` section** - Highest precedence for sensitive values

This design allows you to have docker-compose.yaml files that work locally for development while automatically getting production values when deployed via Simple Container.

## How It Works

### Resource Usage (`uses` section)

When you reference a resource from your parent stack:

```yaml
# client.yaml
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [postgres-db, s3-storage]
      runs: [web-app, worker]
```

The compute processors automatically inject the necessary environment variables and secrets into your containers.

### Cross-Service Dependencies (`dependencies` section)

When you need to connect to another service's shared resources:

```yaml
# client.yaml
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [redis-cache]
      runs: [api-service]
      dependencies:
        - name: billing-db
          owner: myproject/billing-service
          resource: postgres-cluster
```

## Template Placeholders

### Resource Placeholders

Access properties of resources defined in your parent stack in client.yaml:

```yaml
# client.yaml - Using resource placeholders in env/secrets sections
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [postgres-db, s3-storage, redis-cache]
      runs: [web-app]
      env:
        DATABASE_URL: "${resource:postgres-db.url}"
        S3_BUCKET: "${resource:s3-storage.bucket}"
        REDIS_HOST: "${resource:redis-cache.host}"
```

### Dependency Placeholders

Access properties of resources from other services in client.yaml:

```yaml
# client.yaml - Using dependency placeholders in env/secrets sections
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [redis-cache]
      runs: [api-service]
      dependencies:
        - name: billing-db
          owner: myproject/billing-service
          resource: postgres-cluster
        - name: shared-files
          owner: myproject/file-service
          resource: s3-storage
      env:
        BILLING_DB_URL: "${dependency:billing-db.url}"
        SHARED_STORAGE: "${dependency:shared-files.bucket}"
```

## Compute Processors by Resource Type

### AWS Resources

#### S3 Bucket
**Auto-injected Environment Variables:**

- `S3_<BUCKET>_REGION` - Bucket region
- `S3_<BUCKET>_BUCKET` - Bucket name
- `S3_<BUCKET>_ACCESS_KEY` - Access key (secret)
- `S3_<BUCKET>_SECRET_KEY` - Secret key (secret)
- `S3_REGION` - Generic bucket region
- `S3_BUCKET` - Generic bucket name
- `S3_ACCESS_KEY` - Generic access key (secret)
- `S3_SECRET_KEY` - Generic secret key (secret)

**Template Placeholders:**

- `${resource:bucket-name.bucket}` - Bucket name
- `${resource:bucket-name.region}` - Bucket region
- `${resource:bucket-name.access-key}` - Access key
- `${resource:bucket-name.secret-key}` - Secret key

#### RDS PostgreSQL
**Auto-injected Environment Variables:**

- `PGHOST_<NAME>` - Database host
- `PGUSER_<NAME>` - Database username
- `PGPORT_<NAME>` - Database port
- `PGDATABASE_<NAME>` - Database name
- `PGPASSWORD_<NAME>` - Database password (secret)
- `PGHOST` - Generic database host
- `PGUSER` - Generic database username
- `PGPORT` - Generic database port
- `PGDATABASE` - Generic database name
- `PGPASSWORD` - Generic database password (secret)

**Template Placeholders:**

- `${resource:postgres-name.url}` - Full connection URL
- `${resource:postgres-name.host}` - Database host
- `${resource:postgres-name.port}` - Database port
- `${resource:postgres-name.user}` - Database username
- `${resource:postgres-name.database}` - Database name
- `${resource:postgres-name.password}` - Database password

#### RDS MySQL
Similar to PostgreSQL but with MySQL-specific environment variables:

- `MYSQL_HOST_<NAME>`, `MYSQL_USER_<NAME>`, etc.
- Generic variables: `MYSQL_HOST`, `MYSQL_USER`, etc.

### GCP Resources

#### GCP Bucket
Auto-injected environment variables and template placeholders for Google Cloud Storage buckets.

#### GKE Autopilot
Kubernetes cluster connection details and configuration.

#### PostgreSQL Cloud SQL
PostgreSQL database connection details similar to AWS RDS.

### Kubernetes Resources

#### Helm Postgres Operator
PostgreSQL database connections managed by Kubernetes operators.

#### Helm RabbitMQ Operator
Message queue connection details and configuration.

#### Helm Redis Operator
Redis cache connection details and configuration.

### MongoDB Atlas

#### MongoDB Atlas Cluster
**Template Placeholders:**

- `${resource:mongodb-name.connectionString}` - Full MongoDB connection string
- `${resource:mongodb-name.host}` - MongoDB host
- `${resource:mongodb-name.database}` - Database name

## Practical Examples

### Using S3 Storage

```yaml
# parent stack (server.yaml)
resources:
  resources:
    production:
      file-storage:
        type: aws-s3-bucket
        config:
          name: myapp-files
          allowOnlyHttps: true
```

```yaml
# service stack (client.yaml)
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [file-storage]
      runs: [web-app]
```

```yaml
# docker-compose.yaml (for local development)
services:
  web-app:
    build: .
    environment:
      S3_BUCKET: "local-dev-bucket"
      S3_REGION: "us-east-1"
      S3_ACCESS_KEY: "dev-access-key"
      S3_SECRET_KEY: "dev-secret-key"
    # When deployed via Simple Container, these values are automatically
    # overridden by compute processor environment variables
```

### Using Database Connection

```yaml
# parent stack (server.yaml)
resources:
  resources:
    production:
      main-db:
        type: aws-rds-postgres
        config:
          name: myapp-db
          instanceClass: db.t3.micro
          username: postgres
          password: "${secret:DB_ROOT_PASSWORD}"
```

```yaml
# service stack (client.yaml)
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [main-db]
      runs: [api-service]
```

```yaml
# docker-compose.yaml (for local development)
services:
  api-service:
    build: .
    environment:
      PGHOST: "localhost"
      PGUSER: "postgres"
      PGPORT: "5432"
      PGDATABASE: "myapp_dev"
      PGPASSWORD: "dev-password"
    # When deployed via Simple Container, these values are automatically
    # overridden by compute processor environment variables
```

### Using Template Placeholders in Client Configuration

```yaml
# client.yaml - Custom environment variables using template placeholders
stacks:
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      uses: [main-db, redis-cache, file-storage]
      runs: [api-service]
      env:
        # Custom environment variables using template placeholders
        DATABASE_URL: "postgresql://${resource:main-db.user}:${resource:main-db.password}@${resource:main-db.host}:${resource:main-db.port}/${resource:main-db.database}"
        DB_HOST: "${resource:main-db.host}"
        REDIS_URL: "${resource:redis-cache.url}"
      secrets:
        # Custom secrets using template placeholders
        API_SECRET_KEY: "${resource:main-db.password}"
        S3_UPLOAD_KEY: "${resource:file-storage.access-key}"
```

## Benefits

1. **Automatic Configuration** - No manual connection string management
2. **Security** - Sensitive data automatically handled as secrets
3. **Consistency** - Standardized environment variable naming
4. **Flexibility** - Use both auto-injected variables and explicit placeholders
5. **Cross-Service Integration** - Easy sharing of resources between services

## Best Practices

1. **Prefer Auto-injected Variables** - Let compute processors handle standard environment variables
2. **Use Template Placeholders for Custom Config** - When you need specific formatting or custom configuration files
3. **Leverage Dependencies** - Share resources between services using the dependencies section
4. **Follow Naming Conventions** - Use descriptive resource names that map to clear environment variables

This system makes Simple Container deployments truly declarative - you declare what resources you need, and Simple Container handles all the connection details automatically.
