package brotli

import (
	"io"

	"github.com/nijaru/brotli/internal/decoder"
)

// Reader wraps the internal decoder.Reader for API encapsulation,
// allowing clients to use it as a standard io.Reader.
type Reader struct {
	dec decoder.Reader
}

// Read implements io.Reader to read uncompressed bytes from the wrapped decompressor.
func (r *Reader) Read(p []byte) (n int, err error) {
	return r.dec.Read(p)
}

// WriteTo implements io.WriterTo to stream decompressed bytes directly into w.
// This allows io.Copy(w, r) to achieve zero heap allocations in steady state.
func (r *Reader) WriteTo(w io.Writer) (n int64, err error) {
	buf := copyBufPool.Get().(*[32768]byte)
	defer copyBufPool.Put(buf)
	b := buf[:]

	for {
		nr, rErr := r.Read(b)
		if nr > 0 {
			nw, wErr := w.Write(b[:nr])
			if nw > 0 {
				n += int64(nw)
			}
			if wErr != nil {
				return n, wErr
			}
			if nw != nr {
				return n, io.ErrShortWrite
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				return n, nil
			}
			return n, rErr
		}
	}
}

// Reset discards the Reader's state and makes it equivalent to the result of
// its original state from NewReader, but reading from src instead.
// This permits reusing a Reader rather than allocating a new one.
func (r *Reader) Reset(src io.Reader) error {
	return r.dec.Reset(src)
}

// ResetBytes discards the Reader's state and makes it equivalent to the result of
// its original state from NewReader, but reading from the provided slice instead.
// This permits reusing a Reader without allocating a bytes.Reader or copying input data.
func (r *Reader) ResetBytes(src []byte) {
	r.dec.ResetBytes(src)
}

// SetCustomDictionary configures a pre-shared dictionary on the Reader.
func (r *Reader) SetCustomDictionary(dict []byte) {
	r.dec.SetCustomDictionary(dict)
}

// NewReader creates a new Reader reading the given reader.
func NewReader(src io.Reader) *Reader {
	r := &Reader{}
	r.Reset(src)
	return r
}
