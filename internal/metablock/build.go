package metablock

import (
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/context"
)

const maxStaticContexts = 13

var buildMetaBlock_kMaxNumberOfHistograms uint = 256

type blockSplitIterator struct {
	split_  *BlockSplit
	idx_    uint
	type_   uint
	length_ uint
}

func initBlockSplitIterator(self *blockSplitIterator, split *BlockSplit) {
	self.split_ = split
	self.idx_ = 0
	self.type_ = 0
	if len(split.Lengths) > 0 {
		self.length_ = uint(split.Lengths[0])
	} else {
		self.length_ = 0
	}
}

func blockSplitIteratorNext(self *blockSplitIterator) {
	if self.length_ == 0 {
		self.idx_++
		self.type_ = uint(self.split_.Types[self.idx_])
		self.length_ = uint(self.split_.Lengths[self.idx_])
	}
	self.length_--
}

func buildHistogramsWithContext(cmds []Command, literal_split *BlockSplit, insert_and_copy_split *BlockSplit, dist_split *BlockSplit, ringbuffer []byte, start_pos uint, mask uint, prev_byte byte, prev_byte2 byte, context_modes []int, literal_histograms []common.HistogramLiteral, insert_and_copy_histograms []common.HistogramCommand, copy_dist_histograms []common.HistogramDistance) {
	var pos uint = start_pos
	var literal_it blockSplitIterator
	var insert_and_copy_it blockSplitIterator
	var dist_it blockSplitIterator

	initBlockSplitIterator(&literal_it, literal_split)
	initBlockSplitIterator(&insert_and_copy_it, insert_and_copy_split)
	initBlockSplitIterator(&dist_it, dist_split)
	for i := range cmds {
		var cmd *Command = &cmds[i]
		var j uint
		blockSplitIteratorNext(&insert_and_copy_it)
		common.HistogramAddCommand(&insert_and_copy_histograms[insert_and_copy_it.type_], uint(cmd.Cmd_prefix_))

		for j = uint(cmd.Insert_len_); j != 0; j-- {
			var ctx uint
			blockSplitIteratorNext(&literal_it)
			ctx = literal_it.type_
			if context_modes != nil {
				var lut context.ContextLUT = context.GetContextLUT(context_modes[ctx])
				ctx = (ctx << common.LiteralContextBits) + uint(context.GetContext(prev_byte, prev_byte2, lut))
			}

			common.HistogramAddLiteral(&literal_histograms[ctx], uint(ringbuffer[pos&mask]))
			prev_byte2 = prev_byte
			prev_byte = ringbuffer[pos&mask]
			pos++
		}

		pos += uint(CommandCopyLen(cmd))
		if CommandCopyLen(cmd) != 0 {
			prev_byte2 = ringbuffer[(pos-2)&mask]
			prev_byte = ringbuffer[(pos-1)&mask]
			if cmd.Cmd_prefix_ >= 128 {
				var ctx uint
				blockSplitIteratorNext(&dist_it)
				ctx = uint(uint32(dist_it.type_<<common.DistanceContextBits) + CommandDistanceContext(cmd))
				common.HistogramAddDistance(&copy_dist_histograms[ctx], uint(cmd.Dist_prefix_)&0x3FF)
			}
		}
	}
}

type contextBlockSplitter struct {
	alphabet_size_     uint
	num_contexts_      uint
	max_block_types_   uint
	min_block_size_    uint
	split_threshold_   float64
	num_blocks_        uint
	split_             *BlockSplit
	histograms_        []common.HistogramLiteral
	histograms_size_   *uint
	target_block_size_ uint
	block_size_        uint
	curr_histogram_ix_ uint
	last_histogram_ix_ [2]uint
	last_entropy_      [2 * maxStaticContexts]float64
	merge_last_count_  uint
}

