package brotli

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

func inputBlockSize(s *Writer) uint {
	return uint(1) << uint(s.params.Lgblock)
}

func unprocessedInputSize(s *Writer) uint64 {
	return s.inputPos - s.lastProcessedPos
}

func remainingInputBlockSize(s *Writer) uint {
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

func (s *Writer) getStorage(size int) []byte {
	if len(s.storage) < size {
		s.storage = make([]byte, size)
	}

	return s.storage
}

func hashTableSize(maxTableSize uint, inputSize uint) uint {
	htsize := uint(256)
	for htsize < maxTableSize && htsize < inputSize {
		htsize <<= 1
	}

	return htsize
}

func getHashTable(s *Writer, quality int, inputSize uint, tableSize *uint) []int {
	var maxTableSize uint
	if quality == fastOnePassCompressionQuality {
		maxTableSize = 1 << 15
	} else {
		maxTableSize = 1 << 16
	}
	htsize := hashTableSize(maxTableSize, inputSize)
	var table []int
	if quality == fastOnePassCompressionQuality {
		if htsize <= uint(len(s.smallTable)) {
			*tableSize = htsize
			table = s.smallTable[:]
		} else {
			if htsize > s.largeTableSize {
				s.largeTableSize = htsize
				s.largeTable = make([]int, htsize)
			}
			*tableSize = htsize
			table = s.largeTable
		}
	} else {
		if htsize > s.largeTableSize {
			s.largeTableSize = htsize
			s.largeTable = make([]int, htsize)
		}

		*tableSize = htsize
		table = s.largeTable
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
	params.Dist.Alphabet_size = uint32(distanceAlphabetSize(0, 0, maxDistanceBits))
	params.Dist.Max_distance = maxDistance
}

func encoderInitState(s *Writer) {
	encoderInitParams(&s.params)
	s.inputPos = 0
	s.commands = s.commands[:0]
	s.numLiterals = 0
	s.lastInsertLen = 0
	s.lastFlushPos = 0
	s.lastProcessedPos = 0
	s.prevByte = 0
	s.prevByte2 = 0
	if s.hasher_ != nil {
		s.hasher_.Common().Is_prepared_ = false
	}
	s.cmdCodeNumbits = 0
	s.streamState = streamProcessing
	s.isLastBlockEmitted = false
	s.isInitialized = false

	ringbuffer.RingBufferInit(&s.ringbuffer_)

	/* Initialize distance cache. */
	s.distCache[0] = 4
	s.distCache[1] = 11
	s.distCache[2] = 15
	s.distCache[3] = 16
	copy(s.savedDistCache[:], s.distCache[:])

	s.remainingMetadataBytes = math.MaxUint32
}

func injectBytePaddingBlock(s *Writer) {
	seal := uint32(s.lastBytes)
	sealBits := uint(s.lastBytesBits)
	s.lastBytes = 0
	s.lastBytesBits = 0

	/* is_last = 0, data_nibbles = 11, reserved = 0, meta_nibbles = 00 */
	seal |= 0x6 << sealBits
	sealBits += 6

	destination := s.tinyBuf.u8[:]
	destination[0] = byte(seal)
	if sealBits > 8 {
		destination[1] = byte(seal >> 8)
	}
	if sealBits > 16 {
		destination[2] = byte(seal >> 16)
	}
	s.writeOutput(destination[:(sealBits+7)>>3])
}

func checkFlushComplete(s *Writer) {
	if s.streamState == streamFlushRequested && s.lastBytesBits == 0 {
		s.streamState = streamProcessing
	}
}

func encoderCompressStreamFast(s *Writer, op int, availableIn *uint, nextIn *[]byte) bool {
	blockSizeLimit := uint(1) << s.params.Lgwin
	var commandBuf []uint32 = nil
	var literalBuf []byte = nil
	if s.plan.Tier > quality.TierQ1 {
		return false
	}

	if s.plan.Tier == quality.TierQ1 {
		twoPassBufSize := min(kCompressFragmentTwoPassBlockSize, min(*availableIn, blockSizeLimit))
		if s.commandBuf == nil || cap(s.commandBuf) < int(twoPassBufSize) {
			s.commandBuf = make([]uint32, twoPassBufSize)
			s.literalBuf = make([]byte, twoPassBufSize)
		} else {
			s.commandBuf = s.commandBuf[:twoPassBufSize]
			s.literalBuf = s.literalBuf[:twoPassBufSize]
		}

		commandBuf = s.commandBuf
		literalBuf = s.literalBuf
	}

	for {
		if s.streamState == streamFlushRequested && s.lastBytesBits != 0 {
			injectBytePaddingBlock(s)
			continue
		}

		if s.streamState == streamProcessing && (*availableIn != 0 || op != operationProcess) {
			blockSize := min(blockSizeLimit, *availableIn)
			isLast := (*availableIn == blockSize) && (op == operationFinish)
			forceFlush := (*availableIn == blockSize) && (op == operationFlush)
			maxOutSize := 2*blockSize + 503
			var storage []byte = nil
			storageIx := uint(s.lastBytesBits)
			var tableSize uint
			var table []int

			if forceFlush && blockSize == 0 {
				s.streamState = streamFlushRequested
				continue
			}

			storage = s.getStorage(int(maxOutSize))

			storage[0] = byte(s.lastBytes)
			storage[1] = byte(s.lastBytes >> 8)
			table = getHashTable(s, s.params.Quality, blockSize, &tableSize)

			if s.plan.Tier == quality.TierQ0 {
				compressFragmentFast(*nextIn, blockSize, isLast, table, tableSize, s.cmdDepths[:], s.cmdBits[:], &s.cmdCodeNumbits, s.cmdCode[:], &storageIx, storage)
			} else {
				compressFragmentTwoPass(*nextIn, blockSize, isLast, commandBuf, literalBuf, table, tableSize, &storageIx, storage)
			}

			*nextIn = (*nextIn)[blockSize:]
			*availableIn -= blockSize
			outBytes := storageIx >> 3
			s.writeOutput(storage[:outBytes])

			s.lastBytes = uint16(storage[storageIx>>3])
			s.lastBytesBits = byte(storageIx & 7)

			if forceFlush {
				s.streamState = streamFlushRequested
			}
			if isLast {
				s.streamState = streamFinished
			}
			continue
		}

		break
	}

	checkFlushComplete(s)
	return true
}

func processMetadata(s *Writer, availableIn *uint, nextIn *[]byte) bool {
	if *availableIn > 1<<24 {
		return false
	}

	if s.streamState == streamProcessing {
		s.remainingMetadataBytes = uint32(*availableIn)
		s.streamState = streamMetadataHead
	}

	if s.streamState != streamMetadataHead && s.streamState != streamMetadataBody {
		return false
	}

	for {
		if s.streamState == streamFlushRequested && s.lastBytesBits != 0 {
			injectBytePaddingBlock(s)
			continue
		}

		if s.inputPos != s.lastFlushPos {
			var result bool = encodeData(s, false, true)
			if !result {
				return false
			}
			continue
		}

		if s.streamState == streamMetadataHead {
			// n := writeMetadataHeader(s, uint(s.remainingMetadataBytes), s.tinyBuf.u8[:])
			// s.writeOutput(s.tinyBuf.u8[:n])
			s.streamState = streamMetadataBody
			continue
		} else {
			if s.remainingMetadataBytes == 0 {
				s.remainingMetadataBytes = math.MaxUint32
				s.streamState = streamProcessing
				break
			}

			c := min(s.remainingMetadataBytes, 16)
			copy(s.tinyBuf.u8[:], (*nextIn)[:c])
			*nextIn = (*nextIn)[c:]
			*availableIn -= uint(c)
			s.remainingMetadataBytes -= c
			s.writeOutput(s.tinyBuf.u8[:c])

			continue
		}
	}

	return true
}

func updateSizeHint(s *Writer, availableIn uint) {
	if s.params.Size_hint == 0 {
		delta := unprocessedInputSize(s)
		tail := uint64(availableIn)
		limit := uint32(1 << 30)
		var total uint32
		if (delta >= uint64(limit)) || (tail >= uint64(limit)) || ((delta + tail) >= uint64(limit)) {
			total = limit
		} else {
			total = uint32(delta + tail)
		}

		s.params.Size_hint = uint(total)
	}
}

func (w *Writer) writeOutput(data []byte) {
	if w.err != nil {
		return
	}
	_, w.err = w.dst.Write(data)
}

func ensureInitialized(s *Writer) bool {
	if s.isInitialized {
		return true
	}

	s.lastBytesBits = 0
	s.lastBytes = 0
	s.remainingMetadataBytes = math.MaxUint32

	sanitizeParams(&s.params)
	s.params.Lgblock = computeLgBlock(&s.params)
	chooseDistanceParams(&s.params)

	ringbuffer.RingBufferSetup(&s.params, &s.ringbuffer_)

	/* Initialize last byte with stream header. */
	{
		lgwin := int(s.params.Lgwin)
		if s.plan.Tier == quality.TierQ0 || s.plan.Tier == quality.TierQ1 {
			lgwin = max(lgwin, 18)
		}

		encodeWindowBits(lgwin, s.params.Large_window, &s.lastBytes, &s.lastBytesBits)
	}

	if s.plan.Tier == quality.TierQ0 {
		s.cmdDepths = [128]byte{
			0, 4, 4, 5, 6, 6, 7, 7, 7, 7, 7, 8, 8, 8, 8, 8,
			0, 0, 0, 4, 4, 4, 4, 4, 5, 5, 6, 6, 6, 6, 7, 7,
			7, 7, 10, 10, 10, 10, 10, 10, 0, 4, 4, 5, 5, 5, 6, 6,
			7, 8, 8, 9, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10,
			5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			6, 6, 6, 6, 6, 6, 5, 5, 5, 5, 5, 5, 4, 4, 4, 4,
			4, 4, 4, 5, 5, 5, 5, 5, 5, 6, 6, 7, 7, 7, 8, 10,
			12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12, 12,
		}
		s.cmdBits = [128]uint16{
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
		s.cmdCode = [512]byte{
			0xff, 0x77, 0xd5, 0xbf, 0xe7, 0xde, 0xea, 0x9e, 0x51, 0x5d, 0xde, 0xc6,
			0x70, 0x57, 0xbc, 0x58, 0x58, 0x58, 0xd8, 0xd8, 0x58, 0xd5, 0xcb, 0x8c,
			0xea, 0xe0, 0xc3, 0x87, 0x1f, 0x83, 0xc1, 0x60, 0x1c, 0x67, 0xb2, 0xaa,
			0x06, 0x83, 0xc1, 0x60, 0x30, 0x18, 0xcc, 0xa1, 0xce, 0x88, 0x54, 0x94,
			0x46, 0xe1, 0xb0, 0xd0, 0x4e, 0xb2, 0xf7, 0x04, 0x00,
		}
		s.cmdCodeNumbits = 448
	}

	s.isInitialized = true
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

func chooseContextMap(quality int, bigramHisto []uint32, numLiteralContexts *uint, literalContextMap *[]uint32) {
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

	if quality < minQualityForHqContextModeling {
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

func shouldUseComplexStaticContextMap(input []byte, startPos uint, length uint, mask uint, quality int, Size_hint uint, numLiteralContexts *uint, literalContextMap *[]uint32) bool {
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

func decideOverLiteralContextModeling(input []byte, startPos uint, length uint, mask uint, quality int, Size_hint uint, numLiteralContexts *uint, literalContextMap *[]uint32) {
	if quality < minQualityForContextModeling || length < 64 {
		return
	} else if shouldUseComplexStaticContextMap(input, startPos, length, mask, quality, Size_hint, numLiteralContexts, literalContextMap) {
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

		chooseContextMap(quality, bigramPrefixHisto[0:], numLiteralContexts, literalContextMap)
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
	if params.Quality < minQualityForBlockSplit {
		bitstream.StoreMetaBlockTrivial(data, uint(wrappedLastFlushPos), bytes, mask, isLast, params, commands, storageIx, storage)
	} else {
		mb := metablock.GetMetaBlockSplit()
		if params.Quality < minQualityForHqBlockSplitting {
			var numLiteralContexts uint = 1
			var literalContextMap []uint32 = nil
			if !params.Disable_literal_context_modeling {
				decideOverLiteralContextModeling(data, uint(wrappedLastFlushPos), bytes, mask, params.Quality, params.Size_hint, &numLiteralContexts, &literalContextMap)
			}

			metablock.BuildMetaBlockGreedy(data, uint(wrappedLastFlushPos), mask, prevByte, prevByte2, literalContextLut, numLiteralContexts, literalContextMap, commands, mb)
		} else {
			metablock.BuildMetaBlock(data, uint(wrappedLastFlushPos), mask, &blockParams, prevByte, prevByte2, commands, literalContextMode, mb)
		}

		if params.Quality >= minQualityForOptimizeHistograms {
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

	if params.Quality >= minQualityForNonzeroDistanceParams {
		var ndirectMsb uint32
		if params.Mode == modeFont {
			distancePostfixBits = 1
			numDirectDistanceCodes = 12
		} else {
			distancePostfixBits = params.Dist.Distance_postfix_bits
			numDirectDistanceCodes = params.Dist.Num_direct_distance_codes
		}

		ndirectMsb = (numDirectDistanceCodes >> distancePostfixBits) & 0x0F
		if distancePostfixBits > maxNpostfix || numDirectDistanceCodes > maxNdirect || ndirectMsb<<distancePostfixBits != numDirectDistanceCodes {
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

func updateLastProcessedPos(s *Writer) bool {
	wrappedLastProcessedPos := wrapPosition(s.lastProcessedPos)
	wrappedInputPos := wrapPosition(s.inputPos)
	s.lastProcessedPos = s.inputPos
	return wrappedInputPos < wrappedLastProcessedPos
}

func extendLastCommand(s *Writer, bytes *uint32, wrappedLastProcessedPos *uint32) {
	lastCommand := &s.commands[len(s.commands)-1]
	data := s.ringbuffer_.Buffer_
	mask := s.ringbuffer_.Mask_
	maxBackwardDistance := ((uint64(1)) << s.params.Lgwin) - windowGap
	lastCopyLen := uint64(lastCommand.Copy_len_) & 0x1FFFFFF
	lastProcessedPos := s.lastProcessedPos - lastCopyLen
	var maxDistance uint64
	if lastProcessedPos < maxBackwardDistance {
		maxDistance = lastProcessedPos
	} else {
		maxDistance = maxBackwardDistance
	}
	cmdDist := uint64(s.distCache[0])
	distanceCode := metablock.CommandRestoreDistanceCode(lastCommand, &s.params.Dist)
	if distanceCode < numDistanceShortCodes || uint64(distanceCode-(numDistanceShortCodes-1)) == cmdDist {
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

func encodeData(s *Writer, isLast bool, forceFlush bool) bool {
	delta := unprocessedInputSize(s)
	bytes := uint32(delta)
	wrappedLastProcessedPos := wrapPosition(s.lastProcessedPos)
	var data []byte
	var mask uint32
	var literalContextMode int

	data = s.ringbuffer_.Buffer_
	mask = s.ringbuffer_.Mask_

	if s.isLastBlockEmitted {
		return false
	}
	if isLast {
		s.isLastBlockEmitted = true
	}

	if delta > uint64(inputBlockSize(s)) {
		return false
	}

	if s.plan.Tier == quality.TierQ1 {
		const kCompressFragmentTwoPassBlockSize uint = 1 << 17
		if s.commandBuf == nil || cap(s.commandBuf) < int(kCompressFragmentTwoPassBlockSize) {
			s.commandBuf = make([]uint32, kCompressFragmentTwoPassBlockSize)
			s.literalBuf = make([]byte, kCompressFragmentTwoPassBlockSize)
		} else {
			s.commandBuf = s.commandBuf[:kCompressFragmentTwoPassBlockSize]
			s.literalBuf = s.literalBuf[:kCompressFragmentTwoPassBlockSize]
		}
	}

	if s.plan.Tier == quality.TierQ0 || s.plan.Tier == quality.TierQ1 {
		var storage []byte
		storageIx := uint(s.lastBytesBits)
		var tableSize uint
		var table []int

		if delta == 0 && !isLast {
			return true
		}

		storage = s.getStorage(int(2*bytes + 503))
		storage[0] = byte(s.lastBytes)
		storage[1] = byte(s.lastBytes >> 8)
		table = getHashTable(s, s.params.Quality, uint(bytes), &tableSize)
		if s.plan.Tier == quality.TierQ0 {
			compressFragmentFast(data[wrappedLastProcessedPos&mask:], uint(bytes), isLast, table, tableSize, s.cmdDepths[:], s.cmdBits[:], &s.cmdCodeNumbits, s.cmdCode[:], &storageIx, storage)
		} else {
			compressFragmentTwoPass(data[wrappedLastProcessedPos&mask:], uint(bytes), isLast, s.commandBuf, s.literalBuf, table, tableSize, &storageIx, storage)
		}

		s.lastBytes = uint16(storage[storageIx>>3])
		s.lastBytesBits = byte(storageIx & 7)
		updateLastProcessedPos(s)
		s.writeOutput(storage[:storageIx>>3])
		return true
	}

	{
		newsize := len(s.commands) + int(bytes)/2 + 1
		if newsize > cap(s.commands) {
			newsize += int(bytes/4) + 16
			newCommands := make([]metablock.Command, len(s.commands), newsize)
			if s.commands != nil {
				copy(newCommands, s.commands)
			}
			s.commands = newCommands
		}
	}

	chooseHasher(&s.params, &s.params.Hasher)
	hasher.InitOrStitchToPreviousBlock(&s.hasher_, data, uint(mask), &s.params, uint(wrappedLastProcessedPos), uint(bytes), isLast)

	literalContextMode = chooseContextMode(&s.params, data, uint(wrapPosition(s.lastFlushPos)), uint(mask), uint(s.inputPos-s.lastFlushPos))

	if len(s.commands) != 0 && s.lastInsertLen == 0 {
		extendLastCommand(s, &bytes, &wrappedLastProcessedPos)
	}

	if s.params.Quality == 10 {
		createZopfliBackwardReferences(uint(bytes), uint(wrappedLastProcessedPos), data, uint(mask), &s.params, s.hasher_.(*hasher.H10), s.distCache[:], &s.lastInsertLen, &s.commands, &s.numLiterals)
	} else if s.params.Quality == 11 {
		createHqZopfliBackwardReferences(uint(bytes), uint(wrappedLastProcessedPos), data, uint(mask), &s.params, s.hasher_, s.distCache[:], &s.lastInsertLen, &s.commands, &s.numLiterals)
	} else {
		createBackwardReferences(uint(bytes), uint(wrappedLastProcessedPos), data, uint(mask), &s.params, s.hasher_, s.distCache[:], &s.lastInsertLen, &s.commands, &s.numLiterals)
	}

	{
		maxLength := maxMetablockSize(&s.params)
		maxLiterals := maxLength / 8
		maxCommands := int(maxLength / 8)
		processedBytes := uint(s.inputPos - s.lastFlushPos)
		nextInputFitsMetablock := (processedBytes+inputBlockSize(s) <= maxLength)
		shouldFlush := (!s.plan.BlockSplit && s.numLiterals+uint(len(s.commands)) >= maxNumDelayedSymbols)

		if !isLast && !forceFlush && !shouldFlush && nextInputFitsMetablock && s.numLiterals < maxLiterals && len(s.commands) < maxCommands {
			if updateLastProcessedPos(s) {
				hasher.HasherReset(s.hasher_)
			}
			return true
		}
	}

	if s.lastInsertLen > 0 {
		s.commands = append(s.commands, metablock.MakeInsertCommand(s.lastInsertLen))
		s.numLiterals += s.lastInsertLen
		s.lastInsertLen = 0
	}

	if !isLast && s.inputPos == s.lastFlushPos {
		return true
	}

	{
		metablockSize := uint32(s.inputPos - s.lastFlushPos)
		storage := s.getStorage(int(2*metablockSize + 503))
		storageIx := uint(s.lastBytesBits)
		storage[0] = byte(s.lastBytes)
		storage[1] = byte(s.lastBytes >> 8)
		writeMetaBlockInternal(data, uint(mask), s.lastFlushPos, uint(metablockSize), isLast, literalContextMode, &s.params, s.prevByte, s.prevByte2, s.numLiterals, s.commands, s.savedDistCache[:], s.distCache[:], &storageIx, storage)
		s.lastBytes = uint16(storage[storageIx>>3])
		s.lastBytesBits = byte(storageIx & 7)
		s.lastFlushPos = s.inputPos
		if updateLastProcessedPos(s) {
			hasher.HasherReset(s.hasher_)
		}

		if s.lastFlushPos > 0 {
			s.prevByte = data[(uint32(s.lastFlushPos)-1)&mask]
		}
		if s.lastFlushPos > 1 {
			s.prevByte2 = data[uint32(s.lastFlushPos-2)&mask]
		}

		s.commands = s.commands[:0]
		s.numLiterals = 0
		copy(s.savedDistCache[:], s.distCache[:])
		s.writeOutput(storage[:storageIx>>3])
		return true
	}
}

func copyInputToRingBuffer(s *Writer, inputSize uint, inputBuffer []byte) {
	ringbuffer.RingBufferWrite(inputBuffer, inputSize, &s.ringbuffer_)
	s.inputPos += uint64(inputSize)

	if s.ringbuffer_.Pos_ <= s.ringbuffer_.Mask_ {
		clear(s.ringbuffer_.Buffer_[s.ringbuffer_.Pos_:][:7])
	}
}

func encoderCompressStream(s *Writer, op int, availableIn *uint, nextIn *[]byte) bool {
	if !ensureInitialized(s) {
		return false
	}

	if s.remainingMetadataBytes != math.MaxUint32 {
		if uint32(*availableIn) != s.remainingMetadataBytes {
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

	if s.streamState == streamMetadataHead || s.streamState == streamMetadataBody {
		return false
	}

	if s.streamState != streamProcessing && *availableIn != 0 {
		return false
	}

	if s.plan.Tier == quality.TierQ0 || s.plan.Tier == quality.TierQ1 {
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

		if s.streamState == streamFlushRequested && s.lastBytesBits != 0 {
			injectBytePaddingBlock(s)
			continue
		}

		if s.streamState == streamProcessing {
			if remainingBlockSize == 0 || op != operationProcess {
				isLast := (*availableIn == 0) && op == operationFinish && unprocessedInputSize(s) == 0
				forceFlush := (*availableIn == 0) && op == operationFlush
				updateSizeHint(s, *availableIn)
				if !encodeData(s, isLast, forceFlush) {
					return false
				}
				if forceFlush {
					s.streamState = streamFlushRequested
				}
				if isLast {
					s.streamState = streamFinished
				}
				continue
			}
		}
		break
	}

	checkFlushComplete(s)
	return true
}
