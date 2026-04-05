package bitstream

import (
	"slices"
	"sync"

	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/context"
	"github.com/nijaru/brotli/internal/metablock"
)

const maxHuffmanTreeSize = (2*common.NumCommandSymbols + 1)

/*
The maximum size of Huffman dictionary for distances assuming that

	NPOSTFIX = 0 and NDIRECT = 0.
*/
const maxSimpleDistanceAlphabetSize = 140

/*
Represents the range of values belonging to a prefix code:

	[offset, offset + 2^nbits)
*/
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
	for code < (common.NumBlockLenSymbols-1) && len >= common.BlockLengthPrefixCode[code+1].Offset {
		code++
	}
	return code
}

func getBlockLengthPrefixCode(len uint32, code *uint, n_extra *uint32, extra *uint32) {
	*code = uint(blockLengthPrefixCode(uint32(len)))
	*n_extra = common.BlockLengthPrefixCode[*code].Nbits
	*extra = len - common.BlockLengthPrefixCode[*code].Offset
}

type blockTypeCodeCalculator struct {
	last_type        uint
	second_last_type uint
}

func initBlockTypeCodeCalculator(self *blockTypeCodeCalculator) {
	self.last_type = 1
	self.second_last_type = 0
}

func nextBlockTypeCode(calculator *blockTypeCodeCalculator, type_ byte) uint {
	var type_code uint
	if uint(type_) == calculator.last_type+1 {
		type_code = 1
	} else if uint(type_) == calculator.second_last_type {
		type_code = 0
	} else {
		type_code = uint(type_) + 2
	}
	calculator.second_last_type = calculator.last_type
	calculator.last_type = uint(type_)
	return type_code
}

/*
|nibblesbits| represents the 2 bits to encode MNIBBLES (0-3)

	REQUIRES: length > 0
	REQUIRES: length <= (1 << 24)
*/
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
	common.Assert(length > 0)
	common.Assert(length <= 1<<24)
	common.Assert(lg <= 24)
	*nibblesbits = uint64(mnibbles) - 4
	*numbits = mnibbles * 4
	*bits = uint64(length) - 1
}

func storeCommandExtra(cmd *metablock.Command, storage_ix *uint, storage []byte) {
	var copylen_code uint32 = metablock.CommandCopyLenCode(cmd)
	var inscode uint16 = metablock.GetInsertLengthCode(uint(cmd.Insert_len_))
	var copycode uint16 = metablock.GetCopyLengthCode(uint(copylen_code))
	var insnumextra uint32 = metablock.GetInsertExtra(inscode)
	var insextraval uint64 = uint64(cmd.Insert_len_) - uint64(metablock.GetInsertBase(inscode))
	var copyextraval uint64 = uint64(copylen_code) - uint64(metablock.GetCopyBase(copycode))
	var bits uint64 = copyextraval<<insnumextra | insextraval
	WriteBits(uint(insnumextra+metablock.GetCopyExtra(copycode)), bits, storage_ix, storage)
}

/*
Data structure that stores almost everything that is needed to encode each

	block switch command.
*/
type blockSplitCode struct {
	type_code_calculator blockTypeCodeCalculator
	type_depths          [common.MaxBlockTypeSymbols]byte
	type_bits            [common.MaxBlockTypeSymbols]uint16
	length_depths        [common.NumBlockLenSymbols]byte
	length_bits          [common.NumBlockLenSymbols]uint16
}

/* Stores a number between 0 and 255. */
func storeVarLenUint8(n uint, storage_ix *uint, storage []byte) {
	if n == 0 {
		WriteBits(1, 0, storage_ix, storage)
	} else {
		var nbits uint = uint(common.Log2FloorNonZero(n))
		WriteBits(1, 1, storage_ix, storage)
		WriteBits(3, uint64(nbits), storage_ix, storage)
		WriteBits(nbits, uint64(n)-(uint64(uint(1))<<nbits), storage_ix, storage)
	}
}

/*
Stores the compressed meta-block header.

	REQUIRES: length > 0
	REQUIRES: length <= (1 << 24)
*/
func storeCompressedMetaBlockHeader(is_final_block bool, length uint, storage_ix *uint, storage []byte) {
	var lenbits uint64
	var nlenbits uint
	var nibblesbits uint64
	var is_final uint64
	if is_final_block {
		is_final = 1
	} else {
		is_final = 0
	}

	/* Write ISLAST bit. */
	WriteBits(1, is_final, storage_ix, storage)

	/* Write ISEMPTY bit. */
	if is_final_block {
		WriteBits(1, 0, storage_ix, storage)
	}

	encodeMlen(length, &lenbits, &nlenbits, &nibblesbits)
	WriteBits(2, nibblesbits, storage_ix, storage)
	WriteBits(nlenbits, lenbits, storage_ix, storage)

	if !is_final_block {
		/* Write ISUNCOMPRESSED bit. */
		WriteBits(1, 0, storage_ix, storage)
	}
}

/*
Stores the uncompressed meta-block header.

	REQUIRES: length > 0
	REQUIRES: length <= (1 << 24)
*/
func storeUncompressedMetaBlockHeader(length uint, storage_ix *uint, storage []byte) {
	var lenbits uint64
	var nlenbits uint
	var nibblesbits uint64

	/* Write ISLAST bit.
	   Uncompressed block cannot be the last one, so set to 0. */
	WriteBits(1, 0, storage_ix, storage)

	encodeMlen(length, &lenbits, &nlenbits, &nibblesbits)
	WriteBits(2, nibblesbits, storage_ix, storage)
	WriteBits(nlenbits, lenbits, storage_ix, storage)

	/* Write ISUNCOMPRESSED bit. */
	WriteBits(1, 1, storage_ix, storage)
}

var StoreHuffmanTreeOfHuffmanTreeToBitMask_kStorageOrder = [common.CodeLengthCodes]byte{1, 2, 3, 4, 0, 5, 17, 6, 16, 7, 8, 9, 10, 11, 12, 13, 14, 15}

var StoreHuffmanTreeOfHuffmanTreeToBitMask_kHuffmanBitLengthHuffmanCodeSymbols = [6]byte{0, 7, 3, 2, 1, 15}
var StoreHuffmanTreeOfHuffmanTreeToBitMask_kHuffmanBitLengthHuffmanCodeBitLengths = [6]byte{2, 4, 3, 2, 2, 4}

