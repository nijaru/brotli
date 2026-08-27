package brotli

// sliceWriter is a zero-allocation io.Writer adapter that appends directly into a byte slice.
type sliceWriter struct {
	buf []byte
}

func (sw *sliceWriter) Write(p []byte) (int, error) {
	sw.buf = append(sw.buf, p...)
	return len(p), nil
}

// Encode compresses src and writes to dst, returning the compressed slice.
// If cap(dst) is sufficient, it writes directly into it with 0 heap allocations and 0 intermediate copies.
// If dst is nil or too small, a new slice is allocated and grown automatically.
// The compressed data is overwritten starting at index 0 of dst.
func Encode(dst, src []byte, quality int) []byte {
	pool := getWriterPool(quality)

	if dst != nil {
		dst = dst[:0]
	}

	var sw sliceWriter
	sw.buf = dst

	w := pool.Get(&sw)
	_, _ = w.Write(src)
	_ = w.Close()
	pool.Put(w)

	return sw.buf
}
