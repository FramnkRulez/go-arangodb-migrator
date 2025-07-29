# Examples

This directory contains examples demonstrating how to use the go-arangodb-migrator library.

## Basic Example

The `basic/` directory contains a simple example that:

1. Connects to an ArangoDB instance
2. Creates or uses an existing database
3. Runs migrations from JSON files

### Running the Basic Example

1. Make sure you have ArangoDB running locally on the default port (8529)
2. Navigate to the basic example directory:
   ```bash
   cd examples/basic
   ```
3. Run the example:
   ```bash
   go run main.go
   ```

### Example Migration

The basic example includes sample migration files that create a library management system:
- `000001_initial_schema.json` - Creates books and authors collections with indexes and a graph
- `000002_add_locations_and_categories.json` - Adds libraries with geo indexes and categories
- `000003_add_sample_data.json` - Adds sample books, authors, and relationships

The example demonstrates:
- Document and edge collections
- Persistent and geo indexes
- Named graphs with multiple edge definitions
- Complex relationships between entities

## Environment Variables Example

The `environment-variables/` directory contains an example that demonstrates how to use environment variables with the CLI tool. This is especially useful for:

- Containerized environments (Docker, Kubernetes)
- CI/CD pipelines
- Production deployments
- Avoiding sensitive data in command-line arguments

### Running the Environment Variables Example

1. Set up your environment variables:
   ```bash
   export DATABASE=example_db
   export ARANGO_PASSWORD=your_password
   export ARANGO_ADDRESS=http://localhost:8529
   ```

2. Run the example:
   ```bash
   cd examples/environment-variables
   go run main.go
   ```

See `examples/environment-variables/README.md` for detailed documentation.

## Customizing Examples

To use these examples with your own ArangoDB instance:

1. Update the connection details in `main.go`
2. Modify the database name
3. Create your own migration files in the `migrations/` directory
4. Run the example

## Migration File Format

See the main README.md for detailed documentation on the migration file format and supported operations. 