func StoreHuffmanTreeOfHuffmanTreeToBitMask(num_codes int, code_length_bitdepth []byte, storage_ix *uint, storage []byte) {
	var skip_some uint = 0
	var codes_to_store uint = common.CodeLengthCodes
	/* The bit lengths of the Huffman code over the code length alphabet
	   are compressed with the following static Huffman code:
	     Symbol   Code
	     ------   ----
	     0          00
	     1        1110
	     2         110
	     3          01
	     4          10
	     5        1111 */

	/* Throw away trailing zeros: */
	if num_codes > 1 {
		for ; codes_to_store > 0; codes_to_store-- {
			if code_length_bitdepth[StoreHuffmanTreeOfHuffmanTreeToBitMask_kStorageOrder[codes_to_store-1]] != 0 {
				break
			}
		}
	}

	if code_length_bitdepth[StoreHuffmanTreeOfHuffmanTreeToBitMask_kStorageOrder[0]] == 0 && code_length_bitdepth[StoreHuffmanTreeOfHuffmanTreeToBitMask_kStorageOrder[1]] == 0 {
		skip_some = 2 /* skips two. */
		if code_length_bitdepth[StoreHuffmanTreeOfHuffmanTreeToBitMask_kStorageOrder[2]] == 0 {
			skip_some = 3 /* skips three. */
		}
	}

	WriteBits(2, uint64(skip_some), storage_ix, storage)
	{
		var i uint
		for i = skip_some; i < codes_to_store; i++ {
			var l uint = uint(code_length_bitdepth[StoreHuffmanTreeOfHuffmanTreeToBitMask_kStorageOrder[i]])
			WriteBits(uint(StoreHuffmanTreeOfHuffmanTreeToBitMask_kHuffmanBitLengthHuffmanCodeBitLengths[l]), uint64(StoreHuffmanTreeOfHuffmanTreeToBitMask_kHuffmanBitLengthHuffmanCodeSymbols[l]), storage_ix, storage)
		}
	}
}

func StoreHuffmanTreeToBitMask(huffman_tree_size uint, huffman_tree []byte, huffman_tree_extra_bits []byte, code_length_bitdepth []byte, code_length_bitdepth_symbols []uint16, storage_ix *uint, storage []byte) {
	var i uint
	for i = 0; i < huffman_tree_size; i++ {
		var ix uint = uint(huffman_tree[i])
		WriteBits(uint(code_length_bitdepth[ix]), uint64(code_length_bitdepth_symbols[ix]), storage_ix, storage)

		/* Extra bits */
		switch ix {
		case common.RepeatPreviousCodeLength:
			WriteBits(2, uint64(huffman_tree_extra_bits[i]), storage_ix, storage)

		case common.RepeatZeroCodeLength:
			WriteBits(3, uint64(huffman_tree_extra_bits[i]), storage_ix, storage)
		}
	}
}

func storeSimpleHuffmanTree(depths []byte, symbols []uint, num_symbols uint, max_bits uint, storage_ix *uint, storage []byte) {
	/* value of 1 indicates a simple Huffman code */
	WriteBits(2, 1, storage_ix, storage)

	WriteBits(2, uint64(num_symbols)-1, storage_ix, storage) /* NSYM - 1 */
	{
		/* Sort */
		var i uint
		for i = 0; i < num_symbols; i++ {
			var j uint
			for j = i + 1; j < num_symbols; j++ {
				if depths[symbols[j]] < depths[symbols[i]] {
					var tmp uint = symbols[j]
					symbols[j] = symbols[i]
					symbols[i] = tmp
				}
			}
		}
	}

	if num_symbols == 2 {
		WriteBits(max_bits, uint64(symbols[0]), storage_ix, storage)
		WriteBits(max_bits, uint64(symbols[1]), storage_ix, storage)
	} else if num_symbols == 3 {
		WriteBits(max_bits, uint64(symbols[0]), storage_ix, storage)
		WriteBits(max_bits, uint64(symbols[1]), storage_ix, storage)
		WriteBits(max_bits, uint64(symbols[2]), storage_ix, storage)
	} else {
		WriteBits(max_bits, uint64(symbols[0]), storage_ix, storage)
		WriteBits(max_bits, uint64(symbols[1]), storage_ix, storage)
		WriteBits(max_bits, uint64(symbols[2]), storage_ix, storage)
		WriteBits(max_bits, uint64(symbols[3]), storage_ix, storage)

		/* tree-select */
		var tmp int
		if depths[symbols[0]] == 1 {
			tmp = 1
		} else {
			tmp = 0
		}
		WriteBits(1, uint64(tmp), storage_ix, storage)
	}
}

/*
num = alphabet size

	depths = symbol depths
*/
func StoreHuffmanTree(depths []byte, num uint, tree []HuffmanTree, storage_ix *uint, storage []byte) {
	var huffman_tree [common.NumCommandSymbols]byte
	var huffman_tree_extra_bits [common.NumCommandSymbols]byte
	var huffman_tree_size uint = 0
	var code_length_bitdepth = [common.CodeLengthCodes]byte{0}
	var code_length_bitdepth_symbols [common.CodeLengthCodes]uint16
	var huffman_tree_histogram = [common.CodeLengthCodes]uint32{0}
	var i uint
	var num_codes int = 0
	/* Write the Huffman tree into the brotli-representation.
	   The command alphabet is the largest, so this allocation will fit all
	   alphabets. */

	var code uint = 0

	common.Assert(num <= common.NumCommandSymbols)

	WriteHuffmanTree(depths, num, &huffman_tree_size, huffman_tree[:], huffman_tree_extra_bits[:])

	/* Calculate the statistics of the Huffman tree in brotli-representation. */
	for i = 0; i < huffman_tree_size; i++ {
		huffman_tree_histogram[huffman_tree[i]]++
	}

	for i = 0; i < common.CodeLengthCodes; i++ {
		if huffman_tree_histogram[i] != 0 {
			if num_codes == 0 {
				code = i
				num_codes = 1
			} else if num_codes == 1 {
				num_codes = 2
				break
			}
		}
	}

	/* Calculate another Huffman tree to use for compressing both the
	   earlier Huffman tree with. */
	CreateHuffmanTree(huffman_tree_histogram[:], common.CodeLengthCodes, 5, tree, code_length_bitdepth[:])

	ConvertBitDepthsToSymbols(code_length_bitdepth[:], common.CodeLengthCodes, code_length_bitdepth_symbols[:])

	/* Now, we have all the data, let's start storing it */
	StoreHuffmanTreeOfHuffmanTreeToBitMask(num_codes, code_length_bitdepth[:], storage_ix, storage)

	if num_codes == 1 {
		code_length_bitdepth[code] = 0
	}

	/* Store the real Huffman tree now. */
	StoreHuffmanTreeToBitMask(huffman_tree_size, huffman_tree[:], huffman_tree_extra_bits[:], code_length_bitdepth[:], code_length_bitdepth_symbols[:], storage_ix, storage)
}

