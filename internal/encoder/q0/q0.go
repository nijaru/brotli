package q0

import (
	"encoding/binary"

	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/hasher"
	"github.com/nijaru/brotli/matchfinder"
)

/* Copyright 2015 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

const (
	maxDistance    = 262128
	minMatchLen    = 5
	firstBlockSize = 3 << 15
	mergeBlockSize = 1 << 16
)

var kCmdHistoSeed = [128]uint32{
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
}

var kCmdCodeDefault = [56]byte{
	0xff, 0x77, 0xd5, 0xbf, 0xe7, 0xde, 0xea, 0xe0, 0xc3, 0x87, 0x1f, 0x83, 0xc1, 0x60, 0x1c, 0x67, 0xb2, 0xaa, 0x06, 0x83, 0xc1, 0x60, 0x30, 0x18, 0xcc, 0xa1, 0xce, 0x88, 0x54, 0x94, 0x46, 0xe1, 0xb0, 0xd0, 0x4e, 0xb2, 0xf7, 0x04,
}

type Encoder struct {
	CmdDepths      [128]byte
	CmdBits        [128]uint16
	CmdCode        [512]byte
	CmdCodeNumbits uint
	table          []int
	storage        []byte

	litDepth [256]byte
	litBits  [256]uint16

	lastBytes     uint16
	lastBytesBits byte
	wroteHeader   bool
}

func (e *Encoder) Reset() {
	e.CmdDepths = [128]byte{
		0, 4, 4, 5, 6, 6, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8, 0, 0, 0, 4, 4, 4, 4, 4, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7, 10, 10, 10, 10, 10, 10, 0, 4, 4, 5, 5, 5, 6, 6, 7, 8, 8, 9, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 6, 6, 6, 6, 6, 6, 5, 5, 5, 5, 5, 5, 4, 4, 4, 4, 4, 4, 4, 5, 5, 5, 5, 5, 5, 6, 6, 7, 7, 7, 8, 10, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12,
	}
	e.CmdBits = [128]uint16{
		0, 0, 8, 9, 3, 35, 7, 71, 39, 103, 23, 47, 175, 111, 239, 31, 0, 0, 0, 4, 12, 2, 10, 6, 13, 29, 11, 43, 27, 59, 87, 55, 15, 79, 319, 831, 191, 703, 447, 959, 0, 14, 1, 25, 5, 21, 19, 51, 119, 159, 95, 223, 479, 991, 63, 575, 127, 639, 383, 895, 255, 767, 511, 1023, 14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 27, 59, 7, 39, 23, 55, 30, 1, 17, 9, 25, 5, 0, 8, 4, 12, 2, 10, 6, 21, 13, 29, 3, 19, 11, 15, 47, 31, 95, 63, 127, 255, 767, 2815, 1791, 3839, 511, 2559, 1535, 3583, 1023, 3071, 2047, 4095,
	}
	copy(e.CmdCode[:], kCmdCodeDefault[:])
	e.CmdCodeNumbits = 448
	e.lastBytes = 0
	e.lastBytesBits = 0
	e.wroteHeader = false
}

func (e *Encoder) getTable(size uint) []int {
	if uint(len(e.table)) < size {
		e.table = make([]int, size)
	}
	clear(e.table[:size])
	return e.table[:size]
}

func (e *Encoder) getStorage(size uint) []byte {
	if uint(len(e.storage)) < size {
		newStorage := make([]byte, size)
		if len(e.storage) > 0 {
			copy(newStorage, e.storage)
		}
		e.storage = newStorage
	}
	return e.storage
}

func (e *Encoder) writeHeader(lgwin uint, storageIx *uint, storage []byte) {
	if lgwin == 16 {
		bitstream.WriteBits(1, 0, storageIx, storage)
	} else if lgwin > 17 {
		bitstream.WriteBits(4, uint64(uint32(lgwin-17)<<1|0x01), storageIx, storage)
	} else {
		bitstream.WriteBits(7, uint64(uint32(lgwin-8)<<4|0x01), storageIx, storage)
	}
}

func (e *Encoder) Encode(dst []byte, src []byte, matches []matchfinder.Match, lastBlock bool) []byte {
	inputSize := uint(len(src))
	if !e.wroteHeader && e.CmdCodeNumbits == 0 {
		e.Reset()
	}

	maxOutSize := uint(len(dst)) + 2*inputSize + 1024
	storage := e.getStorage(maxOutSize)
	if len(dst) > 0 {
		copy(storage, dst)
	}
	var storageIx uint = uint(len(dst)) << 3
	
	if e.lastBytesBits > 0 {
		storage[storageIx>>3] = byte(e.lastBytes)
		if e.lastBytesBits > 8 {
			storage[(storageIx>>3)+1] = byte(e.lastBytes >> 8)
		}
		storageIx += uint(e.lastBytesBits)
	}

	if !e.wroteHeader {
		e.writeHeader(22, &storageIx, storage)
		e.wroteHeader = true
	}

	if inputSize > 0 {
		maxTableSize := uint(1 << 15)
		tableSize := uint(256)
		for tableSize < maxTableSize && tableSize < inputSize {
			tableSize <<= 1
		}
		table := e.getTable(tableSize)

		if (storageIx & 7) != 0 {
			storage[storageIx>>3] &= (1 << (storageIx & 7)) - 1
		} else {
			storage[storageIx>>3] = 0
		}
		clear(storage[(storageIx+7)>>3:])

		e.compressFragmentFast(src, inputSize, lastBlock, table, tableSize, e.CmdDepths[:], e.CmdBits[:], &e.CmdCodeNumbits, e.CmdCode[:], &storageIx, storage)
	} else if lastBlock {
		bitstream.WriteBits(1, 1, &storageIx, storage) // islast
		bitstream.WriteBits(1, 1, &storageIx, storage) // isempty
		storageIx = (storageIx + 7) &^ 7
	}

	if lastBlock {
		e.lastBytes = 0
		e.lastBytesBits = 0
	} else {
		e.lastBytesBits = byte(storageIx & 7)
		if e.lastBytesBits > 0 {
			e.lastBytes = uint16(storage[storageIx>>3])
			if e.lastBytesBits > 8 {
				e.lastBytes |= uint16(storage[(storageIx>>3)+1]) << 8
			}
			e.lastBytes &= (1 << e.lastBytesBits) - 1
		}
	}

	return storage[:(storageIx+7)>>3]
}

func hash5(p []byte, shift uint) uint32 {
	var h uint64 = (binary.LittleEndian.Uint64(p) << 24) * uint64(hasher.KHashMul32)
	return uint32(h >> shift)
}

func hashAtOffset(v uint64, offset int, shift uint) uint32 {
	assert(offset >= 0)
	assert(offset <= 3)
	{
		var h uint64 = ((v >> uint(8*offset)) << 24) * uint64(hasher.KHashMul32)
		return uint32(h >> shift)
	}
}

func isMatch5(p1 []byte, p2 []byte) bool {
	return binary.LittleEndian.Uint32(p1) == binary.LittleEndian.Uint32(p2) &&
		p1[4] == p2[4]
}

func buildAndStoreHuffmanTreeFast(histogram []uint32, histogram_total uint, max_bits uint, depths []byte, bits []uint16, storage_ix *uint, storage []byte) uint {
	var i uint
	if histogram_total == 0 {
		bitstream.WriteBits(1, 0, storage_ix, storage) /* ISLAST */
		bitstream.WriteBits(1, 1, storage_ix, storage) /* ISEMPTY */
		*storage_ix = (*storage_ix + 7) &^ 7
		return 0
	}
	for i = 0; i < 256; i++ {
		if histogram[i] != 0 {
			var adjust uint32 = 1 + 2*min(histogram[i], 11)
			histogram[i] += adjust
			histogram_total += uint(adjust)
		}
	}

	bitstream.BuildAndStoreHuffmanTreeFast(histogram, histogram_total, max_bits, depths, bits, storage_ix, storage)
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

