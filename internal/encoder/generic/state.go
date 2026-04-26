package generic

import (
	"io"

	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/hasher"
	"github.com/nijaru/brotli/internal/metablock"
	"github.com/nijaru/brotli/internal/quality"
	"github.com/nijaru/brotli/internal/ringbuffer"
)

/* Copyright 2016 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

// State holds the encoder state for the generic (Q1-Q9) encoding path.
type State struct {
	// Output destination and error tracking
	Dst  io.Writer
	Err  error
	Plan quality.Plan

	Params           common.EncoderParams
	Hasher_          hasher.Handle
	InputPos         uint64
	Ringbuffer_      ringbuffer.RingBuffer
	Commands         []metablock.Command
	NumLiterals      uint
	LastInsertLen    uint
	LastFlushPos     uint64
	LastProcessedPos uint64
	DistCache        [common.NumDistanceShortCodes]int
	SavedDistCache   [4]int
	LastBytes        uint16
	LastBytesBits    byte
	PrevByte         byte
	PrevByte2        byte
	Storage          []byte
	SmallTable       [1 << 10]int
	LargeTable       []int
	LargeTableSize   uint
	CmdDepths        [128]byte
	CmdBits          [128]uint16
	CmdCode          [512]byte
	CmdCodeNumbits   uint
	CommandBuf       []uint32
	LiteralBuf       []byte
	// Zopfli reusable buffers (Q10-Q11)
	ZopfliNodes       []zopfliNode
	ZopfliMatches      []hasher.BackwardMatch
	ZopfliNumMatches   []uint32
	ZopfliLiteralCosts []float32
	ZopfliCostDist     []float32
	TinyBuf          struct {
		U64 [2]uint64
		U8  [16]byte
	}
	RemainingMetadataBytes uint32
	StreamState            int
	IsLastBlockEmitted     bool
	IsMetadata             bool
	MetaBlockRemainingLen  int
	IsInitialized          bool
}

// InitState initializes or resets the encoder state.
func InitState(s *State) {
	initParams(&s.Params)
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

	s.RemainingMetadataBytes = 0x7FFFFFFF // math.MaxUint32
}

// ResetForReuse resets per-compression state while preserving initialized buffers.
// Call this on Writer.Reset when quality hasn't changed to avoid re-allocating.
func ResetForReuse(s *State) {
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
	s.RemainingMetadataBytes = 0x7FFFFFFF

	ringbuffer.RingBufferInit(&s.Ringbuffer_)

	/* Re-initialize distance cache. */
	s.DistCache[0] = 4
	s.DistCache[1] = 11
	s.DistCache[2] = 15
	s.DistCache[3] = 16
	copy(s.SavedDistCache[:], s.DistCache[:])
}

func initParams(params *common.EncoderParams) {
	params.Quality = 11
	params.Lgwin = 22
	params.Size_hint = 0
	params.Lgblock = 0
	params.Large_window = false
	params.Mode = 0
}
