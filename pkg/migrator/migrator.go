// Package migrator provides a simple and reliable way to manage database migrations for ArangoDB.
// It supports creating collections, indexes, graphs, and documents with automatic rollback on failure.
//
// # Overview
//
// This package provides a migration system for ArangoDB that:
//   - Applies migrations in order based on numeric filename prefixes
//   - Supports rollback on failure
//   - Verifies file integrity using SHA256 hashes
//   - Tracks applied migrations in a collection
//   - Supports various ArangoDB operations (collections, indexes, graphs, documents)
//
// # Migration Files
//
// Migration files should be JSON files with numeric prefixes (e.g., "000001.json", "000002.json").
// Each file contains a description and operations to perform.
//
// # Supported Operations
//
//   - createCollection: Create document or edge collections
//   - deleteCollection: Remove collections
//   - createPersistentIndex: Create persistent indexes
//   - createGeoIndex: Create geo indexes
//   - deleteIndex: Remove indexes
//   - createGraph: Create named graphs
//   - deleteGraph: Remove graphs
//   - addEdgeDefinition: Add edge definitions to graphs
//   - deleteEdgeDefinition: Remove edge definitions
//   - addDocument: Add documents to collections
//   - updateDocument: Update existing documents
//   - deleteDocument: Remove documents
//
// # Example Usage
//
//	import "github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator"
//
//	// Connect to ArangoDB and get a database instance
//	db, err := client.GetDatabase(ctx, "my_database", nil)
//	if err != nil {
//		return err
//	}
//
//	// Run migrations
//	err = migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
//		MigrationFolder:     "./migrations",
//		MigrationCollection: "migrations",
//		Force:               false, // Set to true to bypass file modification checks
//	})
//
// # CLI Tool
//
// A command-line tool is also available for running migrations:
//
//	go install github.com/FramnkRulez/go-arangodb-migrator/cmd/migrator@latest
//
// Or use Docker:
//
//	docker run --rm framnkrulez/go-arangodb-migrator:latest --help
//
// # Security
//
// The package includes SHA256 hash verification to prevent modified migration files
// from being applied. Use the Force option to bypass this check if needed.
package migrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/arangodb/go-driver/v2/arangodb/shared"
	"github.com/sirupsen/logrus"
)

// MigrationOptions configures the migration process.
type MigrationOptions struct {
	// MigrationFolder is the path to the directory containing migration files.
	// Migration files should be named with a numeric prefix (e.g., "000001.json").
	MigrationFolder string

	// MigrationCollection is the name of the collection that tracks applied migrations.
	// This collection will be created automatically if it doesn't exist.
	MigrationCollection string

	// Force allows migration to proceed even if migration files have been modified
	// since they were last applied. This bypasses the SHA256 integrity check.
	Force bool
}

// Operation represents a single migration operation.
type Operation struct {
	// Type specifies the operation type (e.g., "createCollection", "createPersistentIndex").
	Type string `json:"type"`

	// Name is the name of the resource being operated on (e.g., collection name, index name).
	Name string `json:"name"`

	// Options contains operation-specific configuration.
	Options map[string]interface{} `json:"options"`
}

// Migration represents a complete migration with up and down operations.
type Migration struct {
	// Description provides a human-readable description of what this migration does.
	Description string `json:"description"`

	// Up contains the operations to apply when migrating forward.
	Up []Operation `json:"up"`

	// Down contains the operations to apply when rolling back (currently not implemented).
	Down []Operation `json:"down"`
}

// AppliedMigration tracks a migration that has been successfully applied.
type AppliedMigration struct {
	// MigrationNumber is the migration file name without extension (e.g., "000001").
	MigrationNumber string `json:"_key"`

	// AppliedAt is the timestamp when the migration was applied.
	AppliedAt time.Time `json:"appliedAt"`

	// Sha256 is the hash of the migration file for integrity verification.
	Sha256 string `json:"sha256"`
}

