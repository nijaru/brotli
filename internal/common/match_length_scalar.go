//go:build !goexperiment.simd

package common

import (
	"encoding/binary"
	"math/bits"
)

// FindMatchLengthWithLimit computes the number of matching bytes between s1 and s2 up to limit.
//
// Invariant: callers ensure len(s1) >= limit and len(s2) >= limit.
// Uses 64-bit unaligned word matching with trailing zero bit counts for byte-granularity offsets.
func FindMatchLengthWithLimit(s1 []byte, s2 []byte, limit uint) uint {
	matched := uint(0)
	if limit >= 8 {
		for matched+8 <= limit {
			w1 := binary.LittleEndian.Uint64(s1[matched:])
			w2 := binary.LittleEndian.Uint64(s2[matched:])
			if w1 != w2 {
				return matched + uint(bits.TrailingZeros64(w1^w2)>>3)
			}
			matched += 8
		}
	}
	for matched < limit && s1[matched] == s2[matched] {
		matched++
	}
	return matched
}
