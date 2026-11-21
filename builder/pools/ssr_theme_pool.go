//go:build !js || !wasm

package pools

import (
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// MapStringSSRThemePairPool manages a pool of map[string]models.SSRThemePair objects.
// Kept out of the WASM build so pools does not depend on builder/models.
type MapStringSSRThemePairPool struct {
	pool sync.Pool // stores map[string]models.SSRThemePair
}

// NewMapStringSSRThemePairPool returns a new MapStringSSRThemePairPool.
func NewMapStringSSRThemePairPool() *MapStringSSRThemePairPool {
	return &MapStringSSRThemePairPool{
		pool: sync.Pool{
			New: func() any {
				return make(map[string]models.SSRThemePair)
			},
		},
	}
}

// Get returns a map from the pool.
func (pool *MapStringSSRThemePairPool) Get() map[string]models.SSRThemePair {
	return pool.pool.Get().(map[string]models.SSRThemePair)
}

// Put returns a map to the pool.
func (pool *MapStringSSRThemePairPool) Put(m map[string]models.SSRThemePair) {
	if m == nil {
		return
	}
	clear(m)
	pool.pool.Put(m)
}

// SharedMapStringSSRThemePairPool is the shared map[string]models.SSRThemePair pool instance.
var SharedMapStringSSRThemePairPool = NewMapStringSSRThemePairPool()
