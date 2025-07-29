package migrator

import (
	"context"
	"os"
	"testing"

	"github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration runs integration tests only when the INTEGRATION_TEST environment variable is set
func TestIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_integration")

	// Get the test migrations directory
	testMigrationsDir := "testdata/migrations"

	t.Run("full migration workflow", func(t *testing.T) {
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

// TestOperationsIntegration tests individual operations with a real ArangoDB instance
func TestOperationsIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_operations_integration")

	t.Run("create collection", func(t *testing.T) {
		// Test creating a document collection
		err := createCollection(ctx, db, "test_doc_collection", map[string]interface{}{
			"type": "document",
		})
		require.NoError(t, err)

		// Verify collection exists
		exists, err := db.CollectionExists(ctx, "test_doc_collection")
		require.NoError(t, err)
		assert.True(t, exists)

		// Test creating an edge collection
		err = createCollection(ctx, db, "test_edge_collection", map[string]interface{}{
			"type": "edge",
		})
		require.NoError(t, err)

		// Verify edge collection exists
		exists, err = db.CollectionExists(ctx, "test_edge_collection")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("create index", func(t *testing.T) {
		// Create a collection first
		err := createCollection(ctx, db, "test_collection", map[string]interface{}{
			"type": "document",
		})
		require.NoError(t, err)

		// Create a persistent index
		err = createPersistentIndex(ctx, db, "idx_test_field", map[string]interface{}{
			"collection": "test_collection",
			"fields":     []string{"field1"},
			"unique":     true,
			"sparse":     true,
		})
		require.NoError(t, err)

		// Verify index exists
		coll, err := db.GetCollection(ctx, "test_collection", nil)
		require.NoError(t, err)

		indexes, err := coll.Indexes(ctx)
		require.NoError(t, err)

		found := false
		for _, index := range indexes {
			if index.Name == "idx_test_field" {
				found = true
				break
			}
		}
		assert.True(t, found, "Index should exist")
	})

	t.Run("create graph", func(t *testing.T) {
		// Create collections first
		err := createCollection(ctx, db, "vertices", map[string]interface{}{
			"type": "document",
		})
		require.NoError(t, err)

		err = createCollection(ctx, db, "edges", map[string]interface{}{
			"type": "edge",
		})
		require.NoError(t, err)

		// Create a graph
		err = createGraph(ctx, db, "test_graph", map[string]interface{}{
			"edgeDefinitions": []map[string]interface{}{
				{
					"collection": "edges",
					"from":       []string{"vertices"},
					"to":         []string{"vertices"},
				},
			},
			"orphanedCollections": []string{},
		})
		require.NoError(t, err)

		// Verify graph exists
		exists, err := db.GraphExists(ctx, "test_graph")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("document operations", func(t *testing.T) {
		// Create a collection first
		err := createCollection(ctx, db, "test_doc_collection", map[string]interface{}{
			"type": "document",
		})
		require.NoError(t, err)

		// Add a document
		document := map[string]interface{}{
			"_key":  "test_doc",
			"name":  "Test Document",
			"value": 42,
		}

		err = addDocument(ctx, db, "test_doc_collection", map[string]interface{}{
			"document": document,
		})
		require.NoError(t, err)

		// Verify document exists
		coll, err := db.GetCollection(ctx, "test_doc_collection", nil)
		require.NoError(t, err)

		var retrievedDoc map[string]interface{}
		_, err = coll.ReadDocument(ctx, "test_doc", &retrievedDoc)
		require.NoError(t, err)

		assert.Equal(t, "Test Document", retrievedDoc["name"])
		assert.Equal(t, float64(42), retrievedDoc["value"])

		// Update the document
		err = updateDocument(ctx, db, "test_doc_collection", map[string]interface{}{
			"_key": "test_doc",
			"name": "Updated Name",
		})
		require.NoError(t, err)

		// Verify document was updated
		_, err = coll.ReadDocument(ctx, "test_doc", &retrievedDoc)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrievedDoc["name"])

		// Delete the document
		err = deleteDocument(ctx, db, "test_doc_collection", map[string]interface{}{
			"_key": "test_doc",
		})
		require.NoError(t, err)

		// Verify document was deleted
		exists, err := coll.DocumentExists(ctx, "test_doc")
		require.NoError(t, err)
		assert.False(t, exists, "Document should be deleted")
	})
}
