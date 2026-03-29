package bitstream

import (
	"github.com/nijaru/brotli/internal/common"
	"math"
)

/* Copyright 2010 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Entropy encoding (Huffman) utilities. */

/* A node of a Huffman tree. */
type HuffmanTree struct {
	Total_count_          uint32
	Index_left_           int16
	Index_right_or_value_ int16
}

func InitHuffmanTree(self *HuffmanTree, count uint32, left int16, right int16) {
	self.Total_count_ = count
	self.Index_left_ = left
	self.Index_right_or_value_ = right
}

/* Input size optimized Shell sort. */
type HuffmanTreeComparator func(HuffmanTree, HuffmanTree) bool

var SortHuffmanTreeItems_gaps = []uint{132, 57, 23, 10, 4, 1}

func SortHuffmanTreeItems(items []HuffmanTree, n uint, comparator HuffmanTreeComparator) {
	if n < 13 {
		/* Insertion sort. */
		var i uint
		for i = 1; i < n; i++ {
			var tmp HuffmanTree = items[i]
			var k uint = i
			var j uint = i - 1
			for comparator(tmp, items[j]) {
				items[k] = items[j]
				k = j
				if j == 0 {
					break
				}
				j--
			}

			items[k] = tmp
		}

		return
	} else {
		var g int
		if n < 57 {
			g = 2
		} else {
			g = 0
		}
		for ; g < 6; g++ {
			var gap uint = SortHuffmanTreeItems_gaps[g]
			var i uint
			for i = gap; i < n; i++ {
				var j uint = i
				var tmp HuffmanTree = items[i]
				for ; j >= gap && comparator(tmp, items[j-gap]); j -= gap {
					items[j] = items[j-gap]
				}

				items[j] = tmp
			}
		}
	}
}

/* Returns 1 if assignment of depths succeeded, otherwise 0. */
func SetDepth(p0 int, pool []HuffmanTree, depth []byte, max_depth int) bool {
	var stack [16]int
	var level int = 0
	var p int = p0
	stack[0] = -1
	for {
		if pool[p].Index_left_ >= 0 {
			level++
			if level > max_depth {
				return false
			}
			stack[level] = int(pool[p].Index_right_or_value_)
			p = int(pool[p].Index_left_)
			continue
		} else {
			depth[pool[p].Index_right_or_value_] = byte(level)
		}

		for level >= 0 && stack[level] == -1 {
			level--
		}
		if level < 0 {
			return true
		}
		p = stack[level]
		stack[level] = -1
	}
}

/* Sort the root nodes, least popular first. */
func SortHuffmanTree(v0 HuffmanTree, v1 HuffmanTree) bool {
	if v0.Total_count_ != v1.Total_count_ {
		return v0.Total_count_ < v1.Total_count_
	}

	return v0.Index_right_or_value_ > v1.Index_right_or_value_
}

func CreateHuffmanTree(data []uint32, length uint, tree_limit int, tree []HuffmanTree, depth []byte) {
	var count_limit uint32
	var sentinel HuffmanTree
	InitHuffmanTree(&sentinel, math.MaxUint32, -1, -1)

	for count_limit = 1; ; count_limit *= 2 {
		var n uint = 0
		var i uint
		var j uint
		var k uint
		for i = length; i != 0; {
			i--
			if data[i] != 0 {
				var count uint32 = min(data[i], count_limit)
				if data[i] > count_limit {
					count = data[i]
				} else {
					count = count_limit
				}
				InitHuffmanTree(&tree[n], count, -1, int16(i))
				n++
			}
		}

		if n == 1 {
			depth[tree[0].Index_right_or_value_] = 1 /* Only one element. */
			break
		}

		SortHuffmanTreeItems(tree, n, HuffmanTreeComparator(SortHuffmanTree))

		tree[n] = sentinel
		tree[n+1] = sentinel

		i = 0     /* Points to the next leaf node. */
		j = n + 1 /* Points to the next non-leaf node. */
		for k = n - 1; k != 0; k-- {
			var left uint
			var right uint
			if tree[i].Total_count_ <= tree[j].Total_count_ {
				left = i
				i++
			} else {
				left = j
				j++
			}

			if tree[i].Total_count_ <= tree[j].Total_count_ {
				right = i
				i++
			} else {
				right = j
				j++
			}
			{
				/* The sentinel node becomes the parent node. */
				var j_end uint = 2*n - k
				tree[j_end].Total_count_ = tree[left].Total_count_ + tree[right].Total_count_
				tree[j_end].Index_left_ = int16(left)
				tree[j_end].Index_right_or_value_ = int16(right)

				/* Add back the last sentinel node. */
				tree[j_end+1] = sentinel
			}
		}

		if SetDepth(int(2*n-1), tree[0:], depth, tree_limit) {
			break
		}
	}
}

func Reverse(v []byte, start uint, end uint) {
	end--
	for start < end {
		var tmp byte = v[start]
		v[start] = v[end]
		v[end] = tmp
		start++
		end--
	}
}

