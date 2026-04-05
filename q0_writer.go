package brotli

import (
	"io"
)

const (
	q0MinWindowBits     = 10
	q0MaxWindowBits     = 24
	q0DefaultWindowBits = 22
)

type q0Writer struct {
	dst io.Writer
	err error

	windowBits    uint
	lastBytes     uint16
	lastBytesBits uint

	table [1 << 15]int

	cmdDepths      [128]byte
	cmdBits        [128]uint16
	cmdCode        [512]byte
	cmdCodeNumbits uint
}

var q0CmdDepths = [128]byte{
	0, 4, 4, 5, 6, 6, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8,
	0, 0, 0, 4, 4, 4, 4, 4, 5, 5, 6, 6, 6, 6, 7, 7,
	7, 7, 10, 10, 10, 10, 10, 10, 0, 4, 4, 5, 5, 5, 6, 6,
	7, 8, 8, 9, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10,
	5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	6, 6, 6, 6, 6, 6, 5, 5, 5, 5, 5, 5, 4, 4, 4, 4,
	4, 4, 4, 5, 5, 5, 5, 5, 5, 6, 6, 7, 7, 7, 8, 10,
	12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12,
}

var q0CmdBits = [128]uint16{
	0, 0, 8, 9, 3, 35, 7, 71,
	39, 103, 23, 47, 175, 111, 239, 31,
	0, 0, 0, 4, 12, 2, 10, 6,
	13, 29, 11, 43, 27, 59, 87, 55,
	15, 79, 319, 831, 191, 703, 447, 959,
	0, 14, 1, 25, 5, 21, 19, 51,
	119, 159, 95, 223, 479, 991, 63, 575,
	127, 639, 383, 895, 255, 767, 511, 1023,
	14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	27, 59, 7, 39, 23, 55, 30, 1, 17, 9, 25, 5, 0, 8, 4, 12,
	2, 10, 6, 21, 13, 29, 3, 19, 11, 15, 47, 31, 95, 63, 127, 255,
	767, 2815, 1791, 3839, 511, 2559, 1535, 3583, 1023, 3071, 2047, 4095,
}

func (w *q0Writer) encodeBlock(src []byte, isLast bool) {
	if w.err != nil {
		return
	}

	// Initialize command depths/bits on first use
	if w.cmdDepths[0] == 0 && w.cmdDepths[1] == 0 {
		w.cmdDepths = q0CmdDepths
		w.cmdBits = q0CmdBits
	}

	maxOutSize := 2*len(src) + 503
	storage := make([]byte, maxOutSize)
	storageIx := w.lastBytesBits

	storage[0] = byte(w.lastBytes)
	storage[1] = byte(w.lastBytes >> 8)

	clear(w.table[:])
	tableSize := uint(1 << 15)
	if sz := uint(len(src)); sz < tableSize {
		tableSize = 256
		for tableSize < (1<<15) && tableSize < sz {
			tableSize <<= 1
		}
		if tableSize&0xAAAAA == 0 {
			tableSize <<= 1
		}
	}
	compressFragmentFast(src, uint(len(src)), isLast, w.table[:], tableSize, w.cmdDepths[:], w.cmdBits[:], &w.cmdCodeNumbits, w.cmdCode[:], &storageIx, storage)

	w.lastBytes = uint16(storage[storageIx>>3]) | uint16(storage[(storageIx>>3)+1])<<8
	w.lastBytesBits = storageIx & 7

	w.writeOutput(storage[:storageIx>>3])
}

func (w *q0Writer) writeOutput(data []byte) {
	if w.err != nil {
		return
	}
	_, w.err = w.dst.Write(data)
}

func (w *q0Writer) writeHeader() {
	if w.windowBits == 0 {
		w.windowBits = q0DefaultWindowBits
	}

	if w.windowBits == 16 {
		w.lastBytes = 0
		w.lastBytesBits = 1
	} else if w.windowBits > 17 {
		w.lastBytes = uint16((w.windowBits-17)<<1 | 0x01)
		w.lastBytesBits = 4
	} else {
		w.lastBytes = uint16((w.windowBits-8)<<4 | 0x01)
		w.lastBytesBits = 7
	}
}
