package brotli

/* Copyright 2016 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Specification: 3.3. Alphabet sizes: insert-and-copy length */
const numLiteralSymbols = 256

const numCommandSymbols = 704

const numBlockLenSymbols = 26

/* Specification: 3.5. Complex prefix codes */
const repeatPreviousCodeLength = 16

const repeatZeroCodeLength = 17

const codeLengthCodes = (repeatZeroCodeLength + 1)

/* "code length of 8 is repeated" */
const initialRepeatedCodeLength = 8

/* "Large Window Brotli" */
const largeMaxDistanceBits = 62

const largeMinWbits = 10

const largeMaxWbits = 30

/* Specification: 4. Encoding of distances */
const numDistanceShortCodes = 16

const maxNpostfix = 3

const maxDistanceBits = 24

func distanceAlphabetSize(nPostfix, nDirect, maxNBits uint) uint {
	return numDistanceShortCodes + nDirect + uint(maxNBits<<(nPostfix+1))
}

const maxAllowedDistance = 0x7FFFFFFC

/* 7.1. Context modes and context ID lookup for literals */
/* "context IDs for literals are in the range of 0..63" */
const literalContextBits = 6

/* 7.2. Context ID for distances */
const distanceContextBits = 2

/* 9.1. Format of the Stream Header */
/* Number of slack bytes for window size. Don't confuse
   with BROTLI_NUM_DISTANCE_SHORT_CODES. */
const windowGap = 16
