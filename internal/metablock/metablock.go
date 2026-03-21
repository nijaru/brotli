package metablock

import (
	"sync"
	"github.com/nijaru/brotli/internal/common"
)

/* Copyright 2014 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Algorithms for distributing the literals and commands of a metablock between
   block types and contexts. */

type MetaBlockSplit struct {
	Literal_split             BlockSplit
	Command_split             BlockSplit
	Distance_split            BlockSplit
	Literal_context_map       []uint32
	Literal_context_map_size  uint
	Distance_context_map      []uint32
	Distance_context_map_size uint
	Literal_histograms        []common.HistogramLiteral
	Literal_histograms_size   uint
	Command_histograms        []common.HistogramCommand
	Command_histograms_size   uint
	Distance_histograms       []common.HistogramDistance
	Distance_histograms_size  uint
}

var metaBlockPool sync.Pool

func GetMetaBlockSplit() *MetaBlockSplit {
	mb, _ := metaBlockPool.Get().(*MetaBlockSplit)

	if mb == nil {
		mb = &MetaBlockSplit{}
	} else {
		InitBlockSplit(&mb.Literal_split)
		InitBlockSplit(&mb.Command_split)
		InitBlockSplit(&mb.Distance_split)
		mb.Literal_context_map = mb.Literal_context_map[:0]
		mb.Literal_context_map_size = 0
		mb.Distance_context_map = mb.Distance_context_map[:0]
		mb.Distance_context_map_size = 0
		mb.Literal_histograms = mb.Literal_histograms[:0]
		mb.Command_histograms = mb.Command_histograms[:0]
		mb.Distance_histograms = mb.Distance_histograms[:0]
	}
	return mb
}

func FreeMetaBlockSplit(mb *MetaBlockSplit) {
	metaBlockPool.Put(mb)
}

func InitDistanceParams(params *common.EncoderParams, npostfix uint32, ndirect uint32) {
	var dist_params *common.DistanceParams = &params.Dist
	var alphabet_size uint32
	var max_distance uint32

	dist_params.Distance_postfix_bits = npostfix
	dist_params.Num_direct_distance_codes = ndirect

	alphabet_size = uint32(common.DistanceAlphabetSize(uint(npostfix), uint(ndirect), common.MaxDistanceBits))
	max_distance = ndirect + (1 << (common.MaxDistanceBits + npostfix + 2)) - (1 << (npostfix + 2))

	if params.Large_window {
		var bound = [common.MaxNpostfix + 1]uint32{0, 4, 12, 28}
		var postfix uint32 = 1 << npostfix
		alphabet_size = uint32(common.DistanceAlphabetSize(uint(npostfix), uint(ndirect), common.LargeMaxDistanceBits))

		/* The maximum distance is set so that no distance symbol used can encode
		   a distance larger than BROTLI_MAX_ALLOWED_DISTANCE with all
		   its extra bits set. */
		if ndirect < bound[npostfix] {
			max_distance = common.MaxAllowedDistance - (bound[npostfix] - ndirect)
		} else if ndirect >= bound[npostfix]+postfix {
			max_distance = (3 << 29) - 4 + (ndirect - bound[npostfix])
		} else {
			max_distance = common.MaxAllowedDistance
		}
	}

	dist_params.Alphabet_size = alphabet_size
	dist_params.Max_distance = uint(max_distance)
}

func RecomputeDistancePrefixes(cmds []Command, orig_params *common.DistanceParams, new_params *common.DistanceParams) {
	if orig_params.Distance_postfix_bits == new_params.Distance_postfix_bits && orig_params.Num_direct_distance_codes == new_params.Num_direct_distance_codes {
		return
	}

	for i := range cmds {
		var cmd *Command = &cmds[i]
		if CommandCopyLen(cmd) != 0 && cmd.Cmd_prefix_ >= 128 {
			common.PrefixEncodeCopyDistance(uint(CommandRestoreDistanceCode(cmd, orig_params)), uint(new_params.Num_direct_distance_codes), uint(new_params.Distance_postfix_bits), &cmd.Dist_prefix_, &cmd.Dist_extra_)
		}
	}
}
