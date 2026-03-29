package common

/* Copyright 2017 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Specification: 7.3. Encoding of the context map */
const ContextMapMaxRle = 16

/* Specification: 2. Compressed representation overview */
const MaxNumberOfBlockTypes = 256

/* Specification: 3.3. Alphabet sizes: insert-and-copy length */
const NumLiteralSymbols = 256
const NumCommandSymbols = 704
const NumBlockLenSymbols = 26

const MaxContextMapSymbols = (MaxNumberOfBlockTypes + ContextMapMaxRle)
const MaxBlockTypeSymbols = (MaxNumberOfBlockTypes + 2)

/* Specification: 3.5. Complex prefix codes */
const RepeatPreviousCodeLength = 16
const RepeatZeroCodeLength = 17
const CodeLengthCodes = (RepeatZeroCodeLength + 1)

/* "code length of 8 is repeated" */
const InitialRepeatedCodeLength = 8

/* "Large Window Brotli" */
const LargeMaxDistanceBits = 62
const LargeMinWbits = 10
const LargeMaxWbits = 30

/* Specification: 4. Encoding of distances */
const NumDistanceShortCodes = 16
const MaxNpostfix = 3
const MaxNdirect = 120
const MaxDistanceBits = 24

func DistanceAlphabetSize(NPOSTFIX uint, NDIRECT uint, MAXNBITS uint) uint {
	return NumDistanceShortCodes + NDIRECT + uint(MAXNBITS<<(NPOSTFIX+1))
}

/* numDistanceSymbols == 1128 */
const NumDistanceSymbols = 1128
const MaxDistance = 0x3FFFFFC
const MaxAllowedDistance = 0x7FFFFFFC

/* 7.1. Context modes and context ID lookup for literals */
/* "context IDs for literals are in the range of 0..63" */
const LiteralContextBits = 6

/* 7.2. Context ID for distances */
const DistanceContextBits = 2

/* 9.1. Format of the Stream Header */
/* Number of slack bytes for window size. Don't confuse
   with BROTLI_NUM_DISTANCE_SHORT_CODES. */
const WindowGap = 16

func MaxBackwardLimit(W uint) uint {
	return (uint(1) << W) - WindowGap
}

type HasherParams struct {
	Type_                       int
	Bucket_bits                 int
	Block_bits                  int
	Hash_len                    int
	Num_last_distances_to_check int
}

type DistanceParams struct {
	Distance_postfix_bits     uint32
	Num_direct_distance_codes uint32
	Alphabet_size             uint32
	Max_distance              uint
}

/* Parameters for the Brotli encoder with chosen quality levels. */
type EncoderParams struct {
	Mode                             int
	Quality                          int
	Lgwin                            uint
	Lgblock                          int
	Size_hint                        uint
	Disable_literal_context_modeling bool
	Large_window                     bool
	Hasher                           HasherParams
	Dist                             DistanceParams
	Dictionary                       EncoderDictionary
}

type EncoderDictionary struct {
	// We might need to move dictionary and dictWord as well.
	// For now using any to avoid immediate cyclic dependencies if we move them later.
	Words                 any
	CutoffTransformsCount uint32
	CutoffTransforms      uint64
	Hash_table            []uint16
	Buckets               []uint16
	Dict_words            []any
}

func Assert(cond bool) {
	if !cond {
		panic("assertion failure")
	}
}

type PrefixCodeRange struct {
	Offset uint32
	Nbits  uint32
}

var KBlockLengthPrefixCode = [NumBlockLenSymbols]PrefixCodeRange{
	PrefixCodeRange{1, 2},
	PrefixCodeRange{5, 2},
	PrefixCodeRange{9, 2},
	PrefixCodeRange{13, 2},
	PrefixCodeRange{17, 3},
	PrefixCodeRange{25, 3},
	PrefixCodeRange{33, 3},
	PrefixCodeRange{41, 3},
	PrefixCodeRange{49, 4},
	PrefixCodeRange{65, 4},
	PrefixCodeRange{81, 4},
	PrefixCodeRange{97, 4},
	PrefixCodeRange{113, 5},
	PrefixCodeRange{145, 5},
	PrefixCodeRange{177, 5},
	PrefixCodeRange{209, 5},
	PrefixCodeRange{241, 6},
	PrefixCodeRange{305, 6},
	PrefixCodeRange{369, 7},
	PrefixCodeRange{497, 8},
	PrefixCodeRange{753, 9},
	PrefixCodeRange{1265, 10},
	PrefixCodeRange{2289, 11},
	PrefixCodeRange{4337, 12},
	PrefixCodeRange{8433, 13},
	PrefixCodeRange{16625, 24},
}

func ComputeRbBits(params *EncoderParams) int {
	window_bits := int(params.Lgwin)
	if params.Large_window && window_bits > 24 {
		window_bits = 24
	}
	return window_bits
}

const HqZopflificationQuality = 10

func FindMatchLengthWithLimit(s1 []byte, s2 []byte, limit uint) uint {
	matched := uint(0)
	for matched < limit && s1[matched] == s2[matched] {
		matched++
	}
	return matched
}

func EnsureCapacity[T any](s *[]T, offset *uint, capacity uint) {
	if *offset+capacity > uint(cap(*s)) {
		var new_size uint = uint(cap(*s))
		if new_size == 0 {
			new_size = capacity
		} else {
			for new_size < *offset+capacity {
				new_size *= 2
			}
		}
		new_s := make([]T, new_size)
		copy(new_s, *s)
		*s = new_s
	}
}

func PrefixEncodeCopyDistance(distance_code uint, num_direct_distance_codes uint, distance_postfix_bits uint, code *uint16, extra_bits *uint32) {
	if distance_code < NumDistanceShortCodes+num_direct_distance_codes {
		*code = uint16(distance_code)
		*extra_bits = 0
		return
	} else {
		var dist uint = (uint(1) << (distance_postfix_bits + 2)) + (distance_code - NumDistanceShortCodes - num_direct_distance_codes)
		var bucket uint = uint(Log2FloorNonZero(dist) - 1)
		var postfix_mask uint = (uint(1) << distance_postfix_bits) - 1
		var postfix uint = dist & postfix_mask
		var prefix uint = (dist >> bucket) & 1
		var offset uint = (2 + prefix) << bucket
		var nbits uint = bucket - distance_postfix_bits
		*code = uint16(nbits<<10 | (NumDistanceShortCodes + num_direct_distance_codes + ((2*(nbits-1) + prefix) << distance_postfix_bits) + postfix))
		*extra_bits = uint32((dist - offset) >> distance_postfix_bits)
		return
	}
}
