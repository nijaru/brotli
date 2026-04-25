package generic

import (
	"math"

	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/context"
	"github.com/nijaru/brotli/internal/hasher"
	"github.com/nijaru/brotli/internal/metablock"
	"github.com/nijaru/brotli/internal/quality"
	"github.com/nijaru/brotli/internal/ringbuffer"
)

/* Copyright 2016 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/** Minimal value for ::BROTLI_PARAM_LGWIN parameter. */

/** Options for ::BROTLI_PARAM_MODE parameter. */
const (
	modeGeneric = 0
	modeText    = 1
	modeFont    = 2
)

/** Default value for ::BROTLI_PARAM_QUALITY parameter. */
const defaultQuality = 11

/** Default value for ::BROTLI_PARAM_LGWIN parameter. */
const defaultWindow = 22

/** Default value for ::BROTLI_PARAM_MODE parameter. */
const defaultMode = modeGeneric

/** Operations that can be performed by streaming encoder. */
const (
	operationProcess      = 0
	operationFlush        = 1
	operationFinish       = 2
	operationEmitMetadata = 3
)

const (
	streamProcessing     = 0
	streamFlushRequested = 1
	streamFinished       = 2
	streamMetadataHead   = 3
	streamMetadataBody   = 4
)

type baselineEncoder struct {
	params           common.EncoderParams
	hasher_          hasher.Handle
	inputPos         uint64
	ringbuffer_      ringbuffer.RingBuffer
	commands         []metablock.Command
	numLiterals      uint
	lastInsertLen    uint
	lastFlushPos     uint64
	lastProcessedPos uint64
	distCache        [common.NumDistanceShortCodes]int
	savedDistCache   [4]int
	lastBytes        uint16
	lastBytesBits    byte
	prevByte         byte
	prevByte2        byte
	storage          []byte
	smallTable       [1 << 10]int
	largeTable       []int
	largeTableSize   uint
	cmdDepths        [128]byte
	cmdBits          [128]uint16
	cmdCode          [512]byte
	cmdCodeNumbits   uint
	commandBuf       []uint32
	literalBuf       []byte
	tinyBuf          struct {
		u64 [2]uint64
		u8  [16]byte
	}
	remainingMetadataBytes uint32
	streamState            int
	isLastBlockEmitted     bool
	isMetadata             bool
	metaBlockRemainingLen  int
	isInitialized          bool
}

func inputBlockSize(s *State) uint {
	return uint(1) << uint(s.Params.Lgblock)
}

func unprocessedInputSize(s *State) uint64 {
	return s.InputPos - s.LastProcessedPos
}

func remainingInputBlockSize(s *State) uint {
	delta := unprocessedInputSize(s)
	blockSize := inputBlockSize(s)
	if delta >= uint64(blockSize) {
		return 0
	}
	return blockSize - uint(delta)
}

func wrapPosition(position uint64) uint32 {
	result := uint32(position)
	gb := position >> 30
	if gb > 2 {
		/* Wrap every 2GiB; The first 3GB are continuous. */
		result = result&((1<<30)-1) | (uint32((gb-1)&1)+1)<<30
	}

	return result
}

func (s *State) getStorage(size int) []byte {
	if len(s.Storage) < size {
		s.Storage = make([]byte, size)
	}

	return s.Storage
}

func hashTableSize(maxTableSize uint, inputSize uint) uint {
	htsize := uint(256)
	for htsize < maxTableSize && htsize < inputSize {
		htsize <<= 1
	}

	return htsize
}

func getHashTable(s *State, qLevel int, inputSize uint, tableSize *uint) []int {
	var maxTableSize uint
	if qLevel == quality.Q0 {
		maxTableSize = 1 << 15
	} else {
		maxTableSize = 1 << 16
	}
	htsize := hashTableSize(maxTableSize, inputSize)
	var table []int
	if qLevel == quality.Q0 {
		if htsize <= uint(len(s.SmallTable)) {
			*tableSize = htsize
			table = s.SmallTable[:]
		} else {
			if htsize > s.LargeTableSize {
				s.LargeTableSize = htsize
				s.LargeTable = make([]int, htsize)
			}
			*tableSize = htsize
			table = s.LargeTable
		}
	} else {
		if htsize > s.LargeTableSize {
			s.LargeTableSize = htsize
			s.LargeTable = make([]int, htsize)
		}

		*tableSize = htsize
		table = s.LargeTable
	}

	*tableSize = htsize
	clear(table[:htsize])
	return table
}

func encodeWindowBits(lgwin int, largeWindow bool, lastBytes *uint16, lastBytesBits *byte) {
	if lgwin == 16 {
		*lastBytes = 0
		*lastBytesBits = 1
	} else if lgwin > 17 {
		*lastBytes = uint16(uint32(lgwin-17)<<1 | 0x01)
		*lastBytesBits = 4
	} else {
		*lastBytes = uint16(uint32(lgwin-8)<<4 | 0x01)
		*lastBytesBits = 7
	}

	if largeWindow {
		*lastBytes |= 1 << *lastBytesBits
		*lastBytesBits++
	}
}

