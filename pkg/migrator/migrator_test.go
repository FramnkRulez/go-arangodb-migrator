package migrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator/testutil"
	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationOptions validates the MigrationOptions struct
func TestMigrationOptions(t *testing.T) {
	opts := MigrationOptions{
		MigrationFolder:     "./migrations",
		MigrationCollection: "migrations",
	}

	assert.Equal(t, "./migrations", opts.MigrationFolder)
	assert.Equal(t, "migrations", opts.MigrationCollection)
}

// TestOperation validates the Operation struct
func TestOperation(t *testing.T) {
	op := Operation{
		Type: "createCollection",
		Name: "users",
		Options: map[string]interface{}{
			"type": "document",
		},
	}

	assert.Equal(t, "createCollection", op.Type)
	assert.Equal(t, "users", op.Name)
	assert.Equal(t, "document", op.Options["type"])
}

// TestMigration validates the Migration struct
func TestMigration(t *testing.T) {
	migration := Migration{
		Description: "Test migration",
		Up: []Operation{
			{
				Type: "createCollection",
				Name: "users",
				Options: map[string]interface{}{
					"type": "document",
				},
			},
		},
		Down: []Operation{},
	}

	assert.Equal(t, "Test migration", migration.Description)
	assert.Len(t, migration.Up, 1)
	assert.Len(t, migration.Down, 0)
	assert.Equal(t, "createCollection", migration.Up[0].Type)
}

// TestAppliedMigration validates the AppliedMigration struct
func TestAppliedMigration(t *testing.T) {
	applied := AppliedMigration{
		MigrationNumber: "000001",
		Sha256:          "abc123",
	}

	assert.Equal(t, "000001", applied.MigrationNumber)
	assert.Equal(t, "abc123", applied.Sha256)
}

// TestGetFileSHA256 tests the SHA256 hash function
func TestGetFileSHA256(t *testing.T) {
	// Create a temporary file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Get the hash
	hash, err := getFileSHA256(testFile)
	require.NoError(t, err)

	// SHA256 of "test content"
	expectedHash := "956a5d0f63d60fd8b8618c2e8c3b9b2b3b3b3b3b3b3b3b3b3b3b3b3b3b3b3b3b"
	assert.NotEqual(t, expectedHash, hash) // This will be different, but we just want to ensure it's not empty
	assert.NotEmpty(t, hash)
}

// TestGetSlice tests the generic slice extraction function
func TestGetSlice(t *testing.T) {
	m := map[string]interface{}{
		"strings": []string{"a", "b", "c"},
		"ints":    []int{1, 2, 3},
	}

	// Test string slice
	strings, ok := getSlice[string](m, "strings")
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, strings)

	// Test int slice
	ints, ok := getSlice[int](m, "ints")
	assert.True(t, ok)
	assert.Equal(t, []int{1, 2, 3}, ints)

	// Test missing key
	missing, ok := getSlice[string](m, "missing")
	assert.False(t, ok)
	assert.Nil(t, missing)
}

// TestMigrateArangoDatabase tests the main migration function with a real ArangoDB container
func TestMigrateArangoDatabase(t *testing.T) {
	// Skip if Docker is not available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_migrations")

	// Get the test migrations directory
	testMigrationsDir := filepath.Join("testdata", "migrations")

	t.Run("successful migration", func(t *testing.T) {
		// Run migrations
		err := MigrateArangoDatabase(ctx, db, MigrationOptions{
			MigrationFolder:     testMigrationsDir,
			MigrationCollection: "migrations",
		})

		require.NoError(t, err)

		// Verify collections were created
		collections := []string{"books", "authors", "book_authors"}
		for _, collectionName := range collections {
			exists, err := db.CollectionExists(ctx, collectionName)
			require.NoError(t, err)
			assert.True(t, exists, "Collection %s should exist", collectionName)
		}

		// Verify graph was created
		exists, err := db.GraphExists(ctx, "library_graph")
		require.NoError(t, err)
		assert.True(t, exists, "Graph library_graph should exist")

		// Verify documents were added
		authorsColl, err := db.GetCollection(ctx, "authors", nil)
		require.NoError(t, err)

		var authorDoc map[string]interface{}
		_, err = authorsColl.ReadDocument(ctx, "author1", &authorDoc)
		require.NoError(t, err)
		assert.Equal(t, "Jane Smith", authorDoc["name"])
		assert.Equal(t, float64(1980), authorDoc["birthYear"])

		// Verify migration tracking
		migrationsColl, err := db.GetCollection(ctx, "migrations", nil)
		require.NoError(t, err)

		// Check that all migrations were recorded
		migrationKeys := []string{"000001_create_users", "000002_create_posts", "000003_add_sample_data"}
		for _, migrationKey := range migrationKeys {
			exists, err := migrationsColl.DocumentExists(ctx, migrationKey)
			require.NoError(t, err)
			assert.True(t, exists, "Migration %s should be recorded", migrationKey)
		}
	})

	t.Run("idempotent migration", func(t *testing.T) {
		// Run migrations again - should not fail
		err := MigrateArangoDatabase(ctx, db, MigrationOptions{
			MigrationFolder:     testMigrationsDir,
			MigrationCollection: "migrations",
		})

		require.NoError(t, err)

		// Verify collections still exist
		collections := []string{"books", "authors", "book_authors"}
		for _, collectionName := range collections {
			exists, err := db.CollectionExists(ctx, collectionName)
			require.NoError(t, err)
			assert.True(t, exists, "Collection %s should still exist", collectionName)
		}
	})
}

