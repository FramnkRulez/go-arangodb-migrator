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

	// AutoRollback enables automatic rollback of all migrations in the current batch
	// if any migration fails. This ensures database consistency by rolling back
	// to the state before the migration batch started.
	AutoRollback bool
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

// OperationResult tracks the result of a single operation for potential rollback.
type OperationResult struct {
	// Type is the operation type (e.g., "createCollection", "addDocument").
	Type string `json:"type"`

	// Name is the name of the resource being operated on.
	Name string `json:"name"`

	// Options contains the original operation options.
	Options map[string]interface{} `json:"options"`

	// Result contains operation-specific result data for rollback.
	// For documents: contains the created document ID
	// For collections: contains collection properties
	// For indexes: contains index details
	Result map[string]interface{} `json:"result,omitempty"`

	// RollbackData contains additional data needed for rollback.
	// For documents: contains the original document content
	// For updates: contains the original document state
	RollbackData map[string]interface{} `json:"rollbackData,omitempty"`
}

// AppliedMigration tracks a migration that has been successfully applied.
type AppliedMigration struct {
	// MigrationNumber is the migration file name without extension (e.g., "000001").
	MigrationNumber string `json:"_key"`

	// AppliedAt is the timestamp when the migration was applied.
	AppliedAt time.Time `json:"appliedAt"`

	// Sha256 is the hash of the migration file for integrity verification.
	Sha256 string `json:"sha256"`

	// OperationResults tracks the results of each operation for potential rollback.
	OperationResults []OperationResult `json:"operationResults,omitempty"`
}

// MigrateArangoDatabase applies all pending migrations to the specified database.
// Migrations are applied in order based on their numeric filename prefix.
// If any migration fails, the behavior depends on the AutoRollback option:
//   - With AutoRollback=true: All migrations in the current batch are rolled back
//   - With AutoRollback=false: Only operations from the failed migration are rolled back
//
// The function performs the following steps:
//  1. Creates the migration collection if it doesn't exist
//  2. Reads all .json files from the migration folder
//  3. Sorts migrations by numeric filename prefix
//  4. Applies migrations that haven't been applied yet
//  5. Verifies file integrity using SHA256 hashes (unless Force is true)
//  6. Tracks operation results for potential rollback
//  7. Rolls back on failure based on AutoRollback setting
//
// # Parameters
//
//   - ctx: Context for cancellation and timeouts
//   - db: ArangoDB database instance
//   - options: Migration configuration options
//
// # Returns
//
// Returns an error if any migration fails. On failure, operations are rolled back
// according to the AutoRollback setting.
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
// With auto-rollback enabled:
//
//	err := migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
//		MigrationFolder:     "./migrations",
//		MigrationCollection: "migrations",
//		AutoRollback:        true, // Rollback entire batch on failure
//	})
//
// With force option:
//
//	err := migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
//		MigrationFolder:     "./migrations",
//		MigrationCollection: "migrations",
//		Force:               true, // Bypass file modification checks
//	})
//
// PendingMigration represents a migration that needs to be applied.
type PendingMigration struct {
	MigrationNumber string
	Migration       *Migration
	Hash            string
	FilePath        string
}

