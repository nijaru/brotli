package brotli

import (
	"io"

	"github.com/nijaru/brotli/internal/decoder")

// Reader wraps the internal decoder.Reader for API encapsulation,
// allowing clients to use it as a standard io.Reader.
type Reader struct {
	dec decoder.Reader
}

// Read implements io.Reader to read uncompressed bytes from the wrapped decompressor.
func (r *Reader) Read(p []byte) (n int, err error) {
	return r.dec.Read(p)
}

// Reset discards the Reader's state and makes it equivalent to the result of
// its original state from NewReader, but reading from src instead.
// This permits reusing a Reader rather than allocating a new one.
func (r *Reader) Reset(src io.Reader) error {
	return r.dec.Reset(src)
}

// NewReader creates a new Reader reading the given reader.
func NewReader(src io.Reader) *Reader {
	r := &Reader{}
	r.Reset(src)
	return r
}
