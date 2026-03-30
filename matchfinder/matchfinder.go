// The matchfinder package defines reusable components for data compression.
//
// Many compression libraries have two main parts:
//   - Something that looks for repeated sequences of bytes
//   - An encoder for the compressed data format (often an entropy coder)
//
// Although these are logically two separate steps, the implementations are
// usually closely tied together. You can't use flate's matcher with snappy's
// encoder, for example. This package defines interfaces and an intermediate
// representation to allow mixing and matching compression components.
package matchfinder

import (
	"encoding/binary"
	"io"
	"math/bits"
)

// A Match is the basic unit of LZ77 compression.
type Match struct {
	Unmatched int // the number of unmatched bytes since the previous match
	Length    int // the number of bytes in the matched string; it may be 0 at the end of the input
	Distance  int // how far back in the stream to copy from
}

// A MatchFinder performs the LZ77 stage of compression, looking for matches.
type MatchFinder interface {
	// FindMatches looks for matches in src, appends them to dst, and returns dst.
	FindMatches(dst []Match, src []byte) []Match

	// Reset clears any internal state, preparing the MatchFinder to be used with
	// a new stream.
	Reset()
}

// An Encoder encodes the data in its final format.
type Encoder interface {
	// Encode appends the encoded format of src to dst, using the match
	// information from matches.
	Encode(dst []byte, src []byte, matches []Match, lastBlock bool) []byte

	// Reset clears any internal state, preparing the Encoder to be used with
	// a new stream.
	Reset()
}

// A Writer uses MatchFinder and Encoder to write compressed data to Dest.
type Writer struct {
	Dest        io.Writer
	MatchFinder MatchFinder
	Encoder     Encoder

	// BlockSize is the number of bytes to compress at a time. If it is zero,
	// each Write operation will be treated as one block.
	BlockSize int

	err     error
	inBuf   []byte
	outBuf  []byte
	matches []Match
}

func (w *Writer) Write(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}

	if w.BlockSize == 0 {
		return w.writeBlock(p, false)
	}

	w.inBuf = append(w.inBuf, p...)
	var pos int
	for pos = 0; pos+w.BlockSize <= len(w.inBuf) && w.err == nil; pos += w.BlockSize {
		w.writeBlock(w.inBuf[pos:pos+w.BlockSize], false)
	}
	if pos > 0 {
		n := copy(w.inBuf, w.inBuf[pos:])
		w.inBuf = w.inBuf[:n]
	}

	return len(p), w.err
}

func (w *Writer) writeBlock(p []byte, lastBlock bool) (n int, err error) {
	w.outBuf = w.outBuf[:0]
	w.matches = w.MatchFinder.FindMatches(w.matches[:0], p)
	w.outBuf = w.Encoder.Encode(w.outBuf, p, w.matches, lastBlock)
	_, w.err = w.Dest.Write(w.outBuf)
	return len(p), w.err
}

// Flush writes out any buffered data without closing the stream.
// The data is written as a non-final block.
func (w *Writer) Flush() error {
	if len(w.inBuf) > 0 {
		w.writeBlock(w.inBuf, false)
		w.inBuf = w.inBuf[:0]
	}
	return w.err
}

func (w *Writer) Close() error {
	w.writeBlock(w.inBuf, true)
	w.inBuf = w.inBuf[:0]
	return w.err
}

func (w *Writer) Reset(newDest io.Writer) {
	w.MatchFinder.Reset()
	w.Encoder.Reset()
	w.err = nil
	w.inBuf = w.inBuf[:0]
	w.outBuf = w.outBuf[:0]
	w.matches = w.matches[:0]
	w.Dest = newDest
}

// extendMatch returns the largest k such that k <= len(src) and that
// src[i:i+k-j] and src[j:k] have the same contents.
//
// It assumes that:
//
//	0 <= i && i < j && j <= len(src)
func extendMatch(src []byte, i, j int) int {
	for j+8 < len(src) {
		iBytes := binary.LittleEndian.Uint64(src[i:])
		jBytes := binary.LittleEndian.Uint64(src[j:])
		if iBytes != jBytes {
			return j + bits.TrailingZeros64(iBytes^jBytes)>>3
		}
		i, j = i+8, j+8
	}
	for ; j < len(src) && src[i] == src[j]; i, j = i+1, j+1 {
	}
	return j
}

// Given a 4-byte match at src[start] and src[candidate], extendMatch2 extends it
// upward as far as possible, and downward no farther than to min.
func extendMatch2(src []byte, start, candidate, min int) absoluteMatch {
	end := extendMatch(src, candidate+4, start+4)
	for start > min && candidate > 0 && src[start-1] == src[candidate-1] {
		start--
		candidate--
	}
	return absoluteMatch{
		Start: start,
		End:   end,
		Match: candidate,
	}
}
