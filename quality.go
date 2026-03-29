package brotli

import (
	"github.com/nijaru/brotli/internal/common"
)

const fastOnePassCompressionQuality = 0

const fastTwoPassCompressionQuality = 1

const zopflificationQuality = 10

const hqZopflificationQuality = 11

const maxQualityForStaticEntropyCodes = 2

const minQualityForBlockSplit = 4

const minQualityForNonzeroDistanceParams = 4

const minQualityForOptimizeHistograms = 4

const minQualityForExtensiveReferenceSearch = 5

const minQualityForContextModeling = 5

const minQualityForHqContextModeling = 7

const minQualityForHqBlockSplitting = 10

/*
For quality below MIN_QUALITY_FOR_BLOCK_SPLIT there is no block splitting,

	so we buffer at most this much literals and commands.
*/
const maxNumDelayedSymbols = 0x2FFF

/* Returns hash-table size for quality levels 0 and 1. */
func maxHashTableSize(quality int) uint {
	if quality == fastOnePassCompressionQuality {
		return 1 << 15
	} else {
		return 1 << 17
	}
}

/* The maximum length for which the zopflification uses distinct distances. */
const maxZopfliLenQuality10 = 150

const maxZopfliLenQuality11 = 325

/* Do not thoroughly search when a long copy is found. */
const longCopyQuickStep = 16384

func maxZopfliLen(params *common.EncoderParams) uint {
	if params.Quality <= 10 {
		return maxZopfliLenQuality10
	} else {
		return maxZopfliLenQuality11
	}
}

/* Number of best candidates to evaluate to expand Zopfli chain. */
func maxZopfliCandidates(params *common.EncoderParams) uint {
	if params.Quality <= 10 {
		return 1
	} else {
		return 5
	}
}

func sanitizeParams(params *common.EncoderParams) {
	params.Quality = min(maxQuality, max(minQuality, params.Quality))
	if params.Quality <= maxQualityForStaticEntropyCodes {
		params.Large_window = false
	}

	if params.Lgwin < minWindowBits {
		params.Lgwin = minWindowBits
	} else {
		var max_lgwin int
		if params.Large_window {
			max_lgwin = largeMaxWindowBits
		} else {
			max_lgwin = maxWindowBits
		}
		if params.Lgwin > uint(max_lgwin) {
			params.Lgwin = uint(max_lgwin)
		}
	}
}

/* Returns optimized lg_block value. */
func computeLgBlock(params *common.EncoderParams) int {
	var lgblock int = params.Lgblock
	if params.Quality == fastOnePassCompressionQuality || params.Quality == fastTwoPassCompressionQuality {
		lgblock = int(params.Lgwin)
	} else if params.Quality < minQualityForBlockSplit {
		lgblock = 14
	} else if lgblock == 0 {
		lgblock = 16
		if params.Quality >= 9 && params.Lgwin > uint(lgblock) {
			lgblock = min(18, int(params.Lgwin))
		}
	} else {
		lgblock = min(maxInputBlockBits, max(minInputBlockBits, lgblock))
	}

	return lgblock
}

/*
Returns log2 of the size of main ring buffer area.

	Allocate at least lgwin + 1 bits for the ring buffer so that the newly
	added block fits there completely and we still get lgwin bits and at least
	read_block_size_bits + 1 bits because the copy tail length needs to be
	smaller than ring-buffer size.
*/
func computeRbBits(params *common.EncoderParams) int {
	return 1 + max(int(params.Lgwin), params.Lgblock)
}

func maxMetablockSize(params *common.EncoderParams) uint {
	var bits int = min(computeRbBits(params), maxInputBlockBits)
	return uint(1) << uint(bits)
}

/*
When searching for backward references and have not seen matches for a long

	time, we can skip some match lookups. Unsuccessful match lookups are very
	expensive and this kind of a heuristic speeds up compression quite a lot.
	At first 8 byte strides are taken and every second byte is put to hasher.
	After 4x more literals stride by 16 bytes, every put 4-th byte to hasher.
	Applied only to qualities 2 to 9.
*/
func literalSpreeLengthForSparseSearch(params *common.EncoderParams) uint {
	if params.Quality < 9 {
		return 64
	} else {
		return 512
	}
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