func collectPendingMigrations(ctx context.Context, db arangodb.Database, options MigrationOptions) ([]PendingMigration, arangodb.Collection, error) {
	var migrationColl arangodb.Collection
	migrationColl, err := db.GetCollection(ctx, options.MigrationCollection, &arangodb.GetCollectionOptions{
		SkipExistCheck: false,
	})
	if err != nil {
		if shared.IsNotFound(err) {
			migrationColl, err = db.CreateCollection(ctx, options.MigrationCollection, &arangodb.CreateCollectionProperties{
				Type: arangodb.CollectionTypeDocument,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create migration collection in specified db: %v", err)
			}
		} else {
			return nil, nil, err
		}
	}

	// Get all migrations from the migration folder
	migrations, err := os.ReadDir(options.MigrationFolder)
	if err != nil {
		return nil, nil, err
	}

	var pendingMigrations []PendingMigration

	for _, migration := range migrations {
		if strings.HasSuffix(migration.Name(), ".json") {
			migrationNumber := strings.TrimSuffix(migration.Name(), ".json")
			fullpath := filepath.Join(options.MigrationFolder, migration.Name())

			hash, err := getFileSHA256(fullpath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to compute hash for migration file: %v", err)
			}

			var appliedMigration *AppliedMigration
			_, err = migrationColl.ReadDocument(ctx, migrationNumber, &appliedMigration)
			if err != nil {
				if !shared.IsNotFound(err) {
					return nil, nil, fmt.Errorf("failed to read applied migration: %v", err)
				}
			}

			if appliedMigration != nil {
				if appliedMigration.Sha256 != hash {
					if options.Force {
						logrus.Warnf("migration file %s has been modified since last applied, but continuing due to force flag", migrationNumber)
					} else {
						return nil, nil, fmt.Errorf("migration file has been modified since last applied: %s (use --force to override)", migrationNumber)
					}
				}
			}

			exists, err := migrationColl.DocumentExists(ctx, migrationNumber)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to check if migration exists: %v", err)
			}

			if exists {
				logrus.Infof("migration %s already applied, skipping...", migrationNumber)
				continue
			}

			// Read the migration file
			migrationFile, err := os.ReadFile(filepath.Join(options.MigrationFolder, migration.Name()))
			if err != nil {
				return nil, nil, err
			}

			// Parse the migration file
			migrationData := &Migration{}
			err = json.Unmarshal(migrationFile, migrationData)
			if err != nil {
				return nil, nil, err
			}

			// Validate migration structure
			if len(migrationData.Up) == 0 {
				return nil, nil, fmt.Errorf("migration file %s does not include a valid 'up' list of migrations to apply", migrationNumber)
			}

			if len(migrationData.Down) > 0 {
				return nil, nil, fmt.Errorf("migration file %s has a 'down' list of migrations, but down migrations are not yet supported", migrationNumber)
			}

			pendingMigrations = append(pendingMigrations, PendingMigration{
				MigrationNumber: migrationNumber,
				Migration:       migrationData,
				Hash:            hash,
				FilePath:        fullpath,
			})

		} else {
			logrus.Warnf("unrecognized file suffix for migration file: %s, skipping...", migration.Name())
		}
	}

	return pendingMigrations, migrationColl, nil
}

