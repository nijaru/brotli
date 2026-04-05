package brotli

import (
	"encoding/binary"

	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/common"
)

const (
	maxDistanceCompressFragment = 262128
	hashMul32                   = 0x1e35a7bd
)

func q0BuildAndStoreLiteralPrefixCode(input []byte, inputSize uint, depths []byte, bits []uint16, storageIx *uint, storage []byte) uint {
	var histogram [256]uint32
	var histogramTotal uint
	if inputSize < 1<<15 {
		for i := uint(0); i < inputSize; i++ {
			histogram[input[i]]++
		}

		histogramTotal = inputSize
		for i := 0; i < 256; i++ {
			adjust := 2 * min(histogram[i], 11)
			histogram[i] += adjust
			histogramTotal += uint(adjust)
		}
	} else {
		const sampleRate uint = 29
		for i := uint(0); i < inputSize; i += sampleRate {
			histogram[input[i]]++
		}

		histogramTotal = (inputSize + sampleRate - 1) / sampleRate
		for i := 0; i < 256; i++ {
			adjust := 1 + 2*min(histogram[i], 11)
			histogram[i] += adjust
			histogramTotal += uint(adjust)
		}
	}

	bitstream.BuildAndStoreHuffmanTreeFast(histogram[:], histogramTotal, 8, depths, bits, storageIx, storage)

	var literalRatio uint
	for i, count := range histogram {
		if count != 0 {
			literalRatio += uint(count * uint32(depths[i]))
		}
	}

	return (literalRatio * 125) / histogramTotal
}

func q0BuildAndStoreCommandPrefixCode1(histogram []uint32, depths []byte, bits []uint16, storageIx *uint, storage []byte) {
	var tree [129]bitstream.HuffmanTree
	var cmdDepth [common.NumCommandSymbols]byte

	bitstream.CreateHuffmanTree(histogram, 64, 15, tree[:], depths)
	bitstream.CreateHuffmanTree(histogram[64:], 64, 14, tree[:], depths[64:])

	var cmdBits [64]uint16
	copy(cmdDepth[:], depths[:24])
	copy(cmdDepth[24:], depths[40:48])
	copy(cmdDepth[32:], depths[24:32])
	copy(cmdDepth[40:], depths[48:56])
	copy(cmdDepth[48:], depths[32:40])
	copy(cmdDepth[56:], depths[56:64])
	bitstream.ConvertBitDepthsToSymbols(cmdDepth[:], 64, cmdBits[:])
	copy(bits, cmdBits[:24])
	copy(bits[24:], cmdBits[32:40])
	copy(bits[32:], cmdBits[48:56])
	copy(bits[40:], cmdBits[24:32])
	copy(bits[48:], cmdBits[40:48])
	copy(bits[56:], cmdBits[56:64])
	bitstream.ConvertBitDepthsToSymbols(depths[64:], 64, bits[64:])

	for i := 0; i < 64; i++ {
		cmdDepth[i] = 0
	}
	copy(cmdDepth[:], depths[:8])
	copy(cmdDepth[64:], depths[8:16])
	copy(cmdDepth[128:], depths[16:24])
	copy(cmdDepth[192:], depths[24:32])
	copy(cmdDepth[384:], depths[32:40])
	for i := 0; i < 8; i++ {
		cmdDepth[128+8*i] = depths[40+i]
		cmdDepth[256+8*i] = depths[48+i]
		cmdDepth[448+8*i] = depths[56+i]
	}

	bitstream.StoreHuffmanTree(cmdDepth[:], common.NumCommandSymbols, tree[:], storageIx, storage)

	for i := 0; i < 64; i++ {
		depths[i] = cmdDepth[i]
		bits[i] = cmdBits[i]
	}
}