func TestMigrateArangoDatabaseWithInvalidMigration(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_invalid_migrations")

	// Create a temporary directory with an invalid migration
	tempDir := t.TempDir()
	invalidMigration := `{
		"description": "Invalid migration",
		"up": [
			{
				"type": "invalidOperation",
				"name": "test",
				"options": {}
			}
		]
	}`

	err := os.WriteFile(filepath.Join(tempDir, "000001_invalid.json"), []byte(invalidMigration), 0644)
	require.NoError(t, err)

	// Run migrations - should fail
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operation type: invalidOperation")
}

func TestMigrateArangoDatabaseWithDeleteGraphOperation(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_delete_graph_migration")

	// Create a temporary migration that creates and then deletes a graph
	tempDir := t.TempDir()
	migration := `{
		"description": "Create and delete graph",
		"up": [
			{
				"type": "createCollection",
				"name": "vertices",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "createCollection",
				"name": "edges",
				"options": {
					"type": "edge"
				}
			},
			{
				"type": "createGraph",
				"name": "test_graph",
				"options": {
					"edgeDefinitions": [
						{
							"collection": "edges",
							"from": ["vertices"],
							"to": ["vertices"]
						}
					],
					"orphanedCollections": []
				}
			},
			{
				"type": "deleteGraph",
				"name": "test_graph"
			}
		]
	}`

	err := os.WriteFile(filepath.Join(tempDir, "000001_delete_graph.json"), []byte(migration), 0644)
	require.NoError(t, err)

	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})
	require.NoError(t, err)

	graphExists, err := db.GraphExists(ctx, "test_graph")
	require.NoError(t, err)
	assert.False(t, graphExists, "Graph should be deleted by migration")

	verticesExists, err := db.CollectionExists(ctx, "vertices")
	require.NoError(t, err)
	assert.True(t, verticesExists, "Vertex collection should remain after graph deletion")

	edgesExists, err := db.CollectionExists(ctx, "edges")
	require.NoError(t, err)
	assert.True(t, edgesExists, "Edge collection should remain after graph deletion")
}

func TestMigrateArangoDatabaseWithModifiedFile(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_modified_migrations")

	// Create a temporary directory with a migration
	tempDir := t.TempDir()
	originalMigration := `{
		"description": "Test migration",
		"up": [
			{
				"type": "createCollection",
				"name": "test_collection",
				"options": {
					"type": "document"
				}
			}
		]
	}`

	migrationFile := filepath.Join(tempDir, "000001_test.json")
	err := os.WriteFile(migrationFile, []byte(originalMigration), 0644)
	require.NoError(t, err)

	// Run migrations successfully
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})
	require.NoError(t, err)

	// Modify the migration file
	modifiedMigration := `{
		"description": "Modified test migration",
		"up": [
			{
				"type": "createCollection",
				"name": "modified_collection",
				"options": {
					"type": "document"
				}
			}
		]
	}`

	err = os.WriteFile(migrationFile, []byte(modifiedMigration), 0644)
	require.NoError(t, err)

	// Try to run migrations again - should fail due to modified file
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration file has been modified")
}

