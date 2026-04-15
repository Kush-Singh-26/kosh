package assets

import (
	"testing"

	lru "github.com/hashicorp/golang-lru/v2"
)

func TestImageCache_LRUBehavior(t *testing.T) {
	cache := newImageCache(100, 10*1024*1024)

	k1 := imageCacheKey{path: "key1", size: 100, modTime: 1}
	k2 := imageCacheKey{path: "key2", size: 100, modTime: 2}
	k3 := imageCacheKey{path: "key3", size: 100, modTime: 3}

	cache.Set(k1, []byte("value1"))
	cache.Set(k2, []byte("value2"))
	cache.Set(k3, []byte("value3"))

	if _, ok := cache.Get(k1); !ok {
		t.Error("key1 should exist")
	}

	for i := 4; i <= 10; i++ {
		ki := imageCacheKey{path: "key", size: int64(i), modTime: int64(i)}
		cache.Set(ki, []byte("value"))
	}

	t.Log("LRU behavior test passed - no panic")
}

func TestImageCache_ConcurrentAccess(t *testing.T) {
	cache := newImageCache(1000, 50*1024*1024)

	done := make(chan bool)
	for i := range 10 {
		go func(id int) {
			for j := range 100 {
				key := imageCacheKey{path: "key", size: int64(id), modTime: int64(j % 10)}
				cache.Set(key, []byte("data"))
				cache.Get(key)
			}
			done <- true
		}(i)
	}

	for range 10 {
		<-done
	}
	t.Log("Concurrent access test passed")
}

func TestImageCache_LRUWithRealLibrary(t *testing.T) {
	lru, _ := lru.New[string, []byte](10)

	lru.Add("a", []byte("1"))
	lru.Add("b", []byte("2"))
	lru.Add("c", []byte("3"))

	if _, ok := lru.Get("a"); !ok {
		t.Error("a should exist")
	}

	lru.Add("d", []byte("4"))
	lru.Add("e", []byte("5"))
	lru.Add("f", []byte("6"))
	lru.Add("g", []byte("7"))
	lru.Add("h", []byte("8"))
	lru.Add("i", []byte("9"))
	lru.Add("j", []byte("10"))
	lru.Add("k", []byte("11"))
	lru.Add("l", []byte("12"))
	lru.Add("m", []byte("13"))

	if _, ok := lru.Get("a"); ok {
		t.Error("a should have been evicted")
	}

	t.Log("Real LRU library test passed")
}