func WriteHuffmanTreeRepetitions(previous_value byte, value byte, repetitions uint, tree_size *uint, tree []byte, extra_bits_data []byte) {
	if previous_value != value {
		tree[*tree_size] = value
		extra_bits_data[*tree_size] = 0
		(*tree_size)++
		repetitions--
	}

	if repetitions == 7 {
		tree[*tree_size] = value
		extra_bits_data[*tree_size] = 0
		(*tree_size)++
		repetitions--
	}

	if repetitions < 3 {
		var i uint
		for i = 0; i < repetitions; i++ {
			tree[*tree_size] = value
			extra_bits_data[*tree_size] = 0
			(*tree_size)++
		}
	} else {
		var start uint = *tree_size
		repetitions -= 3
		for {
			tree[*tree_size] = common.RepeatPreviousCodeLength
			extra_bits_data[*tree_size] = byte(repetitions & 0x3)
			(*tree_size)++
			repetitions >>= 2
			if repetitions == 0 {
				break
			}

			repetitions--
		}

		Reverse(tree, start, *tree_size)
		Reverse(extra_bits_data, start, *tree_size)
	}
}

func WriteHuffmanTreeRepetitionsZeros(repetitions uint, tree_size *uint, tree []byte, extra_bits_data []byte) {
	if repetitions == 11 {
		tree[*tree_size] = 0
		extra_bits_data[*tree_size] = 0
		(*tree_size)++
		repetitions--
	}

	if repetitions < 3 {
		var i uint
		for i = 0; i < repetitions; i++ {
			tree[*tree_size] = 0
			extra_bits_data[*tree_size] = 0
			(*tree_size)++
		}
	} else {
		var start uint = *tree_size
		repetitions -= 3
		for {
			tree[*tree_size] = common.RepeatZeroCodeLength
			extra_bits_data[*tree_size] = byte(repetitions & 0x7)
			(*tree_size)++
			repetitions >>= 3
			if repetitions == 0 {
				break
			}

			repetitions--
		}

		Reverse(tree, start, *tree_size)
		Reverse(extra_bits_data, start, *tree_size)
	}
}

func OptimizeHuffmanCountsForRLE(length uint, counts []uint32, good_for_rle []byte) {
	var nonzero_count uint = 0
	var stride uint
	var limit uint
	var sum uint
	var streak_limit uint = 1240
	var i uint
	/* Let's make the Huffman code more compatible with RLE encoding. */
	for i = 0; i < length; i++ {
		if counts[i] != 0 {
			nonzero_count++
		}
	}

	if nonzero_count < 16 {
		return
	}

	for length != 0 && counts[length-1] == 0 {
		length--
	}

	if length == 0 {
		return /* All zeros. */
	}

	/* Now counts[0..length - 1] does not have trailing zeros. */
	{
		var nonzeros uint = 0
		var smallest_nonzero uint32 = 1 << 30
		for i = 0; i < length; i++ {
			if counts[i] != 0 {
				nonzeros++
				if smallest_nonzero > counts[i] {
					smallest_nonzero = counts[i]
				}
			}
		}

		if nonzeros < 5 {
			/* Small histogram will model it well. */
			return
		}

		if smallest_nonzero < 4 {
			var zeros uint = length - nonzeros
			if zeros < 6 {
				for i = 1; i < length-1; i++ {
					if counts[i-1] != 0 && counts[i] == 0 && counts[i+1] != 0 {
						counts[i] = 1
					}
				}
			}
		}

		if nonzeros < 28 {
			return
		}
	}

	/* 2) Let's mark all population counts that already can be encoded
	   with an RLE code. */
	for i := 0; i < int(length); i++ {
		good_for_rle[i] = 0
	}
	{
		var symbol uint32 = counts[0]
		/* Let's not spoil any of the existing good RLE codes.
		   Mark any seq of 0's that is longer as 5 as a good_for_rle.
		   Mark any seq of non-0's that is longer as 7 as a good_for_rle. */

		var step uint = 0
		for i = 0; i <= length; i++ {
			if i == length || counts[i] != symbol {
				if (symbol == 0 && step >= 5) || (symbol != 0 && step >= 7) {
					var k uint
					for k = 0; k < step; k++ {
						good_for_rle[i-k-1] = 1
					}
				}

				step = 1
				if i != length {
					symbol = counts[i]
				}
			} else {
				step++
			}
		}
	}

	/* 3) Let's replace those population counts that lead to more RLE codes.
	   Math here is in 24.8 fixed point representation. */
	stride = 0

	limit = uint(256*(counts[0]+counts[1]+counts[2])/3 + 420)
	sum = 0
	for i = 0; i <= length; i++ {
		if i == length || good_for_rle[i] != 0 || (i != 0 && good_for_rle[i-1] != 0) || (256*counts[i]-uint32(limit)+uint32(streak_limit)) >= uint32(2*streak_limit) {
			if stride >= 4 || (stride >= 3 && sum == 0) {
				var k uint
				var count uint = (sum + stride/2) / stride
				/* The stride must end, collapse what we have, if we have enough (4). */
				if count == 0 {
					count = 1
				}

				if sum == 0 {
					/* Don't make an all zeros stride to be upgraded to ones. */
					count = 0
				}

				for k = 0; k < stride; k++ {
					/* We don't want to change value at counts[i],
					   that is already belonging to the next stride. Thus - 1. */
					counts[i-k-1] = uint32(count)
				}
			}

			stride = 0
			sum = 0
			if i < length-2 {
				/* All interesting strides have a count of at least 4, */
				/* at least when non-zeros. */
				limit = uint(256*(counts[i]+counts[i+1]+counts[i+2])/3 + 420)
			} else if i < length {
				limit = uint(256 * counts[i])
			} else {
				limit = 0
			}
		}

		stride++
		if i != length {
			sum += uint(counts[i])
			if stride >= 4 {
				limit = (256*sum + stride/2) / stride
			}

			if stride == 4 {
				limit += 120
			}
		}
	}
}

