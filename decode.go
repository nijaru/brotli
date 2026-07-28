package brotli

import (
	"io"
)

// Decode decompresses src and writes to dst, returning the uncompressed slice.
// If cap(dst) is sufficient, it writes directly into it with 0 heap allocations.
// If dst is nil or too small, a new slice is allocated automatically.
// The uncompressed data is overwritten starting at index 0 of dst.
func Decode(dst, src []byte) ([]byte, error) {
	r := GetReaderBytes(src)
	defer PutReader(r)

	if dst != nil {
		dst = dst[:0]
	}

	for {
		if len(dst) == cap(dst) {
			// Ensure we have some capacity to read into
			grow := cap(dst)
			if grow < 8192 {
				grow = 8192
				if hint := len(src) * 2; hint > grow {
					grow = hint
				}
			}
			newDst := make([]byte, len(dst), len(dst)+grow)
			copy(newDst, dst)
			dst = newDst
		}

		freeSlice := dst[len(dst):cap(dst)]
		n, err := r.Read(freeSlice)
		if n > 0 {
			dst = dst[:len(dst)+n]
		}
		if err == io.EOF {
			return dst, nil
		}
		if err != nil {
			return nil, err
		}
	}
}