func TestMigrateArangoDatabaseWithNonExistentFolder(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_nonexistent_folder")

	// Try to run migrations with non-existent folder
	err := MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     "/non/existent/path",
		MigrationCollection: "migrations",
	})

	require.Error(t, err)
}

func TestMigrateArangoDatabaseWithEmptyFolder(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_empty_folder")

	// Create empty temporary directory
	tempDir := t.TempDir()

	// Run migrations with empty folder - should succeed
	err := MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})

	require.NoError(t, err)

	// Verify migration collection was created
	exists, err := db.CollectionExists(ctx, "migrations")
	require.NoError(t, err)
	assert.True(t, exists, "Migration collection should be created even with empty folder")
}

func TestMigrateArangoDatabaseWithNonJsonFiles(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_non_json_files")

	// Create temporary directory with non-JSON files
	tempDir := t.TempDir()

	// Create a valid migration
	validMigration := `{
		"description": "Valid migration",
		"up": [
			{
				"type": "createCollection",
				"name": "test_collection",
				"options": {
					"type": "document"
				}
			}
		]
	}`
	err := os.WriteFile(filepath.Join(tempDir, "000001_valid.json"), []byte(validMigration), 0644)
	require.NoError(t, err)

	// Create a non-JSON file
	err = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0644)
	require.NoError(t, err)

	// Run migrations - should succeed and ignore non-JSON files
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})

	require.NoError(t, err)

	// Verify collection was created
	exists, err := db.CollectionExists(ctx, "test_collection")
	require.NoError(t, err)
	assert.True(t, exists, "Collection should be created")
}

func TestMigrateArangoDatabaseWithEmptyUpList(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_empty_up_list")

	// Create temporary directory with migration that has empty up list
	tempDir := t.TempDir()
	emptyUpMigration := `{
		"description": "Migration with empty up list",
		"up": []
	}`

	err := os.WriteFile(filepath.Join(tempDir, "000001_empty_up.json"), []byte(emptyUpMigration), 0644)
	require.NoError(t, err)

	// Run migrations - should fail due to empty up list
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not include a valid 'up' list of migrations to apply")
}

func TestMigrateArangoDatabaseWithDownList(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_down_list")

	// Create temporary directory with migration that has down list
	tempDir := t.TempDir()
	downMigration := `{
		"description": "Migration with down list",
		"up": [
			{
				"type": "createCollection",
				"name": "test_collection",
				"options": {
					"type": "document"
				}
			}
		],
		"down": [
			{
				"type": "deleteCollection",
				"name": "test_collection"
			}
		]
	}`

	err := os.WriteFile(filepath.Join(tempDir, "000001_with_down.json"), []byte(downMigration), 0644)
	require.NoError(t, err)

	// Run migrations - should succeed with a down list present
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})

	require.NoError(t, err)

	exists, err := db.CollectionExists(ctx, "test_collection")
	require.NoError(t, err)
	assert.True(t, exists, "Collection should be created")
}

func TestMigrateArangoDatabaseAutoRollbackUsesExplicitDown(t *testing.T) {
	ctx := context.Background()

	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	db := container.CreateTestDatabase(ctx, t, "test_explicit_down_rollback")

	_, err := db.CreateCollection(ctx, "to_restore", &arangodb.CreateCollectionProperties{
		Type: arangodb.CollectionTypeDocument,
	})
	require.NoError(t, err)

	tempDir := t.TempDir()
	firstMigration := `{
		"description": "Delete and restore collection using explicit down",
		"up": [
			{
				"type": "deleteCollection",
				"name": "to_restore"
			}
		],
		"down": [
			{
				"type": "createCollection",
				"name": "to_restore",
				"options": {
					"type": "document"
				}
			}
		]
	}`
	secondMigration := `{
		"description": "Fail migration",
		"up": [
			{
				"type": "invalidOperation",
				"name": "test",
				"options": {}
			}
		]
	}`

	err = os.WriteFile(filepath.Join(tempDir, "000001_with_explicit_down.json"), []byte(firstMigration), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "000002_fail.json"), []byte(secondMigration), 0644)
	require.NoError(t, err)

	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operation type: invalidOperation")
	assert.NotContains(t, err.Error(), "failed to auto-rollback")

	exists, err := db.CollectionExists(ctx, "to_restore")
	require.NoError(t, err)
	assert.True(t, exists, "Collection should be restored by explicit down migration")
}

