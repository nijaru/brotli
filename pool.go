package brotli

import (
	"io"
	"sync"
)

// Global default pools for standard compression and decompression
var (
	defaultWriterPool = NewWriterPool(DefaultCompression)
	defaultReaderPool = NewReaderPool()
	copyBufPool       = sync.Pool{
		New: func() any {
			return new([32768]byte)
		},
	}

	writerPools [12]*WriterPool
)

func init() {
	for i := 0; i < 12; i++ {
		writerPools[i] = NewWriterPool(i)
	}
}

// getWriterPool returns the thread-safe WriterPool matching the normalized quality level.
func getWriterPool(quality int) *WriterPool {
	opts, _ := normalizeWriterOptions(WriterOptions{Quality: quality})
	return writerPools[opts.Quality]
}

// GetWriter retrieves a default-compression (*Writer) from a package-level pool
// and resets it to write to dst. Use it for repeated one-shot compression where
// the caller can return the writer with PutWriter after Close.
func GetWriter(dst io.Writer) *Writer {
	return defaultWriterPool.Get(dst)
}

// PutWriter returns a Writer to the package-level default pool. Callers must
// not use w after PutWriter returns.
func PutWriter(w *Writer) {
	defaultWriterPool.Put(w)
}

// GetReader retrieves a (*Reader) from a package-level pool and resets it
// to read from src, eliminating initial allocation overhead.
func GetReader(src io.Reader) *Reader {
	return defaultReaderPool.Get(src)
}

// GetReaderBytes retrieves a (*Reader) from a package-level pool and resets it
// to read from the provided slice, eliminating all allocation overhead.
func GetReaderBytes(src []byte) *Reader {
	return defaultReaderPool.GetBytes(src)
}

// PutReader returns a Reader to the package-level default pool.
// Before returning it, the source is cleared to prevent memory leaks.
func PutReader(r *Reader) {
	defaultReaderPool.Put(r)
}

// WriterPool manages a thread-safe pool of Writer instances to reduce garbage
// collection overhead and heap allocations. Each pool is specific to a compression
// quality level to ensure reused encoders match user-requested behavior.
//
// Pooling is most useful when compressing many independent streams at the same
// quality level. A checked-out Writer still has normal Writer ownership rules:
// write to it from one goroutine at a time, call Close when the stream is
// complete, then return it to the pool and stop using that pointer.
type WriterPool struct {
	pool    sync.Pool
	options WriterOptions
}

// NewWriterPool creates a new WriterPool for the given quality level.
func NewWriterPool(quality int) *WriterPool {
	options, _ := normalizeWriterOptions(WriterOptions{Quality: quality})
	return &WriterPool{
		options: options,
	}
}

// Get retrieves a Writer from the pool and resets it to write to dst.
// If the pool is empty, a new Writer is allocated.
func (p *WriterPool) Get(dst io.Writer) *Writer {
	v := p.pool.Get()
	if v == nil {
		return NewWriterOptions(dst, p.options)
	}
	w := v.(*Writer)
	w.options = p.options
	w.Reset(dst)
	return w
}

// Put returns the Writer to the pool. Callers must not use w after Put returns.
// Before returning it, Put clears the destination to avoid retaining the
// original dst writer.
func (p *WriterPool) Put(w *Writer) {
	if w == nil {
		return
	}
	w.options = p.options
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

// GetBytes retrieves a Reader from the pool and resets it to read from the provided slice.
// If the pool is empty, a new Reader is allocated.
func (p *ReaderPool) GetBytes(src []byte) *Reader {
	v := p.pool.Get()
	if v == nil {
		r := &Reader{}
		r.ResetBytes(src)
		return r
	}
	r := v.(*Reader)
	r.ResetBytes(src)
	return r
}

// Put returns the Reader to the pool.
// Before returning it, it resets the source to nil to prevent memory leaks.
func (p *ReaderPool) Put(r *Reader) {
	if r == nil {
		return
	}
	r.Reset(nil)
	p.pool.Put(r)
}
