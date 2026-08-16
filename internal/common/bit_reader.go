package common

import "encoding/binary"

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Bit reading helpers */

const ShortFillBitWindowRead = (8 >> 1)

func BitMask(n uint32) uint32 {
	if n >= 32 {
		return 0xFFFFFFFF
	}
	return (uint32(1) << n) - 1
}

type BitReader struct {
	Val      uint64
	BitPos   uint32
	Input    []byte
	InputLen uint
	BytePos  uint
}

type BitReaderState struct {
	Val      uint64
	BitPos   uint32
	Input    []byte
	InputLen uint
	BytePos  uint
}

/* Initializes the BrotliBitReader fields. */

func (br *BitReader) SaveState(to *BitReaderState) {
	to.Val = br.Val
	to.BitPos = br.BitPos
	to.Input = br.Input
	to.InputLen = br.InputLen
	to.BytePos = br.BytePos
}

func (br *BitReader) RestoreState(from *BitReaderState) {
	br.Val = from.Val
	br.BitPos = from.BitPos
	br.Input = from.Input
	br.InputLen = from.InputLen
	br.BytePos = from.BytePos
}

func (br *BitReader) GetAvailableBits() uint32 {
	return 64 - br.BitPos
}

func (br *BitReader) GetRemainingBytes() uint {
	return uint(uint32(br.InputLen-br.BytePos) + (br.GetAvailableBits() >> 3))
}

func (br *BitReader) CheckInputAmount(num uint) bool {
	return br.InputLen-br.BytePos >= num
}

func (br *BitReader) FillBitWindow(nBits uint32) {
	if br.BitPos >= 32 {
		br.Val >>= 32
		br.BitPos ^= 32 /* here same as -= 32 because of the if condition */
		br.Val |= uint64(binary.LittleEndian.Uint32(br.Input[br.BytePos:])) << 32
		br.BytePos += 4
	}
}

func (br *BitReader) FillBitWindow16() {
	br.FillBitWindow(17)
}

func (br *BitReader) PullByte() bool {
	if br.BytePos == br.InputLen {
		return false
	}

	br.Val >>= 8
	br.Val |= uint64(br.Input[br.BytePos]) << 56
	br.BitPos -= 8
	br.BytePos++
	return true
}

func (br *BitReader) GetBitsUnmasked() uint64 {
	return br.Val >> br.BitPos
}

func (br *BitReader) Get16BitsUnmasked() uint32 {
	br.FillBitWindow(16)
	return uint32(br.GetBitsUnmasked())
}

func (br *BitReader) GetBits(nBits uint32) uint32 {
	br.FillBitWindow(nBits)
	return uint32(br.GetBitsUnmasked()) & BitMask(nBits)
}

func (br *BitReader) SafeGetBits(nBits uint32, val *uint32) bool {
	for br.GetAvailableBits() < nBits {
		if !br.PullByte() {
			return false
		}
	}

	*val = uint32(br.GetBitsUnmasked()) & BitMask(nBits)
	return true
}

func (br *BitReader) DropBits(nBits uint32) {
	br.BitPos += nBits
}

func (br *BitReader) Unload() {
	unusedBytes := br.GetAvailableBits() >> 3
	unusedBits := unusedBytes << 3
	br.BytePos -= uint(unusedBytes)
	if unusedBits == 64 {
		br.Val = 0
	} else {
		br.Val <<= unusedBits
	}

	br.BitPos += unusedBits
}

func (br *BitReader) TakeBits(nBits uint32, val *uint32) {
	*val = uint32(br.GetBitsUnmasked()) & BitMask(nBits)
	br.DropBits(nBits)
}

func (br *BitReader) ReadBits(nBits uint32) uint32 {
	var val uint32
	br.FillBitWindow(nBits)
	br.TakeBits(nBits, &val)
	return val
}

func (br *BitReader) SafeReadBits(nBits uint32, val *uint32) bool {
	for br.GetAvailableBits() < nBits {
		if !br.PullByte() {
			return false
		}
	}

	br.TakeBits(nBits, val)
	return true
}

func (br *BitReader) JumpToByteBoundary() bool {
	padBitsCount := br.GetAvailableBits() & 0x7
	var padBits uint32
	if padBitsCount != 0 {
		br.TakeBits(padBitsCount, &padBits)
	}

	return padBits == 0
}

func (br *BitReader) CopyBytes(dest []byte, num uint) {
	for br.GetAvailableBits() >= 8 && num > 0 {
		dest[0] = byte(br.GetBitsUnmasked())
		br.DropBits(8)
		dest = dest[1:]
		num--
	}

	copy(dest, br.Input[br.BytePos:][:num])
	br.BytePos += num
}

func (br *BitReader) Init() {
	br.Val = 0
	br.BitPos = 64
}

func (br *BitReader) Warmup() bool {
	if br.GetAvailableBits() == 0 {
		if !br.PullByte() {
			return false
		}
	}

	return true
}