// MigrateArangoDatabase applies all pending migrations to the specified database.
// Migrations are applied in order based on their numeric filename prefix.
// If any migration fails, all previously applied operations in that migration are rolled back.
//
// The function performs the following steps:
//  1. Creates the migration collection if it doesn't exist
//  2. Reads all .json files from the migration folder
//  3. Sorts migrations by numeric filename prefix
//  4. Applies migrations that haven't been applied yet
//  5. Verifies file integrity using SHA256 hashes (unless Force is true)
//  6. Rolls back on failure
//
// # Parameters
//
//   - ctx: Context for cancellation and timeouts
//   - db: ArangoDB database instance
//   - options: Migration configuration options
//
// # Returns
//
// Returns an error if any migration fails. On failure, all operations
// from the failed migration are rolled back.
//
// # Examples
//
// Basic usage:
//
//	err := migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
//		MigrationFolder:     "./migrations",
//		MigrationCollection: "migrations",
//	})
//
// With force option:
//
//	err := migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
//		MigrationFolder:     "./migrations",
//		MigrationCollection: "migrations",
//		Force:               true, // Bypass file modification checks
//	})
func MigrateArangoDatabase(ctx context.Context, db arangodb.Database, options MigrationOptions) error {
	var err error
	var migrationColl arangodb.Collection
	migrationColl, err = db.GetCollection(ctx, options.MigrationCollection, &arangodb.GetCollectionOptions{
		SkipExistCheck: false,
	})
	if err != nil {
		if shared.IsNotFound(err) {
			migrationColl, err = db.CreateCollection(ctx, options.MigrationCollection, &arangodb.CreateCollectionProperties{
				Type: arangodb.CollectionTypeDocument,
			})
			if err != nil {
				return fmt.Errorf("failed to create migration collection in specified db: %v", err)
			}
		} else {
			return err
		}
	}

	// Get all migrations from the migration folder
	migrations, err := os.ReadDir(options.MigrationFolder)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if strings.HasSuffix(migration.Name(), ".json") {
			migrationNumber := strings.TrimSuffix(migration.Name(), ".json")

			fullpath := filepath.Join(options.MigrationFolder, migration.Name())

			hash, err := getFileSHA256(fullpath)
			if err != nil {
				return fmt.Errorf("failed to compute hash for migration file: %v", err)
			}

			var appliedMigration *AppliedMigration
			_, err = migrationColl.ReadDocument(ctx, migrationNumber, &appliedMigration)
			if err != nil {
				if !shared.IsNotFound(err) {
					return fmt.Errorf("failed to read applied migration: %v", err)
				}
			}

			if appliedMigration != nil {
				if appliedMigration.Sha256 != hash {
					if options.Force {
						logrus.Warnf("migration file %s has been modified since last applied, but continuing due to force flag", migrationNumber)
					} else {
						return fmt.Errorf("migration file has been modified since last applied: %s (use --force to override)", migrationNumber)
					}
				}
			}

			exists, err := migrationColl.DocumentExists(ctx, migrationNumber)
			if err != nil {
				return fmt.Errorf("failed to check if migration exists: %v", err)
			}

			if exists {
				logrus.Infof("migration %s already applied, skipping...", migrationNumber)
				continue
			}

			// Read the migration file
			migrationFile, err := os.ReadFile(filepath.Join(options.MigrationFolder, migration.Name()))
			if err != nil {
				return err
			}

			// Parse the migration file
			migration := &Migration{}
			err = json.Unmarshal(migrationFile, migration)
			if err != nil {
				return err
			}

			// Validate migration structure
			if len(migration.Up) == 0 {
				return fmt.Errorf("migration file %s does not include a valid 'up' list of migrations to apply", migrationNumber)
			}

			if len(migration.Down) > 0 {
				return fmt.Errorf("migration file %s has a 'down' list of migrations, but down migrations are not yet supported", migrationNumber)
			}

			appliedOperations := []Operation{}

			for _, operation := range migration.Up {
				switch operation.Type {
				case "createCollection":
					err = createCollection(ctx, db, operation.Name, operation.Options)
				case "createPersistentIndex":
					err = createPersistentIndex(ctx, db, operation.Name, operation.Options)
				case "createGeoIndex":
					err = createGeoIndex(ctx, db, operation.Name, operation.Options)
				case "createGraph":
					err = createGraph(ctx, db, operation.Name, operation.Options)
				case "addEdgeDefinition":
					err = addEdgeDefinition(ctx, db, operation.Name, operation.Options)
				case "deleteIndex":
					err = deleteIndex(ctx, db, operation.Name, operation.Options)
				case "deleteEdgeDefinition":
					err = deleteEdgeDefinition(ctx, db, operation.Name, operation.Options)
				case "deleteCollection":
					err = deleteCollection(ctx, db, operation.Name)
				case "addDocument":
					err = addDocument(ctx, db, operation.Name, operation.Options)
				case "updateDocument":
					err = updateDocument(ctx, db, operation.Name, operation.Options)
				case "deleteDocument":
					err = deleteDocument(ctx, db, operation.Name, operation.Options)
				default:
					err = fmt.Errorf("unsupported operation type: %s", operation.Type)
				}

				if err != nil {
					logrus.Errorf("migration operation failed for migration %s on %s: %v", migrationNumber, operation.Type, err)
					logrus.Error("rolling back applied operations...")
					rollbackErr := rollback(ctx, db, appliedOperations)
					if rollbackErr != nil {
						logrus.Errorf("failed to rollback migration: %v", rollbackErr)
						logrus.Error("database may be in an unclean state")
						return fmt.Errorf("failed to rollback migration: %v", rollbackErr)
					}
					return fmt.Errorf("migration operation failed for migration %s: %v", migrationNumber, err)
				}

				appliedOperations = append(appliedOperations, operation)
			}

			// Mark migration as applied
			_, err = migrationColl.CreateDocument(ctx, &AppliedMigration{
				MigrationNumber: migrationNumber,
				AppliedAt:       time.Now(),
				Sha256:          hash,
			})
			if err != nil {
				return fmt.Errorf("failed to mark migration as applied: %v", err)
			}

			logrus.Infof("migration %s applied successfully.", migrationNumber)

		} else {
			logrus.Warnf("unrecognized file suffix for migration file: %s, skipping...", migration.Name())
		}
	}

	return nil
}

