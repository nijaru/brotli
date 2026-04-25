package flate

import (
	"io"

	"github.com/nijaru/brotli/internal/match"
)

// NewWriter returns a new match.Writer that compresses data at the given level,
// in flate encoding. Levels 1–9 are available; levels outside this range will
// be replaced with the closest level available.
func NewWriter(w io.Writer, level int) *match.Writer {
	return newWriter(w, level, NewEncoder())
}

// NewGZIPWriter returns a new match.Writer that compresses data at the given
// level, in gzip encoding. Levels 1–9 are available; levels outside this range
// will be replaced by the closest level available.
func NewGZIPWriter(w io.Writer, level int) *match.Writer {
	return newWriter(w, level, NewGZIPEncoder())
}

func newWriter(w io.Writer, level int, e match.Encoder) *match.Writer {
	var mf match.MatchFinder
	if level < 2 {
		mf = &match.ZFast{MaxDistance: 1 << 15}
	} else if level == 2 {
		mf = &match.ZDFast{MaxDistance: 1 << 15}
	} else if level == 3 {
		mf = &match.ZM{MaxDistance: 1 << 15}
	} else if level == 4 {
		mf = &match.Trio{MaxDistance: 1 << 15}
	} else if level < 8 {
		chainLen := 32
		switch level {
		case 5:
			chainLen = 8
		case 6:
			chainLen = 16
		}
		mf = &match.M4{
			MaxDistance:     1 << 15,
			ChainLength:     chainLen,
			HashLen:         5,
			DistanceBitCost: 66,
		}
	} else {
		chainLen := 32
		hashLen := 5
		if level == 8 {
			chainLen = 4
		}
		mf = &match.Pathfinder{
			MaxDistance: 1 << 15,
			ChainLength: chainLen,
			HashLen:     hashLen,
		}
	}

	return &match.Writer{
		Dest:        w,
		MatchFinder: mf,
		Encoder:     e,
		BlockSize:   1 << 16,
	}
}
