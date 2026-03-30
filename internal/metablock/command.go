package metablock

import (
	"github.com/nijaru/brotli/internal/common"
)

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

var kInsBase = []uint32{
	0, 1, 2, 3, 4, 5, 6, 8, 10, 14, 18, 26, 34, 50, 66, 98, 130, 194, 322, 578, 1090, 2114, 6210, 22594,
}

var kInsExtra = []uint32{
	0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 7, 8, 9, 10, 12, 14, 24,
}

var kCopyBase = []uint32{
	2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 14, 18, 22, 30, 38, 54, 70, 102, 134, 198, 326, 582, 1094, 2118,
}

var kCopyExtra = []uint32{
	0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 7, 8, 9, 10, 24,
}

func GetInsertLengthCode(insertLen uint) uint16 {
	if insertLen < 6 {
		return uint16(insertLen)
	} else if insertLen < 130 {
		nbits := common.Log2FloorNonZero(insertLen-2) - 1
		return uint16((nbits << 1) + uint32((insertLen-2)>>nbits) + 2)
	} else if insertLen < 2114 {
		return uint16(common.Log2FloorNonZero(insertLen-66) + 10)
	} else if insertLen < 6210 {
		return 21
	} else if insertLen < 22594 {
		return 22
	} else {
		return 23
	}
}

func GetCopyLengthCode(copyLen uint) uint16 {
	if copyLen < 10 {
		return uint16(copyLen - 2)
	} else if copyLen < 134 {
		nbits := common.Log2FloorNonZero(copyLen-6) - 1
		return uint16((nbits << 1) + uint32((copyLen-6)>>nbits) + 4)
	} else if copyLen < 2118 {
		return uint16(common.Log2FloorNonZero(copyLen-70) + 12)
	} else {
		return 23
	}
}

func CombineLengthCodes(insCode, copyCode uint16, useLastDistance bool) uint16 {
	bits64 := uint16(copyCode&0x7 | (insCode&0x7)<<3)
	if useLastDistance && insCode < 8 && copyCode < 16 {
		if copyCode < 8 {
			return bits64
		}
		return bits64 | 64
	}

	/* Specification: 5 Encoding of ... (last table) */
	/* offset = 2 * index, where index is in range [0..8] */
	offset := 2 * ((uint32(copyCode) >> 3) + 3*(uint32(insCode)>>3))

	/* All values in specification are K * 64,
	   where   K = [2, 3, 6, 4, 5, 8, 7, 9, 10],
	       i + 1 = [1, 2, 3, 4, 5, 6, 7, 8,  9],
	   K - i - 1 = [1, 1, 3, 0, 0, 2, 0, 1,  2] = D.
	   All values in D require only 2 bits to encode.
	   Magic constant is shifted 6 bits left, to avoid final multiplication. */
	offset = (offset << 5) + 0x40 + ((0x520D40 >> offset) & 0xC0)

	return uint16(offset | uint32(bits64))
}

func GetInsertBase(insCode uint16) uint32 {
	return kInsBase[insCode]
}

func GetInsertExtra(insCode uint16) uint32 {
	return kInsExtra[insCode]
}

func GetCopyBase(copyCode uint16) uint32 {
	return kCopyBase[copyCode]
}

func GetCopyExtra(copyCode uint16) uint32 {
	return kCopyExtra[copyCode]
}

type DistanceCode struct {
	Code      int
	NExtra    uint
	ExtraBits uint64
}

func GetDistanceCode(distance int) DistanceCode {
	d := uint(distance + 3)
	nbits := uint32(0)
	if d > 1 {
		nbits = common.Log2FloorNonZero(d) - 1
	}
	prefix := (d >> nbits) & 1
	offset := (2 + prefix) << nbits
	distCode := int(2*int32(nbits-1)) + int(prefix) + 16
	extra := int(d) - int(offset)
	return DistanceCode{distCode, uint(nbits), uint64(extra)}
}