func rollback(ctx context.Context, db arangodb.Database, appliedOperations []Operation) error {
	for _, operation := range appliedOperations {
		var err error
		switch operation.Type {
		case "createCollection":
			err = deleteCollection(ctx, db, operation.Name)
		case "createPersistentIndex":
			err = deleteIndex(ctx, db, operation.Name, operation.Options)
		case "createGeoIndex":
			err = deleteIndex(ctx, db, operation.Name, operation.Options)
		case "createGraph":
			err = deleteGraph(ctx, db, operation.Name)
		case "addEdgeDefinition":
			err = deleteEdgeDefinition(ctx, db, operation.Name, operation.Options)
		case "addDocument":
			err = deleteDocument(ctx, db, operation.Name, operation.Options)
		case "updateDocument":
			return fmt.Errorf("cannot rollback document update")
		case "deleteCollection":
			return fmt.Errorf("cannot rollback collection deletion")
		case "deleteIndex":
			return fmt.Errorf("cannot rollback persistent index deletion")
		case "deleteEdgeDefinition":
			return fmt.Errorf("cannot rollback edge definition deletion")
		}

		if err != nil {
			return err
		}
	}
	return nil
}

func createCollection(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	if collType, exists := options["type"]; !exists {
		return fmt.Errorf("collection type not specified")
	} else {
		switch collType {
		case "document":
			_, err := db.CreateCollection(ctx, name, &arangodb.CreateCollectionProperties{
				Type: arangodb.CollectionTypeDocument,
			})
			if err != nil {
				return fmt.Errorf("failed to create document collection: %v", err)
			}
		case "edge":
			_, err := db.CreateCollection(ctx, name, &arangodb.CreateCollectionProperties{
				Type: arangodb.CollectionTypeEdge,
			})
			if err != nil {
				return fmt.Errorf("failed to create edge collection: %v", err)
			}
		default:
			return fmt.Errorf("unrecognized collection type: %s", collType)
		}
	}

	return nil
}

func deleteCollection(ctx context.Context, db arangodb.Database, name string) error {
	coll, err := db.GetCollection(ctx, name, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for delete: %v", name, err)
	}

	return coll.Remove(ctx)
}