func (e *Encoder) buildAndStoreLiteralPrefixCode(input []byte, input_size uint, depths []byte, bits []uint16, storage_ix *uint, storage []byte) uint {
	var histogram = [256]uint32{0}
	var histogram_total uint
	var i uint
	if input_size < 1<<15 {
		for i = 0; i < input_size; i++ {
			histogram[input[i]]++
		}

		histogram_total = input_size
		for i = 0; i < 256; i++ {
			var adjust uint32 = 2 * min(histogram[i], 11)
			histogram[i] += adjust
			histogram_total += uint(adjust)
		}
	} else {
		const kSampleRate uint = 29
		for i = 0; i < input_size; i += kSampleRate {
			histogram[input[i]]++
		}

		histogram_total = (input_size + kSampleRate - 1) / kSampleRate
		for i = 0; i < 256; i++ {
			var adjust uint32 = 1 + 2*min(histogram[i], 11)
			histogram[i] += adjust
			histogram_total += uint(adjust)
		}
	}

	return buildAndStoreHuffmanTreeFast(histogram[:], histogram_total, 8, depths, bits, storage_ix, storage)
}

func (e *Encoder) buildAndStoreCommandPrefixCode(histogram []uint32, depth []byte, bits []uint16, storage_ix *uint, storage []byte) {
	var tree [129]bitstream.HuffmanTree
	var cmd_depth [common.NumCommandSymbols]byte
	var i int

	bitstream.CreateHuffmanTree(histogram, 64, 15, tree[:], depth)
	bitstream.CreateHuffmanTree(histogram[64:], 64, 14, tree[:], depth[64:])

	copy(cmd_depth[:], depth[:24])
	copy(cmd_depth[24:], depth[40:48])
	copy(cmd_depth[32:], depth[24:32])
	copy(cmd_depth[40:], depth[48:56])
	copy(cmd_depth[48:], depth[32:40])
	copy(cmd_depth[56:], depth[56:64])

	var cmd_bits [64]uint16
	bitstream.ConvertBitDepthsToSymbols(cmd_depth[:], 64, cmd_bits[:])
	copy(bits, cmd_bits[:24])
	copy(bits[24:], cmd_bits[32:40])
	copy(bits[32:], cmd_bits[48:56])
	copy(bits[40:], cmd_bits[24:32])
	copy(bits[48:], cmd_bits[40:48])
	copy(bits[56:], cmd_bits[56:64])
	bitstream.ConvertBitDepthsToSymbols(depth[64:], 64, bits[64:])

	for i = 0; i < 64; i++ {
		cmd_depth[i] = 0
	}
	copy(cmd_depth[:], depth[:8])
	copy(cmd_depth[64:], depth[8:16])
	copy(cmd_depth[128:], depth[16:24])
	copy(cmd_depth[192:], depth[24:32])
	copy(cmd_depth[384:], depth[32:40])
	for i = 0; i < 8; i++ {
		cmd_depth[128+8*i] = depth[40+i]
		cmd_depth[256+8*i] = depth[48+i]
		cmd_depth[448+8*i] = depth[56+i]
	}

	bitstream.StoreHuffmanTree(cmd_depth[:], common.NumCommandSymbols, tree[:], storage_ix, storage)
	bitstream.StoreHuffmanTree(depth[64:], 64, tree[:], storage_ix, storage)
}

