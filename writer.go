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
	// The higher the quality, the slower the compression. Range is 0 to 11.
	Quality int
	// LGWin is the base 2 logarithm of the sliding window size.
	// Range is 10 to 24. 0 indicates automatic configuration based on Quality.
	LGWin int
}

var errWriterClosed = fmt.Errorf("brotli: Writer is closed")

// Writer compresses data and writes the compressed output to an io.Writer.
type Writer struct {
	w       matchfinder.Writer
	dst     io.Writer
	options WriterOptions
	err     error
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
	if options.Quality > 11 {
		options.Quality = 11
	}

	lgWin := 16
	if options.LGWin > 0 {
		lgWin = options.LGWin
	}

	w := &Writer{
		w: matchfinder.Writer{
			Dest:        dst,
			MatchFinder: getMatchFinder(options.Quality, options.LGWin),
			Encoder:     &Encoder{},
			BlockSize:   1 << uint(lgWin),
		},
		dst:     dst,
		options: options,
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
	if w.w.MatchFinder == nil {
		w.w.MatchFinder = getMatchFinder(w.options.Quality, w.options.LGWin)
		w.w.Encoder = &Encoder{}
		if w.options.Quality < 1 {
			w.w.Encoder = &FastEncoder{}
		}
		lgWin := 16
		if w.options.LGWin > 0 {
			lgWin = w.options.LGWin
		}
		w.w.BlockSize = 1 << uint(lgWin)
	}
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

func getMatchFinder(level int, lgwin int) matchfinder.MatchFinder {
	maxDistance := 1 << 20 // 1 MB default
	if lgwin > 0 {
		maxDistance = 1 << uint(lgwin)
	}

	switch level {
	case 0, 1:
		return &matchfinder.ZFast{MaxDistance: maxDistance}
	case 2:
		return &matchfinder.ZDFast{MaxDistance: maxDistance}
	case 3:
		return &matchfinder.ZM{MaxDistance: maxDistance}
	case 4:
		return &matchfinder.Trio{MaxDistance: maxDistance}
	case 5, 6:
		return &matchfinder.Bargain1{MaxDistance: maxDistance}
	case 7:
		return &matchfinder.Bargain2{MaxDistance: maxDistance, Skip: true}
	case 8:
		return &matchfinder.Bargain2{MaxDistance: maxDistance}
	case 9:
		return &matchfinder.Bargain3{MaxDistance: maxDistance}
	case 10, 11:
		return &matchfinder.Zopfli{MaxDistance: maxDistance}
	}
	return &matchfinder.Bargain3{MaxDistance: maxDistance}
}

// NewParallelWriter creates a Writer that uses multiple goroutines to
// compress blocks in parallel.
func NewParallelWriter(dst io.Writer, options WriterOptions, concurrency int) *matchfinder.ParallelWriter {
	if options.Quality < 0 {
		options.Quality = 0
	} else if options.Quality > 11 {
		options.Quality = 11
	}

	lgWin := 16
	if options.LGWin > 0 {
		lgWin = options.LGWin
	}

	w := &matchfinder.ParallelWriter{
		Dest: dst,
		MatchFinder: func() matchfinder.MatchFinder {
			if options.Quality >= 5 && options.Quality <= 6 {
				return matchfinder.GetBargain1()
			}
			return getMatchFinder(options.Quality, options.LGWin)
		},
		Encoder: func() matchfinder.Encoder {
			if options.Quality < 1 {
				return matchfinder.GetFastEncoder(func() matchfinder.Encoder { return &FastEncoder{} })
			}
			return matchfinder.GetEncoder(func() matchfinder.Encoder { return &Encoder{} })
		},
		BlockSize:   1 << uint(lgWin),
		Concurrency: concurrency,
	}
	return w
}
