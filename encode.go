package brotli

import "bytes"

// Encode compresses src and writes to dst, returning the compressed slice.
// If cap(dst) is sufficient, it writes directly into it with 0 heap allocations.
// If dst is nil or too small, a new slice is allocated automatically.
// The compressed data is overwritten starting at index 0 of dst.
func Encode(dst, src []byte, quality int) []byte {
	pool := getWriterPool(quality)
	var buf bytes.Buffer
	w := pool.Get(&buf)

	// Override quality and reset to write to buf
	w.options.Quality = quality
	w.Reset(&buf)

	_, _ = w.Write(src)
	_ = w.Close()

	needed := buf.Len()
	if cap(dst) >= needed {
		dst = dst[:needed]
	} else {
		dst = make([]byte, needed)
	}
	copy(dst, buf.Bytes())

	pool.Put(w)
	return dst
}