func initContextBlockSplitter(self *contextBlockSplitter, alphabet_size uint, num_contexts uint, min_block_size uint, split_threshold float64, num_symbols uint, split *BlockSplit, histograms *[]common.HistogramLiteral, histograms_size *uint) {
	var max_num_blocks uint = num_symbols/min_block_size + 1
	var max_num_types uint
	common.Assert(num_contexts <= maxStaticContexts)

	self.alphabet_size_ = alphabet_size
	self.num_contexts_ = num_contexts
	self.max_block_types_ = common.MaxNumberOfBlockTypes / num_contexts
	self.min_block_size_ = min_block_size
	self.split_threshold_ = split_threshold
	self.num_blocks_ = 0
	self.split_ = split
	self.histograms_size_ = histograms_size
	self.target_block_size_ = min_block_size
	self.block_size_ = 0
	self.curr_histogram_ix_ = 0
	self.merge_last_count_ = 0

	max_num_types = min(max_num_blocks, self.max_block_types_+1)

	common.EnsureCapacity(&split.Types, &split.Types_alloc_size, max_num_blocks)
	common.EnsureCapacity(&split.Lengths, &split.Lengths_alloc_size, max_num_blocks)
	split.Types = split.Types[:max_num_blocks]
	split.Lengths = split.Lengths[:max_num_blocks]
	split.Num_blocks = max_num_blocks
	*histograms_size = max_num_types * num_contexts
	if histograms == nil || cap(*histograms) < int(*histograms_size) {
		*histograms = make([]common.HistogramLiteral, (*histograms_size))
	} else {
		*histograms = (*histograms)[:*histograms_size]
	}
	self.histograms_ = *histograms

	common.ClearHistogramsLiteral(self.histograms_[0:], num_contexts)

	self.last_histogram_ix_[1] = 0
	self.last_histogram_ix_[0] = self.last_histogram_ix_[1]
}

func contextBlockSplitterFinishBlock(self *contextBlockSplitter, is_final bool) {
	var split *BlockSplit = self.split_
	var num_contexts uint = self.num_contexts_
	var last_entropy []float64 = self.last_entropy_[:]
	var histograms []common.HistogramLiteral = self.histograms_

	if self.block_size_ < self.min_block_size_ {
		self.block_size_ = self.min_block_size_
	}

	if self.num_blocks_ == 0 {
		var i uint
		split.Lengths[0] = uint32(self.block_size_)
		split.Types[0] = 0

		for i = 0; i < num_contexts; i++ {
			last_entropy[i] = common.BitsEntropy(histograms[i].Data_[:], self.alphabet_size_)
			last_entropy[num_contexts+i] = last_entropy[i]
		}

		self.num_blocks_++
		split.Num_types++
		self.curr_histogram_ix_ += num_contexts
		if self.curr_histogram_ix_ < *self.histograms_size_ {
			common.ClearHistogramsLiteral(self.histograms_[self.curr_histogram_ix_:], self.num_contexts_)
		}
		self.block_size_ = 0
	} else if self.block_size_ > 0 {
		var entropy [maxStaticContexts]float64
		var combined_histo = make([]common.HistogramLiteral, (2 * num_contexts))
		var combined_entropy [2 * maxStaticContexts]float64
		var diff = [2]float64{0.0}
		var i uint
		for i = 0; i < num_contexts; i++ {
			var curr_histo_ix uint = self.curr_histogram_ix_ + i
			var j uint
			entropy[i] = common.BitsEntropy(histograms[curr_histo_ix].Data_[:], self.alphabet_size_)
			for j = 0; j < 2; j++ {
				var jx uint = j*num_contexts + i
				var last_histogram_ix uint = self.last_histogram_ix_[j] + i
				combined_histo[jx] = histograms[curr_histo_ix]
				common.HistogramAddHistogramLiteral(&combined_histo[jx], &histograms[last_histogram_ix])
				combined_entropy[jx] = common.BitsEntropy(combined_histo[jx].Data_[0:], self.alphabet_size_)
				diff[j] += combined_entropy[jx] - entropy[i] - last_entropy[jx]
			}
		}

		if split.Num_types < self.max_block_types_ && diff[0] > self.split_threshold_ && diff[1] > self.split_threshold_ {
			split.Lengths[self.num_blocks_] = uint32(self.block_size_)
			split.Types[self.num_blocks_] = byte(split.Num_types)
			self.last_histogram_ix_[1] = self.last_histogram_ix_[0]
			self.last_histogram_ix_[0] = split.Num_types * num_contexts
			for i = 0; i < num_contexts; i++ {
				last_entropy[num_contexts+i] = last_entropy[i]
				last_entropy[i] = entropy[i]
			}
			self.num_blocks_++
			split.Num_types++
			self.curr_histogram_ix_ += num_contexts
			if self.curr_histogram_ix_ < *self.histograms_size_ {
				common.ClearHistogramsLiteral(self.histograms_[self.curr_histogram_ix_:], self.num_contexts_)
			}
			self.block_size_ = 0
			self.merge_last_count_ = 0
			self.target_block_size_ = self.min_block_size_
		} else if diff[1] < diff[0]-20.0 {
			split.Lengths[self.num_blocks_] = uint32(self.block_size_)
			split.Types[self.num_blocks_] = split.Types[self.num_blocks_-2]
			var tmp uint = self.last_histogram_ix_[0]
			self.last_histogram_ix_[0] = self.last_histogram_ix_[1]
			self.last_histogram_ix_[1] = tmp
			for i = 0; i < num_contexts; i++ {
				histograms[self.last_histogram_ix_[0]+i] = combined_histo[num_contexts+i]
				last_entropy[num_contexts+i] = last_entropy[i]
				last_entropy[i] = combined_entropy[num_contexts+i]
				common.HistogramClearLiteral(&histograms[self.curr_histogram_ix_+i])
			}
			self.num_blocks_++
			self.block_size_ = 0
			self.merge_last_count_ = 0
			self.target_block_size_ = self.min_block_size_
		} else {
			split.Lengths[self.num_blocks_-1] += uint32(self.block_size_)
			for i = 0; i < num_contexts; i++ {
				histograms[self.last_histogram_ix_[0]+i] = combined_histo[i]
				last_entropy[i] = combined_entropy[i]
				if split.Num_types == 1 {
					last_entropy[num_contexts+i] = last_entropy[i]
				}
				common.HistogramClearLiteral(&histograms[self.curr_histogram_ix_+i])
			}
			self.block_size_ = 0
			self.merge_last_count_++
			if self.merge_last_count_ > 1 {
				self.target_block_size_ += self.min_block_size_
			}
		}
	}

	if is_final {
		*self.histograms_size_ = split.Num_types * num_contexts
		split.Num_blocks = self.num_blocks_
	}
}

