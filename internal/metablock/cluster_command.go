package metablock

import (
	"math"

	"github.com/nijaru/brotli/internal/common"
)

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/*
Computes the bit cost reduction by combining out[idx1] and out[idx2] and if

	it is below a threshold, stores the pair (idx1, idx2) in the *pairs queue.
*/
func compareAndPushToQueueCommand(out []common.HistogramCommand, cluster_size []uint32, idx1 uint32, idx2 uint32, max_num_pairs uint, pairs []HistogramPair, num_pairs *uint) {
	var is_good_pair bool = false
	var p HistogramPair
	p.Idx2 = 0
	p.Idx1 = p.Idx2
	p.Cost_combo = 0
	p.Cost_diff = p.Cost_combo
	if idx1 == idx2 {
		return
	}

	if idx2 < idx1 {
		var t uint32 = idx2
		idx2 = idx1
		idx1 = t
	}

	p.Idx1 = idx1
	p.Idx2 = idx2
	p.Cost_diff = 0.5 * ClusterCostDiff(uint(cluster_size[idx1]), uint(cluster_size[idx2]))
	p.Cost_diff -= out[idx1].Bit_cost_
	p.Cost_diff -= out[idx2].Bit_cost_

	if out[idx1].Total_count_ == 0 {
		p.Cost_combo = out[idx2].Bit_cost_
		is_good_pair = true
	} else if out[idx2].Total_count_ == 0 {
		p.Cost_combo = out[idx1].Bit_cost_
		is_good_pair = true
	} else {
		var threshold float64
		if *num_pairs == 0 {
			threshold = 1e99
		} else {
			threshold = math.Max(0.0, pairs[0].Cost_diff)
		}
		var combo common.HistogramCommand = out[idx1]
		var cost_combo float64
		common.HistogramAddHistogramCommand(&combo, &out[idx2])
		cost_combo = common.PopulationCostCommand(&combo)
		if cost_combo < threshold-p.Cost_diff {
			p.Cost_combo = cost_combo
			is_good_pair = true
		}
	}

	if is_good_pair {
		p.Cost_diff += p.Cost_combo
		if *num_pairs > 0 && HistogramPairIsLess(&pairs[0], &p) {
			/* Replace the top of the queue if needed. */
			if *num_pairs < max_num_pairs {
				pairs[*num_pairs] = pairs[0]
				(*num_pairs)++
			}

			pairs[0] = p
		} else if *num_pairs < max_num_pairs {
			pairs[*num_pairs] = p
			(*num_pairs)++
		}
	}
}

func HistogramCombineCommand(out []common.HistogramCommand, cluster_size []uint32, symbols []uint32, clusters []uint32, pairs []HistogramPair, num_clusters uint, symbols_size uint, max_clusters uint, max_num_pairs uint) uint {
	var cost_diff_threshold float64 = 0.0
	var min_cluster_size uint = 1
	var num_pairs uint = 0
	{
		/* We maintain a vector of histogram pairs, with the property that the pair
		   with the maximum bit cost reduction is the first. */
		var idx1 uint
		for idx1 = 0; idx1 < num_clusters; idx1++ {
			var idx2 uint
			for idx2 = idx1 + 1; idx2 < num_clusters; idx2++ {
				compareAndPushToQueueCommand(out, cluster_size, clusters[idx1], clusters[idx2], max_num_pairs, pairs[0:], &num_pairs)
			}
		}
	}

	for num_clusters > min_cluster_size {
		var best_idx1 uint32
		var best_idx2 uint32
		var i uint
		if pairs[0].Cost_diff >= cost_diff_threshold {
			cost_diff_threshold = 1e99
			min_cluster_size = max_clusters
			continue
		}

		/* Take the best pair from the top of heap. */
		best_idx1 = pairs[0].Idx1

		best_idx2 = pairs[0].Idx2
		common.HistogramAddHistogramCommand(&out[best_idx1], &out[best_idx2])
		out[best_idx1].Bit_cost_ = pairs[0].Cost_combo
		cluster_size[best_idx1] += cluster_size[best_idx2]
		for i = 0; i < symbols_size; i++ {
			if symbols[i] == best_idx2 {
				symbols[i] = best_idx1
			}
		}

		for i = 0; i < num_clusters; i++ {
			if clusters[i] == best_idx2 {
				copy(clusters[i:], clusters[i+1:][:num_clusters-i-1])
				break
			}
		}

		num_clusters--
		{
			/* Remove pairs intersecting the just combined best pair. */
			var copy_to_idx uint = 0
			for i = 0; i < num_pairs; i++ {
				var p *HistogramPair = &pairs[i]
				if p.Idx1 == best_idx1 || p.Idx2 == best_idx1 || p.Idx1 == best_idx2 || p.Idx2 == best_idx2 {
					/* Remove invalid pair from the queue. */
					continue
				}

				if HistogramPairIsLess(&pairs[0], p) {
					/* Replace the top of the queue if needed. */
					var front HistogramPair = pairs[0]
					pairs[0] = *p
					pairs[copy_to_idx] = front
				} else {
					pairs[copy_to_idx] = *p
				}

				copy_to_idx++
			}

			num_pairs = copy_to_idx
		}

		/* Push new pairs formed with the combined histogram to the heap. */
		for i = 0; i < num_clusters; i++ {
			compareAndPushToQueueCommand(out, cluster_size, best_idx1, clusters[i], max_num_pairs, pairs[0:], &num_pairs)
		}
	}

	return num_clusters
}

/* What is the bit cost of moving histogram from cur_symbol to candidate. */
func HistogramBitCostDistanceCommand(histogram *common.HistogramCommand, candidate *common.HistogramCommand) float64 {
	if histogram.Total_count_ == 0 {
		return 0.0
	} else {
		var tmp common.HistogramCommand = *histogram
		common.HistogramAddHistogramCommand(&tmp, candidate)
		return common.PopulationCostCommand(&tmp) - candidate.Bit_cost_
	}
}
