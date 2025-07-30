package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FramnkRulez/go-arangodb-migrator/pkg/migrator"
	"github.com/arangodb/go-driver/v2/arangodb"
	"github.com/arangodb/go-driver/v2/connection"
	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
)

type Options struct {
	// Connection options
	ArangoAddress  string `long:"arango-address" description:"Address for ArangoDB (default: http://localhost:8529)" env:"ARANGO_ADDRESS" default:"http://localhost:8529"`
	ArangoUser     string `long:"arango-user" description:"ArangoDB User (default: root)" env:"ARANGO_USER" default:"root"`
	ArangoPassword string `long:"arango-password" description:"ArangoDB password" env:"ARANGO_PASSWORD"`

	// Database options
	Database string `long:"database" description:"Database name" env:"DATABASE"`

	// Migration options
	MigrationFolder     string `long:"migration-folder" description:"Folder containing migration files (default: ./migrations)" env:"MIGRATION_FOLDER" default:"./migrations"`
	MigrationCollection string `long:"migration-collection" description:"Collection name for tracking migrations (default: migrations)" env:"MIGRATION_COLLECTION" default:"migrations"`

	// Behavior options
	DryRun bool `long:"dry-run" description:"Show what would be migrated without actually running migrations" env:"DRY_RUN"`
	Force  bool `long:"force" description:"Force migration even if files have been modified" env:"FORCE"`

	// Output options
	Verbose bool `long:"verbose" short:"v" description:"Enable verbose logging" env:"VERBOSE"`
	Quiet   bool `long:"quiet" short:"q" description:"Suppress all output except errors" env:"QUIET"`

	// Version
	Version bool `long:"version" description:"Show version information"`
}

func parseArguments() Options {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = "arangodb-migrator"
	parser.Usage = "[OPTIONS]"

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			logrus.Fatal(err)
		}
	}

	// Echo parameters in verbose mode
	if opts.Verbose {
		logrus.Info("=== Configuration Parameters ===")
		logrus.Infof("Database: %s", opts.Database)
		logrus.Infof("ArangoDB Address: %s", opts.ArangoAddress)
		logrus.Infof("ArangoDB User: %s", opts.ArangoUser)
		if opts.ArangoPassword != "" {
			logrus.Infof("ArangoDB Password: [MASKED] (length: %d)", len(opts.ArangoPassword))
		} else {
			logrus.Info("ArangoDB Password: [NOT SET]")
		}
		logrus.Infof("Migration Folder: %s", opts.MigrationFolder)
		logrus.Infof("Migration Collection: %s", opts.MigrationCollection)
		logrus.Infof("Dry Run: %t", opts.DryRun)
		logrus.Infof("Force: %t", opts.Force)
		logrus.Infof("Verbose: %t", opts.Verbose)
		logrus.Infof("Quiet: %t", opts.Quiet)
		logrus.Info("=== End Configuration ===")
	}

	return opts
}

func setupLogging(opts Options) {
	if opts.Quiet {
		logrus.SetLevel(logrus.ErrorLevel)
	} else if opts.Verbose {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}
}