func (e *Encoder) buildAndStoreCommandPrefixCodeToBuffer(histogram []uint32, depth []byte, bits []uint16, storage_ix *uint, storage []byte) {
	var tree [129]bitstream.HuffmanTree
	var cmd_depth [common.NumCommandSymbols]byte
	var i int

	bitstream.CreateHuffmanTree(histogram, 64, 15, tree[:], depth)
	bitstream.CreateHuffmanTree(histogram[64:], 64, 14, tree[:], depth[64:])

	copy(cmd_depth[:], depth[:24])
	copy(cmd_depth[24:], depth[40:48])
	copy(cmd_depth[32:], depth[24:32])
	copy(cmd_depth[40:], depth[48:56])
	copy(cmd_depth[48:], depth[32:40])
	copy(cmd_depth[56:], depth[56:64])

	var cmd_bits [64]uint16
	bitstream.ConvertBitDepthsToSymbols(cmd_depth[:], 64, cmd_bits[:])
	copy(bits, cmd_bits[:24])
	copy(bits[24:], cmd_bits[32:40])
	copy(bits[32:], cmd_bits[48:56])
	copy(bits[40:], cmd_bits[24:32])
	copy(bits[48:], cmd_bits[40:48])
	copy(bits[56:], cmd_bits[56:64])
	bitstream.ConvertBitDepthsToSymbols(depth[64:], 64, bits[64:])

	for i = 0; i < 64; i++ {
		cmd_depth[i] = 0
	}
	copy(cmd_depth[:], depth[:8])
	copy(cmd_depth[64:], depth[8:16])
	copy(cmd_depth[128:], depth[16:24])
	copy(cmd_depth[192:], depth[24:32])
	copy(cmd_depth[384:], depth[32:40])
	for i = 0; i < 8; i++ {
		cmd_depth[128+8*i] = depth[40+i]
		cmd_depth[256+8*i] = depth[48+i]
		cmd_depth[448+8*i] = depth[56+i]
	}

	bitstream.StoreHuffmanTree(cmd_depth[:], common.NumCommandSymbols, tree[:], storage_ix, storage)
	bitstream.StoreHuffmanTree(depth[64:], 64, tree[:], storage_ix, storage)
}

