# Testing

This document explains how to run tests for the go-arangodb-migrator library.

## Test Types

### Unit Tests
Unit tests test individual functions and components without requiring external dependencies like ArangoDB.

### Integration Tests
Integration tests use testcontainers to spin up a real ArangoDB instance and test the full migration workflow.

## Running Tests

### Unit Tests Only (Default)
```bash
go test ./pkg/migrator
```

### Unit Tests with Verbose Output
```bash
go test -v ./pkg/migrator
```

### Unit Tests with Race Detection
```bash
go test -race ./pkg/migrator
```

### Integration Tests
Integration tests require Docker to be running and will spin up ArangoDB containers.

```bash
# Run all integration tests
INTEGRATION_TEST=true go test -v ./pkg/migrator -run "TestIntegration|TestOperationsIntegration"

# Run specific integration test
INTEGRATION_TEST=true go test -v ./pkg/migrator -run "TestIntegration"
```

### All Tests (Unit + Integration)
```bash
# Run unit tests
go test -v ./pkg/migrator

# Run integration tests
INTEGRATION_TEST=true go test -v ./pkg/migrator -run "TestIntegration|TestOperationsIntegration"
```

## Test Structure

### Unit Tests
- `TestMigrationOptions` - Tests the MigrationOptions struct
- `TestOperation` - Tests the Operation struct
- `TestMigration` - Tests the Migration struct
- `TestAppliedMigration` - Tests the AppliedMigration struct
- `TestGetFileSHA256` - Tests SHA256 hash calculation
- `TestGetSlice` - Tests generic slice extraction

### Integration Tests
- `TestIntegration` - Tests the full migration workflow
- `TestOperationsIntegration` - Tests individual operations with real ArangoDB

## Test Data

Test migration files are located in `pkg/migrator/testdata/migrations/`:
- `000001_create_users.json` - Creates books collection with indexes
- `000002_create_posts.json` - Creates authors collection and library graph
- `000003_add_sample_data.json` - Adds sample books and authors

## CI/CD

In CI/CD environments:
- Unit tests run by default
- Integration tests run when Docker is available
- Tests are run with race detection enabled
- Coverage reports are generated

## Troubleshooting

### Docker Issues
If integration tests fail due to Docker issues:
1. Ensure Docker is running
2. Check that you have sufficient resources allocated to Docker
3. Try running with `-timeout 10m` if tests are timing out

### ArangoDB Container Issues
If ArangoDB containers fail to start:
1. Check Docker logs for container startup issues
2. Ensure port 8529 is not already in use
3. Try using a different ArangoDB image version

### Test Timeouts
If tests timeout:
1. Increase the test timeout: `go test -timeout 10m ./pkg/migrator`
2. Check system resources (CPU, memory)
3. Ensure Docker has sufficient resources allocated 