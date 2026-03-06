package cache

import (
	"encoding/binary"
	"log/slog"
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestRunMigrations_NoOp(t *testing.T) {
	dbPath := "test_migrations_noop.db"
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer func() { _ = db.Close() }()
	defer func() { _ = os.Remove(dbPath) }()

	// Ensure BucketMeta exists
	_ = db.Update(func(tx *bolt.Tx) error {
		_, _ = tx.CreateBucketIfNotExists([]byte(BucketMeta))
		return nil
	})

	// Current schema version is exactly SchemaVersion
	newVer, err := RunMigrations(db, uint32(SchemaVersion), slog.Default())
	if err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	if newVer != uint32(SchemaVersion) {
		t.Errorf("Expected version to remain %d, got %d", SchemaVersion, newVer)
	}
}

func TestRunMigrations_V1ToV2(t *testing.T) {
	dbPath := "test_migrations_v1v2.db"
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer func() { _ = db.Close() }()
	defer func() { _ = os.Remove(dbPath) }()

	_ = db.Update(func(tx *bolt.Tx) error {
		meta, _ := tx.CreateBucketIfNotExists([]byte(BucketMeta))
		v := make([]byte, 4)
		binary.BigEndian.PutUint32(v, 1) // Start at version 1
		return meta.Put([]byte(KeySchemaVersion), v)
	})

	// Temporarily override registeredMigrations for testing
	originalMigrations := registeredMigrations
	defer func() { registeredMigrations = originalMigrations }()

	migrationRun := false
	registeredMigrations = []Migration{
		{
			FromVersion: 1,
			ToVersion:   2,
			Description: "Test migration V1 to V2",
			Migrate: func(tx *bolt.Tx, logger *slog.Logger) error {
				migrationRun = true
				return nil
			},
		},
	}

	newVer, err := RunMigrations(db, 1, slog.Default())
	if err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	if newVer != 2 {
		t.Errorf("Expected version to update to 2, got %d", newVer)
	}

	if !migrationRun {
		t.Error("Migration function was not executed")
	}

	// Verify schema version in DB was stored correctly
	_ = db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket([]byte(BucketMeta)).Get([]byte(KeySchemaVersion))
		storedVer := binary.BigEndian.Uint32(v)
		if storedVer != 2 {
			t.Errorf("Expected stored version in DB to be 2, got %d", storedVer)
		}
		return nil
	})
}