func q0EmitInsertLen(insertlen uint, depths []byte, bits []uint16, histo []uint32, storageIx *uint, storage []byte) {
	if insertlen < 6 {
		code := uint32(insertlen + 128)
		bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
		histo[code]++
	} else if insertlen < 130 {
		insertlen -= 2
		nbits := uint32(common.Log2FloorNonZero(insertlen) - 1)
		prefix := insertlen >> nbits
		code := uint32(2*nbits + uint32(prefix) + 124)
		bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
		bitstream.WriteBits(uint(nbits), uint64(insertlen)-uint64(uint(prefix)<<nbits), storageIx, storage)
		histo[code]++
	} else if insertlen < 2114 {
		insertlen -= 66
		nbits := uint32(common.Log2FloorNonZero(insertlen) - 1)
		prefix := insertlen >> nbits
		code := uint32(2*nbits + uint32(prefix) + 124)
		bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
		bitstream.WriteBits(uint(nbits), uint64(insertlen)-uint64(uint(prefix)<<nbits), storageIx, storage)
		histo[code]++
	} else {
		bitstream.WriteBits(uint(depths[61]), uint64(bits[61]), storageIx, storage)
		bitstream.WriteBits(12, uint64(insertlen)-2114, storageIx, storage)
		histo[61]++
	}
}

func q0EmitCopyLen(copylen uint, depths []byte, bits []uint16, histo []uint32, storageIx *uint, storage []byte) {
	if copylen < 10 {
		bitstream.WriteBits(uint(depths[copylen+128]), uint64(bits[copylen+128]), storageIx, storage)
		histo[copylen+128]++
	} else if copylen < 134 {
		copylen -= 6
		nbits := uint32(common.Log2FloorNonZero(copylen) - 1)
		prefix := copylen >> nbits
		code := uint32(2*nbits + uint32(prefix) + 124)
		bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
		bitstream.WriteBits(uint(nbits), uint64(copylen)-uint64(uint(prefix)<<nbits), storageIx, storage)
		histo[code]++
	} else if copylen < 2118 {
		copylen -= 70
		nbits := uint32(common.Log2FloorNonZero(copylen) - 1)
		prefix := copylen >> nbits
		code := uint32(2*nbits + uint32(prefix) + 124)
		bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
		bitstream.WriteBits(uint(nbits), uint64(copylen)-uint64(uint(prefix)<<nbits), storageIx, storage)
		histo[code]++
	} else {
		bitstream.WriteBits(uint(depths[61]), uint64(bits[61]), storageIx, storage)
		bitstream.WriteBits(12, uint64(copylen)-2118, storageIx, storage)
		histo[61]++
	}
}

func q0EmitCopyLenLastDistance(copylen uint, depths []byte, bits []uint16, histo []uint32, storageIx *uint, storage []byte) {
	if copylen < 8 {
		bitstream.WriteBits(uint(depths[copylen+128]), uint64(bits[copylen+128]), storageIx, storage)
		histo[copylen+128]++
	} else if copylen < 132 {
		copylen -= 4
		nbits := uint32(common.Log2FloorNonZero(copylen) - 1)
		prefix := copylen >> nbits
		code := uint32(2*nbits + uint32(prefix) + 124)
		bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
		bitstream.WriteBits(uint(nbits), uint64(copylen)-uint64(uint(prefix)<<nbits), storageIx, storage)
		histo[code]++
	} else if copylen < 2116 {
		copylen -= 68
		nbits := uint32(common.Log2FloorNonZero(copylen) - 1)
		prefix := copylen >> nbits
		code := uint32(2*nbits + uint32(prefix) + 124)
		bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
		bitstream.WriteBits(uint(nbits), uint64(copylen)-uint64(uint(prefix)<<nbits), storageIx, storage)
		histo[code]++
	} else {
		bitstream.WriteBits(uint(depths[61]), uint64(bits[61]), storageIx, storage)
		bitstream.WriteBits(12, uint64(copylen)-2116, storageIx, storage)
		histo[61]++
	}
}

func q0EmitDistance(distance uint32, depths []byte, bits []uint16, histo []uint32, storageIx *uint, storage []byte) {
	distance += 3
	nbits := uint32(common.Log2FloorNonZero(uint(distance)) - 1)
	prefix := distance >> nbits
	code := uint32(2*nbits + uint32(prefix) + 12)
	bitstream.WriteBits(uint(depths[code]), uint64(bits[code]), storageIx, storage)
	bitstream.WriteBits(uint(nbits), uint64(distance)-uint64(uint(prefix)<<nbits), storageIx, storage)
	histo[code]++
}

