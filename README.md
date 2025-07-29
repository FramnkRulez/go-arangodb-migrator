# go-arangodb-migrator

A simple database migration tool for ArangoDB written in Go. This library provides a clean way to manage database schema changes with automatic rollback on failure.

## Features

- **Simple JSON-based migrations** - Easy to read and write migration files
- **Automatic rollback** - If a migration fails, all operations are automatically rolled back
- **Integrity verification** - SHA256 hash verification prevents modified migration files from being applied
- **Comprehensive operations** - Support for collections, indexes, graphs, and documents
- **Ordered execution** - Migrations are applied in numeric order
- **Minimal dependencies** - Only depends on the official ArangoDB Go driver

## Requirements

- Go 1.22 or later
- ArangoDB 3.7 or later

## Installation

### For Go Developers (Library Usage)

Add the dependency to your Go module:

```bash
go get github.com/FramnkRulez/go-arangodb-migrator
```

Or add it directly to your `go.mod` file:

```go
require github.com/FramnkRulez/go-arangodb-migrator v0.1.0
```

### For DevOps/Operations (CLI Tool)

See the [Command Line Tool](#command-line-tool) section below for installation options.

## Quick Start

### 1. Create Migration Files

Create migration files in JSON format with numeric prefixes:

```json
// 000001_initial_schema.json
{
    "description": "Create initial database schema",
    "up": [
        {
            "type": "createCollection",
            "name": "books",
            "options": {
                "type": "document"
            }
        },
        {
            "type": "createPersistentIndex",
            "name": "idx_books_isbn",
            "options": {
                "collection": "books",
                "fields": ["isbn"],
                "unique": true,
                "sparse": true
            }
        }
    ]
}
```

### 2. Use in Your Code

```go
package main

import (
    "context"
    "log"

    "github.com/arangodb/go-driver/v2/arangodb"
    "github.com/arangodb/go-driver/v2/connection"
    "github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator"
)

func main() {
    ctx := context.Background()
    
    // Connect to ArangoDB
    conn := connection.NewHttpConnection(connection.HttpConfiguration{
        Endpoint: connection.NewRoundRobinEndpoints([]string{"http://localhost:8529"}),
    })
    
    auth := connection.NewBasicAuth("root", "password")
    conn.SetAuthentication(auth)
    
    client := arangodb.NewClient(conn)
    
    // Get or create database
    db, err := client.GetDatabase(ctx, "myapp", nil)
    if err != nil {
        log.Fatal(err)
    }
    
    // Run migrations
    err = migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
        MigrationFolder:     "./migrations",
        MigrationCollection: "migrations",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("Migrations completed successfully!")
}
```

## Migration File Format

Migration files are JSON files with the following structure:

```json
{
    "description": "Human-readable description of the migration",
    "up": [
        {
            "type": "operation_type",
            "name": "resource_name",
            "options": {
                // Operation-specific options
            }
        }
    ],
    "down": [
        // Rollback operations (currently not implemented)
    ]
}
```

## Supported Operations

### Collections

#### createCollection
Creates a new collection.

```json
{
    "type": "createCollection",
    "name": "users",
    "options": {
        "type": "document"
    }
}
```

#### deleteCollection
Deletes a collection.

```json
{
    "type": "deleteCollection",
    "name": "users"
}
```

### Indexes

#### createPersistentIndex
Creates a persistent index on a collection.

```json
{
    "type": "createPersistentIndex",
    "name": "idx_users_email",
    "options": {
        "collection": "users",
        "fields": ["email"],
        "unique": true,
        "sparse": true
    }
}
```

#### createGeoIndex
Creates a geo index on a collection.

```json
{
    "type": "createGeoIndex",
    "name": "idx_events_location",
    "options": {
        "collection": "events",
        "fields": ["location"]
    }
}
```

#### deleteIndex
Deletes an index.

```json
{
    "type": "deleteIndex",
    "name": "idx_users_email",
    "options": {
        "collection": "users"
    }
}
```

### Graphs

#### createGraph
Creates a new graph.

```json
{
    "type": "createGraph",
    "name": "user_network",
    "options": {
        "edgeDefinitions": [
            {
                "collection": "user_follows_user",
                "from": ["users"],
                "to": ["users"]
            }
        ]
    }
}
```

#### deleteGraph
Deletes a graph.

```json
{
    "type": "deleteGraph",
    "name": "user_network"
}
```

#### addEdgeDefinition
Adds an edge definition to an existing graph.

```json
{
    "type": "addEdgeDefinition",
    "name": "user_network",
    "options": {
        "edgeDefinition": {
            "collection": "user_likes_post",
            "from": ["users"],
            "to": ["posts"]
        }
    }
}
```

#### deleteEdgeDefinition
Removes an edge definition from a graph.

```json
{
    "type": "deleteEdgeDefinition",
    "name": "user_network",
    "options": {
        "edgeDefinition": {
            "collection": "user_likes_post",
            "from": ["users"],
            "to": ["posts"]
        }
    }
}
```

### Documents

#### addDocument
Adds a document to a collection.

```json
{
    "type": "addDocument",
    "name": "users",
    "options": {
        "document": {
            "_key": "admin",
            "email": "admin@example.com",
            "role": "admin"
        }
    }
}
```

#### updateDocument
Updates an existing document.

```json
{
    "type": "updateDocument",
    "name": "users",
    "options": {
        "key": "admin",
        "document": {
            "role": "super_admin"
        }
    }
}
```

#### deleteDocument
Deletes a document from a collection.

```json
{
    "type": "deleteDocument",
    "name": "users",
    "options": {
        "key": "admin"
    }
}
```

## Command Line Tool

A command-line tool is also provided for easy migration management:

### Installation

**Option 1: Docker (Recommended)**
```bash
# Run with Docker
docker run --rm framnk/go-arangodb-migrator:latest --help

# Or pull and run
docker pull framnk/go-arangodb-migrator:latest
docker run --rm framnk/go-arangodb-migrator:latest --database myapp --arango-password mypassword
```

**Option 2: Download pre-built binaries**
Download the latest release from [GitHub Releases](https://github.com/FramnkRulez/go-arangodb-migrator/releases).

**Option 3: Build from source**
```bash
# Build the CLI tool
go build -o migrator ./cmd/migrator

# Or install globally (requires Go 1.22+)
go install github.com/FramnkRulez/go-arangodb-migrator/cmd/migrator@latest
```

# Docker usage (recommended)
docker run --rm framnk/go-arangodb-migrator:latest \
  --database myapp --arango-password password

# Basic usage (local binary)
./migrator --database myapp --arango-password password

# With custom ArangoDB connection
./migrator --arango-address http://localhost:8529 \
           --arango-user root \
           --arango-password password \
           --database myapp

## Using Environment Variables

All CLI options can also be set using environment variables, which is especially useful in containerized environments:

```bash
# Set environment variables
export DATABASE=myapp
export ARANGO_PASSWORD=mypassword
export ARANGO_ADDRESS=http://localhost:8529
export ARANGO_USER=root
export MIGRATION_FOLDER=./db/migrations
export MIGRATION_COLLECTION=db_migrations
export DRY_RUN=true
export VERBOSE=true

# Run without command-line arguments
./migrator

# Or with Docker
docker run --rm \
  -e DATABASE=myapp \
  -e ARANGO_PASSWORD=mypassword \
  -e ARANGO_ADDRESS=http://localhost:8529 \
  -e VERBOSE=true \
  framnk/go-arangodb-migrator:latest
```

**Note**: Environment variables take precedence over default values but can be overridden by command-line arguments.

# With custom migration settings
docker run --rm -v $(pwd)/db/migrations:/app/migrations framnk/go-arangodb-migrator:latest \
  --database myapp --arango-password password \
  --migration-folder /app/migrations \
  --migration-collection db_migrations

# Dry run to see what would be migrated
docker run --rm framnk/go-arangodb-migrator:latest \
  --database myapp --arango-password password --dry-run

# Force migration even if files have been modified
docker run --rm framnk/go-arangodb-migrator:latest \
  --database myapp --arango-password password --force

# Verbose output
docker run --rm framnk/go-arangodb-migrator:latest \
  --database myapp --arango-password password --verbose

# Local binary examples
./migrator --database myapp --arango-password password \
  --migration-folder ./db/migrations \
  --migration-collection db_migrations

./migrator --database myapp --arango-password password --dry-run
./migrator --database myapp --arango-password password --force
./migrator --database myapp --arango-password password --verbose
./migrator --database myapp --arango-password password --quiet
```

### CLI Options

| Option | Description | Default | Environment Variable |
|--------|-------------|---------|---------------------|
| `--database` | Database name | Required | `DATABASE` |
| `--arango-password` | ArangoDB password | Required | `ARANGO_PASSWORD` |
| `--arango-address` | ArangoDB address | `http://localhost:8529` | `ARANGO_ADDRESS` |
| `--arango-user` | ArangoDB user | `root` | `ARANGO_USER` |
| `--migration-folder` | Migration files folder | `./migrations` | `MIGRATION_FOLDER` |
| `--migration-collection` | Collection for tracking migrations | `migrations` | `MIGRATION_COLLECTION` |
| `--dry-run` | Show what would be migrated without running | `false` | `DRY_RUN` |
| `--force` | Force migration even if files modified | `false` | `FORCE` |
| `--verbose` | Enable verbose logging | `false` | `VERBOSE` |
| `--quiet` | Suppress all output except errors | `false` | `QUIET` |
| `--version` | Show version information | - | - |

## Examples

See the `examples/` directory for complete working examples.

## Documentation

### Go Documentation

The package includes comprehensive Go documentation that can be viewed using:

```bash
# View package documentation
go doc ./pkg/migrator

# View function documentation
go doc ./pkg/migrator.MigrateArangoDatabase

# View struct documentation
go doc ./pkg/migrator.MigrationOptions

# Serve documentation locally (requires godoc)
./scripts/serve-docs.sh
```

### API Reference

The main function is `MigrateArangoDatabase` which takes:
- `context.Context` for cancellation and timeouts
- `arangodb.Database` instance
- `MigrationOptions` for configuration

See the [examples/](examples/) directory for complete working examples.

## Testing

This project includes comprehensive unit and integration tests. See [TESTING.md](TESTING.md) for detailed information on running tests.

### Quick Test Commands

```bash
# Run unit tests
go test ./pkg/migrator

# Run integration tests (requires Docker)
INTEGRATION_TEST=true go test ./pkg/migrator -run "TestIntegration"
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Setup

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run the test suite
6. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