func MigrateArangoDatabase(ctx context.Context, db arangodb.Database, options MigrationOptions) error {
	// Collect all pending migrations
	pendingMigrations, migrationColl, err := collectPendingMigrations(ctx, db, options)
	if err != nil {
		return err
	}

	if len(pendingMigrations) == 0 {
		logrus.Info("no pending migrations to apply")
		return nil
	}

	// If auto-rollback is enabled, we need to track all applied migrations
	// so we can rollback the entire batch if any migration fails
	var appliedMigrations []AppliedMigration
	var appliedOperations []OperationResult

	// Apply each migration
	for _, pendingMigration := range pendingMigrations {
		migrationNumber := pendingMigration.MigrationNumber
		migration := pendingMigration.Migration

		logrus.Infof("applying migration %s...", migrationNumber)

		// Track operations for this migration
		var migrationOperations []OperationResult

		// Apply each operation in the migration
		for _, operation := range migration.Up {
			var operationResult OperationResult
			var err error

			switch operation.Type {
			case "createCollection":
				operationResult, err = createCollectionWithTracking(ctx, db, operation.Name, operation.Options)
			case "createPersistentIndex":
				operationResult, err = createPersistentIndexWithTracking(ctx, db, operation.Name, operation.Options)
			case "createGeoIndex":
				operationResult, err = createGeoIndexWithTracking(ctx, db, operation.Name, operation.Options)
			case "createGraph":
				operationResult, err = createGraphWithTracking(ctx, db, operation.Name, operation.Options)
			case "addEdgeDefinition":
				operationResult, err = addEdgeDefinitionWithTracking(ctx, db, operation.Name, operation.Options)
			case "deleteIndex":
				operationResult, err = deleteIndexWithTracking(ctx, db, operation.Name, operation.Options)
			case "deleteEdgeDefinition":
				operationResult, err = deleteEdgeDefinitionWithTracking(ctx, db, operation.Name, operation.Options)
			case "deleteCollection":
				operationResult, err = deleteCollectionWithTracking(ctx, db, operation.Name)
			case "addDocument":
				operationResult, err = addDocumentWithTracking(ctx, db, operation.Name, operation.Options)
			case "updateDocument":
				operationResult, err = updateDocumentWithTracking(ctx, db, operation.Name, operation.Options)
			case "deleteDocument":
				operationResult, err = deleteDocumentWithTracking(ctx, db, operation.Name, operation.Options)
			default:
				err = fmt.Errorf("unsupported operation type: %s", operation.Type)
			}

			if err != nil {
				logrus.Errorf("migration operation failed for migration %s on %s: %v", migrationNumber, operation.Type, err)

				if options.AutoRollback {
					logrus.Error("auto-rollback enabled, rolling back all applied migrations...")
					rollbackErr := autoRollback(ctx, db, appliedOperations)
					if rollbackErr != nil {
						logrus.Errorf("failed to auto-rollback migrations: %v", rollbackErr)
						logrus.Error("database may be in an inconsistent state")
						return fmt.Errorf("failed to auto-rollback migrations: %v", rollbackErr)
					}
					return fmt.Errorf("migration operation failed for migration %s: %v", migrationNumber, err)
				} else {
					// Legacy rollback behavior - only rollback operations from current migration
					logrus.Error("rolling back applied operations from current migration...")
					// Convert Operation to OperationResult for legacy rollback
					legacyOperation := OperationResult{
						Type:    operation.Type,
						Name:    operation.Name,
						Options: operation.Options,
					}
					rollbackErr := autoRollback(ctx, db, []OperationResult{legacyOperation})
					if rollbackErr != nil {
						logrus.Errorf("failed to rollback migration: %v", rollbackErr)
						logrus.Error("database may be in an unclean state")
						return fmt.Errorf("failed to rollback migration: %v", rollbackErr)
					}
					return fmt.Errorf("migration operation failed for migration %s: %v", migrationNumber, err)
				}
			}

			// Track the operation result
			operationResult.Type = operation.Type
			operationResult.Name = operation.Name
			operationResult.Options = operation.Options
			migrationOperations = append(migrationOperations, operationResult)
			appliedOperations = append(appliedOperations, operationResult)
		}

		// Store migration for later application (only if entire batch succeeds)
		appliedMigrations = append(appliedMigrations, AppliedMigration{
			MigrationNumber:  migrationNumber,
			AppliedAt:        time.Now(),
			Sha256:           pendingMigration.Hash,
			OperationResults: migrationOperations,
		})
		logrus.Infof("migration %s applied successfully.", migrationNumber)
	}

	// Mark all migrations as applied only after the entire batch succeeds
	for _, appliedMigration := range appliedMigrations {
		_, err := migrationColl.CreateDocument(ctx, &appliedMigration)
		if err != nil {
			return fmt.Errorf("failed to mark migration as applied: %v", err)
		}
	}

	logrus.Infof("all %d migrations applied successfully", len(pendingMigrations))
	return nil
}