func DecideOverRLEUse(depth []byte, length uint, use_rle_for_non_zero *bool, use_rle_for_zero *bool) {
	var total_reps_zero uint = 0
	var total_reps_non_zero uint = 0
	var count_reps_zero uint = 1
	var count_reps_non_zero uint = 1
	var i uint
	for i = 0; i < length; {
		var value byte = depth[i]
		var reps uint = 1
		var k uint
		for k = i + 1; k < length && depth[k] == value; k++ {
			reps++
		}

		if reps >= 3 && value == 0 {
			total_reps_zero += reps
			count_reps_zero++
		}

		if reps >= 4 && value != 0 {
			total_reps_non_zero += reps
			count_reps_non_zero++
		}

		i += reps
	}

	*use_rle_for_non_zero = total_reps_non_zero > count_reps_non_zero*2
	*use_rle_for_zero = total_reps_zero > count_reps_zero*2
}

func WriteHuffmanTree(depth []byte, length uint, tree_size *uint, tree []byte, extra_bits_data []byte) {
	var previous_value byte = common.InitialRepeatedCodeLength
	var i uint
	var use_rle_for_non_zero bool = false
	var use_rle_for_zero bool = false
	var new_length uint = length
	/* Throw away trailing zeros. */
	for i = 0; i < length; i++ {
		if depth[length-i-1] == 0 {
			new_length--
		} else {
			break
		}
	}

	/* First gather statistics on if it is a good idea to do RLE. */
	if length > 50 {
		/* Find RLE coding for longer codes.
		   Shorter codes seem not to benefit from RLE. */
		DecideOverRLEUse(depth, new_length, &use_rle_for_non_zero, &use_rle_for_zero)
	}

	/* Actual RLE coding. */
	for i = 0; i < new_length; {
		var value byte = depth[i]
		var reps uint = 1
		if (value != 0 && use_rle_for_non_zero) || (value == 0 && use_rle_for_zero) {
			var k uint
			for k = i + 1; k < new_length && depth[k] == value; k++ {
				reps++
			}
		}

		if value == 0 {
			WriteHuffmanTreeRepetitionsZeros(reps, tree_size, tree, extra_bits_data)
		} else {
			WriteHuffmanTreeRepetitions(previous_value, value, reps, tree_size, tree, extra_bits_data)
			previous_value = value
		}

		i += reps
	}
}

var ReverseBits_kLut = [16]uint{
	0x00, 0x08, 0x04, 0x0C, 0x02, 0x0A, 0x06, 0x0E, 0x01, 0x09, 0x05, 0x0D, 0x03, 0x0B, 0x07, 0x0F,
}

func ReverseBits(num_bits uint, bits uint16) uint16 {
	var retval uint = ReverseBits_kLut[bits&0x0F]
	var i uint
	for i = 4; i < num_bits; i += 4 {
		retval <<= 4
		bits = uint16(bits >> 4)
		retval |= ReverseBits_kLut[bits&0x0F]
	}

	retval >>= ((0 - num_bits) & 0x03)
	return uint16(retval)
}

/* 0..15 are values for bits */
const maxHuffmanBits = 16

/* Get the actual bit values for a tree of bit depths. */
func ConvertBitDepthsToSymbols(depth []byte, len uint, bits []uint16) {
	var bl_count = [maxHuffmanBits]uint16{0}
	var next_code [maxHuffmanBits]uint16
	var i uint
	/* In Brotli, all bit depths are [1..15]
	   0 bit depth means that the symbol does not exist. */

	var code int = 0
	for i = 0; i < len; i++ {
		bl_count[depth[i]]++
	}

	bl_count[0] = 0
	next_code[0] = 0
	for i = 1; i < maxHuffmanBits; i++ {
		code = (code + int(bl_count[i-1])) << 1
		next_code[i] = uint16(code)
	}

	for i = 0; i < len; i++ {
		if depth[i] != 0 {
			bits[i] = ReverseBits(uint(depth[i]), next_code[depth[i]])
			next_code[depth[i]]++
		}
	}
}
