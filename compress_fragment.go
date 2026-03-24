package brotli

import (
	"encoding/binary"
	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/hasher"
	"github.com/nijaru/brotli/internal/metablock"
)

/* Copyright 2015 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Function for fast encoding of an input fragment, independently from the input
   history. This function uses one-pass processing: when we find a backward
   match, we immediately emit the corresponding command and literal codes to
   the bit stream.

   Adapted from the CompressFragment() function in
   https://github.com/google/snappy/blob/master/snappy.cc */

const maxDistance_compress_fragment = 262128

func hash_compress_fragment(data []byte) uint32 {
	var h uint32 = binary.LittleEndian.Uint32(data) * hasher.KHashMul32
	return h >> (32 - 14)
}

func isMatch_compress_fragment(data []byte, p1 int, p2 int) bool {
	return binary.LittleEndian.Uint32(data[p1:]) == binary.LittleEndian.Uint32(data[p2:])
}

func BuildAndStoreHuffmanTreeFast(histogram []uint32, histogram_total uint, max_bits uint, depths []byte, bits []uint16, storage_ix *uint, storage []byte) uint {
	var i uint
	if histogram_total == 0 {
		bitstream.WriteBits(1, 0, storage_ix, storage) /* ISLAST */
		bitstream.WriteBits(1, 1, storage_ix, storage) /* ISEMPTY */
		*storage_ix = (*storage_ix + 7) &^ 7
		return 0
	} else {
		for i = 0; i < 256; i++ {
			if histogram[i] != 0 {
				/* We add 1 to each population count to avoid 0 bit depths (since this is
				   only a sample and we don't know if the symbol appears or not), and we
				   weigh the first 11 samples with weight 3 to account for the balancing
				   effect of the LZ77 phase on the histogram (more frequent symbols are
				   more likely to be in backward references instead as literals). */
				var adjust uint32 = 1 + 2*common.BrotliMinUint32T(histogram[i], 11)
				histogram[i] += adjust
				histogram_total += uint(adjust)
			}
		}

		var tree = make([]bitstream.HuffmanTree, 512)
		bitstream.BuildAndStoreHuffmanTree(histogram[:], 256, 256, tree, depths, bits, storage_ix, storage)
		{
			var literal_ratio uint = 0
			for i = 0; i < 256; i++ {
				if histogram[i] != 0 {
					literal_ratio += uint(histogram[i] * uint32(depths[i]))
				}
			}

			/* Estimated encoding ratio, millibytes per symbol. */
			return (literal_ratio * 125) / histogram_total
		}
	}
}

func buildAndStoreCommandPrefixCode(histogram []uint32, depth []byte, bits []uint16, storage_ix *uint, storage []byte) {
	var tree [129]bitstream.HuffmanTree
	bitstream.CreateHuffmanTree(histogram, 64, 15, tree[:], depth)
	bitstream.CreateHuffmanTree(histogram[64:], 64, 14, tree[:], depth[64:])
	bitstream.ConvertBitDepthsToSymbols(depth, 128, bits)
	bitstream.StoreHuffmanTree(depth, 128, tree[:], storage_ix, storage)
}

func emitLiteral_compress_fragment(data []byte, length int, storage_ix *uint, storage []byte, depths []byte, bits []uint16) {
	for i := 0; i < length; i++ {
		bitstream.WriteBits(uint(depths[data[i]]), uint64(bits[data[i]]), storage_ix, storage)
	}
}

func emitCopy_compress_fragment(length int, distance int, storage_ix *uint, storage []byte, depths []byte, bits []uint16) {
	var length_code uint32 = uint32(metablock.GetCopyLengthCode(uint(length)))
	var distCode metablock.DistanceCode = metablock.GetDistanceCode(distance)
	bitstream.WriteBits(uint(depths[256+length_code]), uint64(bits[256+length_code]), storage_ix, storage)
	bitstream.WriteBits(uint(depths[256+64+uint32(distCode.Code)]), uint64(bits[256+64+uint32(distCode.Code)]), storage_ix, storage)
}

func emitDistance_compress_fragment(distance int, storage_ix *uint, storage []byte, depths []byte, bits []uint16) {
	var distCode metablock.DistanceCode = metablock.GetDistanceCode(distance)
	bitstream.WriteBits(uint(depths[256+64+uint32(distCode.Code)]), uint64(bits[256+64+uint32(distCode.Code)]), storage_ix, storage)
}