func encoderInitParams(params *common.EncoderParams) {
	params.Mode = defaultMode
	params.Large_window = false
	params.Quality = defaultQuality
	params.Lgwin = defaultWindow
	params.Lgblock = 0
	params.Size_hint = 0
	params.Disable_literal_context_modeling = false
	initEncoderDictionary(&params.Dictionary)
	params.Dist.Distance_postfix_bits = 0
	params.Dist.Num_direct_distance_codes = 0
	params.Dist.Alphabet_size = uint32(common.DistanceAlphabetSize(0, 0, common.MaxDistanceBits))
	params.Dist.Max_distance = common.MaxDistance
}

func encoderInitState(s *State) {
	encoderInitParams(&s.Params)
	s.InputPos = 0
	s.Commands = s.Commands[:0]
	s.NumLiterals = 0
	s.LastInsertLen = 0
	s.LastFlushPos = 0
	s.LastProcessedPos = 0
	s.PrevByte = 0
	s.PrevByte2 = 0
	if s.Hasher_ != nil {
		s.Hasher_.Common().Is_prepared_ = false
	}
	s.CmdCodeNumbits = 0
	s.StreamState = streamProcessing
	s.IsLastBlockEmitted = false
	s.IsInitialized = false

	ringbuffer.RingBufferInit(&s.Ringbuffer_)

	/* Initialize distance cache. */
	s.DistCache[0] = 4
	s.DistCache[1] = 11
	s.DistCache[2] = 15
	s.DistCache[3] = 16
	copy(s.SavedDistCache[:], s.DistCache[:])

	s.RemainingMetadataBytes = math.MaxUint32
}

func injectBytePaddingBlock(s *State) {
	seal := uint32(s.LastBytes)
	sealBits := uint(s.LastBytesBits)
	s.LastBytes = 0
	s.LastBytesBits = 0

	/* is_last = 0, data_nibbles = 11, reserved = 0, meta_nibbles = 00 */
	seal |= 0x6 << sealBits
	sealBits += 6

	destination := s.TinyBuf.U8[:]
	destination[0] = byte(seal)
	if sealBits > 8 {
		destination[1] = byte(seal >> 8)
	}
	if sealBits > 16 {
		destination[2] = byte(seal >> 16)
	}
	s.writeOutput(destination[:(sealBits+7)>>3])
}

func checkFlushComplete(s *State) {
	if s.StreamState == streamFlushRequested && s.LastBytesBits == 0 {
		s.StreamState = streamProcessing
	}
}

func encoderCompressStreamFast(s *State, op int, availableIn *uint, nextIn *[]byte) bool {
	blockSizeLimit := uint(1) << s.Params.Lgwin
	var commandBuf []uint32 = nil
	var literalBuf []byte = nil
	if s.Plan.Tier > quality.TierQ1 {
		return false
	}

	if s.Plan.Tier == quality.TierQ1 {
		twoPassBufSize := min(kCompressFragmentTwoPassBlockSize, min(*availableIn, blockSizeLimit))
		if s.CommandBuf == nil || cap(s.CommandBuf) < int(twoPassBufSize) {
			s.CommandBuf = make([]uint32, twoPassBufSize)
			s.LiteralBuf = make([]byte, twoPassBufSize)
		} else {
			s.CommandBuf = s.CommandBuf[:twoPassBufSize]
			s.LiteralBuf = s.LiteralBuf[:twoPassBufSize]
		}

		commandBuf = s.CommandBuf
		literalBuf = s.LiteralBuf
	}

	for {
		if s.StreamState == streamFlushRequested && s.LastBytesBits != 0 {
			injectBytePaddingBlock(s)
			continue
		}

		if s.StreamState == streamProcessing && (*availableIn != 0 || op != operationProcess) {
			blockSize := min(blockSizeLimit, *availableIn)
			isLast := (*availableIn == blockSize) && (op == operationFinish)
			forceFlush := (*availableIn == blockSize) && (op == operationFlush)
			maxOutSize := 2*blockSize + 503
			var storage []byte = nil
			storageIx := uint(s.LastBytesBits)
			var tableSize uint
			var table []int

			if forceFlush && blockSize == 0 {
				s.StreamState = streamFlushRequested
				continue
			}

			storage = s.getStorage(int(maxOutSize))

			storage[0] = byte(s.LastBytes)
			storage[1] = byte(s.LastBytes >> 8)
			table = getHashTable(s, s.Params.Quality, blockSize, &tableSize)

			if s.Plan.Tier == quality.TierQ0 {
				compressFragmentFast(*nextIn, blockSize, isLast, table, tableSize, s.CmdDepths[:], s.CmdBits[:], &s.CmdCodeNumbits, s.CmdCode[:], &storageIx, storage)
			} else {
				compressFragmentTwoPass(*nextIn, blockSize, isLast, commandBuf, literalBuf, table, tableSize, &storageIx, storage)
			}

			*nextIn = (*nextIn)[blockSize:]
			*availableIn -= blockSize
			outBytes := storageIx >> 3
			s.writeOutput(storage[:outBytes])

			s.LastBytes = uint16(storage[storageIx>>3])
			s.LastBytesBits = byte(storageIx & 7)

			if forceFlush {
				s.StreamState = streamFlushRequested
			}
			if isLast {
				s.StreamState = streamFinished
			}
			continue
		}

		break
	}

	checkFlushComplete(s)
	return true
}

