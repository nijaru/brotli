package hash

import (
	"bytes"
	"math/bits"
	"math/rand"
	"testing"

	"github.com/nijaru/brotli/internal/common"
)

func BenchmarkStoreRangeH5(b *testing.B) {
	params := &common.EncoderParams{
		Hasher: common.HasherParams{
			Bucket_bits: 15,
			Block_bits:  7,
		},
	}

	h := &h5{}
	h.params = params.Hasher
	h.Initialize(params)

	data := pseudoRandom(1 << 20)
	mask := uint(len(data) - 1)

	h.Prepare(false, uint(len(data)), data)

	b.ReportAllocs()
	b.SetBytes(int64(len(data) - 64))
	b.ResetTimer()

	for b.Loop() {
		h.StoreRange(data, mask, 0, uint(len(data)-64))
	}
}

func BenchmarkStoreRangeH6(b *testing.B) {
	params := &common.EncoderParams{
		Hasher: common.HasherParams{
			Bucket_bits: 15,
			Block_bits:  4,
		},
	}

	h := &h6{}
	h.params = params.Hasher
	h.Initialize(params)

	data := pseudoRandom(1 << 20)
	mask := uint(len(data) - 1)

	h.Prepare(false, uint(len(data)), data)

	b.ReportAllocs()
	b.SetBytes(int64(len(data) - 64))
	b.ResetTimer()

	for b.Loop() {
		h.StoreRange(data, mask, 0, uint(len(data)-64))
	}
}

// pseudoRandom returns deterministic pseudo-random data so the benchmark
// touches the full bucket table, matching real encode working sets.
func pseudoRandom(n int) []byte {
	rng := rand.New(rand.NewSource(0xB10721))
	data := make([]byte, n)
	rng.Read(data)
	return data
}

// TestStoreRangeParity verifies the unrolled StoreRange produces identical
// table state (num[] and buckets[]) to sequential Store calls.
func TestStoreRangeParity(t *testing.T) {
	for name, params := range map[string]common.HasherParams{
		"Q8": {Bucket_bits: 15, Block_bits: 7},
		"Q5": {Bucket_bits: 15, Block_bits: 4},
	} {
		params := params
		e := &common.EncoderParams{Hasher: params}

		for _, tc := range []struct {
			name string
			data []byte
		}{
			{"random", pseudoRandom(1 << 16)},
			{"allSame", bytes.Repeat([]byte{0x41}, 1<<10)},
			{"periodic", bytes.Repeat([]byte("abcdefghij"), 128)},
			{"shortOdd", bytes.Repeat([]byte("xyz"), 501)}, // odd length, tail path
		} {
			tc := tc
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				for _, ht := range []struct {
					name string
					new  func() Handle
					same func(a, b Handle) bool
				}{
					{"h5", newH5ForTest, sameH5},
					{"h6", newH6ForTest, sameH6},
				} {
					t.Run(ht.name, func(t *testing.T) {
						ref := ht.new()
						got := ht.new()
						ref.Initialize(e)
						got.Initialize(e)
						ref.Prepare(false, uint(len(tc.data)), tc.data)
						got.Prepare(false, uint(len(tc.data)), tc.data)

						// Realistic ring-buffer mask: next power of two minus one.
						mask := (uint(1) << uint(bits.Len(uint(len(tc.data)-1)))) - 1
						end := uint(len(tc.data) - 64)
						var i uint
						for i = 0; i < end; i++ {
							ref.Store(tc.data, mask, i)
						}
						got.StoreRange(tc.data, mask, 0, end)
						if !ht.same(ref, got) {
							t.Fatalf("table state differs after StoreRange")
						}
					})
				}
			})
		}
	}
}

func newH5ForTest() Handle { return &h5{} }
func newH6ForTest() Handle { return &h6{} }

func sameH5(a, b Handle) bool {
	x, y := a.(*h5), b.(*h5)
	return sameTables(x.num, y.num, x.buckets, y.buckets)
}

func sameH6(a, b Handle) bool {
	x, y := a.(*h6), b.(*h6)
	return sameTables(x.num, y.num, x.buckets, y.buckets)
}

func sameTables(n1, n2 []uint16, b1, b2 []uint32) bool {
	for i := range n1 {
		if n1[i] != n2[i] {
			return false
		}
	}
	for i := range b1 {
		if b1[i] != b2[i] {
			return false
		}
	}
	return true
}