func emitInsertLen(insertlen uint, depth []byte, bits []uint16, histo []uint32, storage_ix *uint, storage []byte) {
	if insertlen < 6 {
		var code uint = insertlen + 40
		bitstream.WriteBits(uint(depth[code]), uint64(bits[code]), storage_ix, storage)
		histo[code]++
	} else if insertlen < 130 {
		var tail uint = insertlen - 2
		var nbits uint32 = common.Log2FloorNonZero(tail) - 1
		var prefix uint = tail >> nbits
		var inscode uint = uint(nbits<<1) + prefix + 42
		bitstream.WriteBits(uint(depth[inscode]), uint64(bits[inscode]), storage_ix, storage)
		bitstream.WriteBits(uint(nbits), uint64(tail)-(uint64(prefix)<<nbits), storage_ix, storage)
		histo[inscode]++
	} else if insertlen < 2114 {
		var tail uint = insertlen - 66
		var nbits uint32 = common.Log2FloorNonZero(tail)
		var code uint = uint(nbits + 50)
		bitstream.WriteBits(uint(depth[code]), uint64(bits[code]), storage_ix, storage)
		bitstream.WriteBits(uint(nbits), uint64(tail)-(1<<nbits), storage_ix, storage)
		histo[code]++
	} else {
		bitstream.WriteBits(uint(depth[61]), uint64(bits[61]), storage_ix, storage)
		bitstream.WriteBits(12, uint64(insertlen)-2114, storage_ix, storage)
		histo[61]++
	}
}

func (e *Encoder) emitLongInsertLen(insertlen uint, histo []uint32, storage_ix *uint, storage []byte) {
	if insertlen < 22594 {
		bitstream.WriteBits(uint(e.CmdDepths[62]), uint64(e.CmdBits[62]), storage_ix, storage)
		bitstream.WriteBits(14, uint64(insertlen)-6210, storage_ix, storage)
		histo[62]++
	} else {
		bitstream.WriteBits(uint(e.CmdDepths[63]), uint64(e.CmdBits[63]), storage_ix, storage)
		bitstream.WriteBits(24, uint64(insertlen)-22594, storage_ix, storage)
		histo[63]++
	}
}