// autoRollback rolls back all operations in reverse order using the tracked operation results
func autoRollback(ctx context.Context, db arangodb.Database, appliedOperations []OperationResult) error {
	logrus.Info("starting auto-rollback of all applied operations...")

	// Rollback in reverse order (LIFO)
	for i := len(appliedOperations) - 1; i >= 0; i-- {
		operation := appliedOperations[i]
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
			// Use the tracked document ID for deletion
			if docID, ok := operation.Result["documentID"].(string); ok {
				err = deleteDocumentByID(ctx, db, operation.Name, docID)
			} else {
				err = deleteDocument(ctx, db, operation.Name, operation.Options)
			}
		case "updateDocument":
			// Restore the original document state
			if originalDoc, ok := operation.RollbackData["originalDocument"].(map[string]interface{}); ok {
				err = restoreDocument(ctx, db, operation.Name, originalDoc)
			} else {
				err = fmt.Errorf("cannot rollback document update - no original state available")
			}
		case "deleteCollection":
			err = fmt.Errorf("cannot rollback collection deletion")
		case "deleteIndex":
			err = fmt.Errorf("cannot rollback index deletion")
		case "deleteEdgeDefinition":
			err = fmt.Errorf("cannot rollback edge definition deletion")
		case "deleteDocument":
			// Restore the deleted document
			if originalDoc, ok := operation.RollbackData["originalDocument"].(map[string]interface{}); ok {
				err = restoreDocument(ctx, db, operation.Name, originalDoc)
			} else {
				err = fmt.Errorf("cannot rollback document deletion - no original state available")
			}
		}

		if err != nil {
			logrus.Errorf("failed to rollback operation %s: %v", operation.Type, err)
			return fmt.Errorf("failed to rollback operation %s: %v", operation.Type, err)
		}

		logrus.Infof("rolled back operation: %s (%s)", operation.Type, operation.Name)
	}

	logrus.Info("auto-rollback completed successfully")
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

// Tracking versions of operations that return OperationResult for rollback
func createCollectionWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "createCollection",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	err := createCollection(ctx, db, name, options)
	if err != nil {
		return result, err
	}

	// Store collection properties for potential rollback
	result.Result["collectionName"] = name
	result.Result["collectionType"] = options["type"]
	return result, nil
}

func createPersistentIndexWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "createPersistentIndex",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	err := createPersistentIndex(ctx, db, name, options)
	if err != nil {
		return result, err
	}

	result.Result["indexName"] = name
	result.Result["collection"] = options["collection"]
	return result, nil
}

func createGeoIndexWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "createGeoIndex",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	err := createGeoIndex(ctx, db, name, options)
	if err != nil {
		return result, err
	}

	result.Result["indexName"] = name
	result.Result["collection"] = options["collection"]
	return result, nil
}

func createGraphWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "createGraph",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	err := createGraph(ctx, db, name, options)
	if err != nil {
		return result, err
	}

	result.Result["graphName"] = name
	return result, nil
}

func addEdgeDefinitionWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "addEdgeDefinition",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	err := addEdgeDefinition(ctx, db, name, options)
	if err != nil {
		return result, err
	}

	result.Result["graphName"] = name
	result.Result["collection"] = options["collection"]
	return result, nil
}

func deleteIndexWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "deleteIndex",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	err := deleteIndex(ctx, db, name, options)
	if err != nil {
		return result, err
	}

	result.Result["indexName"] = name
	result.Result["collection"] = options["collection"]
	return result, nil
}

func deleteEdgeDefinitionWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "deleteEdgeDefinition",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	err := deleteEdgeDefinition(ctx, db, name, options)
	if err != nil {
		return result, err
	}

	result.Result["graphName"] = name
	result.Result["collection"] = options["collection"]
	return result, nil
}

func deleteCollectionWithTracking(ctx context.Context, db arangodb.Database, name string) (OperationResult, error) {
	result := OperationResult{
		Type:    "deleteCollection",
		Name:    name,
		Options: make(map[string]interface{}),
		Result:  make(map[string]interface{}),
	}

	err := deleteCollection(ctx, db, name)
	if err != nil {
		return result, err
	}

	result.Result["collectionName"] = name
	return result, nil
}

func addDocumentWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:    "addDocument",
		Name:    name,
		Options: options,
		Result:  make(map[string]interface{}),
	}

	// Store original document for potential rollback
	if document, ok := options["document"].(map[string]interface{}); ok {
		result.RollbackData = make(map[string]interface{})
		result.RollbackData["originalDocument"] = document
	}

	// Get the collection
	coll, err := db.GetCollection(ctx, name, &arangodb.GetCollectionOptions{})
	if err != nil {
		return result, fmt.Errorf("failed to get collection '%s' for document addition: %v", name, err)
	}

	document, ok := options["document"].(map[string]interface{})
	if !ok {
		return result, fmt.Errorf("document field missing or not an object")
	}

	// Process special values
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
					return result, fmt.Errorf("no string field '%s' found in document for computing hash", field)
				}
			}
		}
	}

	// Create the document and capture the result
	meta, err := coll.CreateDocument(ctx, document)
	if err != nil {
		return result, fmt.Errorf("failed to add document: %v", err)
	}

	// Store the document ID for rollback
	result.Result["documentID"] = meta.Key
	return result, nil
}

func updateDocumentWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:         "updateDocument",
		Name:         name,
		Options:      options,
		Result:       make(map[string]interface{}),
		RollbackData: make(map[string]interface{}),
	}

	// Get the collection
	coll, err := db.GetCollection(ctx, name, &arangodb.GetCollectionOptions{})
	if err != nil {
		return result, fmt.Errorf("failed to get collection '%s' for document update: %v", name, err)
	}

	key, ok := options["_key"].(string)
	if !ok {
		return result, fmt.Errorf("document key missing or not a string")
	}

	// Read the original document for rollback
	var originalDoc map[string]interface{}
	_, err = coll.ReadDocument(ctx, key, &originalDoc)
	if err != nil {
		return result, fmt.Errorf("failed to read original document for rollback: %v", err)
	}

	// Store original document for rollback
	result.RollbackData["originalDocument"] = originalDoc
	result.Result["documentKey"] = key

	// Process special values
	for k, v := range options {
		if v == "NOW()" {
			options[k] = time.Now().UTC().Format(time.RFC3339)
		}
	}

	// Update the document
	_, err = coll.UpdateDocument(ctx, key, options)
	if err != nil {
		return result, fmt.Errorf("failed to update document: %v", err)
	}

	return result, nil
}

func deleteDocumentWithTracking(ctx context.Context, db arangodb.Database, name string, options map[string]interface{}) (OperationResult, error) {
	result := OperationResult{
		Type:         "deleteDocument",
		Name:         name,
		Options:      options,
		Result:       make(map[string]interface{}),
		RollbackData: make(map[string]interface{}),
	}

	// Get the collection
	coll, err := db.GetCollection(ctx, name, &arangodb.GetCollectionOptions{})
	if err != nil {
		return result, fmt.Errorf("failed to get collection '%s' for document deletion: %v", name, err)
	}

	key, ok := options["_key"].(string)
	if !ok {
		return result, fmt.Errorf("document key missing or not a string")
	}

	// Read the original document for rollback
	var originalDoc map[string]interface{}
	_, err = coll.ReadDocument(ctx, key, &originalDoc)
	if err != nil {
		return result, fmt.Errorf("failed to read original document for rollback: %v", err)
	}

	// Store original document for rollback
	result.RollbackData["originalDocument"] = originalDoc
	result.Result["documentKey"] = key

	// Delete the document
	_, err = coll.DeleteDocument(ctx, key)
	if err != nil {
		return result, fmt.Errorf("failed to remove document: %v", err)
	}

	return result, nil
}

// Helper functions for auto-rollback
func deleteDocumentByID(ctx context.Context, db arangodb.Database, collectionName, documentID string) error {
	coll, err := db.GetCollection(ctx, collectionName, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for document deletion: %v", collectionName, err)
	}

	_, err = coll.DeleteDocument(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to remove document: %v", err)
	}

	return nil
}

func restoreDocument(ctx context.Context, db arangodb.Database, collectionName string, document map[string]interface{}) error {
	coll, err := db.GetCollection(ctx, collectionName, &arangodb.GetCollectionOptions{})
	if err != nil {
		return fmt.Errorf("failed to get collection '%s' for document restoration: %v", collectionName, err)
	}

	_, err = coll.CreateDocument(ctx, document)
	if err != nil {
		return fmt.Errorf("failed to restore document: %v", err)
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
