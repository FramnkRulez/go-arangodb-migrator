package main

import (
	"context"
	"log"

	"github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator"
	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/arangodb/go-driver/v2/connection"
)

func main() {
	ctx := context.Background()

	// Connect to ArangoDB
	conn := connection.NewHttpConnection(connection.HttpConfiguration{
		Endpoint: connection.NewRoundRobinEndpoints([]string{"http://localhost:8529"}),
	})

	// Set up authentication
	auth := connection.NewBasicAuth("root", "password")
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
	dbName := "library_system"
	exists, err := client.DatabaseExists(ctx, dbName)
	if err != nil {
		log.Fatalf("Failed to check if database exists: %v", err)
	}

	var db arangodb.Database
	if !exists {
		db, err = client.CreateDatabase(ctx, dbName, nil)
		if err != nil {
			log.Fatalf("Failed to create database: %v", err)
		}
		log.Printf("Created database: %s", dbName)
	} else {
		db, err = client.GetDatabase(ctx, dbName, nil)
		if err != nil {
			log.Fatalf("Failed to get database: %v", err)
		}
		log.Printf("Using existing database: %s", dbName)
	}

	// Run migrations
	log.Println("Starting migrations...")
	err = migrator.MigrateArangoDatabase(ctx, db, migrator.MigrationOptions{
		MigrationFolder:     "./migrations",
		MigrationCollection: "migrations",
	})
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migrations completed successfully!")
}
