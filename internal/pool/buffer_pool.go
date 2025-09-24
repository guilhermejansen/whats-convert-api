package pool

import (
	"sync"
	"sync/atomic"
)

// BufferPool manages reusable byte buffers for memory optimization
// Pre-allocates buffers to avoid GC pressure under high load
type BufferPool struct {
	pool        sync.Pool
	size        int
	allocated   int32
	inUse       int32
	hits        int64
	misses      int64
}

// NewBufferPool creates a new buffer pool with pre-allocated buffers
func NewBufferPool(count, size int) *BufferPool {
	bp := &BufferPool{
		size: size,
	}

	// Configure the pool with buffer factory
	bp.pool = sync.Pool{
		New: func() interface{} {
			atomic.AddInt32(&bp.allocated, 1)
			atomic.AddInt64(&bp.misses, 1)
			return make([]byte, size)
		},
	}

	// Pre-allocate buffers for immediate availability
	buffers := make([][]byte, count)
	for i := 0; i < count; i++ {
		buffers[i] = make([]byte, size)
		atomic.AddInt32(&bp.allocated, 1)
	}

	// Put pre-allocated buffers into pool
	for i := 0; i < count; i++ {
		bp.pool.Put(buffers[i])
	}

	return bp
}

// Get retrieves a buffer from the pool
func (bp *BufferPool) Get() []byte {
	atomic.AddInt32(&bp.inUse, 1)
	atomic.AddInt64(&bp.hits, 1)
	return bp.pool.Get().([]byte)
}

// Put returns a buffer to the pool for reuse
func (bp *BufferPool) Put(buf []byte) {
	if buf == nil || cap(buf) < bp.size {
		return
	}

	// Reset buffer to full capacity before returning
	buf = buf[:bp.size]

	// Clear sensitive data (optional, impacts performance)
	// for i := range buf {
	//     buf[i] = 0
	// }

	atomic.AddInt32(&bp.inUse, -1)
	bp.pool.Put(buf)
}

// GetSized returns a buffer of specific size
// Uses pool buffer if size fits, otherwise allocates new
func (bp *BufferPool) GetSized(size int) []byte {
	if size <= bp.size {
		buf := bp.Get()
		return buf[:size]
	}
	// Size exceeds pool buffer, allocate new
	atomic.AddInt32(&bp.inUse, 1)
	return make([]byte, size)
}

// PutSized returns a sized buffer, handling both pool and non-pool buffers
func (bp *BufferPool) PutSized(buf []byte) {
	if cap(buf) >= bp.size {
		// This is a pool buffer, return it
		bp.Put(buf)
	} else {
		// Non-pool buffer, just decrease counter
		atomic.AddInt32(&bp.inUse, -1)
	}
}

// Stats returns current pool statistics
type BufferPoolStats struct {
	Allocated int32
	InUse     int32
	Available int32
	Hits      int64
	Misses    int64
	HitRate   float64
}

func (bp *BufferPool) Stats() BufferPoolStats {
	allocated := atomic.LoadInt32(&bp.allocated)
	inUse := atomic.LoadInt32(&bp.inUse)
	hits := atomic.LoadInt64(&bp.hits)
	misses := atomic.LoadInt64(&bp.misses)

	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return BufferPoolStats{
		Allocated: allocated,
		InUse:     inUse,
		Available: allocated - inUse,
		Hits:      hits,
		Misses:    misses,
		HitRate:   hitRate,
	}
}