package brotli

import (
	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/metablock"
	"github.com/nijaru/brotli/matchfinder"
)

type FastEncoder struct {
	wroteHeader bool
	bw          bitstream.BitWriter
	distRb      [4]int
}

func (e *FastEncoder) Reset() {
	e.wroteHeader = false
	e.bw = bitstream.BitWriter{}
	e.distRb = [4]int{16, 15, 11, 4}
}

func (e *FastEncoder) Close() error {
	e.Reset()
	matchfinder.PutFastEncoder(e)
	return nil
}

func (e *FastEncoder) Encode(dst []byte, src []byte, matches []matchfinder.Match, lastBlock bool) []byte {
	e.bw.Dst = dst
	if !e.wroteHeader {
		e.bw.WriteBits(4, 15)
		e.wroteHeader = true
		e.distRb = [4]int{16, 15, 11, 4}
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

	// First pass: build histograms
	pos := 0
	d := e.distRb
	for _, m := range matches {
		for _, c := range src[pos : pos+m.Unmatched] {
			literalHisto[c]++
		}
		literalCount += m.Unmatched

		var command int
		lengthFinished := false
		if m.Unmatched < 6 {
			if m.Length < 10 && m.Length != 0 {
				command = m.Unmatched<<3 + m.Length - 2 + 128
				lengthFinished = true
			} else {
				if m.Length == 0 {
					command = m.Unmatched << 3
				} else {
					command = m.Unmatched<<3 + 128
				}
			}
			commandHisto[command]++
			commandCount++
		} else {
			insertCode := metablock.GetInsertLengthCode(uint(m.Unmatched))
			command = int(metablock.CombineLengthCodes(insertCode, 0, m.Length == 0))
			commandHisto[command]++
			commandCount++
		}

		if command >= 128 && m.Length != 0 {
			var distCode metablock.DistanceCode
			if m.Distance == d[3] {
				distCode.Code = 0
			} else {
				distCode = metablock.GetDistanceCode(m.Distance)
			}
			distanceHisto[distCode.Code]++
			distanceCount++

			if distCode.Code != 0 {
				d[0], d[1], d[2], d[3] = d[1], d[2], d[3], m.Distance
			}
		}

		if m.Length != 0 {
			switch {
			case lengthFinished:
			case m.Length < 12:
				command := m.Length - 4
				commandHisto[command]++
				commandCount++
			case m.Length < 72:
				copyCode := metablock.GetCopyLengthCode(uint(m.Length - 2))
				command := metablock.CombineLengthCodes(0, copyCode, true)
				commandHisto[command]++
				commandCount++
			default:
				copyCode := metablock.GetCopyLengthCode(uint(m.Length - 2))
				command := metablock.CombineLengthCodes(0, copyCode, false)
				commandHisto[command]++
				commandCount++
				distanceHisto[0]++
				distanceCount++
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

	// Second pass: write data
	pos = 0
	d = e.distRb
	for _, m := range matches {
		lengthFinished := false
		var command int
		if m.Unmatched < 6 {
			if m.Length < 10 && m.Length != 0 {
				command = m.Unmatched<<3 + m.Length - 2 + 128
				lengthFinished = true
			} else {
				if m.Length == 0 {
					command = m.Unmatched << 3
				} else {
					command = m.Unmatched<<3 + 128
				}
			}
			e.bw.WriteBits(uint(commandDepths[command]), uint64(commandBits[command]))
		} else {
			insertCode := metablock.GetInsertLengthCode(uint(m.Unmatched))
			command = int(metablock.CombineLengthCodes(insertCode, 0, m.Length == 0))
			e.bw.WriteBits(uint(commandDepths[command]), uint64(commandBits[command]))
			e.bw.WriteBits(uint(metablock.GetInsertExtra(insertCode)), uint64(m.Unmatched)-uint64(metablock.GetInsertBase(insertCode)))
		}

		if m.Unmatched > 0 {
			for _, c := range src[pos : pos+m.Unmatched] {
				e.bw.WriteBits(uint(literalDepths[c]), uint64(literalBits[c]))
			}
		}

		if command >= 128 && m.Length != 0 {
			var distCode metablock.DistanceCode
			if m.Distance == d[3] {
				distCode.Code = 0
			} else {
				distCode = metablock.GetDistanceCode(m.Distance)
			}
			e.bw.WriteBits(uint(distanceDepths[distCode.Code]), uint64(distanceBits[distCode.Code]))
			if distCode.NExtra > 0 {
				e.bw.WriteBits(distCode.NExtra, distCode.ExtraBits)
			}
			if distCode.Code != 0 {
				d[0], d[1], d[2], d[3] = d[1], d[2], d[3], m.Distance
			}
		}

		if m.Length != 0 {
			switch {
			case lengthFinished:
			case m.Length < 12:
				command := m.Length - 4
				e.bw.WriteBits(uint(commandDepths[command]), uint64(commandBits[command]))
			case m.Length < 72:
				copyCode := metablock.GetCopyLengthCode(uint(m.Length - 2))
				command := metablock.CombineLengthCodes(0, copyCode, true)
				e.bw.WriteBits(uint(commandDepths[command]), uint64(commandBits[command]))
				e.bw.WriteBits(uint(metablock.GetCopyExtra(copyCode)), uint64(m.Length-2)-uint64(metablock.GetCopyBase(copyCode)))
			default:
				copyCode := metablock.GetCopyLengthCode(uint(m.Length - 2))
				command := metablock.CombineLengthCodes(0, copyCode, false)
				e.bw.WriteBits(uint(commandDepths[command]), uint64(commandBits[command]))
				e.bw.WriteBits(uint(metablock.GetCopyExtra(copyCode)), uint64(m.Length-2)-uint64(metablock.GetCopyBase(copyCode)))
				e.bw.WriteBits(uint(distanceDepths[0]), uint64(distanceBits[0]))
			}
		}

		pos += m.Unmatched + m.Length
	}

	e.distRb = d

	if lastBlock {
		e.bw.WriteBits(2, 3) // islast + isempty
		e.bw.JumpToByteBoundary()
	} else {
		// Empty metadata metablock to force byte alignment.
		e.bw.WriteBits(1, 0) // islast = 0
		e.bw.WriteBits(2, 3) // mnibbles = 3 (metadata)
		e.bw.WriteBits(2, 0) // msizenibbles = 0
		e.bw.WriteBits(1, 0) // reserved = 0
		e.bw.JumpToByteBoundary()
	}
	return e.bw.Dst
}
