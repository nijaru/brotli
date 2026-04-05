package brotli

import (
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