/*
Builds a Huffman tree from histogram[0:length] into depth[0:length] and

	bits[0:length] and stores the encoded tree to the bit stream.
*/
func BuildAndStoreHuffmanTree(histogram []uint32, histogram_length uint, alphabet_size uint, tree []HuffmanTree, depth []byte, bits []uint16, storage_ix *uint, storage []byte) {
	var count uint = 0
	var s4 = [4]uint{0}
	var i uint
	var max_bits uint = 0
	for i = 0; i < histogram_length; i++ {
		if histogram[i] != 0 {
			if count < 4 {
				s4[count] = i
			} else if count > 4 {
				break
			}

			count++
		}
	}
	{
		var max_bits_counter uint = alphabet_size - 1
		for max_bits_counter != 0 {
			max_bits_counter >>= 1
			max_bits++
		}
	}

	if count <= 1 {
		WriteBits(4, 1, storage_ix, storage)
		WriteBits(max_bits, uint64(s4[0]), storage_ix, storage)
		depth[s4[0]] = 0
		bits[s4[0]] = 0
		return
	}

	for i := 0; i < int(histogram_length); i++ {
		depth[i] = 0
	}
	CreateHuffmanTree(histogram, histogram_length, 15, tree, depth)
	ConvertBitDepthsToSymbols(depth, histogram_length, bits)

	if count <= 4 {
		storeSimpleHuffmanTree(depth, s4[:], count, max_bits, storage_ix, storage)
	} else {
		StoreHuffmanTree(depth, histogram_length, tree, storage_ix, storage)
	}
}

func BuildAndStoreHuffmanTreeFast(histogram []uint32, histogram_total uint, max_bits uint, depth []byte, bits []uint16, storage_ix *uint, storage []byte) {
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
		WriteBits(4, 1, storage_ix, storage)
		WriteBits(max_bits, uint64(symbols[0]), storage_ix, storage)
		depth[symbols[0]] = 0
		bits[symbols[0]] = 0
		return
	}

	chooseBitDepths(histogram[:length], depth[:length], 14)

	ConvertBitDepthsToSymbols(depth, length, bits)
	if count <= 4 {
		var i uint

		/* value of 1 indicates a simple Huffman code */
		WriteBits(2, 1, storage_ix, storage)

		WriteBits(2, uint64(count)-1, storage_ix, storage) /* NSYM - 1 */

		/* Sort */
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
			WriteBits(max_bits, uint64(symbols[0]), storage_ix, storage)
			WriteBits(max_bits, uint64(symbols[1]), storage_ix, storage)
		} else if count == 3 {
			WriteBits(max_bits, uint64(symbols[0]), storage_ix, storage)
			WriteBits(max_bits, uint64(symbols[1]), storage_ix, storage)
			WriteBits(max_bits, uint64(symbols[2]), storage_ix, storage)
		} else {
			WriteBits(max_bits, uint64(symbols[0]), storage_ix, storage)
			WriteBits(max_bits, uint64(symbols[1]), storage_ix, storage)
			WriteBits(max_bits, uint64(symbols[2]), storage_ix, storage)
			WriteBits(max_bits, uint64(symbols[3]), storage_ix, storage)

			/* tree-select */
			var tmp int
			if depth[symbols[0]] == 1 {
				tmp = 1
			} else {
				tmp = 0
			}
			WriteBits(1, uint64(tmp), storage_ix, storage)
		}
	} else {
		var previous_value byte = 8
		var i uint

		/* Complex Huffman Tree */
		StoreStaticCodeLengthCode(storage_ix, storage)

		/* Actual RLE coding. */
		for i = 0; i < length; {
			var value byte = depth[i]
			var reps uint = 1
			var k uint
			for k = i + 1; k < length && depth[k] == value; k++ {
				reps++
			}

			i += reps
			if value == 0 {
				WriteBits(uint(zeroRepsDepth[reps]), zeroRepsBits[reps], storage_ix, storage)
			} else {
				if previous_value != value {
					WriteBits(uint(codeLengthDepth[value]), uint64(codeLengthBits[value]), storage_ix, storage)
					reps--
				}

				if reps < 3 {
					for reps != 0 {
						reps--
						WriteBits(uint(codeLengthDepth[value]), uint64(codeLengthBits[value]), storage_ix, storage)
					}
				} else {
					reps -= 3
					WriteBits(uint(nonZeroRepsDepth[reps]), nonZeroRepsBits[reps], storage_ix, storage)
				}

				previous_value = value
			}
		}
	}
}

type symbolAndCount struct {
	symbol uint32
	count  uint32
}

func chooseBitDepths(histogram []uint32, depth []byte, maxBits int) {
	totalCodeSpace := 1 << maxBits
	symbols := make([]symbolAndCount, 0, 704 /* static capacity so that it will be stack allocated */)
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

	// boundaries contains indexes into symbols such that (for example)
	// boundaries[8] is the index of the first 8-bit symbol.
	boundaries := make([]int, maxBits+1, 20 /* static capacity for stack allocation */)

	codeSpaceUsed := 0

	// Assign initial boundaries conservatively, making sure no symbol uses more
	// than its share of code space.
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

	// Move the boundaries till the code space is filled.
	for codeSpaceUsed < totalCodeSpace {
		available := totalCodeSpace - codeSpaceUsed
		// Find the most efficient boundary to move, based on the ratio of how
		// many times the symbol was used to how much code space will be consumed.
		bestRatio := 0.0
		bestBoundary := 0
		for i := 2; i <= maxBits; i++ {
			cost := 1 << (maxBits - i) // code space that would be used by moving this boundary
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

		/* value of 1 indicates a simple Huffman code */
		bw.WriteBits(2, 1)

		bw.WriteBits(2, uint64(count)-1) /* NSYM - 1 */

		/* Sort */
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

			/* tree-select */
			bw.WriteSingleBit(depth[symbols[0]] == 1)
		}
	} else {
		var previous_value byte = 8
		var i uint

		/* Complex Huffman Tree */
		StoreStaticCodeLengthCodeBW(bw)

		/* Actual RLE coding. */
		for i = 0; i < length; {
			var value byte = depth[i]
			var reps uint = 1
			var k uint
			for k = i + 1; k < length && depth[k] == value; k++ {
				reps++
			}

			i += reps
			if value == 0 {
				bw.WriteBits(uint(zeroRepsDepth[reps]), zeroRepsBits[reps])
			} else {
				if previous_value != value {
					bw.WriteBits(uint(codeLengthDepth[value]), uint64(codeLengthBits[value]))
					reps--
				}

				if reps < 3 {
					for reps != 0 {
						reps--
						bw.WriteBits(uint(codeLengthDepth[value]), uint64(codeLengthBits[value]))
					}
				} else {
					reps -= 3
					bw.WriteBits(uint(nonZeroRepsDepth[reps]), nonZeroRepsBits[reps])
				}

				previous_value = value
			}
		}
	}
}

