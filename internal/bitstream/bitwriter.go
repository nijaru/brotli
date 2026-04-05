package bitstream

/* Copyright 2010 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Write bits into a byte array. */

type BitWriter struct {
	Dst []byte

	// Data waiting to be written is the low nbits of bits.
	Bits  uint64
	Nbits uint
}

func (w *BitWriter) WriteBits(nb uint, b uint64) {
	w.Bits |= b << w.Nbits
	w.Nbits += nb
	if w.Nbits >= 32 {
		bits := w.Bits
		w.Bits >>= 32
		w.Nbits -= 32
		w.Dst = append(w.Dst,
			byte(bits),
			byte(bits>>8),
			byte(bits>>16),
			byte(bits>>24),
		)
	}
}

func (w *BitWriter) WriteSingleBit(bit bool) {
	if bit {
		w.WriteBits(1, 1)
	} else {
		w.WriteBits(1, 0)
	}
}

func (w *BitWriter) JumpToByteBoundary() {
	dst := w.Dst
	for w.Nbits != 0 {
		dst = append(dst, byte(w.Bits))
		w.Bits >>= 8
		if w.Nbits > 8 { // Avoid underflow
			w.Nbits -= 8
		} else {
			w.Nbits = 0
		}
	}
	w.Bits = 0
	w.Dst = dst
}
