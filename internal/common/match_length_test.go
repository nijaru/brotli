package common

import (
	"bytes"
	"fmt"
	"testing"
)

// referenceMatchLength is the golden byte-by-byte scalar reference implementation.
func referenceMatchLength(s1, s2 []byte, limit uint) uint {
	matched := uint(0)
	for matched < limit && matched < uint(len(s1)) && matched < uint(len(s2)) && s1[matched] == s2[matched] {
		matched++
	}
	return matched
}

func TestFindMatchLengthDifferential(t *testing.T) {
	sizes := []int{0, 1, 2, 3, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 65, 127, 128, 129, 256, 512, 1024}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size%d", size), func(t *testing.T) {
			if size == 0 {
				res := FindMatchLengthWithLimit([]byte{}, []byte{}, 0)
				if res != 0 {
					t.Fatalf("expected 0, got %d", res)
				}
				return
			}

			s1 := bytes.Repeat([]byte("abcdefghijklmnopqrstuvwxyz012345"), (size/32)+2)[:size]
			s2 := make([]byte, size)
			copy(s2, s1)

			// 1. Exact match test
			want := referenceMatchLength(s1, s2, uint(size))
			got := FindMatchLengthWithLimit(s1, s2, uint(size))
			if got != want {
				t.Fatalf("exact match size %d: want %d, got %d", size, want, got)
			}

			// 2. Mismatch at every possible single byte offset
			for mismatchIdx := 0; mismatchIdx < size; mismatchIdx++ {
				s2[mismatchIdx] ^= 0xFF

				want := referenceMatchLength(s1, s2, uint(size))
				got := FindMatchLengthWithLimit(s1, s2, uint(size))
				if got != want {
					t.Fatalf("mismatch at %d in size %d: want %d, got %d", mismatchIdx, size, want, got)
				}

				// Test with limit smaller than mismatch
				if mismatchIdx > 0 {
					subLimit := uint(mismatchIdx / 2)
					wantSub := referenceMatchLength(s1, s2, subLimit)
					gotSub := FindMatchLengthWithLimit(s1, s2, subLimit)
					if gotSub != wantSub {
						t.Fatalf("subLimit %d (mismatch at %d): want %d, got %d", subLimit, mismatchIdx, wantSub, gotSub)
					}
				}

				s2[mismatchIdx] ^= 0xFF // restore
			}
		})
	}
}

func BenchmarkFindMatchLength(b *testing.B) {
	sizes := []int{8, 16, 32, 64, 128, 256, 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Identical/%dB", size), func(b *testing.B) {
			s1 := bytes.Repeat([]byte("1234567890abcdef"), (size/16)+1)[:size]
			s2 := make([]byte, size)
			copy(s2, s1)

			b.SetBytes(int64(size))
			b.ResetTimer()
			for b.Loop() {
				_ = FindMatchLengthWithLimit(s1, s2, uint(size))
			}
		})

		b.Run(fmt.Sprintf("MismatchAtEnd/%dB", size), func(b *testing.B) {
			s1 := bytes.Repeat([]byte("1234567890abcdef"), (size/16)+1)[:size]
			s2 := make([]byte, size)
			copy(s2, s1)
			s2[size-1] ^= 0x01

			b.SetBytes(int64(size))
			b.ResetTimer()
			for b.Loop() {
				_ = FindMatchLengthWithLimit(s1, s2, uint(size))
			}
		})

		b.Run(fmt.Sprintf("MismatchEarly/%dB", size), func(b *testing.B) {
			s1 := bytes.Repeat([]byte("1234567890abcdef"), (size/16)+1)[:size]
			s2 := make([]byte, size)
			copy(s2, s1)
			s2[3] ^= 0x01 // early mismatch at byte 3

			b.SetBytes(int64(size))
			b.ResetTimer()
			for b.Loop() {
				_ = FindMatchLengthWithLimit(s1, s2, uint(size))
			}
		})
	}
}
