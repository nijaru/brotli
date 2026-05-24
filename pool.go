package brotli

import (
	"io"
	"sync"
)

// Global default pools for standard compression and decompression
var (
	defaultWriterPool = NewWriterPool(DefaultCompression)
	defaultReaderPool = NewReaderPool()
)

// GetWriter retrieves a default-compression (*Writer) from a package-level pool
// and resets it to write to dst, eliminating initial allocation overhead.
func GetWriter(dst io.Writer) *Writer {
	return defaultWriterPool.Get(dst)
}

// PutWriter returns a Writer to the package-level default pool.
// Before returning it, the destination is cleared to prevent memory leaks.
func PutWriter(w *Writer) {
	defaultWriterPool.Put(w)
}

// GetReader retrieves a (*Reader) from a package-level pool and resets it
// to read from src, eliminating initial allocation overhead.
func GetReader(src io.Reader) *Reader {
	return defaultReaderPool.Get(src)
}

// PutReader returns a Reader to the package-level default pool.
// Before returning it, the source is cleared to prevent memory leaks.
func PutReader(r *Reader) {
	defaultReaderPool.Put(r)
}

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
