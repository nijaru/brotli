package hash

import (
	"encoding/binary"

	"github.com/nijaru/brotli/internal/common"
)

/* Copyright 2016 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

func (*hashForgetfulChain) HashTypeLength() uint {
	return 4
}

func (*hashForgetfulChain) StoreLookahead() uint {
	return 4
}

/* HashBytes is the function that chooses the bucket to place the address in.*/
func (h *hashForgetfulChain) HashBytes(data []byte) uint {
	var hash uint32 = binary.LittleEndian.Uint32(data) * KHashMul32

	/* The higher bits contain more mixture from the multiplication,
	   so we take our results from there. */
	return uint(hash >> (32 - h.bucketBits))
}

type slot struct {
	delta uint16
	next  uint16
}

/*
A (forgetful) hash table to the data seen by the compressor, to

	help create backward references to previous data.

	Hashes are stored in chains which are bucketed to groups. Group of chains
	share a storage "bank". When more than "bank size" chain nodes are added,
	oldest nodes are replaced; this way several chains may share a tail.
*/
type hashForgetfulChain struct {
	hasherCommon

	bucketBits              uint
	numBanks                uint
	bankBits                uint
	numLastDistancesToCheck int

	addr          []uint32
	head          []uint16
	tiny_hash     [65536]byte
	banks         [][]slot
	free_slot_idx []uint16
	max_hops      uint
}

func (h *hashForgetfulChain) Initialize(params *common.EncoderParams) {
	var q uint
	if params.Quality > 6 {
		q = 7
	} else {
		q = 8
	}
	h.max_hops = q << uint(params.Quality-4)

	bankSize := 1 << h.bankBits
	bucketSize := 1 << h.bucketBits

	h.addr = make([]uint32, bankSize) // Wait, bucketSize? Original was bucketSize
	h.addr = make([]uint32, bucketSize)
	h.head = make([]uint16, bucketSize)
	h.banks = make([][]slot, h.numBanks)
	for i := range h.banks {
		h.banks[i] = make([]slot, bankSize)
	}
	h.free_slot_idx = make([]uint16, h.numBanks)
}

func (h *hashForgetfulChain) Prepare(one_shot bool, input_size uint, data []byte) {
	var partial_prepare_threshold uint = (1 << h.bucketBits) >> 6
	/* Partial preparation is 100 times slower (per socket). */
	if one_shot && input_size <= partial_prepare_threshold {
		var i uint
		for i = 0; i < input_size; i++ {
			var bucket uint = h.HashBytes(data[i:])

			/* See InitEmpty comment. */
			h.addr[bucket] = 0xCCCCCCCC

			h.head[bucket] = 0xCCCC
		}
	} else {
		/* Fill |addr| array with 0xCCCCCCCC value. Because of wrapping, position
		   processed by hasher never reaches 3GB + 64M; this makes all new chains
		   to be terminated after the first node. */
		for i := range h.addr {
			h.addr[i] = 0xCCCCCCCC
		}

		clear(h.head)
	}

	h.tiny_hash = [65536]byte{}
	clear(h.free_slot_idx)
}

/*
Look at 4 bytes at &data[ix & mask]. Compute a hash from these, and prepend

	node to corresponding chain; also update tiny_hash for current position.
*/
func (h *hashForgetfulChain) Store(data []byte, mask uint, ix uint) {
	var key uint = h.HashBytes(data[ix&mask:])
	var bank uint = key & (h.numBanks - 1)
	idx := uint(h.free_slot_idx[bank]) & ((1 << h.bankBits) - 1)
	h.free_slot_idx[bank]++
	var delta uint = ix - uint(h.addr[key])
	h.tiny_hash[uint16(ix)] = byte(key)
	if delta > 0xFFFF {
		delta = 0xFFFF
	}
	h.banks[bank][idx].delta = uint16(delta)
	h.banks[bank][idx].next = h.head[key]
	h.addr[key] = uint32(ix)
	h.head[key] = uint16(idx)
}

