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
Similar to PostgreSQL but with MySQL-specific environment variables.

**Auto-injected Environment Variables:**
- `MYSQL_HOST` - Database host
- `MYSQL_PORT` - Database port (3306)
- `MYSQL_USER` - Database username (stack name)
- `MYSQL_DB` - Database name (stack name)
- `MYSQL_PASSWORD` - Database password (auto-generated)
- `MYSQL_HOST_<NAME>` - Named MySQL host (where `<NAME>` is the resource name)
- `MYSQL_USER_<NAME>` - Named MySQL username
- `MYSQL_PORT_<NAME>` - Named MySQL port
- `MYSQL_DB_<NAME>` - Named MySQL database
- `MYSQL_PASSWORD_<NAME>` - Named MySQL password

**Template Placeholders:**
- `${resource:mysql-name.host}` - Database host
- `${resource:mysql-name.port}` - Database port
- `${resource:mysql-name.user}` - Database username
- `${resource:mysql-name.database}` - Database name
- `${resource:mysql-name.password}` - Database password
- `${resource:mysql-name.url}` - Full MySQL connection string

### GCP Resources

#### GCP Bucket
**Note:** GCP Bucket compute processor is currently not implemented. No environment variables are automatically injected for GCP Bucket resources at this time.

#### GKE Autopilot
**Note:** GKE Autopilot compute processor is currently not implemented. No environment variables are automatically injected for GKE Autopilot resources at this time.

#### PostgreSQL Cloud SQL
PostgreSQL database connection details similar to AWS RDS.

**Auto-injected Environment Variables:**
- `POSTGRES_HOST` - Database host (localhost via Cloud SQL Proxy)
- `POSTGRES_PORT` - Database port (5432)
- `POSTGRES_USERNAME` - Database username (stack name)
- `POSTGRES_DATABASE` - Database name (stack name)
- `POSTGRES_PASSWORD` - Database password (auto-generated)
- `PGHOST` - PostgreSQL host (localhost via Cloud SQL Proxy)
- `PGPORT` - PostgreSQL port (5432)
- `PGUSER` - PostgreSQL username
- `PGDATABASE` - PostgreSQL database name
- `PGPASSWORD` - PostgreSQL password

**Template Placeholders:**
- `${resource:postgres-name.host}` - Database host
- `${resource:postgres-name.port}` - Database port
- `${resource:postgres-name.user}` - Database username
- `${resource:postgres-name.database}` - Database name
- `${resource:postgres-name.password}` - Database password

### Kubernetes Resources

#### Helm Postgres Operator
PostgreSQL database connections managed by Kubernetes operators.

**Auto-injected Environment Variables:**
- `POSTGRES_HOST` - PostgreSQL service host
- `POSTGRES_PORT` - PostgreSQL service port (5432)
- `POSTGRES_USERNAME` - Database username (stack name)
- `POSTGRES_DATABASE` - Database name (stack name)
- `POSTGRES_PASSWORD` - Database password (auto-generated)
- `PGHOST` - PostgreSQL host
- `PGPORT` - PostgreSQL port
- `PGUSER` - PostgreSQL username
- `PGDATABASE` - PostgreSQL database name
- `PGPASSWORD` - PostgreSQL password

**Template Placeholders:**
- `${resource:postgres-name.host}` - Database host
- `${resource:postgres-name.port}` - Database port
- `${resource:postgres-name.user}` - Database username
- `${resource:postgres-name.database}` - Database name
- `${resource:postgres-name.password}` - Database password
- `${resource:postgres-name.url}` - Full connection URL

#### Helm RabbitMQ Operator
Message queue connection details and configuration.

**Auto-injected Environment Variables:**
- `RABBITMQ_HOST` - RabbitMQ service host
- `RABBITMQ_PORT` - RabbitMQ service port
- `RABBITMQ_USERNAME` - RabbitMQ username
- `RABBITMQ_PASSWORD` - RabbitMQ password (auto-generated)
- `RABBITMQ_URI` - Full AMQP connection string

**Template Placeholders:**
- `${resource:rabbitmq-name.host}` - RabbitMQ host
- `${resource:rabbitmq-name.port}` - RabbitMQ port
- `${resource:rabbitmq-name.user}` - Username
- `${resource:rabbitmq-name.password}` - Password
- `${resource:rabbitmq-name.uri}` - Full AMQP connection string

#### Helm Redis Operator
Redis cache connection details and configuration.

**Auto-injected Environment Variables:**
- `REDIS_HOST` - Redis service host
- `REDIS_PORT` - Redis service port

**Template Placeholders:**
- `${resource:redis-name.host}` - Redis host
- `${resource:redis-name.port}` - Redis port

### MongoDB Atlas

#### MongoDB Atlas Cluster
**Auto-injected Environment Variables:**
- `MONGO_USER` - Database username (stack name)
- `MONGO_DATABASE` - Database name (stack name)
- `MONGO_PASSWORD` - Database password (auto-generated)
- `MONGO_URI` - Full MongoDB connection string with authentication
- `MONGO_DEP_<OWNER>_USER` - Dependency username (for dependency relationships)
- `MONGO_DEP_<OWNER>_PASSWORD` - Dependency password (for dependency relationships)
- `MONGO_DEP_<OWNER>_URI` - Dependency connection string (for dependency relationships)

**Template Placeholders:**
- `${resource:mongodb-name.uri}` - Full MongoDB connection string
- `${resource:mongodb-name.user}` - Database username
- `${resource:mongodb-name.password}` - Database password
- `${resource:mongodb-name.dbName}` - Database name
- `${resource:mongodb-name.oplogUri}` - MongoDB oplog connection string

## Practical Examples

### Using S3 Storage

```yaml
# parent stack (server.yaml)
resources:
  resources:
    production:
      file-storage:
        type: s3-bucket
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
