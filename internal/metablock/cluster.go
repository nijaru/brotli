package metablock

import (
	"github.com/nijaru/brotli/internal/common"
)

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Functions for clustering similar histograms together. */

type HistogramPair struct {
	Idx1       uint32
	Idx2       uint32
	Cost_combo float64
	Cost_diff  float64
}

func HistogramPairIsLess(p1 *HistogramPair, p2 *HistogramPair) bool {
	if p1.Cost_diff != p2.Cost_diff {
		return p1.Cost_diff > p2.Cost_diff
	}

	return (p1.Idx2 - p1.Idx1) > (p2.Idx2 - p2.Idx1)
}

/* Returns entropy reduction of the context map when we combine two clusters. */
func ClusterCostDiff(size_a uint, size_b uint) float64 {
	var size_c uint = size_a + size_b
	return float64(size_a)*common.FastLog2(size_a) + float64(size_b)*common.FastLog2(size_b) - float64(size_c)*common.FastLog2(size_c)
}
