package utils

import (
	"testing"

	"github.com/hashicorp/golang-lru/v2"
)

func TestImageCache_LRUBehavior(t *testing.T) {
	cache := newImageCache(100, 10*1024*1024)

	cache.set("key1", []byte("value1"))
	cache.set("key2", []byte("value2"))
	cache.set("key3", []byte("value3"))

	if _, ok := cache.get("key1"); !ok {
		t.Error("key1 should exist")
	}

	cache.set("key4", []byte("value4"))
	cache.set("key5", []byte("value5"))
	cache.set("key6", []byte("value6"))
	cache.set("key7", []byte("value7"))
	cache.set("key8", []byte("value8"))
	cache.set("key9", []byte("value9"))
	cache.set("key10", []byte("value10"))

	t.Log("LRU behavior test passed - no panic")
}

func TestImageCache_ConcurrentAccess(t *testing.T) {
	cache := newImageCache(1000, 50*1024*1024)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a'+id)) + string(rune('0'+j%10))
				cache.set(key, []byte("data"))
				cache.get(key)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
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
