package utils

import (
	"bytes"
	"sync"
	"testing"
)

func TestNewBufferPool(t *testing.T) {
	pool := NewBufferPool()
	if pool == nil {
		t.Fatal("NewBufferPool() returned nil")
	}
}

func TestBufferPoolGet(t *testing.T) {
	pool := NewBufferPool()

	buf := pool.Get()
	if buf == nil {
		t.Fatal("Get() returned nil")
	}

	// Buffer should be empty initially
	if buf.Len() != 0 {
		t.Errorf("Get() returned buffer with length %d, want 0", buf.Len())
	}
}

func TestBufferPoolPut(t *testing.T) {
	pool := NewBufferPool()

	// Get a buffer, write to it, then put it back
	buf := pool.Get()
	buf.WriteString("test data")

	if buf.Len() == 0 {
		t.Error("Buffer should have data before Put")
	}

	pool.Put(buf)

	// Get the buffer again - it should be reset
	buf2 := pool.Get()
	if buf2.Len() != 0 {
		t.Errorf("Get() after Put() returned buffer with length %d, want 0", buf2.Len())
	}
}

func TestBufferPoolPutOversized(t *testing.T) {
	pool := NewBufferPool()

	// Create a large buffer (> 64KB)
	largeData := make([]byte, 65*1024)
	buf := bytes.NewBuffer(largeData)

	// This should not panic and should discard the buffer
	pool.Put(buf)

	// We can't easily test that the buffer was actually discarded,
	// but we can verify Put doesn't panic
}

func TestBufferPoolReuse(t *testing.T) {
	pool := NewBufferPool()

	// Get and put multiple times
	for i := 0; i < 10; i++ {
		buf := pool.Get()
		buf.WriteString("iteration ")
		buf.WriteByte(byte('0' + i))
		pool.Put(buf)
	}

	// Buffer should be reusable without issues
	buf := pool.Get()
	if buf.Len() != 0 {
		t.Error("Buffer should be reset after Put")
	}
}

func TestBufferPoolConcurrency(t *testing.T) {
	pool := NewBufferPool()
	var wg sync.WaitGroup
	numGoroutines := 100
	iterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				buf := pool.Get()
				buf.WriteString("goroutine ")
				buf.WriteByte(byte('0' + byte(id%10)))
				pool.Put(buf)
			}
		}(i)
	}

	wg.Wait()

	// If we get here without panic or deadlock, the test passes
	buf := pool.Get()
	if buf.Len() != 0 {
		t.Error("Buffer should be empty after concurrent operations")
	}
	pool.Put(buf)
}

func TestSharedBufferPool(t *testing.T) {
	// Test that SharedBufferPool is initialized
	if SharedBufferPool == nil {
		t.Fatal("SharedBufferPool is nil")
	}

	// Test basic operations on shared pool
	buf := SharedBufferPool.Get()
	if buf == nil {
		t.Fatal("SharedBufferPool.Get() returned nil")
	}

	buf.WriteString("shared pool test")
	SharedBufferPool.Put(buf)

	// Get again to verify reset
	buf2 := SharedBufferPool.Get()
	if buf2.Len() != 0 {
		t.Error("SharedBufferPool buffer should be reset after Put")
	}
	SharedBufferPool.Put(buf2)
}

func TestBufferPoolCapacity(t *testing.T) {
	pool := NewBufferPool()

	// Get buffer and grow it
	buf := pool.Get()
	initialCap := buf.Cap()

	// Write enough data to potentially grow the buffer
	data := make([]byte, 1024)
	buf.Write(data)

	grownCap := buf.Cap()
	pool.Put(buf)

	// Get buffer again - capacity should be preserved (if <= 64KB)
	buf2 := pool.Get()
	if buf2.Cap() < grownCap {
		t.Logf("Buffer capacity reduced from %d to %d after Put/Get", grownCap, buf2.Cap())
	}

	_ = initialCap // May be used in future assertions
}

func BenchmarkBufferPoolGetPut(b *testing.B) {
	pool := NewBufferPool()
	data := []byte("benchmark data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get()
		buf.Write(data)
		pool.Put(buf)
	}
}

func BenchmarkBufferPoolParallel(b *testing.B) {
	pool := NewBufferPool()
	data := []byte("parallel benchmark data")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get()
			buf.Write(data)
			pool.Put(buf)
		}
	})
}

func BenchmarkBufferWithoutPool(b *testing.B) {
	data := []byte("benchmark data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := new(bytes.Buffer)
		buf.Write(data)
		// buf goes out of scope, will be GC'd
	}
}