func indexOf(v []byte, v_size uint, value byte) uint {
	var i uint = 0
	for ; i < v_size; i++ {
		if v[i] == value {
			return i
		}
	}

	return i
}

func moveToFront(v []byte, index uint) {
	var value byte = v[index]
	var i uint
	for i = index; i != 0; i-- {
		v[i] = v[i-1]
	}

	v[0] = value
}

func moveToFrontTransform(v_in []uint32, v_size uint, v_out []uint32) {
	var i uint
	var mtf [256]byte
	var max_value uint32
	if v_size == 0 {
		return
	}

	max_value = v_in[0]
	for i = 1; i < v_size; i++ {
		if v_in[i] > max_value {
			max_value = v_in[i]
		}
	}

	common.Assert(max_value < 256)
	for i = 0; uint32(i) <= max_value; i++ {
		mtf[i] = byte(i)
	}
	{
		var mtf_size uint = uint(max_value + 1)
		for i = 0; i < v_size; i++ {
			var index uint = indexOf(mtf[:], mtf_size, byte(v_in[i]))
			common.Assert(index < mtf_size)
			v_out[i] = uint32(index)
			moveToFront(mtf[:], index)
		}
	}
}

/*
Finds runs of zeros in v[0..in_size) and replaces them with a prefix code of

	the run length plus extra bits (lower 9 bits is the prefix code and the rest
	are the extra bits). Non-zero values in v[] are shifted by
	*max_length_prefix. Will not create prefix codes bigger than the initial
	value of *max_run_length_prefix. The prefix code of run length L is simply
	Log2Floor(L) and the number of extra bits is the same as the prefix code.
*/
func runLengthCodeZeros(in_size uint, v []uint32, out_size *uint, max_run_length_prefix *uint32) {
	var max_reps uint32 = 0
	var i uint
	var max_prefix uint32
	for i = 0; i < in_size; {
		var reps uint32 = 0
		for ; i < in_size && v[i] != 0; i++ {
		}
		for ; i < in_size && v[i] == 0; i++ {
			reps++
		}

		max_reps = max(reps, max_reps)
	}

	if max_reps > 0 {
		max_prefix = common.Log2FloorNonZero(uint(max_reps))
	} else {
		max_prefix = 0
	}
	max_prefix = min(max_prefix, *max_run_length_prefix)
	*max_run_length_prefix = max_prefix
	*out_size = 0
	for i = 0; i < in_size; {
		common.Assert(*out_size <= i)
		if v[i] != 0 {
			v[*out_size] = v[i] + *max_run_length_prefix
			i++
			(*out_size)++
		} else {
			var reps uint32 = 1
			var k uint
			for k = i + 1; k < in_size && v[k] == 0; k++ {
				reps++
			}

			i += uint(reps)
			for reps != 0 {
				if reps < 2<<max_prefix {
					var run_length_prefix uint32 = common.Log2FloorNonZero(uint(reps))
					var extra_bits uint32 = reps - (1 << run_length_prefix)
					v[*out_size] = run_length_prefix + (extra_bits << 9)
					(*out_size)++
					break
				} else {
					var extra_bits uint32 = (1 << max_prefix) - 1
					v[*out_size] = max_prefix + (extra_bits << 9)
					reps -= (2 << max_prefix) - 1
					(*out_size)++
				}
			}
		}
	}
}

const symbolBits = 9

var encodeContextMap_kSymbolMask uint32 = (1 << symbolBits) - 1

func encodeContextMap(context_map []uint32, context_map_size uint, num_clusters uint, tree []HuffmanTree, storage_ix *uint, storage []byte) {
	var i uint
	var rle_symbols []uint32
	var max_run_length_prefix uint32 = 6
	var num_rle_symbols uint = 0
	var histogram [common.MaxContextMapSymbols]uint32
	var depths [common.MaxContextMapSymbols]byte
	var bits [common.MaxContextMapSymbols]uint16

	storeVarLenUint8(num_clusters-1, storage_ix, storage)

	if num_clusters == 1 {
		return
	}

	rle_symbols = make([]uint32, context_map_size)
	moveToFrontTransform(context_map, context_map_size, rle_symbols)
	runLengthCodeZeros(context_map_size, rle_symbols, &num_rle_symbols, &max_run_length_prefix)
	histogram = [common.MaxContextMapSymbols]uint32{}
	for i = 0; i < num_rle_symbols; i++ {
		histogram[rle_symbols[i]&encodeContextMap_kSymbolMask]++
	}
	{
		var use_rle bool = (max_run_length_prefix > 0)
		WriteSingleBit(use_rle, storage_ix, storage)
		if use_rle {
			WriteBits(4, uint64(max_run_length_prefix)-1, storage_ix, storage)
		}
	}

	BuildAndStoreHuffmanTree(histogram[:], uint(uint32(num_clusters)+max_run_length_prefix), uint(uint32(num_clusters)+max_run_length_prefix), tree, depths[:], bits[:], storage_ix, storage)
	for i = 0; i < num_rle_symbols; i++ {
		var rle_symbol uint32 = rle_symbols[i] & encodeContextMap_kSymbolMask
		var extra_bits_val uint32 = rle_symbols[i] >> symbolBits
		WriteBits(uint(depths[rle_symbol]), uint64(bits[rle_symbol]), storage_ix, storage)
		if rle_symbol > 0 && rle_symbol <= max_run_length_prefix {
			WriteBits(uint(rle_symbol), uint64(extra_bits_val), storage_ix, storage)
		}
	}

	WriteBits(1, 1, storage_ix, storage) /* use move-to-front */
	rle_symbols = nil
}

/* Stores the block switch command with index block_ix to the bit stream. */
func storeBlockSwitch(code *blockSplitCode, block_len uint32, block_type byte, is_first_block bool, storage_ix *uint, storage []byte) {
	var typecode uint = nextBlockTypeCode(&code.type_code_calculator, block_type)
	var lencode uint
	var len_nextra uint32
	var len_extra uint32
	if !is_first_block {
		WriteBits(uint(code.type_depths[typecode]), uint64(code.type_bits[typecode]), storage_ix, storage)
	}

	getBlockLengthPrefixCode(block_len, &lencode, &len_nextra, &len_extra)

	WriteBits(uint(code.length_depths[lencode]), uint64(code.length_bits[lencode]), storage_ix, storage)
	WriteBits(uint(len_nextra), uint64(len_extra), storage_ix, storage)
}

