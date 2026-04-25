package brotli

import (
	"io"

	"github.com/nijaru/brotli/internal/decoder"
)

type decodeError int

func (err decodeError) Error() string {
	return "brotli: decoder error"
}

// NewReader creates a new Reader reading the given reader.
func NewReader(src io.Reader) *decoder.Reader {
	r := &decoder.Reader{}
	r.Reset(src)
	return r
}
