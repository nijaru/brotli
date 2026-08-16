//go:build goexperiment.simd

package common

import (
	"encoding/binary"
	"math/bits"
	"simd"
)

// FindMatchLengthWithLimit computes the number of matching bytes between s1 and s2 up to limit.
//
// Invariant: callers ensure len(s1) >= limit and len(s2) >= limit.
// Gating: Profiles show 96.6%-99.1% of matches terminate within 16 bytes.
// To avoid vector setup overhead, this kernel first runs a scalar 16-byte fast rejection filter
// before dispatching into the portable SIMD vector loop.
func FindMatchLengthWithLimit(s1 []byte, s2 []byte, limit uint) uint {
	matched := uint(0)

	// Step 1: Scalar fast-rejection check for the first 16 bytes (handles ~98% of candidate matches)
	if limit >= 8 {
		w1 := binary.LittleEndian.Uint64(s1)
		w2 := binary.LittleEndian.Uint64(s2)
		if w1 != w2 {
			return uint(bits.TrailingZeros64(w1^w2) >> 3)
		}
		matched = 8

		if limit >= 16 {
			w1 = binary.LittleEndian.Uint64(s1[8:])
			w2 = binary.LittleEndian.Uint64(s2[8:])
			if w1 != w2 {
				return 8 + uint(bits.TrailingZeros64(w1^w2)>>3)
			}
			matched = 16
		}
	}

	// Step 2: Vectorized loop for long matches (32-byte chunks)
	for matched+32 <= limit {
		v1 := simd.LoadUint8x32(s1[matched:])
		v2 := simd.LoadUint8x32(s2[matched:])
		mask := simd.Equal(v1, v2)
		if !mask.All() {
			// Find first differing byte in the 32-byte vector
			bitmask := mask.ToBitMask()
			return matched + uint(bits.TrailingZeros32(^bitmask))
		}
		matched += 32
	}

	// Step 3: Vectorized 16-byte step if at least 16 bytes remain
	if matched+16 <= limit {
		v1 := simd.LoadUint8x16(s1[matched:])
		v2 := simd.LoadUint8x16(s2[matched:])
		mask := simd.Equal(v1, v2)
		if !mask.All() {
			bitmask := mask.ToBitMask()
			return matched + uint(bits.TrailingZeros16(^bitmask))
		}
		matched += 16
	}

	// Step 4: Scalar tail for remainder (< 16 bytes)
	for matched < limit && s1[matched] == s2[matched] {
		matched++
	}

	return matched
}