/*
Builds a BlockSplitCode data structure from the block split given by the

	vector of block types and block lengths and stores it to the bit stream.
*/
func buildAndStoreBlockSplitCode(types []byte, lengths []uint32, num_blocks uint, num_types uint, tree []HuffmanTree, code *blockSplitCode, storage_ix *uint, storage []byte) {
	var type_histo [common.MaxBlockTypeSymbols]uint32
	var length_histo [common.NumBlockLenSymbols]uint32
	var i uint
	var type_code_calculator blockTypeCodeCalculator
	for i := 0; i < int(num_types+2); i++ {
		type_histo[i] = 0
	}
	length_histo = [common.NumBlockLenSymbols]uint32{}
	initBlockTypeCodeCalculator(&type_code_calculator)
	for i = 0; i < num_blocks; i++ {
		var type_code uint = nextBlockTypeCode(&type_code_calculator, types[i])
		if i != 0 {
			type_histo[type_code]++
		}
		length_histo[blockLengthPrefixCode(lengths[i])]++
	}

	storeVarLenUint8(num_types-1, storage_ix, storage)
	if num_types > 1 { /* TODO: else? could StoreBlockSwitch occur? */
		BuildAndStoreHuffmanTree(type_histo[0:], num_types+2, num_types+2, tree, code.type_depths[0:], code.type_bits[0:], storage_ix, storage)
		BuildAndStoreHuffmanTree(length_histo[0:], common.NumBlockLenSymbols, common.NumBlockLenSymbols, tree, code.length_depths[0:], code.length_bits[0:], storage_ix, storage)
		storeBlockSwitch(code, lengths[0], types[0], true, storage_ix, storage)
	}
}

/* Stores a context map where the histogram type is always the block type. */
func storeTrivialContextMap(num_types uint, context_bits uint, tree []HuffmanTree, storage_ix *uint, storage []byte) {
	storeVarLenUint8(num_types-1, storage_ix, storage)
	if num_types > 1 {
		var repeat_code uint = context_bits - 1
		var repeat_bits uint = (1 << repeat_code) - 1
		var alphabet_size uint = num_types + repeat_code
		var histogram [common.MaxContextMapSymbols]uint32
		var depths [common.MaxContextMapSymbols]byte
		var bits [common.MaxContextMapSymbols]uint16
		var i uint
		for i := 0; i < int(alphabet_size); i++ {
			histogram[i] = 0
		}

		/* Write RLEMAX. */
		WriteBits(1, 1, storage_ix, storage)

		WriteBits(4, uint64(repeat_code)-1, storage_ix, storage)
		histogram[repeat_code] = uint32(num_types)
		histogram[0] = 1
		for i = context_bits; i < alphabet_size; i++ {
			histogram[i] = 1
		}

		BuildAndStoreHuffmanTree(histogram[:], alphabet_size, alphabet_size, tree, depths[:], bits[:], storage_ix, storage)
		for i = 0; i < num_types; i++ {
			var tmp uint
			if i == 0 {
				tmp = 0
			} else {
				tmp = i + context_bits - 1
			}
			var code uint = tmp
			WriteBits(uint(depths[code]), uint64(bits[code]), storage_ix, storage)
			WriteBits(uint(depths[repeat_code]), uint64(bits[repeat_code]), storage_ix, storage)
			WriteBits(repeat_code, uint64(repeat_bits), storage_ix, storage)
		}

		/* Write IMTF (inverse-move-to-front) bit. */
		WriteBits(1, 1, storage_ix, storage)
	}
}

/* Manages the encoding of one block category (literal, command or distance). */
type blockEncoder struct {
	histogram_length_ uint
	num_block_types_  uint
	block_types_      []byte
	block_lengths_    []uint32
	num_blocks_       uint
	block_split_code_ blockSplitCode
	block_ix_         uint
	block_len_        uint
	entropy_ix_       uint
	depths_           []byte
	bits_             []uint16
}

var blockEncoderPool sync.Pool

func getBlockEncoder(histogram_length uint, num_block_types uint, block_types []byte, block_lengths []uint32, num_blocks uint) *blockEncoder {
	self, _ := blockEncoderPool.Get().(*blockEncoder)

	if self != nil {
		self.block_ix_ = 0
		self.entropy_ix_ = 0
		self.depths_ = self.depths_[:0]
		self.bits_ = self.bits_[:0]
	} else {
		self = &blockEncoder{}
	}

	self.histogram_length_ = histogram_length
	self.num_block_types_ = num_block_types
	self.block_types_ = block_types
	self.block_lengths_ = block_lengths
	self.num_blocks_ = num_blocks
	initBlockTypeCodeCalculator(&self.block_split_code_.type_code_calculator)
	if num_blocks == 0 {
		self.block_len_ = 0
	} else {
		self.block_len_ = uint(block_lengths[0])
	}

	return self
}

func cleanupBlockEncoder(self *blockEncoder) {
	blockEncoderPool.Put(self)
}

/*
Creates entropy codes of block lengths and block types and stores them

	to the bit stream.
*/
func buildAndStoreBlockSwitchEntropyCodes(self *blockEncoder, tree []HuffmanTree, storage_ix *uint, storage []byte) {
	buildAndStoreBlockSplitCode(self.block_types_, self.block_lengths_, self.num_blocks_, self.num_block_types_, tree, &self.block_split_code_, storage_ix, storage)
}

/*
Stores the next symbol with the entropy code of the current block type.

	Updates the block type and block length at block boundaries.
*/
func storeSymbol(self *blockEncoder, symbol uint, storage_ix *uint, storage []byte) {
	if self.block_len_ == 0 {
		self.block_ix_++
		var block_ix uint = self.block_ix_
		var block_len uint32 = self.block_lengths_[block_ix]
		var block_type byte = self.block_types_[block_ix]
		self.block_len_ = uint(block_len)
		self.entropy_ix_ = uint(block_type) * self.histogram_length_
		storeBlockSwitch(&self.block_split_code_, block_len, block_type, false, storage_ix, storage)
	}

	self.block_len_--
	{
		var ix uint = self.entropy_ix_ + symbol
		WriteBits(uint(self.depths_[ix]), uint64(self.bits_[ix]), storage_ix, storage)
	}
}

/*
Stores the next symbol with the entropy code of the current block type and

	context value.
	Updates the block type and block length at block boundaries.
*/
func storeSymbolWithContext(self *blockEncoder, symbol uint, context uint, context_map []uint32, storage_ix *uint, storage []byte, context_bits uint) {
	if self.block_len_ == 0 {
		self.block_ix_++
		var block_ix uint = self.block_ix_
		var block_len uint32 = self.block_lengths_[block_ix]
		var block_type byte = self.block_types_[block_ix]
		self.block_len_ = uint(block_len)
		self.entropy_ix_ = uint(block_type) << context_bits
		storeBlockSwitch(&self.block_split_code_, block_len, block_type, false, storage_ix, storage)
	}

	self.block_len_--
	{
		var histo_ix uint = uint(context_map[self.entropy_ix_+context])
		var ix uint = histo_ix*self.histogram_length_ + symbol
		WriteBits(uint(self.depths_[ix]), uint64(self.bits_[ix]), storage_ix, storage)
	}
}

func buildAndStoreEntropyCodesLiteral(self *blockEncoder, histograms []common.HistogramLiteral, histograms_size uint, alphabet_size uint, tree []HuffmanTree, storage_ix *uint, storage []byte) {
	var table_size uint = histograms_size * self.histogram_length_
	if cap(self.depths_) < int(table_size) {
		self.depths_ = make([]byte, table_size)
	} else {
		self.depths_ = self.depths_[:table_size]
	}
	if cap(self.bits_) < int(table_size) {
		self.bits_ = make([]uint16, table_size)
	} else {
		self.bits_ = self.bits_[:table_size]
	}
	{
		var i uint
		for i = 0; i < histograms_size; i++ {
			var ix uint = i * self.histogram_length_
			BuildAndStoreHuffmanTree(histograms[i].Data_[0:], self.histogram_length_, alphabet_size, tree, self.depths_[ix:], self.bits_[ix:], storage_ix, storage)
		}
	}
}

func buildAndStoreEntropyCodesCommand(self *blockEncoder, histograms []common.HistogramCommand, histograms_size uint, alphabet_size uint, tree []HuffmanTree, storage_ix *uint, storage []byte) {
	var table_size uint = histograms_size * self.histogram_length_
	if cap(self.depths_) < int(table_size) {
		self.depths_ = make([]byte, table_size)
	} else {
		self.depths_ = self.depths_[:table_size]
	}
	if cap(self.bits_) < int(table_size) {
		self.bits_ = make([]uint16, table_size)
	} else {
		self.bits_ = self.bits_[:table_size]
	}
	{
		var i uint
		for i = 0; i < histograms_size; i++ {
			var ix uint = i * self.histogram_length_
			BuildAndStoreHuffmanTree(histograms[i].Data_[0:], self.histogram_length_, alphabet_size, tree, self.depths_[ix:], self.bits_[ix:], storage_ix, storage)
		}
	}
}

func buildAndStoreEntropyCodesDistance(self *blockEncoder, histograms []common.HistogramDistance, histograms_size uint, alphabet_size uint, tree []HuffmanTree, storage_ix *uint, storage []byte) {
	var table_size uint = histograms_size * self.histogram_length_
	if cap(self.depths_) < int(table_size) {
		self.depths_ = make([]byte, table_size)
	} else {
		self.depths_ = self.depths_[:table_size]
	}
	if cap(self.bits_) < int(table_size) {
		self.bits_ = make([]uint16, table_size)
	} else {
		self.bits_ = self.bits_[:table_size]
	}
	{
		var i uint
		for i = 0; i < histograms_size; i++ {
			var ix uint = i * self.histogram_length_
			BuildAndStoreHuffmanTree(histograms[i].Data_[0:], self.histogram_length_, alphabet_size, tree, self.depths_[ix:], self.bits_[ix:], storage_ix, storage)
		}
	}
}

func JumpToByteBoundary(storage_ix *uint, storage []byte) {
	*storage_ix = (*storage_ix + 7) &^ 7
	storage[*storage_ix>>3] = 0
}

