package brotli

import (
	"fmt"
	"io"

	"github.com/nijaru/brotli/matchfinder"
)

const (
	BestSpeed          = 0
	BestCompression    = 11
	DefaultCompression = 6
)

// WriterOptions configures Writer.
type WriterOptions struct {
	// Quality controls the compression-speed vs compression-density trade-offs.
	// The higher the quality, the slower the compression. Range is 0 to 9.
	Quality int
	// LGWin is the base 2 logarithm of the sliding window size.
	// Range is 10 to 24. 0 indicates automatic configuration based on Quality.
	LGWin int
}

var errWriterClosed = fmt.Errorf("brotli: Writer is closed")

// Writer compresses data and writes the compressed output to an io.Writer.
type Writer struct {
	w   matchfinder.Writer
	dst io.Writer
	err error
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

// NewWriterOptions is like NewWriter but specifies WriterOptions.
func NewWriterOptions(dst io.Writer, options WriterOptions) *Writer {
	if options.Quality < 0 {
		options.Quality = 0
	}
	if options.Quality > 9 {
		options.Quality = 9
	}

	w := &Writer{
		w: matchfinder.Writer{
			Dest:        dst,
			MatchFinder: getMatchFinder(options.Quality),
			Encoder:     &Encoder{},
			BlockSize:   1 << 16,
		},
		dst: dst,
	}
	if options.Quality < 1 {
		w.w.Encoder = &FastEncoder{}
	}
	return w
}

// Reset discards the Writer's state and makes it equivalent to the result of
// its original state from NewWriter or NewWriterLevel, but writing to dst
// instead. This permits reusing a Writer rather than allocating a new one.
func (w *Writer) Reset(dst io.Writer) {
	w.w.Reset(dst)
	w.dst = dst
	w.err = nil
}

// Flush outputs encoded data for all input provided to Write. The resulting
// output can be decoded to match all input before Flush, but the stream is
// not yet complete until after Close.
// Flush has a negative impact on compression.
func (w *Writer) Flush() error {
	if w.err != nil {
		return w.err
	}
	if err := w.w.Flush(); err != nil {
		w.err = err
		return err
	}
	return nil
}

// Close flushes remaining data to the decorated writer.
func (w *Writer) Close() error {
	if w.err != nil {
		return w.err
	}
	if err := w.w.Close(); err != nil {
		w.err = err
		return err
	}
	w.err = errWriterClosed
	return nil
}

// Write implements io.Writer. Flush or Close must be called to ensure that the
// encoded bytes are actually flushed to the underlying Writer.
func (w *Writer) Write(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.w.Write(p)
}

func getMatchFinder(level int) matchfinder.MatchFinder {
	switch level {
	case 0, 1:
		return &matchfinder.ZFast{MaxDistance: 1 << 20}
	case 2:
		return &matchfinder.ZDFast{MaxDistance: 1 << 20}
	case 3:
		return &matchfinder.ZM{MaxDistance: 1 << 20}
	case 4:
		return &matchfinder.Trio{MaxDistance: 1 << 20}
	case 5, 6:
		return &matchfinder.Bargain1{MaxDistance: 1 << 20}
	case 7:
		return &matchfinder.Bargain2{MaxDistance: 1 << 20, Skip: true}
	case 8:
		return &matchfinder.Bargain2{MaxDistance: 1 << 20}
	case 9:
		return &matchfinder.Bargain3{MaxDistance: 1 << 20}
	}
	return &matchfinder.Bargain3{MaxDistance: 1 << 20}
}

// NewParallelWriter creates a Writer that uses multiple goroutines to
// compress blocks in parallel.
func NewParallelWriter(dst io.Writer, level int, concurrency int) *matchfinder.ParallelWriter {
	if level < 0 {
		level = 0
	} else if level > 9 {
		level = 9
	}

	w := &matchfinder.ParallelWriter{
		Dest: dst,
		MatchFinder: func() matchfinder.MatchFinder {
			if level >= 5 && level <= 6 {
				return matchfinder.GetBargain1()
			}
			return getMatchFinder(level)
		},
		Encoder: func() matchfinder.Encoder {
			if level < 1 {
				return matchfinder.GetFastEncoder(func() matchfinder.Encoder { return &FastEncoder{} })
			}
			return matchfinder.GetEncoder(func() matchfinder.Encoder { return &Encoder{} })
		},
		BlockSize:   1 << 16,
		Concurrency: concurrency,
	}
	return w
}
