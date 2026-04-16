package cache

import (
	"bytes"
	"sort"

	"go.etcd.io/bbolt"
)

// EncodedPost holds pre-encoded data for batch commit
type EncodedPost struct {
	ContentID     []byte
	Data       []byte
	Path       []byte
	SearchData []byte
	DepsData   []byte
	Version    string
	Taxonomies map[string][]string
	Templates  []string
	Includes   []string
}

// batchOp represents a single key-value operation for bucket writes
type batchOp struct {
	key   []byte
	value []byte
}

// bucketOps groups all operations by bucket for sequential writes
type bucketOps struct {
	posts      []batchOp
	paths      []batchOp
	search     []batchOp
	deps       []batchOp
	taxonomies []batchOp
	templates  []batchOp
	includes   []batchOp
}

// writeOps performs sequential writes to a bucket
func writeOps(bucket *bbolt.Bucket, ops []batchOp) error {
	if bucket == nil {
		return nil
	}
	for _, op := range ops {
		if err := bucket.Put(op.key, op.value); err != nil {
			return err
		}
	}
	return nil
}

// sortOps sorts a slice of batch operations by key for sequential write performance
func sortOps(ops []batchOp) {
	sort.Slice(ops, func(i, j int) bool {
		return bytes.Compare(ops[i].key, ops[j].key) < 0
	})
}
