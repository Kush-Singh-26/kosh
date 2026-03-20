package migrate

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"go.etcd.io/bbolt"
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
		FromVersion: 5,
		ToVersion:   6,
		Description: "Migration to XXH128 hashing (Clean Break)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to hash algorithm change (BLAKE3 -> XXH3)")
			for _, b := range core.AllBuckets() {
				// Don't delete the meta bucket itself, just its contents
				if b == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(b)); err != nil && !errors.Is(err, bbolt.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: 7,
		ToVersion:   10,
		Description: "Migration to align cache schema with search schema (v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v7 -> v10)")
			// Purge all cache buckets except meta bucket
			for _, b := range core.AllBuckets() {
				if b == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(b)); err != nil && !errors.Is(err, bbolt.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: 6,
		ToVersion:   10,
		Description: "Migration to align cache schema with search schema (v6 -> v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v6 -> v10)")
			for _, b := range core.AllBuckets() {
				if b == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(b)); err != nil && !errors.Is(err, bbolt.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: 8,
		ToVersion:   10,
		Description: "Migration to align cache schema with search schema (v8 -> v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v8 -> v10)")
			for _, b := range core.AllBuckets() {
				if b == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(b)); err != nil && !errors.Is(err, bbolt.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		FromVersion: 9,
		ToVersion:   10,
		Description: "Migration to align cache schema with search schema (v9 -> v10)",
		Migrate: func(tx *bbolt.Tx, logger *slog.Logger) error {
			logger.Info("Purging all cache buckets due to schema version alignment (v9 -> v10)")
			for _, b := range core.AllBuckets() {
				if b == core.BucketMeta {
					continue
				}
				if err := tx.DeleteBucket([]byte(b)); err != nil && !errors.Is(err, bbolt.ErrBucketNotFound) {
					return err
				}
				if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
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

	for _, m := range registeredMigrations {
		if currentVersion == m.FromVersion {
			logger.Info("Running cache migration", "from", m.FromVersion, "to", m.ToVersion, "desc", m.Description)
			if err := db.Update(func(tx *bbolt.Tx) error {
				return m.Migrate(tx, logger)
			}); err != nil {
				return currentVersion, fmt.Errorf("migration %d->%d failed: %w", m.FromVersion, m.ToVersion, err)
			}
			currentVersion = m.ToVersion

			// Update the version in the database
			if err := db.Update(func(tx *bbolt.Tx) error {
				meta := tx.Bucket([]byte(core.BucketMeta))
				if meta == nil {
					return fmt.Errorf("metadata bucket missing")
				}
				v := make([]byte, 4)
				binary.BigEndian.PutUint32(v, currentVersion)
				return meta.Put([]byte(core.KeySchemaVersion), v)
			}); err != nil {
				return currentVersion, fmt.Errorf("failed to update schema version to %d: %w", currentVersion, err)
			}
		}
	}
	return currentVersion, nil
}