func processMetadata(s *State, availableIn *uint, nextIn *[]byte) bool {
	if *availableIn > 1<<24 {
		return false
	}

	if s.StreamState == streamProcessing {
		s.RemainingMetadataBytes = uint32(*availableIn)
		s.StreamState = streamMetadataHead
	}

	if s.StreamState != streamMetadataHead && s.StreamState != streamMetadataBody {
		return false
	}

	for {
		if s.StreamState == streamFlushRequested && s.LastBytesBits != 0 {
			injectBytePaddingBlock(s)
			continue
		}

		if s.InputPos != s.LastFlushPos {
			var result bool = encodeData(s, false, true)
			if !result {
				return false
			}
			continue
		}

		if s.StreamState == streamMetadataHead {
			// n := writeMetadataHeader(s, uint(s.RemainingMetadataBytes), s.TinyBuf.U8[:])
			// s.writeOutput(s.TinyBuf.U8[:n])
			s.StreamState = streamMetadataBody
			continue
		} else {
			if s.RemainingMetadataBytes == 0 {
				s.RemainingMetadataBytes = math.MaxUint32
				s.StreamState = streamProcessing
				break
			}

			c := min(s.RemainingMetadataBytes, 16)
			copy(s.TinyBuf.U8[:], (*nextIn)[:c])
			*nextIn = (*nextIn)[c:]
			*availableIn -= uint(c)
			s.RemainingMetadataBytes -= c
			s.writeOutput(s.TinyBuf.U8[:c])

			continue
		}
	}

	return true
}

func updateSizeHint(s *State, availableIn uint) {
	if s.Params.Size_hint == 0 {
		delta := unprocessedInputSize(s)
		tail := uint64(availableIn)
		limit := uint32(1 << 30)
		var total uint32
		if (delta >= uint64(limit)) || (tail >= uint64(limit)) || ((delta + tail) >= uint64(limit)) {
			total = limit
		} else {
			total = uint32(delta + tail)
		}

		s.Params.Size_hint = uint(total)
	}
}

func (s *State) writeOutput(data []byte) {
	if s.Err != nil {
		return
	}
	_, s.Err = s.Dst.Write(data)
}

