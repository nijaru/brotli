package brotli

import (
	"io"
	"slices"
)

// Decode decompresses src and writes to dst, returning the uncompressed slice.
// If cap(dst) is sufficient, it writes directly into it with 0 heap allocations.
// If dst is nil or too small, a new slice is allocated automatically.
// The uncompressed data is overwritten starting at index 0 of dst.
func Decode(dst, src []byte) ([]byte, error) {
	r := GetReaderBytes(src)
	defer PutReader(r)

	return decode(dst, src, r)
}

// DecodeLimit decompresses src and returns ErrOutputLimitExceeded if the
// decoded output would exceed limit bytes. A non-negative limit is required.
// The limit also caps decode-output buffer growth, so a small compressed
// payload cannot force the output slice to grow without bound.
func DecodeLimit(dst, src []byte, limit int) ([]byte, error) {
	if limit < 0 {
		return nil, ErrInvalidOutputLimit
	}

	r := GetReaderBytes(src)
	r.limited = true
	r.limit = int64(limit)
	r.read = 0
	defer PutReader(r)

	return decode(dst, src, r)
}

func decode(dst, src []byte, r *Reader) ([]byte, error) {
	if dst != nil {
		dst = dst[:0]
	}

	limit := -1
	if r.limited {
		limit = int(r.limit)
	}

	for {
		if len(dst) == cap(dst) {
			if limit >= 0 && len(dst) >= limit {
				// The output is exactly at the limit. Probe without growing
				// to distinguish a complete stream from an overflow.
				n, err := r.Read(nil)
				if n != 0 {
					return nil, ErrOutputLimitExceeded
				}
				if err == io.EOF {
					return dst, nil
				}
				if err != nil {
					return nil, err
				}
				continue
			}

			grow := cap(dst)
			if grow < 8192 {
				grow = 8192
				if hint := len(src) * 2; hint > grow {
					grow = hint
				}
			}
			if limit >= 0 {
				if remaining := limit - len(dst); grow > remaining {
					grow = remaining
				}
			}
			dst = slices.Grow(dst, grow)
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