//	{
//		"type": "createPersistentIndex",
//		"name": "idx_unique_usernames",
//		"options": {
//		  "collection": "users",
//		  "fields": ["normalizedUserName"],
//		  "unique": true,
//		  "sparse": true
//		}
//	}
func createPersistentIndex(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	collName, ok := options["collection"].(string)
	if !ok {
		return fmt.Errorf("collection name missing or not a string")
	}

	coll, err := db.GetCollection(ctx, collName, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for index creation: %v", collName, err)
	}

	fields, ok := getSlice[string](options, "fields")
	if !ok {
		return fmt.Errorf("fields option missing or not a string array")
	}

	indexOptions := arangodb.CreatePersistentIndexOptions{
		Name: name,
	}

	if boolOpt, exists := options["unique"]; exists {
		if unique, ok := boolOpt.(bool); ok {
			indexOptions.Unique = &unique
		} else {
			return fmt.Errorf("unique option not a boolean")
		}
	}

	if boolOpt, exists := options["sparse"]; exists {
		if sparse, ok := boolOpt.(bool); ok {
			indexOptions.Sparse = &sparse
		} else {
			return fmt.Errorf("sparse option not a boolean")
		}
	}

	_, _, err = coll.EnsurePersistentIndex(ctx, fields, &indexOptions)
	if err != nil {
		return fmt.Errorf("failed to create persistent index: %v", err)
	}

	return nil
}

//	{
//		"type": "createGeoIndex",
//		"name": "idx_event_location",
//		"options": {
//		  "collection": "events",
//		  "fields": ["location"]
//		}
//	}
func createGeoIndex(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	collName, ok := options["collection"].(string)
	if !ok {
		return fmt.Errorf("collection name missing or not a string")
	}

	coll, err := db.GetCollection(ctx, collName, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for index creation: %v", collName, err)
	}

	fields, ok := getSlice[string](options, "fields")
	if !ok {
		return fmt.Errorf("fields option missing or not a string array")
	}

	indexOptions := arangodb.CreateGeoIndexOptions{
		Name: name,
	}

	if boolOpt, exists := options["geoJson"]; exists {
		if geoJson, ok := boolOpt.(bool); ok {
			indexOptions.GeoJSON = &geoJson
		} else {
			return fmt.Errorf("geoJson option not a boolean")
		}
	}

	_, _, err = coll.EnsureGeoIndex(ctx, fields, &indexOptions)
	if err != nil {
		return fmt.Errorf("failed to create persistent index: %v", err)
	}

	return nil
}

func deleteIndex(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	collName, ok := options["collection"].(string)
	if !ok {
		return fmt.Errorf("collection name missing or not a string")
	}

	coll, err := db.GetCollection(ctx, collName, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for index deletion: %v", collName, err)
	}

	err = coll.DeleteIndex(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to remove index: %v", err)
	}

	return nil
}

//	{
//		"type": "createGraph",
//		"name": "planitgraph",
//		"options": {
//			"edgeDefinitions": [
//				{
//					"collection": "user_created_event",
//					"from": ["users"],
//					"to": ["events"]
//				},
//				{
//					"collection": "user_reports_user",
//					"from": ["users"],
//					"to": ["users"]
//				}
//			],
//			"orphanCollections": []
//		}
//	}
func createGraph(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	edges, ok := options["edgeDefinitions"]
	if !ok {
		return fmt.Errorf("edgeDefinitions option missing or not an array")
	}

	bytes, err := json.Marshal(edges)
	if err != nil {
		return fmt.Errorf("failed to marshal edge definition options: %v", err)
	}

	var edgeDefinitions []arangodb.EdgeDefinition
	err = json.Unmarshal(bytes, &edgeDefinitions)
	if err != nil {
		return fmt.Errorf("failed to unmarshal edge definition options: %v", err)
	}

	orphanedCollections, ok := getSlice[string](options, "orphanedCollections")
	if !ok {
		// Try the old field name for backward compatibility
		orphanedCollections, ok = getSlice[string](options, "orphanCollections")
		if !ok {
			return fmt.Errorf("orphanedCollections option missing or not a string array")
		}
	}

	_, err = db.CreateGraph(ctx, name, &arangodb.GraphDefinition{
		EdgeDefinitions:   edgeDefinitions,
		OrphanCollections: orphanedCollections,
	}, &arangodb.CreateGraphOptions{})
	if err != nil {
		return fmt.Errorf("failed to create graph '%s': %v", name, err)
	}

	return nil
}

func deleteGraph(ctx context.Context, db arangodb.Database, name string) error {
	graph, err := db.Graph(ctx, name, &arangodb.GetGraphOptions{})
	if err != nil {
		return fmt.Errorf("failed to get graph '%s' for deletion: %v", name, err)
	}

	err = graph.Remove(ctx, &arangodb.RemoveGraphOptions{DropCollections: false})
	if err != nil {
		return fmt.Errorf("failed to remove graph: %v", err)
	}

	return nil
}

