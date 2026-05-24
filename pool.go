package brotli

import (
	"io"
	"sync"
)

// WriterPool manages a thread-safe pool of Writer instances to reduce garbage
// collection overhead and heap allocations. Each pool is specific to a compression
// quality level to ensure reused encoders match user-requested behavior.
type WriterPool struct {
	pool    sync.Pool
	quality int
}

// NewWriterPool creates a new WriterPool for the given quality level.
func NewWriterPool(quality int) *WriterPool {
	return &WriterPool{
		quality: quality,
	}
}

// Get retrieves a Writer from the pool and resets it to write to dst.
// If the pool is empty, a new Writer is allocated.
func (p *WriterPool) Get(dst io.Writer) *Writer {
	v := p.pool.Get()
	if v == nil {
		return NewWriterLevel(dst, p.quality)
	}
	w := v.(*Writer)
	w.Reset(dst)
	return w
}

// Put returns the Writer to the pool.
// Before returning it, it resets the destination to nil to prevent memory leaks
// (holding references to the original dst writer).
func (p *WriterPool) Put(w *Writer) {
	w.Reset(nil)
	p.pool.Put(w)
}

// ReaderPool manages a thread-safe pool of Reader instances to eliminate heap
// allocations during decompressor instantiation under high concurrency.
type ReaderPool struct {
	pool sync.Pool
}

// NewReaderPool creates a new ReaderPool.
func NewReaderPool() *ReaderPool {
	return &ReaderPool{}
}

// Get retrieves a Reader from the pool and resets it to read from src.
// If the pool is empty, a new Reader is allocated.
func (p *ReaderPool) Get(src io.Reader) *Reader {
	v := p.pool.Get()
	if v == nil {
		return NewReader(src)
	}
	r := v.(*Reader)
	r.Reset(src)
	return r
}

// Put returns the Reader to the pool.
// Before returning it, it resets the source to nil to prevent memory leaks.
func (p *ReaderPool) Put(r *Reader) {
	r.Reset(nil)
	p.pool.Put(r)
}
