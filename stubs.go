package brotli

import (
	"github.com/nijaru/brotli/internal/bitstream"
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/metablock"
	"github.com/nijaru/brotli/internal/ringbuffer"
)

// Type aliases for backward compatibility with root package code.
type ringBuffer = ringbuffer.RingBuffer
type command = metablock.Command
type metaBlockSplit = metablock.MetaBlockSplit
type histogramLiteral = common.HistogramLiteral
type histogramCommand = common.HistogramCommand
type histogramDistance = common.HistogramDistance

const kCompressFragmentTwoPassBlockSize uint = 1 << 17

func optimizeHistograms(num_distance_codes uint32, mb *metaBlockSplit) {
	var good_for_rle [common.NumCommandSymbols]byte
	for i := uint(0); i < mb.Literal_histograms_size; i++ {
		bitstream.OptimizeHuffmanCountsForRLE(256, mb.Literal_histograms[i].Data_[:], good_for_rle[:])
	}
	for i := uint(0); i < mb.Command_histograms_size; i++ {
		bitstream.OptimizeHuffmanCountsForRLE(common.NumCommandSymbols, mb.Command_histograms[i].Data_[:], good_for_rle[:])
	}
	for i := uint(0); i < mb.Distance_histograms_size; i++ {
		bitstream.OptimizeHuffmanCountsForRLE(uint(num_distance_codes), mb.Distance_histograms[i].Data_[:], good_for_rle[:])
	}
}

// Stubs for missing fast encoder functions.
// These were in compress_fragment_common.go (deleted years ago) and
// compress_fragment_two_pass.go (restored but had too many issues).

func compressFragmentFast(input []byte, input_size uint, is_last bool, table []int, table_size uint, cmd_depths []byte, cmd_bits []uint16, cmd_code_numbits *uint, cmd_code []byte, storage_ix *uint, storage []byte) {
	panic("brotli: compressFragmentFast not implemented")
}

func compressFragmentTwoPass(input []byte, input_size uint, is_last bool, command_buf []uint32, literal_buf []byte, table []int, table_size uint, storage_ix *uint, storage []byte) {
	panic("brotli: compressFragmentTwoPass not implemented")
}
