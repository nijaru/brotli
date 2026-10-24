package brotli

import (
	"errors"
	"io"

	"github.com/nijaru/brotli/internal/decoder"
)

var (
	// ErrOutputLimitExceeded is returned when decoded output would exceed the
	// limit configured with NewReaderLimit or DecodeLimit.
	ErrOutputLimitExceeded = errors.New("brotli: output limit exceeded")
	// ErrInvalidOutputLimit is returned when an output limit is negative.
	ErrInvalidOutputLimit = errors.New("brotli: output limit must be non-negative")
)

// Reader wraps the internal decoder.Reader for API encapsulation,
// allowing clients to use it as a standard io.Reader.
type Reader struct {
	dec     decoder.Reader
	limit   int64
	limited bool
	read    int64
}

// Read implements io.Reader to read uncompressed bytes from the wrapped decompressor.
// If the Reader has an output limit, Read returns ErrOutputLimitExceeded when
// the stream contains more decoded bytes than the limit allows.
func (r *Reader) Read(p []byte) (int, error) {
	if r.limited {
		return r.readLimited(p)
	}
	return r.dec.Read(p)
}

func (r *Reader) readLimited(p []byte) (int, error) {
	remaining := r.limit - r.read
	if remaining <= 0 {
		var probe [1]byte
		n, err := r.dec.Read(probe[:])
		if n > 0 {
			return 0, ErrOutputLimitExceeded
		}
		return 0, err
	}
	if len(p) == 0 {
		return r.dec.Read(p)
	}
	if int64(len(p)) > remaining {
		p = p[:int(remaining)]
	}

	n, err := r.dec.Read(p)
	r.read += int64(n)
	return n, err
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
// The configured output limit, if any, is preserved and applies to the new stream.
func (r *Reader) Reset(src io.Reader) error {
	r.read = 0
	return r.dec.Reset(src)
}

// ResetBytes discards the Reader's state and makes it equivalent to the result of
// its original state from NewReader, but reading from the provided slice instead.
// This permits reusing a Reader without allocating a bytes.Reader or copying input data.
// The configured output limit, if any, is preserved and applies to the new stream.
func (r *Reader) ResetBytes(src []byte) {
	r.read = 0
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

// NewReaderLimit creates a Reader that returns ErrOutputLimitExceeded if the
// decoded stream contains more than limit bytes. A non-negative limit is
// required; ErrInvalidOutputLimit is returned otherwise.
func NewReaderLimit(src io.Reader, limit int) (*Reader, error) {
	if limit < 0 {
		return nil, ErrInvalidOutputLimit
	}

	r := NewReader(src)
	r.limited = true
	r.limit = int64(limit)
	return r, nil
}
