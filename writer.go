package brotli

import (
	"errors"
	"io"
	"runtime"
	"sync"

	"github.com/nijaru/brotli/internal/encoder/generic"
	"github.com/nijaru/brotli/internal/encoder/q0"
	"github.com/nijaru/brotli/internal/quality"
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

type writerState struct {
	q0State      q0.Encoder
	genericState generic.State
	mu           sync.Mutex
	released     bool
	sessionID    uint64
}

type Writer struct {
	dst       io.Writer
	options   WriterOptions
	err       error
	plan      quality.Plan
	state     *writerState
	sessionID uint64
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

type cleanupArg struct {
	state     *writerState
	sessionID uint64
}

// NewWriterOptions is like NewWriter but specifies WriterOptions
func NewWriterOptions(dst io.Writer, options WriterOptions) *Writer {
	w := &Writer{options: options}
	w.Reset(dst)
	return w
}

func normalizeWriterOptions(options WriterOptions) (WriterOptions, quality.Plan) {
	lgwin := 22
	if options.LGWin > 0 {
		lgwin = options.LGWin
	}
	plan := quality.NewPlan(options.Quality, lgwin, 0, 0, false)
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

	if w.state == nil {
		w.sessionID = nextSessionID()

		pool := getWriterStatePool(w.plan.Quality)
		v := pool.Get()
		if v == nil {
			w.state = &writerState{}
		} else {
			w.state = v.(*writerState)
		}

		w.state.mu.Lock()
		w.state.sessionID = w.sessionID
		w.state.released = false
		w.state.mu.Unlock()

		runtime.AddCleanup(w, func(arg cleanupArg) {
			releaseWriterState(pool, arg.state, arg.sessionID)
		}, cleanupArg{state: w.state, sessionID: w.sessionID})
	}

	if w.plan.Tier == quality.TierQ0 {
		w.state.q0State.Reset(lgwin)
	} else {
		generic.InitState(&w.state.genericState)
		w.state.genericState.Params.Quality = w.plan.Quality
		w.state.genericState.Params.Lgwin = lgwin
		w.state.genericState.Plan = w.plan
		w.state.genericState.Dst = dst
		w.state.genericState.Err = nil
	}
	runtime.KeepAlive(w)
}

func (w *Writer) writeChunk(p []byte, op int) (n int, err error) {
	if w.dst == nil {
		return 0, errWriterClosed
	}
	if w.err != nil {
		return 0, w.err
	}

	if w.plan.Tier == quality.TierQ0 {
		var isLast bool
		if op == operationFinish {
			isLast = true
		}

		if op == operationFlush {
			data := w.state.q0State.Flush()
			if len(data) > 0 {
				_, w.err = w.dst.Write(data)
			}
			return 0, w.err
		}

		data := w.state.q0State.Encode(nil, p, isLast)
		if len(data) > 0 {
			_, w.err = w.dst.Write(data)
		}
		return len(p), w.err
	}

	for {
		availableIn := uint(len(p))
		nextIn := p
		success := generic.CompressStream(&w.state.genericState, op, &availableIn, &nextIn)
		bytesConsumed := len(p) - int(availableIn)
		p = p[bytesConsumed:]
		n += bytesConsumed
		if !success {
			return n, errEncode
		}

		if len(p) == 0 || w.state.genericState.Err != nil {
			w.err = w.state.genericState.Err
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
	runtime.KeepAlive(w)
	return err
}

// Close flushes remaining data to the decorated writer.
func (w *Writer) Close() error {
	// If stream is already closed, it is reported by `writeChunk`.
	_, err := w.writeChunk(nil, operationFinish)
	w.dst = nil
	runtime.KeepAlive(w)
	return err
}

// Write implements io.Writer. Flush or Close must be called to ensure that the
// encoded bytes are actually flushed to the underlying Writer.
func (w *Writer) Write(p []byte) (n int, err error) {
	n, err = w.writeChunk(p, operationProcess)
	runtime.KeepAlive(w)
	return n, err
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }
