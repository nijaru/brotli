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
