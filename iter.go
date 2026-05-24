package brotli

import (
	"bufio"
	"errors"
	"io"
	"iter"
)

// Lines returns an iterator that yields decompressed text line-by-line from the Reader.
// It uses a bufio.Scanner internally to perform streaming decompression on the fly.
func (r *Reader) Lines() iter.Seq[string] {
	return func(yield func(string) bool) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if !yield(scanner.Text()) {
				return
			}
		}
	}
}

// Chunks returns an iterator that yields decompressed chunks of data of up to size bytes
// along with any error encountered during decompression.
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
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				if !yield(chunk, nil) {
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