func contextBlockSplitterAddSymbol(self *contextBlockSplitter, symbol uint, ctx uint) {
	common.HistogramAddLiteral(&self.histograms_[self.curr_histogram_ix_+ctx], symbol)
	self.block_size_++
	if self.block_size_ == self.target_block_size_ {
		contextBlockSplitterFinishBlock(self, false)
	}
}

func mapStaticContexts(num_contexts uint, static_context_map []uint32, mb *MetaBlockSplit) {
	var i uint
	mb.Literal_context_map_size = mb.Literal_split.Num_types << common.LiteralContextBits
	if cap(mb.Literal_context_map) < int(mb.Literal_context_map_size) {
		mb.Literal_context_map = make([]uint32, (mb.Literal_context_map_size))
	} else {
		mb.Literal_context_map = mb.Literal_context_map[:mb.Literal_context_map_size]
	}
	for i = 0; i < mb.Literal_split.Num_types; i++ {
		var offset uint32 = uint32(i * num_contexts)
		var j uint
		for j = 0; j < 1<<common.LiteralContextBits; j++ {
			mb.Literal_context_map[(i<<common.LiteralContextBits)+j] = offset + static_context_map[j]
		}
	}
}

func buildMetaBlockGreedyInternal(ringbuffer []byte, pos uint, mask uint, prev_byte byte, prev_byte2 byte, literal_context_lut context.ContextLUT, num_contexts uint, static_context_map []uint32, commands []Command, mb *MetaBlockSplit) {
	var lit_blocks struct {
		plain blockSplitterLiteral
		ctx   contextBlockSplitter
	}
	var cmd_blocks blockSplitterCommand
	var dist_blocks blockSplitterDistance
	var num_literals uint = 0
	for i := range commands {
		num_literals += uint(commands[i].Insert_len_)
	}

	if num_contexts == 1 {
		initBlockSplitterLiteral(&lit_blocks.plain, 256, 512, 400.0, num_literals, &mb.Literal_split, &mb.Literal_histograms, &mb.Literal_histograms_size)
	} else {
		initContextBlockSplitter(&lit_blocks.ctx, 256, num_contexts, 512, 400.0, num_literals, &mb.Literal_split, &mb.Literal_histograms, &mb.Literal_histograms_size)
	}

	initBlockSplitterCommand(&cmd_blocks, common.NumCommandSymbols, 1024, 500.0, uint(len(commands)), &mb.Command_split, &mb.Command_histograms, &mb.Command_histograms_size)
	initBlockSplitterDistance(&dist_blocks, 64, 512, 100.0, uint(len(commands)), &mb.Distance_split, &mb.Distance_histograms, &mb.Distance_histograms_size)

	for _, cmd := range commands {
		var j uint
		blockSplitterAddSymbolCommand(&cmd_blocks, uint(cmd.Cmd_prefix_))
		for j = uint(cmd.Insert_len_); j != 0; j-- {
			var literal byte = ringbuffer[pos&mask]
			if num_contexts == 1 {
				blockSplitterAddSymbolLiteral(&lit_blocks.plain, uint(literal))
			} else {
				var ctx uint = uint(context.GetContext(prev_byte, prev_byte2, literal_context_lut))
				contextBlockSplitterAddSymbol(&lit_blocks.ctx, uint(literal), uint(static_context_map[ctx]))
			}
			prev_byte2 = prev_byte
			prev_byte = literal
			pos++
		}
		pos += uint(CommandCopyLen(&cmd))
		if CommandCopyLen(&cmd) != 0 {
			prev_byte2 = ringbuffer[(pos-2)&mask]
			prev_byte = ringbuffer[(pos-1)&mask]
			if cmd.Cmd_prefix_ >= 128 {
				blockSplitterAddSymbolDistance(&dist_blocks, uint(cmd.Dist_prefix_)&0x3FF)
			}
		}
	}

	if num_contexts == 1 {
		blockSplitterFinishBlockLiteral(&lit_blocks.plain, true)
	} else {
		contextBlockSplitterFinishBlock(&lit_blocks.ctx, true)
	}
	blockSplitterFinishBlockCommand(&cmd_blocks, true)
	blockSplitterFinishBlockDistance(&dist_blocks, true)

	if num_contexts > 1 {
		mapStaticContexts(num_contexts, static_context_map, mb)
	}
}