func (h *hashForgetfulChain) StoreRange(data []byte, mask uint, ix_start uint, ix_end uint) {
	var i uint = ix_start
	bankSel := h.numBanks - 1
	slotMask := (uint(1) << h.bankBits) - 1
	shift := uint(32 - h.bucketBits)
	freeSlotIdx := h.free_slot_idx
	addr := h.addr
	head := h.head
	banks := h.banks

	for i+4 <= ix_end {
		k0 := uint(uint32(binary.LittleEndian.Uint32(data[(i+0)&mask:])*KHashMul32) >> shift)
		k1 := uint(uint32(binary.LittleEndian.Uint32(data[(i+1)&mask:])*KHashMul32) >> shift)
		k2 := uint(uint32(binary.LittleEndian.Uint32(data[(i+2)&mask:])*KHashMul32) >> shift)
		k3 := uint(uint32(binary.LittleEndian.Uint32(data[(i+3)&mask:])*KHashMul32) >> shift)

		b0 := k0 & bankSel
		b1 := k1 & bankSel
		b2 := k2 & bankSel
		b3 := k3 & bankSel

		// Slot indices: each lane takes the next slot in its bank after the
		// earlier lanes that share that bank.
		idx0 := uint(freeSlotIdx[b0]) & slotMask
		idx1 := (uint(freeSlotIdx[b1]) + eqU(b1, b0)) & slotMask
		idx2 := (uint(freeSlotIdx[b2]) + eqU(b2, b0) + eqU(b2, b1)) & slotMask
		idx3 := (uint(freeSlotIdx[b3]) + eqU(b3, b0) + eqU(b3, b1) + eqU(b3, b2)) & slotMask

		// Lanes are written in order so same-key chains (addr, head) see the
		// previous lane's update, matching sequential Store semantics.
		{
			delta := i - uint(addr[k0])
			if delta > 0xFFFF {
				delta = 0xFFFF
			}
			h.tiny_hash[uint16(i)] = byte(k0)
			banks[b0][idx0].delta = uint16(delta)
			banks[b0][idx0].next = head[k0]
			addr[k0] = uint32(i)
			head[k0] = uint16(idx0)
			freeSlotIdx[b0]++
		}
		{
			j := i + 1
			delta := j - uint(addr[k1])
			if delta > 0xFFFF {
				delta = 0xFFFF
			}
			h.tiny_hash[uint16(j)] = byte(k1)
			banks[b1][idx1].delta = uint16(delta)
			banks[b1][idx1].next = head[k1]
			addr[k1] = uint32(j)
			head[k1] = uint16(idx1)
			freeSlotIdx[b1]++
		}
		{
			j := i + 2
			delta := j - uint(addr[k2])
			if delta > 0xFFFF {
				delta = 0xFFFF
			}
			h.tiny_hash[uint16(j)] = byte(k2)
			banks[b2][idx2].delta = uint16(delta)
			banks[b2][idx2].next = head[k2]
			addr[k2] = uint32(j)
			head[k2] = uint16(idx2)
			freeSlotIdx[b2]++
		}
		{
			j := i + 3
			delta := j - uint(addr[k3])
			if delta > 0xFFFF {
				delta = 0xFFFF
			}
			h.tiny_hash[uint16(j)] = byte(k3)
			banks[b3][idx3].delta = uint16(delta)
			banks[b3][idx3].next = head[k3]
			addr[k3] = uint32(j)
			head[k3] = uint16(idx3)
			freeSlotIdx[b3]++
		}

		i += 4
	}

	for ; i < ix_end; i++ {
		h.Store(data, mask, i)
	}
}

// eqU is a branchless 0/1 equality test for bank indices.
func eqU(a, b uint) uint {
	if a == b {
		return 1
	}
	return 0
}

func (h *hashForgetfulChain) StitchToPreviousBlock(
	num_bytes uint,
	position uint,
	ringbuffer []byte,
	ring_buffer_mask uint,
) {
	if num_bytes >= h.HashTypeLength()-1 && position >= 3 {
		/* Prepare the hashes for three last bytes of the last write.
		   These could not be calculated before, since they require knowledge
		   of both the previous and the current block. */
		h.Store(ringbuffer, ring_buffer_mask, position-3)
		h.Store(ringbuffer, ring_buffer_mask, position-2)
		h.Store(ringbuffer, ring_buffer_mask, position-1)
	}
}