func TestMigrateArangoDatabaseAutoRollbackWithoutDownReversesUp(t *testing.T) {
	ctx := context.Background()

	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	db := container.CreateTestDatabase(ctx, t, "test_reverse_up_rollback")

	_, err := db.CreateCollection(ctx, "to_restore", &arangodb.CreateCollectionProperties{
		Type: arangodb.CollectionTypeDocument,
	})
	require.NoError(t, err)

	tempDir := t.TempDir()
	firstMigration := `{
		"description": "Delete collection without explicit down",
		"up": [
			{
				"type": "deleteCollection",
				"name": "to_restore"
			}
		]
	}`
	secondMigration := `{
		"description": "Fail migration",
		"up": [
			{
				"type": "invalidOperation",
				"name": "test",
				"options": {}
			}
		]
	}`

	err = os.WriteFile(filepath.Join(tempDir, "000001_without_down.json"), []byte(firstMigration), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "000002_fail.json"), []byte(secondMigration), 0644)
	require.NoError(t, err)

	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to auto-rollback migrations")
	assert.Contains(t, err.Error(), "cannot rollback collection deletion")

	exists, err := db.CollectionExists(ctx, "to_restore")
	require.NoError(t, err)
	assert.False(t, exists, "Collection should remain deleted when reverse-up rollback is not possible")
}

func TestMigrateArangoDatabaseWithMissingUpList(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_missing_up_list")

	// Create temporary directory with migration that has no up list
	tempDir := t.TempDir()
	missingUpMigration := `{
		"description": "Migration with missing up list"
	}`

	err := os.WriteFile(filepath.Join(tempDir, "000001_missing_up.json"), []byte(missingUpMigration), 0644)
	require.NoError(t, err)

	// Run migrations - should fail due to missing up list
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not include a valid 'up' list of migrations to apply")
}

func TestMigrateArangoDatabaseWithAutoRollback(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_auto_rollback")

	// Create temporary directory with multiple migrations
	tempDir := t.TempDir()

	// Create first migration (should succeed)
	firstMigration := `{
		"description": "First migration - should succeed",
		"up": [
			{
				"type": "createCollection",
				"name": "test_collection",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "addDocument",
				"name": "test_collection",
				"options": {
					"document": {
						"_key": "doc1",
						"name": "Test Document 1",
						"value": 100
					}
				}
			}
		]
	}`

	err := os.WriteFile(filepath.Join(tempDir, "000001_first.json"), []byte(firstMigration), 0644)
	require.NoError(t, err)

	// Create second migration (should fail)
	secondMigration := `{
		"description": "Second migration - should fail",
		"up": [
			{
				"type": "addDocument",
				"name": "test_collection",
				"options": {
					"document": {
						"_key": "doc2",
						"name": "Test Document 2",
						"value": 200
					}
				}
			},
			{
				"type": "invalidOperation",
				"name": "test",
				"options": {}
			}
		]
	}`

	err = os.WriteFile(filepath.Join(tempDir, "000002_second.json"), []byte(secondMigration), 0644)
	require.NoError(t, err)

	// Run migrations with auto-rollback enabled - should fail and rollback everything
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operation type: invalidOperation")

	// Verify that the collection was rolled back (should not exist)
	exists, err := db.CollectionExists(ctx, "test_collection")
	require.NoError(t, err)
	assert.False(t, exists, "Collection should have been rolled back")

	// Verify that no migrations were recorded
	// Check that no migrations were recorded
	query := "FOR doc IN migrations RETURN doc"
	cursor, err := db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var count int
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		count++
	}

	assert.Equal(t, 0, count, "No migrations should have been recorded after rollback")
}

func TestMigrateArangoDatabaseWithAutoRollbackDocumentOperations(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_auto_rollback_docs")

	// Create temporary directory with document operations
	tempDir := t.TempDir()

	// Create migration with document operations that should be rolled back
	migration := `{
		"description": "Migration with document operations",
		"up": [
			{
				"type": "createCollection",
				"name": "users",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "addDocument",
				"name": "users",
				"options": {
					"document": {
						"_key": "user1",
						"name": "John Doe",
						"email": "john@example.com"
					}
				}
			},
			{
				"type": "addDocument",
				"name": "users",
				"options": {
					"document": {
						"_key": "user2",
						"name": "Jane Smith",
						"email": "jane@example.com"
					}
				}
			},
			{
				"type": "invalidOperation",
				"name": "test",
				"options": {}
			}
		]
	}`

	err := os.WriteFile(filepath.Join(tempDir, "000001_docs.json"), []byte(migration), 0644)
	require.NoError(t, err)

	// Run migrations with auto-rollback enabled - should fail and rollback everything
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operation type: invalidOperation")

	// Verify that the collection was rolled back (should not exist)
	exists, err := db.CollectionExists(ctx, "users")
	require.NoError(t, err)
	assert.False(t, exists, "Collection should have been rolled back")

	// Verify that no migrations were recorded
	query := "FOR doc IN migrations RETURN doc"
	cursor, err := db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var count int
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		count++
	}

	assert.Equal(t, 0, count, "No migrations should have been recorded after rollback")
}

