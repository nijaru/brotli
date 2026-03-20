package brotli

import (
	"sync"
)

/* 
   We pool the largest buffers used by the encoder to eliminate 
   the madvise/GC overhead observed in profiles.
   Brotli requires 2 bytes of prefix and 7 bytes of slack.
*/

const poolSlack = 2 + 7

// bytePool provides a sync.Pool for large byte slices of a specific capacity.
type bytePool struct {
	pool sync.Pool
	size int
}

func newBytePool(size int) *bytePool {
	return &bytePool{
		size: size,
		pool: sync.Pool{
			New: func() any {
				return make([]byte, size)
			},
		},
	}
}

func (p *bytePool) Get() []byte {
	return p.pool.Get().([]byte)
}

func (p *bytePool) Put(b []byte) {
	if b == nil || cap(b) < p.size {
		return
	}
	p.pool.Put(b[:p.size])
}

// Global pools for standard Brotli window sizes + slack + typical tail.
// We use slightly larger sizes to accommodate the tail and slack.
var (
	pool16MB = newBytePool(16*1024*1024 + 1024*1024 + poolSlack) // 16MB + 1MB tail + slack
	pool8MB  = newBytePool(8*1024*1024 + 1024*1024 + poolSlack)  // 8MB + 1MB tail + slack
	pool1MB  = newBytePool(1*1024*1024 + 1024*1024 + poolSlack)  // 1MB + 1MB tail + slack
)

func getBufferForSize(size int) []byte {
	if size <= pool1MB.size {
		return pool1MB.Get()
	} else if size <= pool8MB.size {
		return pool8MB.Get()
	} else if size <= pool16MB.size {
		return pool16MB.Get()
	}
	return make([]byte, size)
}

func putBufferForSize(b []byte) {
	size := cap(b)
	if size >= pool16MB.size {
		pool16MB.Put(b)
	} else if size >= pool8MB.size {
		pool8MB.Put(b)
	} else if size >= pool1MB.size {
		pool1MB.Put(b)
	}
}