func (e *Encoder) emitCopyLen(copylen uint, histo []uint32, storage_ix *uint, storage []byte) {
	if copylen < 10 {
		var code uint = copylen + 14
		bitstream.WriteBits(uint(e.CmdDepths[code]), uint64(e.CmdBits[code]), storage_ix, storage)
		histo[code]++
	} else if copylen < 134 {
		var tail uint = copylen - 6
		var nbits uint32 = common.Log2FloorNonZero(tail) - 1
		var prefix uint = tail >> nbits
		var code uint = uint(nbits<<1) + prefix + 20
		bitstream.WriteBits(uint(e.CmdDepths[code]), uint64(e.CmdBits[code]), storage_ix, storage)
		bitstream.WriteBits(uint(nbits), uint64(tail)-(uint64(prefix)<<nbits), storage_ix, storage)
		histo[code]++
	} else if copylen < 2118 {
		var tail uint = copylen - 70
		var nbits uint32 = common.Log2FloorNonZero(tail)
		var code uint = uint(nbits + 28)
		bitstream.WriteBits(uint(e.CmdDepths[code]), uint64(e.CmdBits[code]), storage_ix, storage)
		bitstream.WriteBits(uint(nbits), uint64(tail)-(1<<nbits), storage_ix, storage)
		histo[code]++
	} else {
		bitstream.WriteBits(uint(e.CmdDepths[39]), uint64(e.CmdBits[39]), storage_ix, storage)
		bitstream.WriteBits(24, uint64(copylen)-2118, storage_ix, storage)
		histo[39]++
	}
}

func (e *Encoder) emitCopyLenLastDistance(copylen uint, histo []uint32, storage_ix *uint, storage []byte) {
	if copylen < 12 {
		var code uint = copylen - 4
		bitstream.WriteBits(uint(e.CmdDepths[code]), uint64(e.CmdBits[code]), storage_ix, storage)
		histo[code]++
	} else if copylen < 72 {
		var tail uint = copylen - 8
		var nbits uint32 = common.Log2FloorNonZero(tail) - 1
		var prefix uint = tail >> nbits
		var code uint = uint(nbits<<1) + prefix + 4
		bitstream.WriteBits(uint(e.CmdDepths[code]), uint64(e.CmdBits[code]), storage_ix, storage)
		bitstream.WriteBits(uint(nbits), uint64(tail)-(uint64(prefix)<<nbits), storage_ix, storage)
		histo[code]++
	} else if copylen < 136 {
		var tail uint = copylen - 8
		var code uint = uint((tail >> 5) + 30)
		bitstream.WriteBits(uint(e.CmdDepths[code]), uint64(e.CmdBits[code]), storage_ix, storage)
		bitstream.WriteBits(5, uint64(tail)&31, storage_ix, storage)
		bitstream.WriteBits(uint(e.CmdDepths[64]), uint64(e.CmdBits[64]), storage_ix, storage)
		histo[code]++
		histo[64]++
	} else if copylen < 2120 {
		var tail uint = copylen - 72
		var nbits uint32 = common.Log2FloorNonZero(tail)
		var code uint = uint(nbits + 28)
		bitstream.WriteBits(uint(e.CmdDepths[code]), uint64(e.CmdBits[code]), storage_ix, storage)
		bitstream.WriteBits(uint(nbits), uint64(tail)-(1<<nbits), storage_ix, storage)
		bitstream.WriteBits(uint(e.CmdDepths[64]), uint64(e.CmdBits[64]), storage_ix, storage)
		histo[code]++
		histo[64]++
	} else {
		bitstream.WriteBits(uint(e.CmdDepths[39]), uint64(e.CmdBits[39]), storage_ix, storage)
		bitstream.WriteBits(24, uint64(copylen)-2120, storage_ix, storage)
		bitstream.WriteBits(uint(e.CmdDepths[64]), uint64(e.CmdBits[64]), storage_ix, storage)
		histo[39]++
		histo[64]++
	}
}

func (e *Encoder) emitDistance(distance uint, histo []uint32, storage_ix *uint, storage []byte) {
	var d uint = distance + 3
	var nbits uint32 = common.Log2FloorNonZero(d) - 1
	var prefix uint = (d >> nbits) & 1
	var offset uint = (2 + prefix) << nbits
	var distcode uint = uint(2*(nbits-1) + uint32(prefix) + 80)
	bitstream.WriteBits(uint(e.CmdDepths[distcode]), uint64(e.CmdBits[distcode]), storage_ix, storage)
	bitstream.WriteBits(uint(nbits), uint64(d-offset), storage_ix, storage)
	histo[distcode]++
}

