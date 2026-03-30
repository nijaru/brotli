package bitstream

import (
	"slices"

	"github.com/nijaru/brotli/internal/common"
)

const maxHuffmanTreeSize = 2*common.NumCommandSymbols + 1

func blockLengthPrefixCode(len uint32) uint32 {
	var code uint32
	if len >= 177 {
		if len >= 753 {
			code = 20
		} else {
			code = 14
		}
	} else if len >= 41 {
		code = 7
	} else {
		code = 0
	}
	for code < (common.NumBlockLenSymbols-1) && len >= common.KBlockLengthPrefixCode[code+1].Offset {
		code++
	}
	return code
}

func encodeMlen(length uint, bits *uint64, numbits *uint, nibblesbits *uint64) {
	var lg uint
	if length == 1 {
		lg = 1
	} else {
		lg = uint(common.Log2FloorNonZero(uint(uint32(length-1)))) + 1
	}
	var tmp uint
	if lg < 16 {
		tmp = 16
	} else {
		tmp = (lg + 3)
	}
	var mnibbles uint = tmp / 4
	*nibblesbits = uint64(mnibbles) - 4
	*numbits = mnibbles * 4
	*bits = uint64(length) - 1
}

func StoreMetaBlockHeaderBW(length uint, is_uncompressed bool, bw *BitWriter) {
	var lenbits uint64
	var nlenbits uint
	var nibblesbits uint64
	bw.WriteBits(1, 0)
	encodeMlen(length, &lenbits, &nlenbits, &nibblesbits)
	bw.WriteBits(2, nibblesbits)
	bw.WriteBits(nlenbits, lenbits)
	if is_uncompressed {
		bw.WriteBits(1, 1)
	} else {
		bw.WriteBits(1, 0)
	}
}

type symbolAndCount struct {
	symbol uint32
	count  uint32
}

func chooseBitDepths(histogram []uint32, depth []byte, maxBits int) {
	totalCodeSpace := 1 << maxBits
	symbols := make([]symbolAndCount, 0, 704)
	var totalCount uint32 = 0
	for i, n := range histogram {
		if n != 0 {
			symbols = append(symbols, symbolAndCount{
				symbol: uint32(i),
				count:  n,
			})
			totalCount += n
		}
	}
	slices.SortFunc(symbols, func(a, b symbolAndCount) int {
		return int(b.count) - int(a.count)
	})

	boundaries := make([]int, maxBits+1, 20)

	codeSpaceUsed := 0
	totalRatio := float64(totalCount) / float64(totalCodeSpace)
	currentDepth := 1
	for i, symbol := range symbols {
		for currentDepth < maxBits && float64(symbol.count)/float64(int(1)<<(maxBits-currentDepth)) < totalRatio {
			currentDepth++
			boundaries[currentDepth] = i
		}
		codeSpaceUsed += 1 << (maxBits - currentDepth)
	}
	for i := currentDepth + 1; i <= maxBits; i++ {
		boundaries[i] = len(symbols)
	}

	for codeSpaceUsed < totalCodeSpace {
		available := totalCodeSpace - codeSpaceUsed
		bestRatio := 0.0
		bestBoundary := 0
		for i := 2; i <= maxBits; i++ {
			cost := 1 << (maxBits - i)
			if cost > available {
				continue
			}
			if boundaries[i] == len(symbols) {
				continue
			}
			if i < maxBits && boundaries[i] == boundaries[i+1] {
				continue
			}
			ratio := float64(symbols[boundaries[i]].count) / float64(cost)
			if ratio > bestRatio {
				bestRatio = ratio
				bestBoundary = i
			}
		}
		boundaries[bestBoundary]++
		codeSpaceUsed += 1 << (maxBits - bestBoundary)
	}

	for i := range depth {
		depth[i] = 0
	}
	for i := 1; i < maxBits; i++ {
		for j := boundaries[i]; j < boundaries[i+1]; j++ {
			depth[symbols[j].symbol] = byte(i)
		}
	}
	for j := boundaries[maxBits]; j < len(symbols); j++ {
		depth[symbols[j].symbol] = byte(maxBits)
	}
}

func BuildAndStoreHuffmanTreeFastBW(histogram []uint32, histogram_total uint, max_bits uint, depth []byte, bits []uint16, bw *BitWriter) {
	var count uint = 0
	var symbols = [4]uint{0}
	var length uint = 0
	var total uint = histogram_total
	for total != 0 {
		if histogram[length] != 0 {
			if count < 4 {
				symbols[count] = length
			}
			count++
			total -= uint(histogram[length])
		}
		length++
	}

	if count <= 1 {
		bw.WriteBits(4, 1)
		bw.WriteBits(max_bits, uint64(symbols[0]))
		depth[symbols[0]] = 0
		bits[symbols[0]] = 0
		return
	}

	chooseBitDepths(histogram[:length], depth[:length], 14)

	ConvertBitDepthsToSymbols(depth, length, bits)
	if count <= 4 {
		var i uint

		bw.WriteBits(2, 1)
		bw.WriteBits(2, uint64(count)-1)

		for i = 0; i < count; i++ {
			var j uint
			for j = i + 1; j < count; j++ {
				if depth[symbols[j]] < depth[symbols[i]] {
					var tmp uint = symbols[j]
					symbols[j] = symbols[i]
					symbols[i] = tmp
				}
			}
		}

		if count == 2 {
			bw.WriteBits(max_bits, uint64(symbols[0]))
			bw.WriteBits(max_bits, uint64(symbols[1]))
		} else if count == 3 {
			bw.WriteBits(max_bits, uint64(symbols[0]))
			bw.WriteBits(max_bits, uint64(symbols[1]))
			bw.WriteBits(max_bits, uint64(symbols[2]))
		} else {
			bw.WriteBits(max_bits, uint64(symbols[0]))
			bw.WriteBits(max_bits, uint64(symbols[1]))
			bw.WriteBits(max_bits, uint64(symbols[2]))
			bw.WriteBits(max_bits, uint64(symbols[3]))
			bw.WriteSingleBit(depth[symbols[0]] == 1)
		}
	} else {
		var previous_value byte = 8
		var i uint

		StoreStaticCodeLengthCodeBW(bw)

		for i = 0; i < length; {
			var value byte = depth[i]
			var reps uint = 1
			var k uint
			for k = i + 1; k < length && depth[k] == value; k++ {
				reps++
			}

			i += reps
			if value == 0 {
				bw.WriteBits(uint(kZeroRepsDepth[reps]), kZeroRepsBits[reps])
			} else {
				if previous_value != value {
					bw.WriteBits(uint(kCodeLengthDepth[value]), uint64(kCodeLengthBits[value]))
					reps--
				}

				if reps < 3 {
					for reps != 0 {
						reps--
						bw.WriteBits(uint(kCodeLengthDepth[value]), uint64(kCodeLengthBits[value]))
					}
				} else {
					reps -= 3
					bw.WriteBits(uint(kNonZeroRepsDepth[reps]), kNonZeroRepsBits[reps])
				}

				previous_value = value
			}
		}
	}
}
