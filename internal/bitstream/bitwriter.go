package bitstream

/* Copyright 2010 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Write bits into a byte array. */

type BitWriter struct {
	Dst   []byte
	Bits  uint64
	Nbits uint
}

func (w *BitWriter) WriteBits(nb uint, b uint64) {
	w.Bits |= b << w.Nbits
	w.Nbits += nb
	for w.Nbits >= 8 {
		w.Dst = append(w.Dst, byte(w.Bits))
		w.Bits >>= 8
		w.Nbits -= 8
	}
}

func (w *BitWriter) WriteSingleBit(bit bool) {
	if bit {
		w.Bits |= 1 << w.Nbits
	}
	w.Nbits++
	if w.Nbits >= 8 {
		w.Dst = append(w.Dst, byte(w.Bits))
		w.Bits >>= 8
		w.Nbits -= 8
	}
}

func (w *BitWriter) JumpToByteBoundary() {
	dst := w.Dst
	for w.Nbits != 0 {
		dst = append(dst, byte(w.Bits))
		w.Bits >>= 8
		if w.Nbits > 8 {
			w.Nbits -= 8
		} else {
			w.Nbits = 0
		}
	}
	w.Bits = 0
	w.Dst = dst
}
