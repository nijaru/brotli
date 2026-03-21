package bitstream

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Utilities for building Huffman decoding tables. */

type SymbolList struct {
	Storage []uint16
	Offset  int
}

func (sl SymbolList) Get(i int) uint16 {
	return sl.Storage[i+sl.Offset]
}

func (sl SymbolList) Put(i int, val uint16) {
	sl.Storage[i+sl.Offset] = val
}
