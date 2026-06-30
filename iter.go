package brotli

import (
	"bufio"
	"errors"
	"io"
	"iter"
)

// Lines returns an iterator that yields decompressed text line-by-line from the Reader.
// It returns a sequence of (line, error), ensuring that any decompression or scanning
// errors (such as lines exceeding scanner buffer limit) are propagated to the caller.
func (r *Reader) Lines() iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if !yield(scanner.Text(), nil) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			yield("", err)
		}
	}
}

// Chunks returns a zero-allocation iterator that yields decompressed chunks of data
// of up to size bytes along with any error encountered during decompression.
//
// To achieve zero allocations in the hot loop, the yielded byte slice is a sub-slice
// of the iterator's internal buffer. The yielded slice is only valid during the
// current iteration; if the caller needs to retain the data, they must copy it.
//
// If size is less than or equal to 0, it yields an error immediately.
func (r *Reader) Chunks(size int) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		if size <= 0 {
			yield(nil, errors.New("brotli: chunk size must be positive"))
			return
		}
		buf := make([]byte, size)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				// Yield slice directly without copying for zero-allocation performance.
				if !yield(buf[:n], nil) {
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					return
				}
				yield(nil, err)
				return
			}
		}
	}
}