// BuildMetaBlockGreedy builds a meta-block using greedy block splitting.
func BuildMetaBlockGreedy(ringbuffer []byte, pos uint, mask uint, prev_byte byte, prev_byte2 byte, literal_context_lut context.ContextLUT, num_contexts uint, static_context_map []uint32, commands []Command, mb *MetaBlockSplit) {
	if num_contexts == 1 {
		buildMetaBlockGreedyInternal(ringbuffer, pos, mask, prev_byte, prev_byte2, literal_context_lut, 1, nil, commands, mb)
	} else {
		buildMetaBlockGreedyInternal(ringbuffer, pos, mask, prev_byte, prev_byte2, literal_context_lut, num_contexts, static_context_map, commands, mb)
	}
}

// BuildMetaBlock builds a meta-block using context modeling.
func BuildMetaBlock(ringbuffer []byte, pos uint, mask uint, params *common.EncoderParams, prev_byte byte, prev_byte2 byte, cmds []Command, literal_context_mode int, mb *MetaBlockSplit) {
	var distance_histograms []common.HistogramDistance
	var literal_histograms []common.HistogramLiteral
	var literal_context_modes []int = nil
	var literal_histograms_size uint
	var distance_histograms_size uint
	var i uint
	var literal_context_multiplier uint = 1
	var npostfix uint32
	var ndirect_msb uint32 = 0
	var check_orig bool = true
	var best_dist_cost float64 = 1e99
	var orig_params common.EncoderParams = *params
	var new_params common.EncoderParams = *params

	for npostfix = 0; npostfix <= common.MaxNpostfix; npostfix++ {
		for ; ndirect_msb < 16; ndirect_msb++ {
			var ndirect uint32 = ndirect_msb << npostfix
			var skip bool
			var dist_cost float64
			InitDistanceParams(&new_params, npostfix, ndirect)
			if npostfix == orig_params.Dist.Distance_postfix_bits && ndirect == orig_params.Dist.Num_direct_distance_codes {
				check_orig = false
			}
			skip = !computeDistanceCost(cmds, &orig_params.Dist, &new_params.Dist, &dist_cost)
			if skip || (dist_cost > best_dist_cost) {
				break
			}
			best_dist_cost = dist_cost
			params.Dist = new_params.Dist
		}
		if ndirect_msb > 0 {
			ndirect_msb--
		}
		ndirect_msb /= 2
	}

	if check_orig {
		var dist_cost float64
		computeDistanceCost(cmds, &orig_params.Dist, &orig_params.Dist, &dist_cost)
		if dist_cost < best_dist_cost {
			params.Dist = orig_params.Dist
		}
	}

	RecomputeDistancePrefixes(cmds, &orig_params.Dist, &params.Dist)
	SplitBlock(cmds, ringbuffer, pos, mask, params, &mb.Literal_split, &mb.Command_split, &mb.Distance_split)

	if !params.Disable_literal_context_modeling {
		literal_context_multiplier = 1 << common.LiteralContextBits
		literal_context_modes = make([]int, (mb.Literal_split.Num_types))
		for i = 0; i < mb.Literal_split.Num_types; i++ {
			literal_context_modes[i] = literal_context_mode
		}
	}

	literal_histograms_size = mb.Literal_split.Num_types * literal_context_multiplier
	literal_histograms = make([]common.HistogramLiteral, literal_histograms_size)
	common.ClearHistogramsLiteral(literal_histograms, literal_histograms_size)

	distance_histograms_size = mb.Distance_split.Num_types << common.DistanceContextBits
	distance_histograms = make([]common.HistogramDistance, distance_histograms_size)
	common.ClearHistogramsDistance(distance_histograms, distance_histograms_size)

	mb.Command_histograms_size = mb.Command_split.Num_types
	if cap(mb.Command_histograms) < int(mb.Command_histograms_size) {
		mb.Command_histograms = make([]common.HistogramCommand, (mb.Command_histograms_size))
	} else {
		mb.Command_histograms = mb.Command_histograms[:mb.Command_histograms_size]
	}
	common.ClearHistogramsCommand(mb.Command_histograms, mb.Command_histograms_size)

	buildHistogramsWithContext(cmds, &mb.Literal_split, &mb.Command_split, &mb.Distance_split, ringbuffer, pos, mask, prev_byte, prev_byte2, literal_context_modes, literal_histograms, mb.Command_histograms, distance_histograms)
	literal_context_modes = nil

	mb.Literal_context_map_size = mb.Literal_split.Num_types << common.LiteralContextBits
	if cap(mb.Literal_context_map) < int(mb.Literal_context_map_size) {
		mb.Literal_context_map = make([]uint32, (mb.Literal_context_map_size))
	} else {
		mb.Literal_context_map = mb.Literal_context_map[:mb.Literal_context_map_size]
	}

	mb.Literal_histograms_size = mb.Literal_context_map_size
	if cap(mb.Literal_histograms) < int(mb.Literal_histograms_size) {
		mb.Literal_histograms = make([]common.HistogramLiteral, (mb.Literal_histograms_size))
	} else {
		mb.Literal_histograms = mb.Literal_histograms[:mb.Literal_histograms_size]
	}

	ClusterHistogramsLiteral(literal_histograms, literal_histograms_size, buildMetaBlock_kMaxNumberOfHistograms, mb.Literal_histograms, &mb.Literal_histograms_size, mb.Literal_context_map)
	literal_histograms = nil

	if params.Disable_literal_context_modeling {
		for i = mb.Literal_split.Num_types; i != 0; {
			var j uint = 0
			i--
			for ; j < 1<<common.LiteralContextBits; j++ {
				mb.Literal_context_map[(i<<common.LiteralContextBits)+j] = mb.Literal_context_map[i]
			}
		}
	}

	mb.Distance_context_map_size = mb.Distance_split.Num_types << common.DistanceContextBits
	if cap(mb.Distance_context_map) < int(mb.Distance_context_map_size) {
		mb.Distance_context_map = make([]uint32, (mb.Distance_context_map_size))
	} else {
		mb.Distance_context_map = mb.Distance_context_map[:mb.Distance_context_map_size]
	}

	mb.Distance_histograms_size = mb.Distance_context_map_size
	if cap(mb.Distance_histograms) < int(mb.Distance_histograms_size) {
		mb.Distance_histograms = make([]common.HistogramDistance, (mb.Distance_histograms_size))
	} else {
		mb.Distance_histograms = mb.Distance_histograms[:mb.Distance_histograms_size]
	}

	ClusterHistogramsDistance(distance_histograms, mb.Distance_context_map_size, buildMetaBlock_kMaxNumberOfHistograms, mb.Distance_histograms, &mb.Distance_histograms_size, mb.Distance_context_map)
	distance_histograms = nil
}

