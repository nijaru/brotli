package brotli

import (
	"errors"
	"io"
	"unsafe"

	"github.com/nijaru/brotli/internal/encoder"
)

const (
	BestSpeed          = 0
	BestCompression    = 11
	DefaultCompression = 6
)

// Operations that can be performed by streaming encoder.
const (
	operationProcess      = 0
	operationFlush        = 1
	operationFinish       = 2
	operationEmitMetadata = 3
)

// WriterOptions configures Writer.
type WriterOptions struct {
	// Quality controls the compression-speed vs compression-density trade-offs.
	// The higher the quality, the slower the compression. Range is 0 to 11.
	Quality int
	// LGWin is the base 2 logarithm of the sliding window size.
	// Range is 10 to 24. 0 indicates automatic configuration based on Quality.
	LGWin int
}

var (
	errEncode       = errors.New("brotli: encode error")
	errWriterClosed = errors.New("brotli: Writer is closed")
)

type Writer struct {
	dst     io.Writer
	options WriterOptions
	err     error
	plan    encoder.Plan

	q0State      encoder.Encoder
	genericState encoder.State
}

// Writes to the returned writer are compressed and written to dst.
// It is the caller's responsibility to call Close on the Writer when done.
// Writes may be buffered and not flushed until Close.
func NewWriter(dst io.Writer) *Writer {
	return NewWriterLevel(dst, DefaultCompression)
}

// NewWriterLevel is like NewWriter but specifies the compression level instead
// of assuming DefaultCompression.
// The compression level can be DefaultCompression or any integer value between
// BestSpeed and BestCompression inclusive.
func NewWriterLevel(dst io.Writer, level int) *Writer {
	return NewWriterOptions(dst, WriterOptions{
		Quality: level,
	})
}

// NewWriterOptions is like NewWriter but specifies WriterOptions
func NewWriterOptions(dst io.Writer, options WriterOptions) *Writer {
	w := &Writer{options: options}
	w.Reset(dst)
	return w
}

func normalizeWriterOptions(options WriterOptions) (WriterOptions, encoder.Plan) {
	lgwin := 22
	if options.LGWin > 0 {
		lgwin = options.LGWin
	}
	plan := encoder.NewPlan(options.Quality, lgwin, 0, 0, false)
	return WriterOptions{
		Quality: plan.Quality,
		LGWin:   plan.Lgwin,
	}, plan
}

// Reset discards the Writer's state and makes it equivalent to the result of
// its original state from NewWriter or NewWriterLevel, but writing to dst
// instead. This permits reusing a Writer rather than allocating a new one.
func (w *Writer) Reset(dst io.Writer) {
	w.dst = dst
	w.err = nil

	w.options, w.plan = normalizeWriterOptions(w.options)
	lgwin := uint(w.plan.Lgwin)

	if w.plan.Tier == encoder.TierQ0 {
		w.q0State.Reset(lgwin)
	} else {
		encoder.InitState(&w.genericState)
		w.genericState.Params.Quality = w.plan.Quality
		w.genericState.Params.Lgwin = lgwin
		w.genericState.Plan = w.plan
		w.genericState.Dst = dst
		w.genericState.Err = nil
	}
}

func (w *Writer) writeChunk(p []byte, op int) (n int, err error) {
	if w.dst == nil {
		return 0, errWriterClosed
	}
	if w.err != nil {
		return 0, w.err
	}

	if w.plan.Tier == encoder.TierQ0 {
		var isLast bool
		if op == operationFinish {
			isLast = true
		}

		if op == operationFlush {
			data := w.q0State.Flush()
			if len(data) > 0 {
				_, w.err = w.dst.Write(data)
			}
			return 0, w.err
		}

		data := w.q0State.Encode(nil, p, isLast)
		if len(data) > 0 {
			_, w.err = w.dst.Write(data)
		}
		return len(p), w.err
	}

	for {
		availableIn := uint(len(p))
		nextIn := p
		success := encoder.CompressStream(&w.genericState, op, &availableIn, &nextIn)
		bytesConsumed := len(p) - int(availableIn)
		p = p[bytesConsumed:]
		n += bytesConsumed
		if !success {
			return n, errEncode
		}

		if len(p) == 0 || w.genericState.Err != nil {
			w.err = w.genericState.Err
			return n, w.err
		}
	}
}

// Flush outputs encoded data for all input provided to Write. The resulting
// output can be decoded to match all input before Flush, but the stream is
// not yet complete until after Close.
// Flush has a negative impact on compression.
func (w *Writer) Flush() error {
	_, err := w.writeChunk(nil, operationFlush)
	return err
}

// Close flushes remaining data to the decorated writer.
func (w *Writer) Close() error {
	// If stream is already closed, it is reported by `writeChunk`.
	_, err := w.writeChunk(nil, operationFinish)
	w.dst = nil
	return err
}

// Write implements io.Writer. Flush or Close must be called to ensure that the
// encoded bytes are actually flushed to the underlying Writer.
func (w *Writer) Write(p []byte) (n int, err error) {
	return w.writeChunk(p, operationProcess)
}

// WriteString implements io.StringWriter to write a string directly.
func (w *Writer) WriteString(s string) (n int, err error) {
	if len(s) == 0 {
		return 0, nil
	}
	return w.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
}

// WriteByte implements io.ByteWriter to write a single byte.
func (w *Writer) WriteByte(c byte) error {
	var buf [1]byte
	buf[0] = c
	_, err := w.Write(buf[:])
	return err
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }
