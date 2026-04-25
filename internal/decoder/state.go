package decoder

import (
	"io"

	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/dictionary"
)

/* Copyright 2015 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Brotli state for partial streaming decoding. */
const (
	stateUninited = iota
	stateLargeWindowBits
	stateInitialize
	stateMetablockBegin
	stateMetablockHeader
	stateMetablockHeader2
	stateContextModes
	stateCommandBegin
	stateCommandInner
	stateCommandPostDecodeLiterals
	stateCommandPostWrapCopy
	stateUncompressed
	stateMetadata
	stateCommandInnerWrite
	stateMetablockDone
	stateCommandPostWrite1
	stateCommandPostWrite2
	stateHuffmanCode0
	stateHuffmanCode1
	stateHuffmanCode2
	stateHuffmanCode3
	stateContextMap1
	stateContextMap2
	stateTreeGroup
	stateDone
)

const (
	stateMetablockHeaderNone = iota
	stateMetablockHeaderEmpty
	stateMetablockHeaderNibbles
	stateMetablockHeaderSize
	stateMetablockHeaderUncompressed
	stateMetablockHeaderReserved
	stateMetablockHeaderBytes
	stateMetablockHeaderMetadata
)

const (
	stateUncompressedNone = iota
	stateUncompressedWrite
)

const (
	stateTreeGroupNone = iota
	stateTreeGroupLoop
)

const (
	stateContextMapNone = iota
	stateContextMapReadPrefix
	stateContextMapHuffman
	stateContextMapDecode
	stateContextMapTransform
)

const (
	stateHuffmanNone = iota
	stateHuffmanSimpleSize
	stateHuffmanSimpleRead
	stateHuffmanSimpleBuild
	stateHuffmanComplex
	stateHuffmanLengthSymbols
)

const (
	stateDecodeUint8None = iota
	stateDecodeUint8Short
	stateDecodeUint8Long
)

const (
	stateReadBlockLengthNone = iota
	stateReadBlockLengthSuffix
)

type Reader struct {
	src io.Reader
	buf []byte // scratch space for reading from src
	in  []byte // current chunk to decode; usually aliases buf

	state        int
	loopCounter int
	br           bitstream.BitReader
	buffer       struct {
		u64 uint64
		u8  [8]byte
	}
	bufferLength               uint32
	pos                         int
	maxBackwardDistance       int
	maxDistance                 int
	ringbufferSize             int
	ringbufferMask             int
	distRbIdx                 int
	distRb                     [4]int
	errorCode                  int
	sub_loopCounter            uint32
	ringbuffer                  []byte
	ringbufferEnd              []byte
	htreeCommand               []bitstream.HuffmanCode
	contextLookup              []byte
	contextMapSlice           []byte
	dist_contextMapSlice      []byte
	literalHGroup              bitstream.HuffmanTreeGroup
	insertCopyHGroup          bitstream.HuffmanTreeGroup
	distanceHGroup             bitstream.HuffmanTreeGroup
	blockTypeTrees            []bitstream.HuffmanCode
	blockLenTrees             []bitstream.HuffmanCode
	trivialLiteralContext     int
	distanceContext            int
	metaBlockRemainingLen    int
	blockLengthIndex          uint32
	blockLength                [3]uint32
	numBlockTypes             [3]uint32
	blockTypeRb               [6]uint32
	distancePostfixBits       uint32
	numDirectDistanceCodes   uint32
	distancePostfixMask       int
	numDistHtrees             uint32
	distContextMap            []byte
	literalHtree               []bitstream.HuffmanCode
	distHtreeIndex            byte
	repeatCodeLen             uint32
	prevCodeLen               uint32
	copyLength                 int
	distanceCode               int
	rbRoundtrips               uint
	partialPosOut             uint
	symbol                      uint32
	repeat                      uint32
	space                       uint32
	table                       [32]bitstream.HuffmanCode
	symbolLists                bitstream.SymbolList
	symbolsListsArray         [bitstream.HuffmanMaxCodeLength + 1 + common.NumCommandSymbols]uint16
	nextSymbol                 [32]int
	codeLengthCodeLengths    [common.CodeLengthCodes]byte
	codeLengthHisto           [16]uint16
	htreeIndex                 int
	next                        []bitstream.HuffmanCode
	contextIndex               uint32
	maxRunLengthPrefix       uint32
	code                        uint32
	contextMapTable           [bitstream.HuffmanMaxSize272]bitstream.HuffmanCode
	substateMetablockHeader   int
	substateTreeGroup         int
	substateContextMap        int
	substateUncompressed       int
	substateHuffman            int
	substateDecodeUint8       int
	substate_read_blockLength  int
	isLastMetablock           uint
	isUncompressed             uint
	isMetadata                 uint
	shouldWrapRingbuffer      uint
	cannyRingbufferAllocation uint
	largeWindow                bool
	sizeNibbles                uint
	windowBits                 uint32
	new_ringbufferSize         int
	num_literalHtrees          uint32
	contextMap                 []byte
	contextModes               []byte
	dictionary                  *dictionary.Dictionary
	transforms                  *dictionary.Transforms
	trivialLiteralContexts    [8]uint32
}

