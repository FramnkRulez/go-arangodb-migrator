package migrator

import (
	"context"
	"testing"

	"github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCollection(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_create_collection")

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
}

func TestCreatePersistentIndex(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_create_index")

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
}

func TestCreateGeoIndex(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_create_geo_index")

	// Create a collection first
	err := createCollection(ctx, db, "test_collection", map[string]interface{}{
		"type": "document",
	})
	require.NoError(t, err)

	// Create a geo index
	err = createGeoIndex(ctx, db, "idx_geo_location", map[string]interface{}{
		"collection": "test_collection",
		"fields":     []string{"location"},
	})
	require.NoError(t, err)

	// Verify geo index exists
	coll, err := db.GetCollection(ctx, "test_collection", nil)
	require.NoError(t, err)

	indexes, err := coll.Indexes(ctx)
	require.NoError(t, err)

	found := false
	for _, index := range indexes {
		if index.Name == "idx_geo_location" {
			found = true
			break
		}
	}
	assert.True(t, found, "Geo index should exist")
}

func TestCreateGraph(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_create_graph")

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
}

func TestAddDocument(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_add_document")

	// Create a collection first
	err := createCollection(ctx, db, "test_collection", map[string]interface{}{
		"type": "document",
	})
	require.NoError(t, err)

	// Add a document
	document := map[string]interface{}{
		"_key":  "test_doc",
		"name":  "Test Document",
		"value": 42,
	}

	err = addDocument(ctx, db, "test_collection", map[string]interface{}{
		"document": document,
	})
	require.NoError(t, err)

	// Verify document exists
	coll, err := db.GetCollection(ctx, "test_collection", nil)
	require.NoError(t, err)

	var retrievedDoc map[string]interface{}
	_, err = coll.ReadDocument(ctx, "test_doc", &retrievedDoc)
	require.NoError(t, err)

	assert.Equal(t, "Test Document", retrievedDoc["name"])
	assert.Equal(t, float64(42), retrievedDoc["value"])
}

func TestUpdateDocument(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_update_document")

	// Create a collection first
	err := createCollection(ctx, db, "test_collection", map[string]interface{}{
		"type": "document",
	})
	require.NoError(t, err)

	// Add a document first
	err = addDocument(ctx, db, "test_collection", map[string]interface{}{
		"document": map[string]interface{}{
			"_key": "test_doc",
			"name": "Original Name",
		},
	})
	require.NoError(t, err)

	// Update the document
	err = updateDocument(ctx, db, "test_collection", map[string]interface{}{
		"_key": "test_doc",
		"name": "Updated Name",
	})
	require.NoError(t, err)

	// Verify document was updated
	coll, err := db.GetCollection(ctx, "test_collection", nil)
	require.NoError(t, err)

	var retrievedDoc map[string]interface{}
	_, err = coll.ReadDocument(ctx, "test_doc", &retrievedDoc)
	require.NoError(t, err)

	assert.Equal(t, "Updated Name", retrievedDoc["name"])
}

func TestDeleteDocument(t *testing.T) {
	ctx := context.Background()

	// Start ArangoDB container
	container := testutil.NewArangoDBContainer(ctx, t)
	defer container.Cleanup(ctx)

	// Create test database
	db := container.CreateTestDatabase(ctx, t, "test_delete_document")

	// Create a collection first
	err := createCollection(ctx, db, "test_collection", map[string]interface{}{
		"type": "document",
	})
	require.NoError(t, err)

	// Add a document first
	err = addDocument(ctx, db, "test_collection", map[string]interface{}{
		"document": map[string]interface{}{
			"_key": "test_doc",
			"name": "Test Document",
		},
	})
	require.NoError(t, err)

	// Delete the document
	err = deleteDocument(ctx, db, "test_collection", map[string]interface{}{
		"_key": "test_doc",
	})
	require.NoError(t, err)

	// Verify document was deleted
	coll, err := db.GetCollection(ctx, "test_collection", nil)
	require.NoError(t, err)

	exists, err := coll.DocumentExists(ctx, "test_doc")
	require.NoError(t, err)
	assert.False(t, exists, "Document should be deleted")
}
