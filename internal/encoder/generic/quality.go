package generic

import (
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/quality"
)

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

func maxZopfliCandidates(params *common.EncoderParams) uint {
	if params.Quality <= 10 {
		return 1
	} else {
		return 5
	}
}

func literalSpreeLengthForSparseSearch(params *common.EncoderParams) uint {
	if params.Quality < 9 {
		return 64
	} else {
		return 512
	}
}

func maxZopfliLen(params *common.EncoderParams) uint {
	if params.Quality <= 10 {
		return quality.MaxZopfliLenQuality10
	} else {
		return quality.MaxZopfliLenQuality11
	}
}

func sanitizeParams(params *common.EncoderParams) {
	params.Quality = min(quality.MaxQuality, max(quality.MinQuality, params.Quality))
	if params.Quality <= quality.MaxQualityForStaticEntropyCodes {
		params.Large_window = false
	}

	if params.Lgwin < quality.MinWindowBits {
		params.Lgwin = quality.MinWindowBits
	} else {
		var maxLgwin int
		if params.Large_window {
			maxLgwin = quality.LargeMaxWindowBits
		} else {
			maxLgwin = quality.MaxWindowBits
		}
		if params.Lgwin > uint(maxLgwin) {
			params.Lgwin = uint(maxLgwin)
		}
	}
}

func computeLgBlock(params *common.EncoderParams) int {
	lgblock := params.Lgblock
	if params.Quality == quality.Q0 || params.Quality == quality.Q1 {
		lgblock = int(params.Lgwin)
	} else if params.Quality < quality.MinQualityForBlockSplit {
		lgblock = 14
	} else if lgblock == 0 {
		lgblock = 16
		if params.Quality >= 9 && params.Lgwin > uint(lgblock) {
			lgblock = min(18, int(params.Lgwin))
		}
	} else {
		lgblock = min(quality.MaxInputBlockBits, max(quality.MinInputBlockBits, lgblock))
	}

	return lgblock
}

func maxMetablockSize(params *common.EncoderParams) uint {
	bits := min(computeRbBits(params), quality.MaxInputBlockBits)
	return uint(1) << uint(bits)
}

func computeRbBits(params *common.EncoderParams) int {
	return 1 + max(int(params.Lgwin), params.Lgblock)
}

func chooseHasher(params *common.EncoderParams, hparams *common.HasherParams) {
	if params.Quality > 9 {
		hparams.Type_ = 10
	} else if params.Quality == 4 && params.Size_hint >= 1<<20 {
		hparams.Type_ = 54
	} else if params.Quality < 5 {
		hparams.Type_ = params.Quality
	} else if params.Lgwin <= 16 {
		if params.Quality < 7 {
			hparams.Type_ = 40
		} else if params.Quality < 9 {
			hparams.Type_ = 41
		} else {
			hparams.Type_ = 42
		}
	} else if params.Size_hint >= 1<<20 && params.Lgwin >= 19 {
		hparams.Type_ = 6
		hparams.Block_bits = params.Quality - 1
		hparams.Bucket_bits = 15
		hparams.Hash_len = 5
		if params.Quality < 7 {
			hparams.Num_last_distances_to_check = 4
		} else if params.Quality < 9 {
			hparams.Num_last_distances_to_check = 10
		} else {
			hparams.Num_last_distances_to_check = 16
		}
	} else {
		hparams.Type_ = 5
		hparams.Block_bits = params.Quality - 1
		if params.Quality < 7 {
			hparams.Bucket_bits = 14
		} else {
			hparams.Bucket_bits = 15
		}
		if params.Quality < 7 {
			hparams.Num_last_distances_to_check = 4
		} else if params.Quality < 9 {
			hparams.Num_last_distances_to_check = 10
		} else {
			hparams.Num_last_distances_to_check = 16
		}
	}

	if params.Lgwin > 24 {
		/* Different hashers for large window brotli: not for qualities <= 2,
		   these are too fast for large window. Not for qualities >= 10: their
		   hasher already works well with large window. So the changes are:
		   H3 --> H35: for quality 3.
		   H54 --> H55: for quality 4 with size hint > 1MB
		   H6 --> H65: for qualities 5, 6, 7, 8, 9. */
		if hparams.Type_ == 3 {
			hparams.Type_ = 35
		}

		if hparams.Type_ == 54 {
			hparams.Type_ = 55
		}

		if hparams.Type_ == 6 {
			hparams.Type_ = 65
		}
	}
}