func emitLiterals(input []byte, len uint, depth []byte, bits []uint16, storage_ix *uint, storage []byte) {
	var j uint
	for j = 0; j < len; j++ {
		var lit byte = input[j]
		bitstream.WriteBits(uint(depth[lit]), uint64(bits[lit]), storage_ix, storage)
	}
}

func storeMetaBlockHeader1(len uint, is_uncompressed bool, storage_ix *uint, storage []byte) {
	bitstream.WriteBits(1, 0, storage_ix, storage) /* ISLAST */
	var lenbits uint64
	var nlenbits uint
	var nibblesbits uint64
	encodeMlen1(len, &lenbits, &nlenbits, &nibblesbits)
	bitstream.WriteBits(2, nibblesbits, storage_ix, storage)
	bitstream.WriteBits(nlenbits, lenbits, storage_ix, storage)
	if is_uncompressed {
		bitstream.WriteBits(1, 1, storage_ix, storage) /* ISUNCOMPRESSED */
	} else {
		bitstream.WriteBits(1, 0, storage_ix, storage) /* ISUNCOMPRESSED */
	}
}

func encodeMlen1(length uint, bits *uint64, nbits *uint, nibblesbits *uint64) {
	assert(length > 0)
	assert(length <= 1<<24)
	length--
	if length < 1<<16 {
		*nibblesbits = 0
		*nbits = 16
	} else if length < 1<<20 {
		*nibblesbits = 1
		*nbits = 20
	} else {
		*nibblesbits = 2
		*nbits = 24
	}
	*bits = uint64(length)
}

func updateBits(n_bits uint, bits uint32, pos uint, array []byte) {
	for n_bits > 0 {
		var byte_pos uint = pos >> 3
		var n_unchanged_bits uint = pos & 7
		var n_changed_bits uint = min(n_bits, 8-n_unchanged_bits)
		var total_bits uint = n_unchanged_bits + n_changed_bits
		var mask uint32 = (^((1 << total_bits) - 1)) | ((1 << n_unchanged_bits) - 1)
		var unchanged_bits uint32 = uint32(array[byte_pos]) & mask
		var changed_bits uint32 = bits & ((1 << n_changed_bits) - 1)
		array[byte_pos] = byte(changed_bits<<n_unchanged_bits | unchanged_bits)
		n_bits -= n_changed_bits
		bits >>= n_changed_bits
		pos += n_changed_bits
	}
}

func (e *Encoder) shouldMergeBlock(data []byte, len uint, depths []byte) bool {
	var histo = [256]uint{0}
	const kSampleRate uint = 43
	var i uint
	for i = 0; i < len; i += kSampleRate {
		histo[data[i]]++
	}
	{
		var total uint = (len + kSampleRate - 1) / kSampleRate
		var r float64 = (common.FastLog2(total) + 0.5) * float64(total) + 200
		for i = 0; i < 256; i++ {
			r -= float64(histo[i]) * (float64(depths[i]) + common.FastLog2(histo[i]))
		}

		return r >= 0.0
	}
}

func emitUncompressedMetaBlock1(begin, end []byte, mlen_storage_ix uint, storage_ix *uint, storage []byte) {
	var length uint = uint(len(begin) - len(end))
	*storage_ix = mlen_storage_ix - 3
	storeMetaBlockHeader1(length, true, storage_ix, storage)
	*storage_ix = (*storage_ix + 7) &^ 7
	copy(storage[*storage_ix>>3:], begin[:length])
	*storage_ix += length << 3
	storage[*storage_ix>>3] = 0
}