func StoreMetaBlock(input []byte, start_pos uint, length uint, mask uint, prev_byte byte, prev_byte2 byte, is_last bool, params *common.EncoderParams, literal_context_mode int, commands []metablock.Command, mb *metablock.MetaBlockSplit, storage_ix *uint, storage []byte) {
	var pos uint = start_pos
	var i uint
	var num_distance_symbols uint32 = params.Dist.Alphabet_size
	var num_effective_distance_symbols uint32 = num_distance_symbols
	var tree []HuffmanTree
	var literal_context_lut context.ContextLUT = context.GetContextLUT(literal_context_mode)
	var dist *common.DistanceParams = &params.Dist
	if params.Large_window && num_effective_distance_symbols > common.NumHistogramDistanceSymbols {
		num_effective_distance_symbols = common.NumHistogramDistanceSymbols
	}

	storeCompressedMetaBlockHeader(is_last, length, storage_ix, storage)

	tree = make([]HuffmanTree, maxHuffmanTreeSize)
	literal_enc := getBlockEncoder(common.NumLiteralSymbols, mb.Literal_split.Num_types, mb.Literal_split.Types, mb.Literal_split.Lengths, mb.Literal_split.Num_blocks)
	command_enc := getBlockEncoder(common.NumCommandSymbols, mb.Command_split.Num_types, mb.Command_split.Types, mb.Command_split.Lengths, mb.Command_split.Num_blocks)
	distance_enc := getBlockEncoder(uint(num_effective_distance_symbols), mb.Distance_split.Num_types, mb.Distance_split.Types, mb.Distance_split.Lengths, mb.Distance_split.Num_blocks)

	buildAndStoreBlockSwitchEntropyCodes(literal_enc, tree, storage_ix, storage)
	buildAndStoreBlockSwitchEntropyCodes(command_enc, tree, storage_ix, storage)
	buildAndStoreBlockSwitchEntropyCodes(distance_enc, tree, storage_ix, storage)

	WriteBits(2, uint64(dist.Distance_postfix_bits), storage_ix, storage)
	WriteBits(4, uint64(dist.Num_direct_distance_codes)>>dist.Distance_postfix_bits, storage_ix, storage)
	for i = 0; i < mb.Literal_split.Num_types; i++ {
		WriteBits(2, uint64(literal_context_mode), storage_ix, storage)
	}

	if mb.Literal_context_map_size == 0 {
		storeTrivialContextMap(mb.Literal_histograms_size, common.LiteralContextBits, tree, storage_ix, storage)
	} else {
		encodeContextMap(mb.Literal_context_map, mb.Literal_context_map_size, mb.Literal_histograms_size, tree, storage_ix, storage)
	}

	if mb.Distance_context_map_size == 0 {
		storeTrivialContextMap(mb.Distance_histograms_size, common.DistanceContextBits, tree, storage_ix, storage)
	} else {
		encodeContextMap(mb.Distance_context_map, mb.Distance_context_map_size, mb.Distance_histograms_size, tree, storage_ix, storage)
	}

	buildAndStoreEntropyCodesLiteral(literal_enc, mb.Literal_histograms, mb.Literal_histograms_size, common.NumLiteralSymbols, tree, storage_ix, storage)
	buildAndStoreEntropyCodesCommand(command_enc, mb.Command_histograms, mb.Command_histograms_size, common.NumCommandSymbols, tree, storage_ix, storage)
	buildAndStoreEntropyCodesDistance(distance_enc, mb.Distance_histograms, mb.Distance_histograms_size, uint(num_distance_symbols), tree, storage_ix, storage)
	tree = nil

	for _, cmd := range commands {
		var cmd_code uint = uint(cmd.Cmd_prefix_)
		storeSymbol(command_enc, cmd_code, storage_ix, storage)
		storeCommandExtra(&cmd, storage_ix, storage)
		if mb.Literal_context_map_size == 0 {
			var j uint
			for j = uint(cmd.Insert_len_); j != 0; j-- {
				storeSymbol(literal_enc, uint(input[pos&mask]), storage_ix, storage)
				pos++
			}
		} else {
			var j uint
			for j = uint(cmd.Insert_len_); j != 0; j-- {
				var ctx uint = uint(context.GetContext(prev_byte, prev_byte2, literal_context_lut))
				var literal byte = input[pos&mask]
				storeSymbolWithContext(literal_enc, uint(literal), ctx, mb.Literal_context_map, storage_ix, storage, common.LiteralContextBits)
				prev_byte2 = prev_byte
				prev_byte = literal
				pos++
			}
		}

		pos += uint(metablock.CommandCopyLen(&cmd))
		if metablock.CommandCopyLen(&cmd) != 0 {
			prev_byte2 = input[(pos-2)&mask]
			prev_byte = input[(pos-1)&mask]
			if cmd.Cmd_prefix_ >= 128 {
				var dist_code uint = uint(cmd.Dist_prefix_) & 0x3FF
				var distnumextra uint32 = uint32(cmd.Dist_prefix_) >> 10
				var distextra uint64 = uint64(cmd.Dist_extra_)
				if mb.Distance_context_map_size == 0 {
					storeSymbol(distance_enc, dist_code, storage_ix, storage)
				} else {
					var ctx uint = uint(metablock.CommandDistanceContext(&cmd))
					storeSymbolWithContext(distance_enc, dist_code, ctx, mb.Distance_context_map, storage_ix, storage, common.DistanceContextBits)
				}

				WriteBits(uint(distnumextra), distextra, storage_ix, storage)
			}
		}
	}

	cleanupBlockEncoder(distance_enc)
	cleanupBlockEncoder(command_enc)
	cleanupBlockEncoder(literal_enc)
	if is_last {
		JumpToByteBoundary(storage_ix, storage)
	}
}

func buildHistograms(input []byte, start_pos uint, mask uint, commands []metablock.Command, lit_histo *common.HistogramLiteral, cmd_histo *common.HistogramCommand, dist_histo *common.HistogramDistance) {
	var pos uint = start_pos
	for _, cmd := range commands {
		var j uint
		common.HistogramAddCommand(cmd_histo, uint(cmd.Cmd_prefix_))
		for j = uint(cmd.Insert_len_); j != 0; j-- {
			common.HistogramAddLiteral(lit_histo, uint(input[pos&mask]))
			pos++
		}

		pos += uint(metablock.CommandCopyLen(&cmd))
		if metablock.CommandCopyLen(&cmd) != 0 && cmd.Cmd_prefix_ >= 128 {
			common.HistogramAddDistance(dist_histo, uint(cmd.Dist_prefix_)&0x3FF)
		}
	}
}

func storeDataWithHuffmanCodes(input []byte, start_pos uint, mask uint, commands []metablock.Command, lit_depth []byte, lit_bits []uint16, cmd_depth []byte, cmd_bits []uint16, dist_depth []byte, dist_bits []uint16, storage_ix *uint, storage []byte) {
	var pos uint = start_pos
	for _, cmd := range commands {
		var cmd_code uint = uint(cmd.Cmd_prefix_)
		var j uint
		WriteBits(uint(cmd_depth[cmd_code]), uint64(cmd_bits[cmd_code]), storage_ix, storage)
		storeCommandExtra(&cmd, storage_ix, storage)
		for j = uint(cmd.Insert_len_); j != 0; j-- {
			var literal byte = input[pos&mask]
			WriteBits(uint(lit_depth[literal]), uint64(lit_bits[literal]), storage_ix, storage)
			pos++
		}

		pos += uint(metablock.CommandCopyLen(&cmd))
		if metablock.CommandCopyLen(&cmd) != 0 && cmd.Cmd_prefix_ >= 128 {
			var dist_code uint = uint(cmd.Dist_prefix_) & 0x3FF
			if dist_code >= uint(len(dist_depth)) {
				dist_code = uint(len(dist_depth)) - 1
			}
			var distnumextra uint32 = uint32(cmd.Dist_prefix_) >> 10
			var distextra uint32 = cmd.Dist_extra_
			WriteBits(uint(dist_depth[dist_code]), uint64(dist_bits[dist_code]), storage_ix, storage)
			WriteBits(uint(distnumextra), uint64(distextra), storage_ix, storage)
		}
	}
}

