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

func DistanceAlphabetSize(npostfix, ndirect, maxnbits uint) uint {
	return NumDistanceShortCodes + ndirect + uint(maxnbits<<(npostfix+1))
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
const WindowGap = 16

func MaxBackwardLimit(w uint) uint {
	return (uint(1) << w) - WindowGap
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
	{1, 2}, {5, 2}, {9, 2}, {13, 2}, {17, 3}, {25, 3}, {33, 3}, {41, 3}, {49, 4}, {65, 4}, {81, 4}, {97, 4}, {113, 5}, {145, 5}, {177, 5}, {209, 5}, {241, 6}, {305, 6}, {369, 7}, {497, 8}, {753, 9}, {1265, 10}, {2289, 11}, {4337, 12}, {8433, 13}, {16625, 24},
}

const HqZopflificationQuality = 10

func FindMatchLengthWithLimit(s1, s2 []byte, limit uint) uint {
	matched := uint(0)
	for matched < limit && s1[matched] == s2[matched] {
		matched++
	}
	return matched
}