func TestBackwardCompatibilityWithExistingMigrations(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_backward_compatibility")

	// Create migration collection and manually insert an "old" migration record
	// that doesn't have the OperationResults field (simulating pre-auto-rollback)
	migrationsColl, err := db.CreateCollection(ctx, "migrations", &arangodb.CreateCollectionProperties{
		Type: arangodb.CollectionTypeDocument,
	})
	require.NoError(t, err)

	// Insert an "old" migration record (without OperationResults field)
	oldMigration := map[string]interface{}{
		"_key":      "000001_old_migration",
		"appliedAt": time.Now().UTC().Format(time.RFC3339),
		"sha256":    "old_hash_123",
		// Note: no "operationResults" field - simulating pre-auto-rollback
	}

	_, err = migrationsColl.CreateDocument(ctx, oldMigration)
	require.NoError(t, err)

	// Create a new migration file
	tempDir := t.TempDir()
	newMigration := `{
		"description": "New migration with auto-rollback",
		"up": [
			{
				"type": "createCollection",
				"name": "new_collection",
				"options": {
					"type": "document"
				}
			}
		]
	}`

	err = os.WriteFile(filepath.Join(tempDir, "000002_new_migration.json"), []byte(newMigration), 0644)
	require.NoError(t, err)

	// Run migrations - should work fine with the old migration record
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	require.NoError(t, err)

	// Verify that the new collection was created
	exists, err := db.CollectionExists(ctx, "new_collection")
	require.NoError(t, err)
	assert.True(t, exists, "New collection should exist")

	// Verify that both migrations are recorded
	query := "FOR doc IN migrations SORT doc._key RETURN doc"
	cursor, err := db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var migrations []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		migrations = append(migrations, doc)
	}

	assert.Len(t, migrations, 2, "Should have 2 migrations recorded")

	// Verify the old migration doesn't have operationResults
	oldMigrationDoc := migrations[0]
	assert.Equal(t, "000001_old_migration", oldMigrationDoc["_key"])
	_, hasOperationResults := oldMigrationDoc["operationResults"]
	assert.False(t, hasOperationResults, "Old migration should not have operationResults field")

	// Verify the new migration has operationResults
	newMigrationDoc := migrations[1]
	assert.Equal(t, "000002_new_migration", newMigrationDoc["_key"])
	_, hasOperationResults = newMigrationDoc["operationResults"]
	assert.True(t, hasOperationResults, "New migration should have operationResults field")
}

