# Environment Variables Example

This example demonstrates how to use environment variables with the ArangoDB Migrator CLI tool.

## Overview

The CLI tool supports both command-line arguments and environment variables. Environment variables are especially useful in:
- Containerized environments (Docker, Kubernetes)
- CI/CD pipelines
- Production deployments
- When you want to avoid passing sensitive data as command-line arguments

## Environment Variables

All CLI options can be set using environment variables:

| CLI Option | Environment Variable | Required | Default |
|------------|---------------------|----------|---------|
| `--database` | `DATABASE` | Yes | - |
| `--arango-password` | `ARANGO_PASSWORD` | Yes | - |
| `--arango-address` | `ARANGO_ADDRESS` | No | `http://localhost:8529` |
| `--arango-user` | `ARANGO_USER` | No | `root` |
| `--migration-folder` | `MIGRATION_FOLDER` | No | `./migrations` |
| `--migration-collection` | `MIGRATION_COLLECTION` | No | `migrations` |
| `--dry-run` | `DRY_RUN` | No | `false` |
| `--force` | `FORCE` | No | `false` |
| `--verbose` | `VERBOSE` | No | `false` |
| `--quiet` | `QUIET` | No | `false` |

## Usage Examples

### 1. Using Environment Variables with CLI

```bash
# Set environment variables
export DATABASE=myapp
export ARANGO_PASSWORD=mypassword
export ARANGO_ADDRESS=http://localhost:8529
export ARANGO_USER=root
export MIGRATION_FOLDER=./db/migrations
export MIGRATION_COLLECTION=db_migrations
export VERBOSE=true

# Run without command-line arguments
./migrator
```

### 2. Using Environment Variables with Docker

```bash
# Run with environment variables
docker run --rm \
  -e DATABASE=myapp \
  -e ARANGO_PASSWORD=mypassword \
  -e ARANGO_ADDRESS=http://localhost:8529 \
  -e VERBOSE=true \
  -v $(pwd)/migrations:/app/migrations \
  framnk/go-arangodb-migrator:latest
```

### 3. Using a .env File

Create a `.env` file:
```bash
DATABASE=myapp
ARANGO_PASSWORD=mypassword
ARANGO_ADDRESS=http://localhost:8529
ARANGO_USER=root
MIGRATION_FOLDER=./migrations
VERBOSE=true
```

Load and run:
```bash
# Load environment variables from .env file
source .env

# Run the migrator
./migrator
```

### 4. Kubernetes ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: arangodb-migrator-config
data:
  ARANGO_ADDRESS: "http://arangodb:8529"
  ARANGO_USER: "root"
  DATABASE: "myapp"
  MIGRATION_FOLDER: "/app/migrations"
  MIGRATION_COLLECTION: "migrations"
  VERBOSE: "true"
---
apiVersion: v1
kind: Secret
metadata:
  name: arangodb-migrator-secret
type: Opaque
data:
  ARANGO_PASSWORD: <base64-encoded-password>
---
apiVersion: batch/v1
kind: Job
metadata:
  name: arangodb-migrator
spec:
  template:
    spec:
      containers:
      - name: migrator
        image: framnk/go-arangodb-migrator:latest
        envFrom:
        - configMapRef:
            name: arangodb-migrator-config
        - secretRef:
            name: arangodb-migrator-secret
        volumeMounts:
        - name: migrations
          mountPath: /app/migrations
      volumes:
      - name: migrations
        configMap:
          name: migrations-config
      restartPolicy: Never
```

## Precedence Order

The CLI follows this precedence order (highest to lowest):
1. Command-line arguments
2. Environment variables
3. Default values

This means you can:
- Set defaults via environment variables
- Override them with command-line arguments when needed

## Security Best Practices

1. **Never commit secrets to version control**
2. **Use secrets management** (Kubernetes Secrets, Docker Secrets, etc.)
3. **Use environment variables for sensitive data** instead of command-line arguments
4. **Consider using a .env file** for local development (add to .gitignore)

## Running This Example

1. Set up your environment variables:
```bash
export DATABASE=example_db
export ARANGO_PASSWORD=your_password
export ARANGO_ADDRESS=http://localhost:8529
```

2. Run the example:
```bash
go run main.go
```

Or use the CLI tool directly:
```bash
./migrator
``` 