package encoder

import (
	"github.com/nijaru/brotli/internal/common"
)

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Block split point selection utilities. */

type BlockSplit struct {
	Num_types          uint
	Num_blocks         uint
	Types              []byte
	Lengths            []uint32
	Types_alloc_size   uint
	Lengths_alloc_size uint
}

const (
	kMaxLiteralHistograms        uint    = 100
	kMaxCommandHistograms        uint    = 50
	kLiteralBlockSwitchCost      float64 = 28.1
	kCommandBlockSwitchCost      float64 = 13.5
	kDistanceBlockSwitchCost     float64 = 14.6
	kLiteralStrideLength         uint    = 70
	kCommandStrideLength         uint    = 40
	kSymbolsPerLiteralHistogram  uint    = 544
	kSymbolsPerCommandHistogram  uint    = 530
	kSymbolsPerDistanceHistogram uint    = 544
	kMinLengthForBlockSplitting  uint    = 128
	kIterMulForRefining          uint    = 2
	kMinItersForRefining         uint    = 100
)

func countLiterals(cmds []Command) uint {
	var total_length uint = 0
	/* Count how many we have. */

	for i := range cmds {
		total_length += uint(cmds[i].Insert_len_)
	}

	return total_length
}

func copyLiteralsToByteArray(cmds []Command, data []byte, offset uint, mask uint, literals []byte) {
	var pos uint = 0
	var from_pos uint = offset & mask
	for i := range cmds {
		var insert_len uint = uint(cmds[i].Insert_len_)
		if from_pos+insert_len > mask {
			var head_size uint = mask + 1 - from_pos
			copy(literals[pos:], data[from_pos:][:head_size])
			from_pos = 0
			pos += head_size
			insert_len -= head_size
		}

		if insert_len > 0 {
			copy(literals[pos:], data[from_pos:][:insert_len])
			pos += insert_len
		}

		from_pos = uint((uint32(from_pos+insert_len) + CommandCopyLen(&cmds[i])) & uint32(mask))
	}
}

func myRand(seed *uint32) uint32 {
	/* Initial seed should be 7. In this case, loop length is (1 << 29). */
	*seed *= 16807

	return *seed
}

func bitCost(count uint) float64 {
	if count == 0 {
		return -2.0
	} else {
		return common.FastLog2(count)
	}
}

const histogramsPerBatch = 64

const clustersPerBatch = 16

func InitBlockSplit(self *BlockSplit) {
	self.Num_types = 0
	self.Num_blocks = 0
	self.Types = self.Types[:0]
	self.Lengths = self.Lengths[:0]
	self.Types_alloc_size = 0
	self.Lengths_alloc_size = 0
}

func SplitBlock(
	cmds []Command,
	data []byte,
	pos uint,
	mask uint,
	params *common.EncoderParams,
	mb *MetaBlockSplit,
) {
	literal_split := &mb.Literal_split
	insert_and_copy_split := &mb.Command_split
	dist_split := &mb.Distance_split

	{
		var literals_count uint = countLiterals(cmds)
		if cap(mb.Literals_buf) < int(literals_count) {
			mb.Literals_buf = make([]byte, literals_count)
		}
		var literals []byte = mb.Literals_buf[:literals_count]

		/* Create a continuous array of literals. */
		copyLiteralsToByteArray(cmds, data, pos, mask, literals)

		/* Create the block split on the array of literals.
		   Literal histograms have alphabet size 256. */
		splitByteVectorLiteral(
			literals,
			literals_count,
			kSymbolsPerLiteralHistogram,
			kMaxLiteralHistograms,
			kLiteralStrideLength,
			kLiteralBlockSwitchCost,
			params,
			literal_split,
		)
	}
	{
		cmd_count := len(cmds)
		if cap(mb.Insert_and_copy_codes_buf) < cmd_count {
			mb.Insert_and_copy_codes_buf = make([]uint16, cmd_count)
		}
		var insert_and_copy_codes []uint16 = mb.Insert_and_copy_codes_buf[:cmd_count]
		/* Compute prefix codes for commands. */

		for i := range cmds {
			insert_and_copy_codes[i] = cmds[i].Cmd_prefix_
		}

		/* Create the block split on the array of command prefixes. */
		splitByteVectorCommand(
			insert_and_copy_codes,
			kSymbolsPerCommandHistogram,
			kMaxCommandHistograms,
			kCommandStrideLength,
			kCommandBlockSwitchCost,
			params,
			insert_and_copy_split,
		)
	}
	{
		cmd_count := len(cmds)
		if cap(mb.Distance_prefixes_buf) < cmd_count {
			mb.Distance_prefixes_buf = make([]uint16, cmd_count)
		}
		var distance_prefixes []uint16 = mb.Distance_prefixes_buf[:cmd_count]
		var j uint = 0
		/* Create a continuous array of distance prefixes. */

		for i := range cmds {
			var cmd *Command = &cmds[i]
			if CommandCopyLen(cmd) != 0 && cmd.Cmd_prefix_ >= 128 {
				distance_prefixes[j] = cmd.Dist_prefix_ & 0x3FF
				j++
			}
		}

		/* Create the block split on the array of distance prefixes. */
		splitByteVectorDistance(
			distance_prefixes,
			j,
			kSymbolsPerDistanceHistogram,
			kMaxCommandHistograms,
			kCommandStrideLength,
			kDistanceBlockSwitchCost,
			params,
			dist_split,
		)
	}
}
