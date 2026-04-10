package migrate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"
)

const (
	schemaV5          = 5
	schemaV6          = 6
	schemaV7          = 7
	schemaV8          = 8
	schemaV9          = 9
	schemaV10         = 10
	schemaVersionSize = 4
)

// Migration represents a schema migration step
type Migration struct {
	FromVersion uint32
	ToVersion   uint32
	Description string
	Migrate     func(tx *bbolt.Tx, logger *slog.Logger) error
}

var registeredMigrations = []Migration{
	{
		FromVersion: schemaV5,
		ToVersion:   schemaV6,
		Description: "Migration to XXH128 hashing (Clean Break)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to hash algorithm change (BLAKE3 -> XXH3)")
			for _, bucketName := range core.AllBuckets() {
				// Don't delete the meta bucket itself, just its contents
				if bucketName == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(bucketName)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: schemaV7,
		ToVersion:   schemaV10,
		Description: "Migration to align cache schema with search schema (v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v7 -> v10)")
			// Purge all cache buckets except meta bucket
			for _, bucketName := range core.AllBuckets() {
				if bucketName == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(bucketName)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: schemaV6,
		ToVersion:   schemaV10,
		Description: "Migration to align cache schema with search schema (v6 -> v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v6 -> v10)")
			for _, bucketName := range core.AllBuckets() {
				if bucketName == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(bucketName)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: schemaV8,
		ToVersion:   schemaV10,
		Description: "Migration to align cache schema with search schema (v8 -> v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v8 -> v10)")
			for _, bucketName := range core.AllBuckets() {
				if bucketName == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(bucketName)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: schemaV9,
		ToVersion:   schemaV10,
		Description: "Migration to align cache schema with search schema (v9 -> v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v9 -> v10)")
			for _, bucketName := range core.AllBuckets() {
				if bucketName == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(bucketName)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
					return err
				}
			}
			return nil
		},
	},
}

// RunMigrations runs all pending migrations for the current schema
func RunMigrations(db *bbolt.DB, currentVersion uint32, logger *slog.Logger) (uint32, error) {
	if logger == nil {
		logger = slog.Default()
	}

	for _, migration := range registeredMigrations {
		if currentVersion == migration.FromVersion {
			logger.Info("Running cache migration", "from", migration.FromVersion, "to", migration.ToVersion, "desc", migration.Description)
			if err := db.Update(func(tx *bbolt.Tx) error {
				return migration.Migrate(tx, logger)
			}); err != nil {
				return currentVersion, fmt.Errorf("migration %d->%d failed: %w", migration.FromVersion, migration.ToVersion, err)
			}
			currentVersion = migration.ToVersion

			// Update the version in the database
			if err := db.Update(func(tx *bbolt.Tx) error {
				meta := tx.Bucket([]byte(core.BucketMeta))
				if meta == nil {
					return errors.New("metadata bucket missing")
				}
				versionData := make([]byte, schemaVersionSize)
				binary.BigEndian.PutUint32(versionData, currentVersion)
				return meta.Put([]byte(core.KeySchemaVersion), versionData)
			}); err != nil {
				return currentVersion, fmt.Errorf("failed to update schema version to %d: %w", currentVersion, err)
			}
		}
	}
	return currentVersion, nil
}
