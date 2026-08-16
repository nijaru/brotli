//go:build goexperiment.simd

package common

import (
	"encoding/binary"
	"math/bits"
	"unsafe"

	"simd/archsimd"
)

// FindMatchLengthWithLimit computes the number of matching bytes between s1 and s2 up to limit.
//
// Invariant: callers ensure len(s1) >= limit and len(s2) >= limit; the limit clamp below
// keeps the function safe if a caller ever violates it.
// Gating: Empirical profiling shows 96.6%-99.1% of candidate matches terminate within 16 bytes,
// so the first 16 bytes are compared with a scalar 64-bit gate. Longer spans use dual 128-bit
// vector registers (32 bytes per iteration) with direct register extraction (GetElem).
// The slices are re-anchored to exact lengths before the vector loop so every in-loop bounds
// check folds into the loop condition.
func FindMatchLengthWithLimit(s1 []byte, s2 []byte, limit uint) uint {
	if limit == 0 {
		return 0
	}
	if uint(len(s1)) < limit {
		limit = uint(len(s1))
	}
	if uint(len(s2)) < limit {
		limit = uint(len(s2))
	}
	if limit == 0 {
		return 0
	}

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

	// Re-anchor to exact-length views: len(s1) == len(s2) == limit-matched.
	p1 := unsafe.Pointer(unsafe.SliceData(s1))
	p2 := unsafe.Pointer(unsafe.SliceData(s2))
	s1 = unsafe.Slice((*byte)(unsafe.Add(p1, matched)), limit-matched)
	s2 = unsafe.Slice((*byte)(unsafe.Add(p2, matched)), limit-matched)

	// Step 2: 32-byte dual-vector loop with direct register extraction
	for len(s1) >= 32 {
		v1a := archsimd.LoadUint8x16(s1[:16])
		v2a := archsimd.LoadUint8x16(s2[:16])
		v1b := archsimd.LoadUint8x16(s1[16:32])
		v2b := archsimd.LoadUint8x16(s2[16:32])

		diffA := v1a.Xor(v2a).ReshapeToUint64s()
		diffB := v1b.Xor(v2b).ReshapeToUint64s()

		w0 := diffA.GetElem(0)
		if w0 != 0 {
			return matched + uint(bits.TrailingZeros64(w0)>>3)
		}
		w1 := diffA.GetElem(1)
		if w1 != 0 {
			return matched + 8 + uint(bits.TrailingZeros64(w1)>>3)
		}
		w2 := diffB.GetElem(0)
		if w2 != 0 {
			return matched + 16 + uint(bits.TrailingZeros64(w2)>>3)
		}
		w3 := diffB.GetElem(1)
		if w3 != 0 {
			return matched + 24 + uint(bits.TrailingZeros64(w3)>>3)
		}
		s1 = s1[32:]
		s2 = s2[32:]
		matched += 32
	}

	// Step 3: Single 16-byte vector step if 16 bytes remain
	if len(s1) >= 16 {
		v1 := archsimd.LoadUint8x16(s1[:16])
		v2 := archsimd.LoadUint8x16(s2[:16])
		diff := v1.Xor(v2).ReshapeToUint64s()

		w0 := diff.GetElem(0)
		if w0 != 0 {
			return matched + uint(bits.TrailingZeros64(w0)>>3)
		}
		w1 := diff.GetElem(1)
		if w1 != 0 {
			return matched + 8 + uint(bits.TrailingZeros64(w1)>>3)
		}
		s1 = s1[16:]
		s2 = s2[16:]
		matched += 16
	}

	// Step 4: Scalar tail for remainder (< 16 bytes)
	for i := 0; i < len(s1) && s1[i] == s2[i]; i++ {
		matched++
	}

	return matched
}
