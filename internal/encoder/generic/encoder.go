package generic

import (
	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/metablock"
	"github.com/nijaru/brotli/internal/match"
)

// An Encoder implements the match.Encoder interface, writing in Brotli format.
type Encoder struct {
	wroteHeader bool
	bw          bitstream.BitWriter
	distCache   []metablock.DistanceCode
}

func (e *Encoder) Reset() {
	e.wroteHeader = false
	e.bw = bitstream.BitWriter{}
}

func (e *Encoder) Close() error {
	e.Reset()
	match.PutEncoder(e)
	return nil
}

func (e *Encoder) Encode(dst []byte, src []byte, matches []match.Match, lastBlock bool) []byte {
	e.bw.Dst = dst
	if !e.wroteHeader {
		e.bw.WriteBits(4, 15)
		e.wroteHeader = true
	}

	if len(src) == 0 {
		if lastBlock {
			e.bw.WriteBits(2, 3) // islast + isempty
			e.bw.JumpToByteBoundary()
			return e.bw.Dst
		}
		return dst
	}

	var literalHisto [256]uint32
	var commandHisto [704]uint32
	var distanceHisto [64]uint32
	literalCount := 0
	commandCount := 0
	distanceCount := 0

	if len(e.distCache) < len(matches) {
		e.distCache = make([]metablock.DistanceCode, len(matches))
	}

	// first pass: build the histograms
	pos := 0

	// d is the ring buffer of the last 4 distances.
	d := [4]int{-10, -10, -10, -10}
	for i, m := range matches {
		if m.Unmatched > 0 {
			for _, c := range src[pos : pos+m.Unmatched] {
				literalHisto[c]++
			}
			literalCount += m.Unmatched
		}

		insertCode := metablock.GetInsertLengthCode(uint(m.Unmatched))
		copyCode := metablock.GetCopyLengthCode(uint(m.Length))
		if m.Length == 0 {
			// If the stream ends with unmatched bytes, we need a dummy copy length.
			copyCode = 2
		}
		command := metablock.CombineLengthCodes(insertCode, copyCode, i > 0 && m.Distance == matches[i-1].Distance)
		commandHisto[command]++
		commandCount++

		if command >= 128 && m.Length != 0 {
			var distCode metablock.DistanceCode
			switch m.Distance {
			case d[3]:
				distCode.Code = 0
			case d[2]:
				distCode.Code = 1
			case d[1]:
				distCode.Code = 2
			case d[0]:
				distCode.Code = 3
			case d[3] - 1:
				distCode.Code = 4
			case d[3] + 1:
				distCode.Code = 5
			case d[3] - 2:
				distCode.Code = 6
			case d[3] + 2:
				distCode.Code = 7
			case d[3] - 3:
				distCode.Code = 8
			case d[3] + 3:
				distCode.Code = 9

				// In my testing, codes 10–15 actually reduced the compression ratio.

			default:
				distCode = metablock.GetDistanceCode(m.Distance)
			}
			e.distCache[i] = distCode
			distanceHisto[distCode.Code]++
			distanceCount++
			if distCode.Code != 0 {
				d[0], d[1], d[2], d[3] = d[1], d[2], d[3], m.Distance
			}
		}

		pos += m.Unmatched + m.Length
	}

	bitstream.StoreMetaBlockHeaderBW(uint(len(src)), false, &e.bw)
	e.bw.WriteBits(13, 0)

	var literalDepths [256]byte
	var literalBits [256]uint16
	bitstream.BuildAndStoreHuffmanTreeFastBW(literalHisto[:], uint(literalCount), 8, literalDepths[:], literalBits[:], &e.bw)

	var commandDepths [704]byte
	var commandBits [704]uint16
	bitstream.BuildAndStoreHuffmanTreeFastBW(commandHisto[:], uint(commandCount), 10, commandDepths[:], commandBits[:], &e.bw)

	var distanceDepths [64]byte
	var distanceBits [64]uint16
	bitstream.BuildAndStoreHuffmanTreeFastBW(distanceHisto[:], uint(distanceCount), 6, distanceDepths[:], distanceBits[:], &e.bw)

	pos = 0
	for i, m := range matches {
		insertCode := metablock.GetInsertLengthCode(uint(m.Unmatched))
		copyCode := metablock.GetCopyLengthCode(uint(m.Length))
		if m.Length == 0 {
			// If the stream ends with unmatched bytes, we need a dummy copy length.
			copyCode = 2
		}
		command := metablock.CombineLengthCodes(insertCode, copyCode, i > 0 && m.Distance == matches[i-1].Distance)
		e.bw.WriteBits(uint(commandDepths[command]), uint64(commandBits[command]))
		if metablock.GetInsertExtra(insertCode) > 0 {
			e.bw.WriteBits(uint(metablock.GetInsertExtra(insertCode)), uint64(m.Unmatched)-uint64(metablock.GetInsertBase(insertCode)))
		}
		if metablock.GetCopyExtra(copyCode) > 0 {
			e.bw.WriteBits(uint(metablock.GetCopyExtra(copyCode)), uint64(m.Length)-uint64(metablock.GetCopyBase(copyCode)))
		}

		if m.Unmatched > 0 {
			for _, c := range src[pos : pos+m.Unmatched] {
				e.bw.WriteBits(uint(literalDepths[c]), uint64(literalBits[c]))
			}
		}

		if command >= 128 && m.Length != 0 {
			distCode := e.distCache[i]
			e.bw.WriteBits(uint(distanceDepths[distCode.Code]), uint64(distanceBits[distCode.Code]))
			if distCode.NExtra > 0 {
				e.bw.WriteBits(distCode.NExtra, distCode.ExtraBits)
			}
		}

		pos += m.Unmatched + m.Length
	}

	if lastBlock {
		e.bw.WriteBits(2, 3) // islast + isempty
		e.bw.JumpToByteBoundary()
	}
	return e.bw.Dst
}