func main() {
	// Enable verbose logging early if VERBOSE env var is set
	if os.Getenv("VERBOSE") == "true" {
		logrus.SetLevel(logrus.DebugLevel)
	}

	opts := parseArguments()

	// Handle version flag
	if opts.Version {
		fmt.Println("ArangoDB Migrator v0.1.0")
		os.Exit(0)
	}

	setupLogging(opts)

	// Debug environment variables if verbose
	if opts.Verbose {
		logrus.Info("=== Environment Variables ===")
		logrus.Infof("DATABASE: %s", os.Getenv("DATABASE"))
		logrus.Infof("ARANGO_ADDRESS: %s", os.Getenv("ARANGO_ADDRESS"))
		logrus.Infof("ARANGO_USER: %s", os.Getenv("ARANGO_USER"))
		if envPass := os.Getenv("ARANGO_PASSWORD"); envPass != "" {
			logrus.Infof("ARANGO_PASSWORD: [MASKED] (length: %d)", len(envPass))
		} else {
			logrus.Info("ARANGO_PASSWORD: [NOT SET]")
		}
		logrus.Infof("MIGRATION_FOLDER: %s", os.Getenv("MIGRATION_FOLDER"))
		logrus.Infof("MIGRATION_COLLECTION: %s", os.Getenv("MIGRATION_COLLECTION"))
		logrus.Infof("DRY_RUN: %s", os.Getenv("DRY_RUN"))
		logrus.Infof("FORCE: %s", os.Getenv("FORCE"))
		logrus.Infof("VERBOSE: %s", os.Getenv("VERBOSE"))
		logrus.Infof("QUIET: %s", os.Getenv("QUIET"))
		logrus.Info("=== End Environment Variables ===")
	}

	// Validate required fields (unless showing version)
	if !opts.Version {
		if opts.Database == "" {
			logrus.Fatal("Database name is required (--database)")
		}
		if opts.ArangoPassword == "" {
			logrus.Fatal("ArangoDB password is required (--arango-password)")
		}

		// Validate migration folder exists
		if _, err := os.Stat(opts.MigrationFolder); os.IsNotExist(err) {
			logrus.Fatalf("Migration folder does not exist: %s", opts.MigrationFolder)
		}
	}

	logrus.Info("Starting ArangoDB migration...")

	ctx := context.Background()
	arangoClient, err := ConnectArango(ctx, opts.ArangoAddress, opts.ArangoUser, opts.ArangoPassword, true)
	if err != nil {
		logrus.Fatalf("Failed to connect to ArangoDB: %v", err)
	}

	_, err = MigrateDatabase(ctx, arangoClient, opts)
	if err != nil {
		logrus.Fatalf("Failed to migrate database: %v", err)
	}

	if opts.DryRun {
		logrus.Info("Dry run completed - no changes were made")
	} else {
		logrus.Info("Migration completed successfully!")
	}
}

func ConnectArango(ctx context.Context, arangoAddress string, user string, password string, verify bool) (arangodb.Client, error) {
	endpoint := connection.NewRoundRobinEndpoints([]string{arangoAddress})

	conn := connection.NewHttpConnection(connection.HttpConfiguration{
		Endpoint: endpoint,
	})

	// Add authentication
	auth := connection.NewBasicAuth(user, password)
	err := conn.SetAuthentication(auth)
	if err != nil {
		return nil, fmt.Errorf("failed to set ArangoDB authentication: %v", err)
	}

	client := arangodb.NewClient(conn)

	if verify {
		ver, err := client.Version(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to verify ArangoDB version: %v", err)
		}

		logrus.Infof("Connected to ArangoDB version: %s", ver.Version)
	}

	return client, nil
}

func MigrateDatabase(ctx context.Context, client arangodb.Client, opts Options) (arangodb.Database, error) {
	// Check if the database exists
	exists, err := client.DatabaseExists(ctx, opts.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to check if database exists: %v", err)
	}

	var db arangodb.Database

	// If the database does not exist, create it
	if !exists {
		if opts.DryRun {
			logrus.Infof("[DRY RUN] Would create database: %s", opts.Database)
			return nil, nil
		}

		db, err = client.CreateDatabase(ctx, opts.Database, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create database: %v", err)
		}
		logrus.Infof("Database %s created successfully", opts.Database)
	} else {
		// If the database exists, get it
		db, err = client.GetDatabase(ctx, opts.Database, &arangodb.GetDatabaseOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get database: %v", err)
		}
		logrus.Infof("Using existing database: %s", opts.Database)
	}

	// Validate migration folder path
	migrationFolder, err := filepath.Abs(opts.MigrationFolder)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve migration folder path: %v", err)
	}

	logrus.Infof("Using migration folder: %s", migrationFolder)
	logrus.Infof("Using migration collection: %s", opts.MigrationCollection)

	if opts.DryRun {
		logrus.Info("[DRY RUN] Would run migrations with the following options:")
		logrus.Infof("  - Migration folder: %s", migrationFolder)
		logrus.Infof("  - Migration collection: %s", opts.MigrationCollection)
		logrus.Infof("  - Force mode: %t", opts.Force)
		return nil, nil
	}

	migrationOpts := migrator.MigrationOptions{
		MigrationCollection: opts.MigrationCollection,
		MigrationFolder:     migrationFolder,
		Force:               opts.Force,
	}

	err = migrator.MigrateArangoDatabase(ctx, db, migrationOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate database: %v", err)
	}

	return db, nil
}
