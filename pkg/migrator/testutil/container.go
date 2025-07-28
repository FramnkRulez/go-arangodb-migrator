// Package testutil provides utilities for testing the migrator package.
// It includes helpers for setting up and managing ArangoDB test containers.
//
// # Overview
//
// This package provides test utilities that make it easy to write integration tests
// for the migrator package. It uses testcontainers to spin up real ArangoDB instances
// for testing.
//
// # Usage
//
//	import "github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator/testutil"
//
//	func TestMyMigration(t *testing.T) {
//		ctx := context.Background()
//
//		// Start ArangoDB container
//		container := testutil.NewArangoDBContainer(ctx, t)
//		defer container.Cleanup(ctx)
//
//		// Create test database
//		db := container.CreateTestDatabase(ctx, t, "test_db")
//
//		// Run your tests...
//	}
//
// # Requirements
//
// This package requires Docker to be running and the testcontainers-go library.
// Integration tests should be skipped if Docker is not available.
package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/arangodb/go-driver/v2/connection"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ArangoDBContainer represents an ArangoDB test container
type ArangoDBContainer struct {
	Container testcontainers.Container
	Client    arangodb.Client
	Endpoint  string
}

// NewArangoDBContainer creates and starts a new ArangoDB container for testing
func NewArangoDBContainer(ctx context.Context, t require.TestingT) *ArangoDBContainer {
	// Define the ArangoDB container
	req := testcontainers.ContainerRequest{
		Image:        "arangodb/arangodb:latest",
		ExposedPorts: []string{"8529/tcp"},
		Env: map[string]string{
			"ARANGO_ROOT_PASSWORD": "testpassword",
		},
		WaitingFor: wait.ForHTTP("/_api/version").WithPort("8529/tcp"),
	}

	// Start the container
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	// Get the container's host and port
	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "8529")
	require.NoError(t, err)

	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	// Wait a bit for ArangoDB to be fully ready
	time.Sleep(2 * time.Second)

	// Create ArangoDB client
	conn := connection.NewHttpConnection(connection.HttpConfiguration{
		Endpoint: connection.NewRoundRobinEndpoints([]string{endpoint}),
	})

	auth := connection.NewBasicAuth("root", "testpassword")
	err = conn.SetAuthentication(auth)
	require.NoError(t, err)

	client := arangodb.NewClient(conn)

	// Verify connection
	_, err = client.Version(ctx)
	require.NoError(t, err)

	return &ArangoDBContainer{
		Container: container,
		Client:    client,
		Endpoint:  endpoint,
	}
}

// Cleanup stops and removes the container
func (c *ArangoDBContainer) Cleanup(ctx context.Context) error {
	return c.Container.Terminate(ctx)
}

// CreateTestDatabase creates a test database and returns it
func (c *ArangoDBContainer) CreateTestDatabase(ctx context.Context, t require.TestingT, dbName string) arangodb.Database {
	db, err := c.Client.CreateDatabase(ctx, dbName, nil)
	require.NoError(t, err)
	return db
}

// GetTestDatabase gets an existing database or creates it if it doesn't exist
func (c *ArangoDBContainer) GetTestDatabase(ctx context.Context, t require.TestingT, dbName string) arangodb.Database {
	exists, err := c.Client.DatabaseExists(ctx, dbName)
	require.NoError(t, err)

	if exists {
		db, err := c.Client.GetDatabase(ctx, dbName, nil)
		require.NoError(t, err)
		return db
	}

	return c.CreateTestDatabase(ctx, t, dbName)
}