func ensureInitialized(s *State) bool {
	if s.IsInitialized {
		return true
	}

	s.LastBytesBits = 0
	s.LastBytes = 0
	s.RemainingMetadataBytes = math.MaxUint32

	sanitizeParams(&s.Params)
	s.Params.Lgblock = computeLgBlock(&s.Params)
	chooseDistanceParams(&s.Params)

	ringbuffer.RingBufferSetup(&s.Params, &s.Ringbuffer_)

	/* Initialize last byte with stream header. */
	{
		lgwin := int(s.Params.Lgwin)
		if s.Plan.Tier == quality.TierQ0 || s.Plan.Tier == quality.TierQ1 {
			lgwin = max(lgwin, 18)
		}

		encodeWindowBits(lgwin, s.Params.Large_window, &s.LastBytes, &s.LastBytesBits)
	}

	if s.Plan.Tier == quality.TierQ0 {
		s.CmdDepths = [128]byte{
			0, 4, 4, 5, 6, 6, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8,
			0, 0, 0, 4, 4, 4, 4, 4, 5, 5, 6, 6, 6, 6, 7, 7,
			7, 7, 10, 10, 10, 10, 10, 10, 0, 4, 4, 5, 5, 5, 6, 6,
			7, 8, 8, 9, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10,
			5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			6, 6, 6, 6, 6, 6, 5, 5, 5, 5, 5, 5, 4, 4, 4, 4,
			4, 4, 4, 5, 5, 5, 5, 5, 5, 6, 6, 7, 7, 7, 8, 10,
			12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12,
		}
		s.CmdBits = [128]uint16{
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
		s.CmdCode = [512]byte{
			0xff, 0x77, 0xd5, 0xbf, 0xe7, 0xde, 0xea, 0x9e, 0x51, 0x5d, 0xde, 0xc6,
			0x70, 0x57, 0xbc, 0x58, 0x58, 0x58, 0xd8, 0xd8, 0x58, 0xd5, 0xcb, 0x8c,
			0xea, 0xe0, 0xc3, 0x87, 0x1f, 0x83, 0xc1, 0x60, 0x1c, 0x67, 0xb2, 0xaa,
			0x06, 0x83, 0xc1, 0x60, 0x30, 0x18, 0xcc, 0xa1, 0xce, 0x88, 0x54, 0x94,
			0x46, 0xe1, 0xb0, 0xd0, 0x4e, 0xb2, 0xf7, 0x04, 0x00,
		}
		s.CmdCodeNumbits = 448
	}

	s.IsInitialized = true
	return true
}

var kStaticContextMapContinuation = [64]uint32{
	1, 1, 2, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

var kStaticContextMapSimpleUTF8 = [64]uint32{
	0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

var kStaticContextMapComplexUTF8 = [64]uint32{
	11, 11, 12, 12, 0, 0, 2, 3, 0, 0, 2, 3, 0, 0, 2, 3,
	1, 1, 9, 10, 1, 1, 9, 10, 1, 1, 9, 10, 1, 1, 9, 10,
	5, 4, 6, 7, 5, 4, 6, 7, 5, 4, 6, 7, 5, 4, 6, 7,
	1, 1, 9, 10, 1, 1, 9, 10, 1, 1, 9, 10, 1, 1, 9, 10,
}

func shouldCompress_encode(data []byte, mask uint, lastFlushPos uint64, bytes uint, numLiterals uint, numCommands uint) bool {
	/* TODO: find more precise minimal block overhead. */
	if bytes <= 2 {
		return false
	}
	if numCommands < (bytes>>8)+2 {
		if float64(numLiterals) > 0.99*float64(bytes) {
			var literalHisto = [256]uint32{0}
			const kSampleRate uint32 = 13
			const kMinEntropy float64 = 7.92
			bitCostThreshold := float64(bytes) * kMinEntropy / float64(kSampleRate)
			t := uint((uint32(bytes) + kSampleRate - 1) / kSampleRate)
			pos := uint32(lastFlushPos)
			var i uint
			for i = 0; i < t; i++ {
				literalHisto[data[pos&uint32(mask)]]++
				pos += kSampleRate
			}

			if common.BitsEntropy(literalHisto[:], 256) > bitCostThreshold {
				return false
			}
		}
	}

	return true
}

func optimizeHistograms(numDistanceCodes uint32, mb *metablock.MetaBlockSplit) {
	var goodForRle [common.NumCommandSymbols]byte
	for i := uint(0); i < mb.Literal_histograms_size; i++ {
		bitstream.OptimizeHuffmanCountsForRLE(256, mb.Literal_histograms[i].Data_[:], goodForRle[:])
	}
	for i := uint(0); i < mb.Command_histograms_size; i++ {
		bitstream.OptimizeHuffmanCountsForRLE(common.NumCommandSymbols, mb.Command_histograms[i].Data_[:], goodForRle[:])
	}
	for i := uint(0); i < mb.Distance_histograms_size; i++ {
		bitstream.OptimizeHuffmanCountsForRLE(uint(numDistanceCodes), mb.Distance_histograms[i].Data_[:], goodForRle[:])
	}
}

func chooseContextMap(qLevel int, bigramHisto []uint32, numLiteralContexts *uint, literalContextMap *[]uint32) {
	var monogramHisto = [3]uint32{0}
	var twoPrefixHisto = [6]uint32{0}
	var total uint
	var i uint
	var dummy uint
	var entropy [4]float64
	for i = 0; i < 9; i++ {
		monogramHisto[i%3] += bigramHisto[i]
		twoPrefixHisto[i%6] += bigramHisto[i]
	}

	entropy[1] = common.ShannonEntropy(monogramHisto[:], 3, &dummy)
	entropy[2] = (common.ShannonEntropy(twoPrefixHisto[:], 3, &dummy) + common.ShannonEntropy(twoPrefixHisto[3:], 3, &dummy))
	entropy[3] = 0
	for i = 0; i < 3; i++ {
		entropy[3] += common.ShannonEntropy(bigramHisto[3*i:], 3, &dummy)
	}

	total = uint(monogramHisto[0] + monogramHisto[1] + monogramHisto[2])
	entropy[0] = 1.0 / float64(total)
	entropy[1] *= entropy[0]
	entropy[2] *= entropy[0]
	entropy[3] *= entropy[0]

	if qLevel < quality.MinQualityForHqContextModeling {
		/* 3 context models is a bit slower, don't use it at lower qualities. */
		entropy[3] = entropy[1] * 10
	}

	/* If expected savings by symbol are less than 0.2 bits, skip the
	   context modeling -- in exchange for faster decoding speed. */
	if entropy[1]-entropy[2] < 0.2 && entropy[1]-entropy[3] < 0.2 {
		*numLiteralContexts = 1
	} else if entropy[2]-entropy[3] < 0.02 {
		*numLiteralContexts = 2
		*literalContextMap = kStaticContextMapSimpleUTF8[:]
	} else {
		*numLiteralContexts = 3
		*literalContextMap = kStaticContextMapContinuation[:]
	}
}

func shouldUseComplexStaticContextMap(input []byte, startPos uint, length uint, mask uint, qLevel int, Size_hint uint, numLiteralContexts *uint, literalContextMap *[]uint32) bool {
	/* Try the more complex static context map only for long data. */
	if Size_hint < 1<<20 {
		return false
	} else {
		endPos := startPos + length
		var combinedHisto = [32]uint32{0}
		var contextHisto = [13][32]uint32{[32]uint32{0}}
		var total uint32 = 0
		var entropy [3]float64
		var dummy uint
		var i uint
		utf8Lut := context.GetContextLUT(2) // contextUTF8
		/* To make entropy calculations faster and to fit on the stack, we collect
		   histograms over the 5 most significant bits of literals. One histogram
		   without context and 13 additional histograms for each context value. */
		for ; startPos+64 <= endPos; startPos += 4096 {
			strideEndPos := startPos + 64
			prev2 := input[startPos&mask]
			prev1 := input[(startPos+1)&mask]
			var pos uint

			/* To make the analysis of the data faster we only examine 64 byte long
			   strides at every 4kB intervals. */
			for pos = startPos + 2; pos < strideEndPos; pos++ {
				literal := input[pos&mask]
				ctx := byte(kStaticContextMapComplexUTF8[context.GetContext(prev1, prev2, utf8Lut)])
				total++
				combinedHisto[literal>>3]++
				contextHisto[ctx][literal>>3]++
				prev2 = prev1
				prev1 = literal
			}
		}

		entropy[1] = common.ShannonEntropy(combinedHisto[:], 32, &dummy)
		entropy[2] = 0
		for i = 0; i < 13; i++ {
			entropy[2] += common.ShannonEntropy(contextHisto[i][0:], 32, &dummy)
		}

		entropy[0] = 1.0 / float64(total)
		entropy[1] *= entropy[0]
		entropy[2] *= entropy[0]

		/* The triggering heuristics below were tuned by compressing the individual
		   files of the silesia corpus. If we skip this kind of context modeling
		   for not very well compressible input (i.e. entropy using context modeling
		   is 60% of maximal entropy) or if expected savings by symbol are less
		   than 0.2 bits, then in every case when it triggers, the final compression
		   ratio is improved. Note however that this heuristics might be too strict
		   for some cases and could be tuned further. */
		if entropy[2] > 3.0 || entropy[1]-entropy[2] < 0.2 {
			return false
		} else {
			*numLiteralContexts = 13
			*literalContextMap = kStaticContextMapComplexUTF8[:]
			return true
		}
	}
}

func decideOverLiteralContextModeling(input []byte, startPos uint, length uint, mask uint, qLevel int, Size_hint uint, numLiteralContexts *uint, literalContextMap *[]uint32) {
	if qLevel < quality.MinQualityForContextModeling || length < 64 {
		return
	} else if shouldUseComplexStaticContextMap(input, startPos, length, mask, qLevel, Size_hint, numLiteralContexts, literalContextMap) {
	} else /* Context map was already set, nothing else to do. */
	{
		endPos := startPos + length
		/* Gather bi-gram data of the UTF8 byte prefixes. To make the analysis of
		   UTF8 data faster we only examine 64 byte long strides at every 4kB
		   intervals. */

		var bigramPrefixHisto = [9]uint32{0}
		for ; startPos+64 <= endPos; startPos += 4096 {
			var lut = [4]int{0, 0, 1, 2}
			strideEndPos := startPos + 64
			prev := lut[input[startPos&mask]>>6] * 3
			var pos uint
			for pos = startPos + 1; pos < strideEndPos; pos++ {
				literal := input[pos&mask]
				bigramPrefixHisto[prev+lut[literal>>6]]++
				prev = lut[literal>>6] * 3
			}
		}

		chooseContextMap(qLevel, bigramPrefixHisto[0:], numLiteralContexts, literalContextMap)
	}
}

func writeMetaBlockInternal(data []byte, mask uint, lastFlushPos uint64, bytes uint, isLast bool, literalContextMode int, params *common.EncoderParams, prevByte byte, prevByte2 byte, numLiterals uint, commands []metablock.Command, savedDistCache []int, distCache []int, storageIx *uint, storage []byte) {
	wrappedLastFlushPos := wrapPosition(lastFlushPos)
	var lastBytes uint16
	var lastBytesBits byte
	literalContextLut := context.GetContextLUT(literalContextMode)
	blockParams := *params

	if bytes == 0 {
		/* Write the ISLAST and ISEMPTY bits. */
		bitstream.WriteBits(2, 3, storageIx, storage)

		*storageIx = (*storageIx + 7) &^ 7
		return
	}

	if !shouldCompress_encode(data, mask, lastFlushPos, bytes, numLiterals, uint(len(commands))) {
		/* Restore the distance cache, as its last update by
		   CreateBackwardReferences is now unused. */
		copy(distCache, savedDistCache[:4])

		bitstream.StoreUncompressedMetaBlock(isLast, data, uint(wrappedLastFlushPos), mask, bytes, storageIx, storage)
		return
	}

	lastBytes = uint16(storage[1])<<8 | uint16(storage[0])
	lastBytesBits = byte(*storageIx)
	if params.Quality < quality.MinQualityForBlockSplit {
		bitstream.StoreMetaBlockTrivial(data, uint(wrappedLastFlushPos), bytes, mask, isLast, params, commands, storageIx, storage)
	} else {
		mb := metablock.GetMetaBlockSplit()
		if params.Quality < quality.MinQualityForHqBlockSplitting {
			var numLiteralContexts uint = 1
			var literalContextMap []uint32 = nil
			if !params.Disable_literal_context_modeling {
				decideOverLiteralContextModeling(data, uint(wrappedLastFlushPos), bytes, mask, params.Quality, params.Size_hint, &numLiteralContexts, &literalContextMap)
			}

			metablock.BuildMetaBlockGreedy(data, uint(wrappedLastFlushPos), mask, prevByte, prevByte2, literalContextLut, numLiteralContexts, literalContextMap, commands, mb)
		} else {
			metablock.BuildMetaBlock(data, uint(wrappedLastFlushPos), mask, &blockParams, prevByte, prevByte2, commands, literalContextMode, mb)
		}

		if params.Quality >= quality.MinQualityForOptimizeHistograms {
			/* The number of distance symbols effectively used for distance
			   histograms. It might be less than distance alphabet size
			   for "Large Window Brotli" (32-bit). */
			numEffectiveDistCodes := blockParams.Dist.Alphabet_size
			if numEffectiveDistCodes > common.NumHistogramDistanceSymbols {
				numEffectiveDistCodes = common.NumHistogramDistanceSymbols
			}

			optimizeHistograms(numEffectiveDistCodes, mb)
		}

		bitstream.StoreMetaBlock(data, uint(wrappedLastFlushPos), bytes, mask, prevByte, prevByte2, isLast, &blockParams, literalContextMode, commands, mb, storageIx, storage)
		metablock.FreeMetaBlockSplit(mb)
	}

	if bytes+4 < *storageIx>>3 {
		/* Restore the distance cache and last byte. */
		copy(distCache, savedDistCache[:4])

		storage[0] = byte(lastBytes)
		storage[1] = byte(lastBytes >> 8)
		*storageIx = uint(lastBytesBits)
		bitstream.StoreUncompressedMetaBlock(isLast, data, uint(wrappedLastFlushPos), mask, bytes, storageIx, storage)
	}
}

func chooseDistanceParams(params *common.EncoderParams) {
	var distancePostfixBits uint32 = 0
	var numDirectDistanceCodes uint32 = 0

	if params.Quality >= quality.MinQualityForNonzeroDistanceParams {
		var ndirectMsb uint32
		if params.Mode == modeFont {
			distancePostfixBits = 1
			numDirectDistanceCodes = 12
		} else {
			distancePostfixBits = params.Dist.Distance_postfix_bits
			numDirectDistanceCodes = params.Dist.Num_direct_distance_codes
		}

		ndirectMsb = (numDirectDistanceCodes >> distancePostfixBits) & 0x0F
		if distancePostfixBits > common.MaxNpostfix || numDirectDistanceCodes > common.MaxNdirect || ndirectMsb<<distancePostfixBits != numDirectDistanceCodes {
			distancePostfixBits = 0
			numDirectDistanceCodes = 0
		}
	}

	metablock.InitDistanceParams(params, distancePostfixBits, numDirectDistanceCodes)
}

func chooseContextMode(params *common.EncoderParams, data []byte, pos uint, mask uint, length uint) int {
	/* We only do the computation for the option of something else than
	   CONTEXT_UTF8 for the highest qualities */
	if params.Quality >= 10 && !context.IsMostlyUTF8(data, pos, mask, length, context.KMinUTF8Ratio) {
		return 1 // contextSigned
	}

	return 2 // contextUTF8
}

func updateLastProcessedPos(s *State) bool {
	wrappedLastProcessedPos := wrapPosition(s.LastProcessedPos)
	wrappedInputPos := wrapPosition(s.InputPos)
	s.LastProcessedPos = s.InputPos
	return wrappedInputPos < wrappedLastProcessedPos
}

func extendLastCommand(s *State, bytes *uint32, wrappedLastProcessedPos *uint32) {
	lastCommand := &s.Commands[len(s.Commands)-1]
	data := s.Ringbuffer_.Buffer_
	mask := s.Ringbuffer_.Mask_
	maxBackwardDistance := ((uint64(1)) << s.Params.Lgwin) - common.WindowGap
	lastCopyLen := uint64(lastCommand.Copy_len_) & 0x1FFFFFF
	lastProcessedPos := s.LastProcessedPos - lastCopyLen
	var maxDistance uint64
	if lastProcessedPos < maxBackwardDistance {
		maxDistance = lastProcessedPos
	} else {
		maxDistance = maxBackwardDistance
	}
	cmdDist := uint64(s.DistCache[0])
	distanceCode := metablock.CommandRestoreDistanceCode(lastCommand, &s.Params.Dist)
	if distanceCode < common.NumDistanceShortCodes || uint64(distanceCode-(common.NumDistanceShortCodes-1)) == cmdDist {
		if cmdDist <= maxDistance {
			for *bytes != 0 && data[*wrappedLastProcessedPos&mask] == data[(uint64(*wrappedLastProcessedPos)-cmdDist)&uint64(mask)] {
				lastCommand.Copy_len_++
				(*bytes)--
				(*wrappedLastProcessedPos)++
			}
		}

		metablock.GetLengthCode(uint(lastCommand.Insert_len_), uint(int(lastCommand.Copy_len_&0x1FFFFFF)+int(lastCommand.Copy_len_>>25)), (lastCommand.Dist_prefix_&0x3FF == 0), &lastCommand.Cmd_prefix_)
	}
}

func encodeData(s *State, isLast bool, forceFlush bool) bool {
	delta := unprocessedInputSize(s)
	bytes := uint32(delta)
	wrappedLastProcessedPos := wrapPosition(s.LastProcessedPos)
	var data []byte
	var mask uint32
	var literalContextMode int

	data = s.Ringbuffer_.Buffer_
	mask = s.Ringbuffer_.Mask_

	if s.IsLastBlockEmitted {
		return false
	}
	if isLast {
		s.IsLastBlockEmitted = true
	}

	if delta > uint64(inputBlockSize(s)) {
		return false
	}

	if s.Plan.Tier == quality.TierQ1 {
		const kCompressFragmentTwoPassBlockSize uint = 1 << 17
		if s.CommandBuf == nil || cap(s.CommandBuf) < int(kCompressFragmentTwoPassBlockSize) {
			s.CommandBuf = make([]uint32, kCompressFragmentTwoPassBlockSize)
			s.LiteralBuf = make([]byte, kCompressFragmentTwoPassBlockSize)
		} else {
			s.CommandBuf = s.CommandBuf[:kCompressFragmentTwoPassBlockSize]
			s.LiteralBuf = s.LiteralBuf[:kCompressFragmentTwoPassBlockSize]
		}
	}

	if s.Plan.Tier == quality.TierQ0 || s.Plan.Tier == quality.TierQ1 {
		var storage []byte
		storageIx := uint(s.LastBytesBits)
		var tableSize uint
		var table []int

		if delta == 0 && !isLast {
			return true
		}

		storage = s.getStorage(int(2*bytes + 503))
		storage[0] = byte(s.LastBytes)
		storage[1] = byte(s.LastBytes >> 8)
		table = getHashTable(s, s.Params.Quality, uint(bytes), &tableSize)
		if s.Plan.Tier == quality.TierQ0 {
			compressFragmentFast(data[wrappedLastProcessedPos&mask:], uint(bytes), isLast, table, tableSize, s.CmdDepths[:], s.CmdBits[:], &s.CmdCodeNumbits, s.CmdCode[:], &storageIx, storage)
		} else {
			compressFragmentTwoPass(data[wrappedLastProcessedPos&mask:], uint(bytes), isLast, s.CommandBuf, s.LiteralBuf, table, tableSize, &storageIx, storage)
		}

		s.LastBytes = uint16(storage[storageIx>>3])
		s.LastBytesBits = byte(storageIx & 7)
		updateLastProcessedPos(s)
		s.writeOutput(storage[:storageIx>>3])
		return true
	}

	{
		newsize := len(s.Commands) + int(bytes)/2 + 1
		if newsize > cap(s.Commands) {
			newsize += int(bytes/4) + 16
			newCommands := make([]metablock.Command, len(s.Commands), newsize)
			if s.Commands != nil {
				copy(newCommands, s.Commands)
			}
			s.Commands = newCommands
		}
	}

	chooseHasher(&s.Params, &s.Params.Hasher)
	hasher.InitOrStitchToPreviousBlock(&s.Hasher_, data, uint(mask), &s.Params, uint(wrappedLastProcessedPos), uint(bytes), isLast)

	literalContextMode = chooseContextMode(&s.Params, data, uint(wrapPosition(s.LastFlushPos)), uint(mask), uint(s.InputPos-s.LastFlushPos))

	if len(s.Commands) != 0 && s.LastInsertLen == 0 {
		extendLastCommand(s, &bytes, &wrappedLastProcessedPos)
	}

	if s.Params.Quality == 10 {
		createZopfliBackwardReferences(uint(bytes), uint(wrappedLastProcessedPos), data, uint(mask), &s.Params, s.Hasher_.(*hasher.H10), s.DistCache[:], &s.LastInsertLen, &s.Commands, &s.NumLiterals)
	} else if s.Params.Quality == 11 {
		createHqZopfliBackwardReferences(uint(bytes), uint(wrappedLastProcessedPos), data, uint(mask), &s.Params, s.Hasher_, s.DistCache[:], &s.LastInsertLen, &s.Commands, &s.NumLiterals)
	} else {
		createBackwardReferences(uint(bytes), uint(wrappedLastProcessedPos), data, uint(mask), &s.Params, s.Hasher_, s.DistCache[:], &s.LastInsertLen, &s.Commands, &s.NumLiterals)
	}

	{
		maxLength := maxMetablockSize(&s.Params)
		maxLiterals := maxLength / 8
		maxCommands := int(maxLength / 8)
		processedBytes := uint(s.InputPos - s.LastFlushPos)
		nextInputFitsMetablock := (processedBytes+inputBlockSize(s) <= maxLength)
		shouldFlush := (!s.Plan.BlockSplit && s.NumLiterals+uint(len(s.Commands)) >= quality.MaxNumDelayedSymbols)

		if !isLast && !forceFlush && !shouldFlush && nextInputFitsMetablock && s.NumLiterals < maxLiterals && len(s.Commands) < maxCommands {
			if updateLastProcessedPos(s) {
				hasher.HasherReset(s.Hasher_)
			}
			return true
		}
	}

	if s.LastInsertLen > 0 {
		s.Commands = append(s.Commands, metablock.MakeInsertCommand(s.LastInsertLen))
		s.NumLiterals += s.LastInsertLen
		s.LastInsertLen = 0
	}

	if !isLast && s.InputPos == s.LastFlushPos {
		return true
	}

	{
		metablockSize := uint32(s.InputPos - s.LastFlushPos)
		storage := s.getStorage(int(2*metablockSize + 503))
		storageIx := uint(s.LastBytesBits)
		storage[0] = byte(s.LastBytes)
		storage[1] = byte(s.LastBytes >> 8)
		writeMetaBlockInternal(data, uint(mask), s.LastFlushPos, uint(metablockSize), isLast, literalContextMode, &s.Params, s.PrevByte, s.PrevByte2, s.NumLiterals, s.Commands, s.SavedDistCache[:], s.DistCache[:], &storageIx, storage)
		s.LastBytes = uint16(storage[storageIx>>3])
		s.LastBytesBits = byte(storageIx & 7)
		s.LastFlushPos = s.InputPos
		if updateLastProcessedPos(s) {
			hasher.HasherReset(s.Hasher_)
		}

		if s.LastFlushPos > 0 {
			s.PrevByte = data[(uint32(s.LastFlushPos)-1)&mask]
		}
		if s.LastFlushPos > 1 {
			s.PrevByte2 = data[uint32(s.LastFlushPos-2)&mask]
		}

		s.Commands = s.Commands[:0]
		s.NumLiterals = 0
		copy(s.SavedDistCache[:], s.DistCache[:])
		s.writeOutput(storage[:storageIx>>3])
		return true
	}
}

func copyInputToRingBuffer(s *State, inputSize uint, inputBuffer []byte) {
	ringbuffer.RingBufferWrite(inputBuffer, inputSize, &s.Ringbuffer_)
	s.InputPos += uint64(inputSize)

	if s.Ringbuffer_.Pos_ <= s.Ringbuffer_.Mask_ {
		clear(s.Ringbuffer_.Buffer_[s.Ringbuffer_.Pos_:][:7])
	}
}

func CompressStream(s *State, op int, availableIn *uint, nextIn *[]byte) bool {
	if !ensureInitialized(s) {
		return false
	}

	if s.RemainingMetadataBytes != math.MaxUint32 {
		if uint32(*availableIn) != s.RemainingMetadataBytes {
			return false
		}
		if op != operationEmitMetadata {
			return false
		}
	}

	if op == operationEmitMetadata {
		updateSizeHint(s, 0)
		return processMetadata(s, availableIn, nextIn)
	}

	if s.StreamState == streamMetadataHead || s.StreamState == streamMetadataBody {
		return false
	}

	if s.StreamState != streamProcessing && *availableIn != 0 {
		return false
	}

	if s.Plan.Tier == quality.TierQ0 || s.Plan.Tier == quality.TierQ1 {
		return encoderCompressStreamFast(s, op, availableIn, nextIn)
	}

	for {
		remainingBlockSize := remainingInputBlockSize(s)

		if remainingBlockSize != 0 && *availableIn != 0 {
			copyInputSize := min(remainingBlockSize, *availableIn)
			copyInputToRingBuffer(s, copyInputSize, *nextIn)
			*nextIn = (*nextIn)[copyInputSize:]
			*availableIn -= copyInputSize
			continue
		}

		if s.StreamState == streamFlushRequested && s.LastBytesBits != 0 {
			injectBytePaddingBlock(s)
			continue
		}

		if s.StreamState == streamProcessing {
			if remainingBlockSize == 0 || op != operationProcess {
				isLast := (*availableIn == 0) && op == operationFinish && unprocessedInputSize(s) == 0
				forceFlush := (*availableIn == 0) && op == operationFlush
				updateSizeHint(s, *availableIn)
				if !encodeData(s, isLast, forceFlush) {
					return false
				}
				if forceFlush {
					s.StreamState = streamFlushRequested
				}
				if isLast {
					s.StreamState = streamFinished
				}
				continue
			}
		}
		break
	}

	checkFlushComplete(s)
	return true
}