func q0CompressFragmentFast(input []byte, inputSize uint, isLast bool, table []int, tableSize uint, storageIx *uint, storage []byte) {
	var initialStorageIx uint = *storageIx
	var cmdHisto [128]uint32
	var ip uint = 0
	var lastIp uint = 0
	var lastProcessedIp uint = 0
	var lastDiff uint32 = 1 << 30
	var literalDepths [256]byte
	var literalBits [256]uint16

	if inputSize == 0 {
		bitstream.WriteBits(1, 1, storageIx, storage)
		bitstream.WriteBits(1, 1, storageIx, storage)
		*storageIx = (*storageIx + 7) &^ 7
		return
	}

	bitstream.WriteBits(1, 0, storageIx, storage)
	literalRatio := q0BuildAndStoreLiteralPrefixCode(input, inputSize, literalDepths[:], literalBits[:], storageIx, storage)

	copy(cmdHisto[:], kCmdHistoSeed[:])

	var cmdDepths [128]byte
	var cmdBits [128]uint16
	q0BuildAndStoreCommandPrefixCode1(cmdHisto[:], cmdDepths[:], cmdBits[:], storageIx, storage)

	for i := uint(0); i < tableSize; i++ {
		table[i] = 0
	}

	if inputSize >= 8 {
		var terminateIp uint = inputSize - 8
		for ip < terminateIp {
			var hash uint32 = (binary.LittleEndian.Uint32(input[ip:]) * hashMul32) >> (32 - 15)
			var pos uint = uint(table[hash])
			var diff uint32 = uint32(ip - pos)
			table[hash] = int(ip)
			if diff < maxDistanceCompressFragment {
				if binary.LittleEndian.Uint32(input[ip:]) == binary.LittleEndian.Uint32(input[pos:]) {
					var mlen uint = 4 + common.FindMatchLengthWithLimit(input[pos+4:], input[ip+4:], inputSize-ip-4)
					if literalRatio < 150 || (mlen >= 7 && literalRatio < 250) {
						q0EmitInsertLen(ip-lastIp, cmdDepths[:], cmdBits[:], cmdHisto[:], storageIx, storage)
						for j := lastIp; j < ip; j++ {
							bitstream.WriteBits(uint(literalDepths[input[j]]), uint64(literalBits[input[j]]), storageIx, storage)
						}
						if diff == lastDiff {
							bitstream.WriteBits(uint(cmdDepths[64]), uint64(cmdBits[64]), storageIx, storage)
							cmdHisto[64]++
							q0EmitCopyLenLastDistance(mlen, cmdDepths[:], cmdBits[:], cmdHisto[:], storageIx, storage)
						} else {
							q0EmitDistance(diff, cmdDepths[:], cmdBits[:], cmdHisto[:], storageIx, storage)
							q0EmitCopyLen(mlen, cmdDepths[:], cmdBits[:], cmdHisto[:], storageIx, storage)
							lastDiff = diff
						}
						ip += mlen
						lastIp = ip
						lastProcessedIp = ip
						continue
					}
				}
			}
			ip++
		}
	}

	q0EmitInsertLen(inputSize-lastIp, cmdDepths[:], cmdBits[:], cmdHisto[:], storageIx, storage)
	for j := lastIp; j < inputSize; j++ {
		bitstream.WriteBits(uint(literalDepths[input[j]]), uint64(literalBits[input[j]]), storageIx, storage)
	}

	if isLast {
		bitstream.WriteBits(1, 1, storageIx, storage)
		bitstream.WriteBits(1, 1, storageIx, storage)
		*storageIx = (*storageIx + 7) &^ 7
	} else {
		// Re-write the command tree with the actual counts.
		// In level 0, we just use the initial counts for simplicity.
	}

	_ = lastProcessedIp
	_ = initialStorageIx
}