func (e *Encoder) compressFragmentFastImpl(in []byte, input_size uint, is_last bool, table []int, table_bits uint, cmd_depth []byte, cmd_bits []uint16, cmd_code_numbits *uint, cmd_code []byte, storage_ix *uint, storage []byte) {
	var initial_storage_ix uint = *storage_ix
	var shift uint = 64 - table_bits
	var block_size uint
	var total_block_size uint
	var mlen_storage_ix uint
	var literal_ratio uint
	var cmd_histo [128]uint32
	var ip_end int
	var next_emit int
	var last_distance int
	var input int = 0
	var last_processed_ip int = 0

	var lit_depth [256]byte
	var lit_bits [256]uint16

next_block:
	if input_size > 0 {
		block_size = min(input_size, firstBlockSize)
		total_block_size = block_size
		mlen_storage_ix = *storage_ix + 3
		storeMetaBlockHeader1(block_size, false, storage_ix, storage)
		bitstream.WriteBits(13, 0, storage_ix, storage)
		literal_ratio = e.buildAndStoreLiteralPrefixCode(in[input:], block_size, lit_depth[:], lit_bits[:], storage_ix, storage)
		{
			var i uint
			for i = 0; i+7 < *cmd_code_numbits; i += 8 {
				bitstream.WriteBits(8, uint64(cmd_code[i>>3]), storage_ix, storage)
			}
			bitstream.WriteBits(*cmd_code_numbits&7, uint64(cmd_code[*cmd_code_numbits>>3]), storage_ix, storage)
		}

	emit_commands:
		copy(cmd_histo[:], kCmdHistoSeed[:])
		ip_end = input + int(block_size)
		last_distance = -1
		next_emit = input
		if block_size >= 16 {
			var ip_limit int = ip_end - 16
			var next_hash uint32
			var ip int = input
			ip++
			for next_hash = hash5(in[ip:], shift); ; {
				var skip uint32 = 32
				var next_ip int = ip
				var candidate int
			trawl:
				for {
					var hash_val uint32 = next_hash
					var bytes_between uint32 = skip >> 5
					skip++
					ip = next_ip
					next_ip = ip + int(bytes_between)
					if next_ip > ip_limit {
						goto emit_remainder
					}
					next_hash = hash5(in[next_ip:], shift)
					candidate = ip - last_distance
					if isMatch5(in[ip:], in[candidate:]) {
						if candidate < ip {
							table[hash_val] = int(ip - last_processed_ip)
							break
						}
					}
					candidate = last_processed_ip + table[hash_val]
					table[hash_val] = int(ip - last_processed_ip)
					if isMatch5(in[ip:], in[candidate:]) {
						break
					}
				}

				if ip-candidate > maxDistance {
					goto trawl
				}

				{
					var base int = ip
					var matched uint = 5 + common.FindMatchLengthWithLimit(in[candidate+5:], in[ip+5:], uint(ip_end-ip)-5)
					var distance int = base - candidate
					var insertlen uint = uint(base - next_emit)
					ip += int(matched)
					emitInsertLen(insertlen, lit_depth[:], lit_bits[:], cmd_histo[:], storage_ix, storage)
					emitLiterals(in[next_emit:], insertlen, lit_depth[:], lit_bits[:], storage_ix, storage)
					if distance == last_distance {
						bitstream.WriteBits(uint(cmd_depth[64]), uint64(cmd_bits[64]), storage_ix, storage)
						cmd_histo[64]++
					} else {
						e.emitDistance(uint(distance), cmd_histo[:], storage_ix, storage)
						last_distance = distance
					}
					e.emitCopyLenLastDistance(matched, cmd_histo[:], storage_ix, storage)

					next_emit = ip
					if ip >= ip_limit {
						goto emit_remainder
					}
					{
						var input_bytes uint64 = binary.LittleEndian.Uint64(in[ip-3:])
						var h0 uint32 = hashAtOffset(input_bytes, 0, shift)
						table[h0] = int(ip - last_processed_ip - 3)
						var h1 uint32 = hashAtOffset(input_bytes, 1, shift)
						table[h1] = int(ip - last_processed_ip - 2)
						var h2 uint32 = hashAtOffset(input_bytes, 2, shift)
						table[h2] = int(ip - last_processed_ip - 1)

						var cur_hash uint32 = hashAtOffset(input_bytes, 3, shift)
						candidate = last_processed_ip + table[cur_hash]
						table[cur_hash] = int(ip - last_processed_ip)
					}
				}

				for isMatch5(in[ip:], in[candidate:]) {
					var base int = ip
					var matched uint = 5 + common.FindMatchLengthWithLimit(in[candidate+5:], in[ip+5:], uint(ip_end-ip)-5)
					if ip-candidate > maxDistance {
						break
					}
					ip += int(matched)
					last_distance = int(base - candidate)
					e.emitCopyLen(matched, cmd_histo[:], storage_ix, storage)
					e.emitDistance(uint(last_distance), cmd_histo[:], storage_ix, storage)

					next_emit = ip
					if ip >= ip_limit {
						goto emit_remainder
					}
					{
						var input_bytes uint64 = binary.LittleEndian.Uint64(in[ip-3:])
						var h0 uint32 = hashAtOffset(input_bytes, 0, shift)
						table[h0] = int(ip - last_processed_ip - 3)
						var h1 uint32 = hashAtOffset(input_bytes, 1, shift)
						table[h1] = int(ip - last_processed_ip - 2)
						var h2 uint32 = hashAtOffset(input_bytes, 2, shift)
						table[h2] = int(ip - last_processed_ip - 1)

						var cur_hash uint32 = hashAtOffset(input_bytes, 3, shift)
						candidate = last_processed_ip + table[cur_hash]
						table[cur_hash] = int(ip - last_processed_ip)
					}
				}
				ip++
				next_hash = hash5(in[ip:], shift)
			}
		}

	emit_remainder:
		input = ip_end
		input_size -= block_size
		block_size = min(input_size, mergeBlockSize)

		if input_size > 0 && total_block_size+block_size <= 1<<20 && e.shouldMergeBlock(in[input:], block_size, lit_depth[:]) {
			total_block_size += block_size
			updateBits(20, uint32(total_block_size-1), mlen_storage_ix, storage)
			goto emit_commands
		}

		if next_emit < ip_end {
			var insertlen uint = uint(ip_end - next_emit)
			emitInsertLen(insertlen, lit_depth[:], lit_bits[:], cmd_histo[:], storage_ix, storage)
			emitLiterals(in[next_emit:], insertlen, lit_depth[:], lit_bits[:], storage_ix, storage)
		}
		next_emit = ip_end

		if *storage_ix-initial_storage_ix > 31+(total_block_size<<3) {
			emitUncompressedMetaBlock1(in[input-int(total_block_size):], in[input:], mlen_storage_ix, storage_ix, storage)
		}

		if input_size > 0 {
			// Update for next block
			e.buildAndStoreCommandPrefixCode(cmd_histo[:], cmd_depth, cmd_bits, storage_ix, storage)
			goto next_block
		}
	}

	if !is_last {
		// Update for next Encode call
		cmd_code[0] = 0
		*cmd_code_numbits = 0
		var startBits uint = 0
		// Build and store into cmd_code buffer
		e.buildAndStoreCommandPrefixCodeToBuffer(cmd_histo[:], cmd_depth, cmd_bits, &startBits, cmd_code)
		*cmd_code_numbits = startBits
	}

	_ = last_processed_ip
	_ = initial_storage_ix
	_ = literal_ratio
}

func (e *Encoder) compressFragmentFast(input []byte, input_size uint, is_last bool, table []int, table_size uint, cmd_depth []byte, cmd_bits []uint16, cmd_code_numbits *uint, cmd_code []byte, storage_ix *uint, storage []byte) {
	if input_size == 0 {
		assert(is_last)
		bitstream.WriteBits(1, 1, storage_ix, storage)
		bitstream.WriteBits(1, 1, storage_ix, storage)
		*storage_ix = (*storage_ix + 7) &^ 7
		return
	}

	e.compressFragmentFastImpl(input, input_size, is_last, table, uint(common.Log2FloorNonZero(table_size)), cmd_depth, cmd_bits, cmd_code_numbits, cmd_code, storage_ix, storage)

	if is_last {
		bitstream.WriteBits(1, 1, storage_ix, storage) /* islast */
		bitstream.WriteBits(1, 1, storage_ix, storage) /* isempty */
		*storage_ix = (*storage_ix + 7) &^ 7
	}
}