//	{
//		"type": "addEdgeDefinition",
//		"name": "social",
//		"options": {
//			"collection": "user_reports_event",
//			"from": ["users"],
//			"to": ["events"]
//		}
//	}
func addEdgeDefinition(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	graph, err := db.Graph(ctx, name, &arangodb.GetGraphOptions{})
	if err != nil {
		return fmt.Errorf("failed to get graph '%s': %v", name, err)
	}

	bytes, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("failed to marshal edge definition options: %v", err)
	}

	var edgeDefinition arangodb.EdgeDefinition
	err = json.Unmarshal(bytes, &edgeDefinition)
	if err != nil {
		return fmt.Errorf("failed to unmarshal edge definition options: %v", err)
	}

	graph.CreateEdgeDefinition(ctx, edgeDefinition.Collection, edgeDefinition.From, edgeDefinition.To, &arangodb.CreateEdgeDefinitionOptions{})
	if err != nil {
		return fmt.Errorf("failed to add edge definition: %v", err)
	}

	return nil
}

func deleteEdgeDefinition(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	graph, err := db.Graph(ctx, name, &arangodb.GetGraphOptions{})
	if err != nil {
		return fmt.Errorf("failed to get graph '%s': %v", name, err)
	}

	collection, ok := options["collection"].(string)
	if !ok {
		return fmt.Errorf("collection option missing or not a string")
	}

	_, err = graph.DeleteEdgeDefinition(ctx, collection, &arangodb.DeleteEdgeDefinitionOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove edge definition: %v", err)
	}

	return nil
}

func addDocument(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	coll, err := db.GetCollection(ctx, name, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for document addition: %v", name, err)
	}

	document, ok := options["document"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("document field missing or not an object")
	}

	for k, v := range document {
		if v == "NOW()" {
			document[k] = time.Now().UTC().Format(time.RFC3339)
		}

		if val, ok := v.(string); ok {
			if strings.HasPrefix(val, "SHA256(") && strings.HasSuffix(val, ")") {
				field := strings.TrimPrefix(val, "SHA256(")
				field = strings.TrimSuffix(field, ")")
				field = strings.TrimSpace(field)

				if field, ok := document[field]; ok {
					hash := sha256.Sum256([]byte(field.(string)))
					document[k] = hex.EncodeToString(hash[:])
				} else {
					return fmt.Errorf("no string field '%s' found in document for computing hash", field)
				}
			}
		}
	}

	_, err = coll.CreateDocument(ctx, document)
	if err != nil {
		return fmt.Errorf("failed to add document: %v", err)
	}

	return nil
}

func updateDocument(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	coll, err := db.GetCollection(ctx, name, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for document addition: %v", name, err)
	}

	key, ok := options["_key"].(string)
	if !ok {
		return fmt.Errorf("document key missing or not a string")
	}

	for k, v := range options {
		if v == "NOW()" {
			options[k] = time.Now().UTC().Format(time.RFC3339)
		}
	}

	_, err = coll.UpdateDocument(ctx, key, options)
	if err != nil {
		return fmt.Errorf("failed to update document: %v", err)
	}

	return nil
}

func deleteDocument(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) error {
	coll, err := db.GetCollection(ctx, name, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for document deletion: %v", name, err)
	}

	key, ok := options["_key"].(string)
	if !ok {
		return fmt.Errorf("document key missing or not a string")
	}

	_, err = coll.DeleteDocument(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to remove document: %v", err)
	}

	return nil
}

func getFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	hashInBytes := hash.Sum(nil)
	hashString := hex.EncodeToString(hashInBytes)

	return hashString, nil
}

func getSlice[T any](m map[string]interface{}, key string) ([]T, bool) {
	raw, exists := m[key]
	if !exists {
		return nil, false
	}

	// Try direct type assertion first (rare but possible)
	if typed, ok := raw.([]T); ok {
		return typed, true
	}

	// Handle []interface{} case from JSON
	ifaceSlice, ok := raw.([]interface{})
	if !ok {
		return nil, false
	}

	result := make([]T, len(ifaceSlice))
	for i, v := range ifaceSlice {
		typed, ok := v.(T)
		if !ok {
			return nil, false
		}
		result[i] = typed
	}

	return result, true
}
