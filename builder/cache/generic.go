package cache

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// TypedStore provides a type-safe interface for storing and retrieving items
type TypedStore[T any] interface {
	Get(bucketName string, key []byte) (*T, error)
	Put(bucketName string, key []byte, value *T) error
}

// typedStoreImpl implements TypedStore using msgpack
type typedStoreImpl[T any] struct {
	db *bolt.DB
}

// NewTypedStore creates a new TypedStore
func NewTypedStore[T any](db *bolt.DB) TypedStore[T] {
	return &typedStoreImpl[T]{db: db}
}

// Get retrieves an item from the cache
func (s *typedStoreImpl[T]) Get(bucketName string, key []byte) (*T, error) {
	var result *T
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", bucketName)
		}
		data := bucket.Get(key)
		if data == nil {
			return nil // Not found is not an error, return nil result
		}

		var item T
		if err := Decode(data, &item); err != nil {
			return err
		}
		result = &item
		return nil
	})
	return result, err
}

// Put stores an item in the cache
func (s *typedStoreImpl[T]) Put(bucketName string, key []byte, value *T) error {
	data, err := Encode(value)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return err
		}
		return bucket.Put(key, data)
	})
}