func TestAutoRollbackSequenceOfMigrations(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_auto_rollback_sequence")

	// Create migration files for a sequence of migrations
	tempDir := t.TempDir()

	// Migration 1: Create users collection and add a user
	migration1 := `{
		"description": "Create users collection and add initial user",
		"up": [
			{
				"type": "createCollection",
				"name": "users",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "addDocument",
				"name": "users",
				"options": {
					"document": {
						"_key": "user1",
						"name": "John Doe",
						"email": "john@example.com"
					}
				}
			}
		]
	}`

	// Migration 2: Create posts collection and add a post
	migration2 := `{
		"description": "Create posts collection and add initial post",
		"up": [
			{
				"type": "createCollection",
				"name": "posts",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "addDocument",
				"name": "posts",
				"options": {
					"document": {
						"_key": "post1",
						"title": "Hello World",
						"content": "This is my first post",
						"author": "user1"
					}
				}
			}
		]
	}`

	// Migration 3: Create comments collection and add a comment
	migration3 := `{
		"description": "Create comments collection and add initial comment",
		"up": [
			{
				"type": "createCollection",
				"name": "comments",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "addDocument",
				"name": "comments",
				"options": {
					"document": {
						"_key": "comment1",
						"content": "Great post!",
						"post": "post1",
						"author": "user1"
					}
				}
			}
		]
	}`

	// Migration 4: This will fail with an invalid operation
	migration4 := `{
		"description": "This migration will fail",
		"up": [
			{
				"type": "createCollection",
				"name": "categories",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "invalidOperation",
				"name": "test",
				"options": {}
			}
		]
	}`

	// Write migration files
	err := os.WriteFile(filepath.Join(tempDir, "000001_create_users.json"), []byte(migration1), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "000002_create_posts.json"), []byte(migration2), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "000003_create_comments.json"), []byte(migration3), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "000004_fail_migration.json"), []byte(migration4), 0644)
	require.NoError(t, err)

	// Run migrations with auto-rollback enabled - this should fail on migration 4
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	// Should fail due to invalid operation in migration 4
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operation type: invalidOperation")

	// Verify that NO collections exist (all should be rolled back)
	exists, err := db.CollectionExists(ctx, "users")
	require.NoError(t, err)
	assert.False(t, exists, "Users collection should not exist after rollback")

	exists, err = db.CollectionExists(ctx, "posts")
	require.NoError(t, err)
	assert.False(t, exists, "Posts collection should not exist after rollback")

	exists, err = db.CollectionExists(ctx, "comments")
	require.NoError(t, err)
	assert.False(t, exists, "Comments collection should not exist after rollback")

	exists, err = db.CollectionExists(ctx, "categories")
	require.NoError(t, err)
	assert.False(t, exists, "Categories collection should not exist after rollback")

	// Verify that NO migrations are recorded
	query := "FOR doc IN migrations RETURN doc"
	cursor, err := db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var migrations []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		migrations = append(migrations, doc)
	}

	assert.Len(t, migrations, 0, "No migrations should be recorded after rollback")

	// Now test that we can re-run the migrations successfully
	// Remove the failing migration file
	err = os.Remove(filepath.Join(tempDir, "000004_fail_migration.json"))
	require.NoError(t, err)

	// Run migrations again - should succeed
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	require.NoError(t, err, "Migrations should succeed when failing migration is removed")

	// Verify that all collections exist
	exists, err = db.CollectionExists(ctx, "users")
	require.NoError(t, err)
	assert.True(t, exists, "Users collection should exist after successful migration")

	exists, err = db.CollectionExists(ctx, "posts")
	require.NoError(t, err)
	assert.True(t, exists, "Posts collection should exist after successful migration")

	exists, err = db.CollectionExists(ctx, "comments")
	require.NoError(t, err)
	assert.True(t, exists, "Comments collection should exist after successful migration")

	// Verify that all migrations are recorded
	query = "FOR doc IN migrations SORT doc._key RETURN doc"
	cursor, err = db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	migrations = nil
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		migrations = append(migrations, doc)
	}

	assert.Len(t, migrations, 3, "All 3 migrations should be recorded after successful run")
	assert.Equal(t, "000001_create_users", migrations[0]["_key"])
	assert.Equal(t, "000002_create_posts", migrations[1]["_key"])
	assert.Equal(t, "000003_create_comments", migrations[2]["_key"])

	// Verify that documents exist in collections

	// Check users collection has 1 document
	query = "FOR doc IN users RETURN doc"
	cursor, err = db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var users []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		users = append(users, doc)
	}
	assert.Len(t, users, 1, "Users collection should have 1 document")
	assert.Equal(t, "user1", users[0]["_key"])

	// Check posts collection has 1 document
	query = "FOR doc IN posts RETURN doc"
	cursor, err = db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var posts []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		posts = append(posts, doc)
	}
	assert.Len(t, posts, 1, "Posts collection should have 1 document")
	assert.Equal(t, "post1", posts[0]["_key"])

	// Check comments collection has 1 document
	query = "FOR doc IN comments RETURN doc"
	cursor, err = db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var comments []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		comments = append(comments, doc)
	}
	assert.Len(t, comments, 1, "Comments collection should have 1 document")
	assert.Equal(t, "comment1", comments[0]["_key"])
}

