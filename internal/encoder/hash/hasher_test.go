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

func BenchmarkStoreRangeForgetful(b *testing.B) {
	// Q7/Q8 configuration: 1 bank, 16-bit slot index, 15-bit buckets.
	h := &hashForgetfulChain{
		bucketBits: 15,
		numBanks:   1,
		bankBits:   16,
	}
	params := &common.EncoderParams{Quality: 7}
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

// TestForgetfulChainStoreRangeParity verifies the unrolled StoreRange
// produces identical table state to sequential Store calls, including the
// 512-bank Q9 configuration where lanes can share a bank.
func TestForgetfulChainStoreRangeParity(t *testing.T) {
	for _, cfg := range []struct {
		name     string
		numBanks uint
		bankBits uint
	}{
		{"Q7-oneBank", 1, 16},
		{"Q9-fiftyOneTwoBanks", 512, 9},
	} {
		cfg := cfg
		t.Run(cfg.name, func(t *testing.T) {
			for _, data := range [][]byte{
				pseudoRandom(1 << 16),
				bytes.Repeat([]byte{0x41}, 1<<10),
				bytes.Repeat([]byte("abcdefghij"), 128),
			} {
				ref := &hashForgetfulChain{bucketBits: 15, numBanks: cfg.numBanks, bankBits: cfg.bankBits}
				got := &hashForgetfulChain{bucketBits: 15, numBanks: cfg.numBanks, bankBits: cfg.bankBits}
				params := &common.EncoderParams{Quality: 7}
				ref.Initialize(params)
				got.Initialize(params)
				ref.Prepare(false, uint(len(data)), data)
				got.Prepare(false, uint(len(data)), data)

				mask := (uint(1) << uint(bits.Len(uint(len(data)-1)))) - 1
				end := uint(len(data) - 64)
				var i uint
				for i = 0; i < end; i++ {
					ref.Store(data, mask, i)
				}
				got.StoreRange(data, mask, 0, end)
				if !sameForgetfulChain(ref, got) {
					t.Fatalf("table state differs after StoreRange (numBanks=%d)", cfg.numBanks)
				}
			}
		})
	}
}

func sameForgetfulChain(a, b *hashForgetfulChain) bool {
	if !bytes.Equal(a.tiny_hash[:], b.tiny_hash[:]) {
		return false
	}
	for i := range a.addr {
		if a.addr[i] != b.addr[i] || a.head[i] != b.head[i] {
			return false
		}
	}
	for i := range a.free_slot_idx {
		if a.free_slot_idx[i] != b.free_slot_idx[i] {
			return false
		}
	}
	for bank := range a.banks {
		for i := range a.banks[bank] {
			if a.banks[bank][i] != b.banks[bank][i] {
				return false
			}
		}
	}
	return true
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
