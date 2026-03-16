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

// TestCacheSchema_MigrationPath verifies that multi-version migrations
// run in the correct sequence.
func TestCacheSchema_MigrationPath(t *testing.T) {
	dbPath := "test_migrations_path.db"
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer func() { _ = db.Close() }()
	defer func() { _ = os.Remove(dbPath) }()

	_ = db.Update(func(tx *bolt.Tx) error {
		meta, _ := tx.CreateBucketIfNotExists([]byte(BucketMeta))
		v := make([]byte, 4)
		binary.BigEndian.PutUint32(v, 1)
		return meta.Put([]byte(KeySchemaVersion), v)
	})

	originalMigrations := registeredMigrations
	defer func() { registeredMigrations = originalMigrations }()

	migrationOrder := []uint32{}
	registeredMigrations = []Migration{
		{
			FromVersion: 1,
			ToVersion:   2,
			Description: "Migration 1->2",
			Migrate: func(tx *bolt.Tx, logger *slog.Logger) error {
				migrationOrder = append(migrationOrder, 2)
				return nil
			},
		},
		{
			FromVersion: 2,
			ToVersion:   3,
			Description: "Migration 2->3",
			Migrate: func(tx *bolt.Tx, logger *slog.Logger) error {
				migrationOrder = append(migrationOrder, 3)
				return nil
			},
		},
		{
			FromVersion: 3,
			ToVersion:   4,
			Description: "Migration 3->4",
			Migrate: func(tx *bolt.Tx, logger *slog.Logger) error {
				migrationOrder = append(migrationOrder, 4)
				return nil
			},
		},
	}

	newVer, err := RunMigrations(db, 1, slog.Default())
	if err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	if newVer != 4 {
		t.Errorf("Expected version 4, got %d", newVer)
	}

	expectedOrder := []uint32{2, 3, 4}
	if len(migrationOrder) != len(expectedOrder) {
		t.Fatalf("Expected %d migrations, got %d", len(expectedOrder), len(migrationOrder))
	}

	for i, expected := range expectedOrder {
		if migrationOrder[i] != expected {
			t.Errorf("Migration order mismatch at index %d: expected %d, got %d", i, expected, migrationOrder[i])
		}
	}
}

// TestCacheSchema_VersionMismatchHandling verifies that opening a cache
// with a newer schema version than the code expects produces an error.
func TestCacheSchema_VersionMismatchHandling(t *testing.T) {
	dbPath := "test_migrations_mismatch.db"
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer func() { _ = db.Close() }()
	defer func() { _ = os.Remove(dbPath) }()

	// Set schema version to a future version
	_ = db.Update(func(tx *bolt.Tx) error {
		meta, _ := tx.CreateBucketIfNotExists([]byte(BucketMeta))
		v := make([]byte, 4)
		binary.BigEndian.PutUint32(v, uint32(SchemaVersion)+100)
		return meta.Put([]byte(KeySchemaVersion), v)
	})

	// Try to open with current schema version - should fail
	_, err = OpenWithTimeout(dbPath, false, 0)
	if err == nil {
		t.Error("Expected error when opening cache with newer schema version")
	}
}

// TestCacheSchema_BackwardCompatibility verifies that caches without
// explicit schema version are treated as v1 and migrated correctly.
func TestCacheSchema_BackwardCompatibility(t *testing.T) {
	dbPath := "test_migrations_compat.db"
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer func() { _ = db.Close() }()
	defer func() { _ = os.Remove(dbPath) }()

	// Create bucket without schema version (simulates old cache)
	_ = db.Update(func(tx *bolt.Tx) error {
		_, _ = tx.CreateBucketIfNotExists([]byte(BucketMeta))
		// Don't set schema version
		return nil
	})

	originalMigrations := registeredMigrations
	defer func() { registeredMigrations = originalMigrations }()

	migrationRan := false
	registeredMigrations = []Migration{
		{
			FromVersion: 0,
			ToVersion:   1,
			Description: "Initialize schema",
			Migrate: func(tx *bolt.Tx, logger *slog.Logger) error {
				migrationRan = true
				return nil
			},
		},
	}

	// Should migrate from v0 to current
	newVer, err := RunMigrations(db, 0, slog.Default())
	if err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	if !migrationRan {
		t.Error("Expected migration from v0 to run")
	}

	if newVer < 1 {
		t.Errorf("Expected version >= 1 after migration, got %d", newVer)
	}
}
