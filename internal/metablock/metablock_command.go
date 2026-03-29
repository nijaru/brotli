package metablock

import (
	"github.com/nijaru/brotli/internal/common"
)

/* Copyright 2015 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Greedy block splitter for one block category (literal, command or distance).
 */
type blockSplitterCommand struct {
	alphabet_size_     uint
	min_block_size_    uint
	split_threshold_   float64
	num_blocks_        uint
	split_             *BlockSplit
	histograms_        []common.HistogramCommand
	histograms_size_   *uint
	target_block_size_ uint
	block_size_        uint
	curr_histogram_ix_ uint
	last_histogram_ix_ [2]uint
	last_entropy_      [2]float64
	merge_last_count_  uint
}

func initBlockSplitterCommand(self *blockSplitterCommand, alphabet_size uint, min_block_size uint, split_threshold float64, num_symbols uint, split *BlockSplit, histograms *[]common.HistogramCommand, histograms_size *uint) {
	var max_num_blocks uint = num_symbols/min_block_size + 1
	var max_num_types uint = min(max_num_blocks, common.MaxNumberOfBlockTypes+1)
	/* We have to allocate one more histogram than the maximum number of block
	   types for the current histogram when the meta-block is too big. */
	self.alphabet_size_ = alphabet_size

	self.min_block_size_ = min_block_size
	self.split_threshold_ = split_threshold
	self.num_blocks_ = 0
	self.split_ = split
	self.histograms_size_ = histograms_size
	self.target_block_size_ = min_block_size
	self.block_size_ = 0
	self.curr_histogram_ix_ = 0
	self.merge_last_count_ = 0

	// Assuming these are moved to common or metablock.
	// For now, let's keep them as is or find them.
	// They were in util.go usually.
	common.BrotliEnsureCapacityUint8(&split.Types, &split.Types_alloc_size, max_num_blocks)
	common.BrotliEnsureCapacityUint32(&split.Lengths, &split.Lengths_alloc_size, max_num_blocks)
	split.Types = split.Types[:max_num_blocks]
	split.Lengths = split.Lengths[:max_num_blocks]
	self.split_.Num_blocks = max_num_blocks
	*histograms_size = max_num_types
	if histograms == nil || cap(*histograms) < int(*histograms_size) {
		*histograms = make([]common.HistogramCommand, (*histograms_size))
	} else {
		*histograms = (*histograms)[:*histograms_size]
	}
	self.histograms_ = *histograms

	/* Clear only current histogram. */
	common.HistogramClearCommand(&self.histograms_[0])

	self.last_histogram_ix_[1] = 0
	self.last_histogram_ix_[0] = self.last_histogram_ix_[1]
}

func blockSplitterFinishBlockCommand(self *blockSplitterCommand, is_final bool) {
	var split *BlockSplit = self.split_
	var last_entropy []float64 = self.last_entropy_[:]
	var histograms []common.HistogramCommand = self.histograms_

	if self.block_size_ < self.min_block_size_ {
		self.block_size_ = self.min_block_size_
	}

	if self.num_blocks_ == 0 {
		/* Create first block. */
		split.Lengths[0] = uint32(self.block_size_)

		split.Types[0] = 0
		last_entropy[0] = common.BitsEntropy(histograms[0].Data_[:], self.alphabet_size_)
		last_entropy[1] = last_entropy[0]
		self.num_blocks_++
		split.Num_types++
		self.curr_histogram_ix_++
		if self.curr_histogram_ix_ < *self.histograms_size_ {
			common.HistogramClearCommand(&histograms[self.curr_histogram_ix_])
		}
		self.block_size_ = 0
	} else if self.block_size_ > 0 {
		var entropy float64 = common.BitsEntropy(histograms[self.curr_histogram_ix_].Data_[:], self.alphabet_size_)
		var combined_histo [2]common.HistogramCommand
		var combined_entropy [2]float64
		var diff [2]float64
		var j uint
		for j = 0; j < 2; j++ {
			var last_histogram_ix uint = self.last_histogram_ix_[j]
			combined_histo[j] = histograms[self.curr_histogram_ix_]
			common.HistogramAddHistogramCommand(&combined_histo[j], &histograms[last_histogram_ix])
			combined_entropy[j] = common.BitsEntropy(combined_histo[j].Data_[0:], self.alphabet_size_)
			diff[j] = combined_entropy[j] - entropy - last_entropy[j]
		}

		if split.Num_types < common.MaxNumberOfBlockTypes && diff[0] > self.split_threshold_ && diff[1] > self.split_threshold_ {
			/* Create new block. */
			split.Lengths[self.num_blocks_] = uint32(self.block_size_)

			split.Types[self.num_blocks_] = byte(split.Num_types)
			self.last_histogram_ix_[1] = self.last_histogram_ix_[0]
			self.last_histogram_ix_[0] = uint(byte(split.Num_types))
			last_entropy[1] = last_entropy[0]
			last_entropy[0] = entropy
			self.num_blocks_++
			split.Num_types++
			self.curr_histogram_ix_++
			if self.curr_histogram_ix_ < *self.histograms_size_ {
				common.HistogramClearCommand(&histograms[self.curr_histogram_ix_])
			}
			self.block_size_ = 0
			self.merge_last_count_ = 0
			self.target_block_size_ = self.min_block_size_
		} else if diff[1] < diff[0]-20.0 {
			split.Lengths[self.num_blocks_] = uint32(self.block_size_)
			split.Types[self.num_blocks_] = split.Types[self.num_blocks_-2]
			/* Combine this block with second last block. */

			var tmp uint = self.last_histogram_ix_[0]
			self.last_histogram_ix_[0] = self.last_histogram_ix_[1]
			self.last_histogram_ix_[1] = tmp
			histograms[self.last_histogram_ix_[0]] = combined_histo[1]
			last_entropy[1] = last_entropy[0]
			last_entropy[0] = combined_entropy[1]
			self.num_blocks_++
			self.block_size_ = 0
			common.HistogramClearCommand(&histograms[self.curr_histogram_ix_])
			self.merge_last_count_ = 0
			self.target_block_size_ = self.min_block_size_
		} else {
			/* Combine this block with last block. */
			split.Lengths[self.num_blocks_-1] += uint32(self.block_size_)

			histograms[self.last_histogram_ix_[0]] = combined_histo[0]
			last_entropy[0] = combined_entropy[0]
			if split.Num_types == 1 {
				last_entropy[1] = last_entropy[0]
			}

			self.block_size_ = 0
			common.HistogramClearCommand(&histograms[self.curr_histogram_ix_])
			self.merge_last_count_++
			if self.merge_last_count_ > 1 {
				self.target_block_size_ += self.min_block_size_
			}
		}
	}

	if is_final {
		*self.histograms_size_ = split.Num_types
		split.Num_blocks = self.num_blocks_
	}
}

func blockSplitterAddSymbolCommand(self *blockSplitterCommand, symbol uint) {
	common.HistogramAddCommand(&self.histograms_[self.curr_histogram_ix_], symbol)
	self.block_size_++
	if self.block_size_ == self.target_block_size_ {
		blockSplitterFinishBlockCommand(self, false) /* is_final = */
	}
}