func StoreMetaBlockTrivial(input []byte, start_pos uint, length uint, mask uint, is_last bool, params *common.EncoderParams, commands []metablock.Command, storage_ix *uint, storage []byte) {
	var lit_histo common.HistogramLiteral
	var cmd_histo common.HistogramCommand
	var dist_histo common.HistogramDistance
	var lit_depth [common.NumLiteralSymbols]byte
	var lit_bits [common.NumLiteralSymbols]uint16
	var cmd_depth [common.NumCommandSymbols]byte
	var cmd_bits [common.NumCommandSymbols]uint16
	var dist_depth [maxSimpleDistanceAlphabetSize]byte
	var dist_bits [maxSimpleDistanceAlphabetSize]uint16
	var tree []HuffmanTree
	var num_distance_symbols uint32 = params.Dist.Alphabet_size

	storeCompressedMetaBlockHeader(is_last, length, storage_ix, storage)

	common.HistogramClearLiteral(&lit_histo)
	common.HistogramClearCommand(&cmd_histo)
	common.HistogramClearDistance(&dist_histo)

	buildHistograms(input, start_pos, mask, commands, &lit_histo, &cmd_histo, &dist_histo)

	WriteBits(13, 0, storage_ix, storage)

	tree = make([]HuffmanTree, maxHuffmanTreeSize)
	BuildAndStoreHuffmanTree(lit_histo.Data_[:], common.NumLiteralSymbols, common.NumLiteralSymbols, tree, lit_depth[:], lit_bits[:], storage_ix, storage)
	BuildAndStoreHuffmanTree(cmd_histo.Data_[:], common.NumCommandSymbols, common.NumCommandSymbols, tree, cmd_depth[:], cmd_bits[:], storage_ix, storage)
	BuildAndStoreHuffmanTree(dist_histo.Data_[:], maxSimpleDistanceAlphabetSize, uint(num_distance_symbols), tree, dist_depth[:], dist_bits[:], storage_ix, storage)
	tree = nil
	storeDataWithHuffmanCodes(input, start_pos, mask, commands, lit_depth[:], lit_bits[:], cmd_depth[:], cmd_bits[:], dist_depth[:], dist_bits[:], storage_ix, storage)
	if is_last {
		JumpToByteBoundary(storage_ix, storage)
	}
}

func StoreMetaBlockFast(input []byte, start_pos uint, length uint, mask uint, is_last bool, params *common.EncoderParams, commands []metablock.Command, storage_ix *uint, storage []byte) {
	var num_distance_symbols uint32 = params.Dist.Alphabet_size
	var distance_alphabet_bits uint32 = common.Log2FloorNonZero(uint(num_distance_symbols-1)) + 1

	storeCompressedMetaBlockHeader(is_last, length, storage_ix, storage)

	WriteBits(13, 0, storage_ix, storage)

	if len(commands) <= 128 {
		var histogram = [common.NumLiteralSymbols]uint32{0}
		var pos uint = start_pos
		var num_literals uint = 0
		var lit_depth [common.NumLiteralSymbols]byte
		var lit_bits [common.NumLiteralSymbols]uint16
		for _, cmd := range commands {
			var j uint
			for j = uint(cmd.Insert_len_); j != 0; j-- {
				histogram[input[pos&mask]]++
				pos++
			}

			num_literals += uint(cmd.Insert_len_)
			pos += uint(metablock.CommandCopyLen(&cmd))
		}

		BuildAndStoreHuffmanTreeFast(histogram[:], num_literals, /* max_bits = */
			8, lit_depth[:], lit_bits[:], storage_ix, storage)

		StoreStaticCommandHuffmanTree(storage_ix, storage)
		StoreStaticDistanceHuffmanTree(storage_ix, storage)
		storeDataWithHuffmanCodes(input, start_pos, mask, commands, lit_depth[:], lit_bits[:], staticCommandCodeDepth[:], staticCommandCodeBits[:], staticDistanceCodeDepth[:], staticDistanceCodeBits[:], storage_ix, storage)
	} else {
		var lit_histo common.HistogramLiteral
		var cmd_histo common.HistogramCommand
		var dist_histo common.HistogramDistance
		var lit_depth [common.NumLiteralSymbols]byte
		var lit_bits [common.NumLiteralSymbols]uint16
		var cmd_depth [common.NumCommandSymbols]byte
		var cmd_bits [common.NumCommandSymbols]uint16
		var dist_depth [maxSimpleDistanceAlphabetSize]byte
		var dist_bits [maxSimpleDistanceAlphabetSize]uint16
		common.HistogramClearLiteral(&lit_histo)
		common.HistogramClearCommand(&cmd_histo)
		common.HistogramClearDistance(&dist_histo)
		buildHistograms(input, start_pos, mask, commands, &lit_histo, &cmd_histo, &dist_histo)
		BuildAndStoreHuffmanTreeFast(lit_histo.Data_[:], lit_histo.Total_count_, /* max_bits = */
			8, lit_depth[:], lit_bits[:], storage_ix, storage)

		BuildAndStoreHuffmanTreeFast(cmd_histo.Data_[:], cmd_histo.Total_count_, /* max_bits = */
			10, cmd_depth[:], cmd_bits[:], storage_ix, storage)

		BuildAndStoreHuffmanTreeFast(dist_histo.Data_[:], dist_histo.Total_count_, /* max_bits = */
			uint(distance_alphabet_bits), dist_depth[:], dist_bits[:], storage_ix, storage)

		storeDataWithHuffmanCodes(input, start_pos, mask, commands, lit_depth[:], lit_bits[:], cmd_depth[:], cmd_bits[:], dist_depth[:], dist_bits[:], storage_ix, storage)
	}

	if is_last {
		JumpToByteBoundary(storage_ix, storage)
	}
}

/*
This is for storing uncompressed blocks (simple raw storage of

	bytes-as-bytes).
*/
func StoreUncompressedMetaBlock(is_final_block bool, input []byte, position uint, mask uint, length uint, storage_ix *uint, storage []byte) {
	var masked_pos uint = position & mask
	storeUncompressedMetaBlockHeader(uint(length), storage_ix, storage)
	JumpToByteBoundary(storage_ix, storage)

	if masked_pos+length > mask+1 {
		var len1 uint = mask + 1 - masked_pos
		copy(storage[*storage_ix>>3:], input[masked_pos:][:len1])
		*storage_ix += len1 << 3
		length -= len1
		masked_pos = 0
	}

	copy(storage[*storage_ix>>3:], input[masked_pos:][:length])
	*storage_ix += uint(length << 3)

	/* We need to clear the next 4 bytes to continue to be
	   compatible with BrotliWriteBits. */
	WriteBitsPrepareStorage(*storage_ix, storage)

	/* Since the uncompressed block itself may not be the final block, add an
	   empty one after this. */
	if is_final_block {
		WriteBits(1, 1, storage_ix, storage) /* islast */
		WriteBits(1, 1, storage_ix, storage) /* isempty */
		JumpToByteBoundary(storage_ix, storage)
	}
}

// StoreMetaBlockHeaderBW stores a compressed meta-block header using a BitWriter.
func StoreMetaBlockHeaderBW(length uint, is_uncompressed bool, bw *BitWriter) {
	var lenbits uint64
	var nlenbits uint
	var nibblesbits uint64
	bw.WriteBits(1, 0) // ISLAST = 0
	encodeMlen(length, &lenbits, &nlenbits, &nibblesbits)
	bw.WriteBits(2, nibblesbits)
	bw.WriteBits(nlenbits, lenbits)
	if is_uncompressed {
		bw.WriteBits(1, 1) // ISUNCOMPRESSED = 1
	} else {
		bw.WriteBits(1, 0)
	}
}