func (h *hashForgetfulChain) PrepareDistanceCache(distance_cache []int) {
	prepareDistanceCache(distance_cache, h.numLastDistancesToCheck)
}

/*
Find a longest backward match of &data[cur_ix] up to the length of

	max_length and stores the position cur_ix in the hash table.

	REQUIRES: PrepareDistanceCachehashForgetfulChain must be invoked for current distance cache
	          values; if this method is invoked repeatedly with the same distance
	          cache values, it is enough to invoke PrepareDistanceCachehashForgetfulChain once.

	Does not look for matches longer than max_length.
	Does not look for matches further away than max_backward.
	Writes the best match into |out|.
	|out|->score is updated only if a better match is found.
*/
func (h *hashForgetfulChain) FindLongestMatch(
	dictionary *common.EncoderDictionary,
	data []byte,
	ring_buffer_mask uint,
	distance_cache []int,
	cur_ix uint,
	max_length uint,
	max_backward uint,
	gap uint,
	max_distance uint,
	out *SearchResult,
) {
	var cur_ix_masked uint = cur_ix & ring_buffer_mask
	var min_score uint = out.Score
	var best_score uint = out.Score
	var best_len uint = out.Len
	var key uint = h.HashBytes(data[cur_ix_masked:])
	var tiny_hash byte = byte(key)
	/* Don't accept a short copy from far away. */
	out.Len = 0

	out.Len_code_delta = 0

	/* Try last distance first. */
	for i := 0; i < h.numLastDistancesToCheck; i++ {
		var backward uint = uint(distance_cache[i])
		var prev_ix uint = (cur_ix - backward)

		/* For distance code 0 we want to consider 2-byte matches. */
		if i > 0 && h.tiny_hash[uint16(prev_ix)] != tiny_hash {
			continue
		}
		if prev_ix >= cur_ix || backward > max_backward {
			continue
		}

		prev_ix &= ring_buffer_mask
		{
			var len uint = common.FindMatchLengthWithLimit(data[prev_ix:], data[cur_ix_masked:], max_length)
			if len >= 2 {
				var score uint = backwardReferenceScoreUsingLastDistance(uint(len))
				if best_score < score {
					if i != 0 {
						score -= backwardReferencePenaltyUsingLastDistance(uint(i))
					}
					if best_score < score {
						best_score = score
						best_len = uint(len)
						out.Len = best_len
						out.Distance = backward
						out.Score = best_score
					}
				}
			}
		}
	}
	{
		var bank uint = key & (h.numBanks - 1)
		var backward uint = 0
		var hops uint = h.max_hops
		var delta uint = cur_ix - uint(h.addr[key])
		var slot uint = uint(h.head[key])
		for {
			tmp6 := hops
			hops--
			if tmp6 == 0 {
				break
			}
			var prev_ix uint
			var last uint = slot
			backward += delta
			if backward > max_backward {
				break
			}
			prev_ix = (cur_ix - backward) & ring_buffer_mask
			slot = uint(h.banks[bank][last].next)
			delta = uint(h.banks[bank][last].delta)
			if cur_ix_masked+best_len > ring_buffer_mask || prev_ix+best_len > ring_buffer_mask ||
				data[cur_ix_masked+best_len] != data[prev_ix+best_len] {
				continue
			}
			{
				var len uint = common.FindMatchLengthWithLimit(data[prev_ix:], data[cur_ix_masked:], max_length)
				if len >= 4 {
					/* Comparing for >= 3 does not change the semantics, but just saves
					   for a few unnecessary binary logarithms in backward reference
					   score, since we are not interested in such short matches. */
					var score uint = backwardReferenceScore(uint(len), backward)
					if best_score < score {
						best_score = score
						best_len = uint(len)
						out.Len = best_len
						out.Distance = backward
						out.Score = best_score
					}
				}
			}
		}

		h.Store(data, ring_buffer_mask, cur_ix)
	}

	if out.Score == min_score {
		searchInStaticDictionary(
			dictionary,
			h,
			data[cur_ix_masked:],
			max_length,
			max_backward+gap,
			max_distance,
			out,
			false,
		)
	}
}