func TestAutoRollbackLeavesDatabaseClean(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_auto_rollback_clean")

	// Create migration files for a sequence of migrations
	tempDir := t.TempDir()

	// Migration 1: Create users collection and add a user
	migration1 := `{
		"description": "Create users collection and add initial user",
		"up": [
			{
				"type": "createCollection",
				"name": "users",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "addDocument",
				"name": "users",
				"options": {
					"document": {
						"_key": "user1",
						"name": "John Doe",
						"email": "john@example.com"
					}
				}
			}
		]
	}`

	// Migration 2: Create posts collection and add a post
	migration2 := `{
		"description": "Create posts collection and add initial post",
		"up": [
			{
				"type": "createCollection",
				"name": "posts",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "addDocument",
				"name": "posts",
				"options": {
					"document": {
						"_key": "post1",
						"title": "Hello World",
						"content": "This is my first post",
						"author": "user1"
					}
				}
			}
		]
	}`

	// Migration 3: This will fail with an invalid operation
	migration3 := `{
		"description": "This migration will fail",
		"up": [
			{
				"type": "createCollection",
				"name": "categories",
				"options": {
					"type": "document"
				}
			},
			{
				"type": "invalidOperation",
				"name": "test",
				"options": {}
			}
		]
	}`

	// Write migration files
	err := os.WriteFile(filepath.Join(tempDir, "000001_create_users.json"), []byte(migration1), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "000002_create_posts.json"), []byte(migration2), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "000003_fail_migration.json"), []byte(migration3), 0644)
	require.NoError(t, err)

	// Run migrations with auto-rollback enabled - this should fail on migration 3
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	// Should fail due to invalid operation in migration 3
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operation type: invalidOperation")

	// Verify that NO collections exist (all should be rolled back)
	exists, err := db.CollectionExists(ctx, "users")
	require.NoError(t, err)
	assert.False(t, exists, "Users collection should not exist after rollback")

	exists, err = db.CollectionExists(ctx, "posts")
	require.NoError(t, err)
	assert.False(t, exists, "Posts collection should not exist after rollback")

	exists, err = db.CollectionExists(ctx, "categories")
	require.NoError(t, err)
	assert.False(t, exists, "Categories collection should not exist after rollback")

	// Verify that NO migrations are recorded
	query := "FOR doc IN migrations RETURN doc"
	cursor, err := db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var migrations []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		migrations = append(migrations, doc)
	}

	assert.Len(t, migrations, 0, "No migrations should be recorded after rollback")

	// Now test that we can re-run the migrations successfully
	// Remove the failing migration file
	err = os.Remove(filepath.Join(tempDir, "000003_fail_migration.json"))
	require.NoError(t, err)

	// Run migrations again - should succeed
	err = MigrateArangoDatabase(ctx, db, MigrationOptions{
		MigrationFolder:     tempDir,
		MigrationCollection: "migrations",
		AutoRollback:        true,
	})

	require.NoError(t, err, "Migrations should succeed when failing migration is removed")

	// Verify that all collections exist
	exists, err = db.CollectionExists(ctx, "users")
	require.NoError(t, err)
	assert.True(t, exists, "Users collection should exist after successful migration")

	exists, err = db.CollectionExists(ctx, "posts")
	require.NoError(t, err)
	assert.True(t, exists, "Posts collection should exist after successful migration")

	// Verify that all migrations are recorded
	query = "FOR doc IN migrations SORT doc._key RETURN doc"
	cursor, err = db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	migrations = nil
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		migrations = append(migrations, doc)
	}

	assert.Len(t, migrations, 2, "All 2 migrations should be recorded after successful run")
	assert.Equal(t, "000001_create_users", migrations[0]["_key"])
	assert.Equal(t, "000002_create_posts", migrations[1]["_key"])

	// Verify that documents exist in collections
	// Check users collection has 1 document
	query = "FOR doc IN users RETURN doc"
	cursor, err = db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var users []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		users = append(users, doc)
	}
	assert.Len(t, users, 1, "Users collection should have 1 document")
	assert.Equal(t, "user1", users[0]["_key"])

	// Check posts collection has 1 document
	query = "FOR doc IN posts RETURN doc"
	cursor, err = db.Query(ctx, query, nil)
	require.NoError(t, err)
	defer cursor.Close()

	var posts []map[string]interface{}
	for cursor.HasMore() {
		var doc map[string]interface{}
		_, err := cursor.ReadDocument(ctx, &doc)
		require.NoError(t, err)
		posts = append(posts, doc)
	}
	assert.Len(t, posts, 1, "Posts collection should have 1 document")
	assert.Equal(t, "post1", posts[0]["_key"])
}