func decoderStateInit(s *Reader) bool {
	s.errorCode = 0 /* BROTLI_DECODER_NO_ERROR */

	s.br.Init()
	s.state = stateUninited
	s.largeWindow = false
	s.substateMetablockHeader = stateMetablockHeaderNone
	s.substateTreeGroup = stateTreeGroupNone
	s.substateContextMap = stateContextMapNone
	s.substateUncompressed = stateUncompressedNone
	s.substateHuffman = stateHuffmanNone
	s.substateDecodeUint8 = stateDecodeUint8None
	s.substate_read_blockLength = stateReadBlockLengthNone

	s.bufferLength = 0
	s.loopCounter = 0
	s.pos = 0
	s.rbRoundtrips = 0
	s.partialPosOut = 0

	s.blockTypeTrees = nil
	s.blockLenTrees = nil
	s.ringbufferSize = 0
	s.new_ringbufferSize = 0
	s.ringbufferMask = 0

	s.contextMap = nil
	s.contextModes = nil
	s.distContextMap = nil
	s.contextMapSlice = nil
	s.dist_contextMapSlice = nil

	s.sub_loopCounter = 0

	s.literalHGroup.Codes = nil
	s.literalHGroup.HTrees = nil
	s.insertCopyHGroup.Codes = nil
	s.insertCopyHGroup.HTrees = nil
	s.distanceHGroup.Codes = nil
	s.distanceHGroup.HTrees = nil

	s.isLastMetablock = 0
	s.isUncompressed = 0
	s.isMetadata = 0
	s.shouldWrapRingbuffer = 0
	s.cannyRingbufferAllocation = 1

	s.windowBits = 0
	s.maxDistance = 0
	s.distRb[0] = 16
	s.distRb[1] = 15
	s.distRb[2] = 11
	s.distRb[3] = 4
	s.distRbIdx = 0
	s.blockTypeTrees = nil
	s.blockLenTrees = nil

	s.symbolLists.Storage = s.symbolsListsArray[:]
	s.symbolLists.Offset = bitstream.HuffmanMaxCodeLength + 1

	s.dictionary = dictionary.GetDictionary()
	s.transforms = dictionary.GetTransforms()

	return true
}

func decoderStateMetablockBegin(s *Reader) {
	s.metaBlockRemainingLen = 0
	s.blockLength[0] = 1 << 24
	s.blockLength[1] = 1 << 24
	s.blockLength[2] = 1 << 24
	s.numBlockTypes[0] = 1
	s.numBlockTypes[1] = 1
	s.numBlockTypes[2] = 1
	s.blockTypeRb[0] = 1
	s.blockTypeRb[1] = 0
	s.blockTypeRb[2] = 1
	s.blockTypeRb[3] = 0
	s.blockTypeRb[4] = 1
	s.blockTypeRb[5] = 0
	s.contextMap = nil
	s.contextModes = nil
	s.distContextMap = nil
	s.contextMapSlice = nil
	s.literalHtree = nil
	s.dist_contextMapSlice = nil
	s.distHtreeIndex = 0
	s.contextLookup = nil
	s.literalHGroup.Codes = nil
	s.literalHGroup.HTrees = nil
	s.insertCopyHGroup.Codes = nil
	s.insertCopyHGroup.HTrees = nil
	s.distanceHGroup.Codes = nil
	s.distanceHGroup.HTrees = nil
}

func decoderStateCleanupAfterMetablock(s *Reader) {
	s.contextModes = nil
	s.contextMap = nil
	s.distContextMap = nil
	s.literalHGroup.HTrees = nil
	s.insertCopyHGroup.HTrees = nil
	s.distanceHGroup.HTrees = nil
}

func decoderHuffmanTreeGroupInit(s *Reader, group *bitstream.HuffmanTreeGroup, alphabetSize uint32, maxSymbol uint32, nTrees uint32) bool {
	var max_table_size uint = uint(bitstream.KMaxHuffmanTableSize[(alphabetSize+31)>>5])
	group.AlphabetSize = uint16(alphabetSize)
	group.MaxSymbol = uint16(maxSymbol)
	group.NumHTrees = uint16(nTrees)
	group.HTrees = make([][]bitstream.HuffmanCode, nTrees)
	group.Codes = make([]bitstream.HuffmanCode, (uint(nTrees) * max_table_size))
	return !(group.Codes == nil)
}