func computeDistanceCost(cmds []Command, orig_params *common.DistanceParams, new_params *common.DistanceParams, cost *float64) bool {
	var equal_params bool = false
	var dist_prefix uint16
	var dist_extra uint32
	var extra_bits float64 = 0.0
	var histo common.HistogramDistance
	common.HistogramClearDistance(&histo)

	if orig_params.Distance_postfix_bits == new_params.Distance_postfix_bits && orig_params.Num_direct_distance_codes == new_params.Num_direct_distance_codes {
		equal_params = true
	}

	for i := range cmds {
		cmd := &cmds[i]
		if CommandCopyLen(cmd) != 0 && cmd.Cmd_prefix_ >= 128 {
			if equal_params {
				dist_prefix = cmd.Dist_prefix_
			} else {
				var distance uint32 = CommandRestoreDistanceCode(cmd, orig_params)
				if distance > uint32(new_params.Max_distance) {
					return false
				}
				common.PrefixEncodeCopyDistance(uint(distance), uint(new_params.Num_direct_distance_codes), uint(new_params.Distance_postfix_bits), &dist_prefix, &dist_extra)
			}
			common.HistogramAddDistance(&histo, uint(dist_prefix)&0x3FF)
			extra_bits += float64(dist_prefix >> 10)
		}
	}

	*cost = common.PopulationCostDistance(&histo) + extra_bits
	return true
}
