package main

import (
	"context"
	"log"
	"os"

	"github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator"
	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/arangodb/go-driver/v2/connection"
)

func main() {
	ctx := context.Background()

	// Get configuration from environment variables with defaults
	arangoAddress := getEnv("ARANGO_ADDRESS", "http://localhost:8529")
	arangoUser := getEnv("ARANGO_USER", "root")
	arangoPassword := getEnv("ARANGO_PASSWORD", "")
	database := getEnv("DATABASE", "")
	migrationFolder := getEnv("MIGRATION_FOLDER", "./migrations")
	migrationCollection := getEnv("MIGRATION_COLLECTION", "migrations")

	// Validate required environment variables
	if arangoPassword == "" {
		log.Fatal("ARANGO_PASSWORD environment variable is required")
	}
	if database == "" {
		log.Fatal("DATABASE environment variable is required")
	}

	// Connect to ArangoDB
	conn := connection.NewHttpConnection(connection.HttpConfiguration{
		Endpoint: connection.NewRoundRobinEndpoints([]string{arangoAddress}),
	})

	// Set up authentication
	auth := connection.NewBasicAuth(arangoUser, arangoPassword)
	err := conn.SetAuthentication(auth)
	if err != nil {
		log.Fatalf("Failed to set authentication: %v", err)
	}

	client := arangodb.NewClient(conn)

	// Verify connection
	ver, err := client.Version(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to ArangoDB: %v", err)
	}
	log.Printf("Connected to ArangoDB version: %s", ver.Version)

	// Get or create database
	exists, err := client.DatabaseExists(ctx, database)
	if err != nil {
		log.Fatalf("Failed to check if database exists: %v", err)
	}

	var db arangodb.Database
	if !exists {
		db, err = client.CreateDatabase(ctx, database, nil)
		if err != nil {
			log.Fatalf("Failed to create database: %v", err)
		}
		log.Printf("Created database: %s", database)
	} else {
		db, err = client.GetDatabase(ctx, database, nil)
		if err != nil {
			log.Fatalf("Failed to get database: %v", err)
		}
		log.Printf("Using existing database: %s", database)
	}

	// Run migrations
	log.Println("Starting migrations...")
	err = migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
		MigrationFolder:     migrationFolder,
		MigrationCollection: migrationCollection,
	})
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migrations completed successfully!")
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Example usage:
// export ARANGO_ADDRESS=http://localhost:8529
// export ARANGO_USER=root
// export ARANGO_PASSWORD=mypassword
// export DATABASE=myapp
// export MIGRATION_FOLDER=./migrations
// export MIGRATION_COLLECTION=migrations
// go run main.go
