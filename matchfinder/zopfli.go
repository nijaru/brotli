package matchfinder

import (
	"encoding/binary"
	"math"
	"math/bits"
	"slices"
)

// Zopfli is a MatchFinder that uses exhaustive candidate search
// with dynamic-programming optimal parsing.
//
// It implements the brotli "quality 10-11" encoder: many more candidates
// per position and Bellman-Ford shortest-path over the match graph.
type Zopfli struct {
	MaxDistance int // maximum match distance; default 1 << 20

	table   []uint32 // hash heads (1-based; 0 = empty)
	chain   []uint32 // hash chain links
	history []byte
}

func (z *Zopfli) Reset() {
	if len(z.table) > 0 {
		clear(z.table)
	}
	z.chain = z.chain[:0]
	z.history = z.history[:0]
}

const (
	zplTableBits = 17
	zplTableSize = 1 << zplTableBits
	zplLenMax    = 262
	zplMinMatch  = 4
	zplChainMax  = 65536
	zplMaxCands  = 128 // max candidates per position
)

func (z *Zopfli) maxDist() int {
	if z.MaxDistance == 0 {
		return 1 << 20
	}
	return z.MaxDistance
}

// FindMatches implements matchfinder.MatchFinder.
func (z *Zopfli) FindMatches(dst []Match, src []byte) []Match {
	if len(src) < 8 {
		return append(dst, Match{Unmatched: len(src)})
	}

	md := z.maxDist()
	if len(z.history) > md*2 {
		delta := len(z.history) - md
		copy(z.history, z.history[delta:])
		z.history = z.history[:md]
		clear(z.table)
		z.chain = z.chain[:0]
	}

	e := matchEmitter{Dst: dst, NextEmit: len(z.history)}
	z.history = append(z.history, src...)
	src = z.history

	blockSize := 1 << 16
	n := len(src) - e.NextEmit
	for off := 0; off < n; off += blockSize {
		blk := src[e.NextEmit+off:]
		if len(blk) > blockSize {
			blk = blk[:blockSize]
		}
		e = z.findBlock(e, blk, md)
	}

	dst = e.Dst
	if e.NextEmit < len(src) {
		dst = append(dst, Match{Unmatched: len(src) - e.NextEmit})
	}
	return dst
}

type zplStep struct {
	isLiteral bool
	dist      uint16
	length    uint16
}

// candScore ranks candidates: higher is better for the DP.
// Length dominates, but closer distance gets a small bonus.
func candScore(length, dist int) int {
	return length*256 - dist/256
}

func (z *Zopfli) findBlock(e matchEmitter, blk []byte, maxDist int) matchEmitter {
	blkLen := len(blk)
	if blkLen < zplMinMatch {
		e.NextEmit += blkLen
		return e
	}

	if len(z.table) < zplTableSize {
		z.table = make([]uint32, zplTableSize)
	}
	if cap(z.chain) < blkLen {
		z.chain = make([]uint32, blkLen)
	} else {
		z.chain = z.chain[:blkLen]
	}
	clear(z.table)

	// Build hash chain.
	for i := 0; i < blkLen-3; i++ {
		key := hashZpl(blk, i) & (zplTableSize - 1)
		if prev := z.table[key]; prev != 0 {
			z.chain[i] = prev - 1
		} else {
			z.chain[i] = math.MaxUint32
		}
		z.table[key] = uint32(i + 1)
	}
	for i := max(0, blkLen-3); i < blkLen; i++ {
		z.chain[i] = math.MaxUint32
	}

	// Phase 1: collect candidates at each position.
	type candidate struct {
		length uint16
		dist   uint16
	}

	// candsByPos tracks the flat candidates slice: cands[start[i]:end[i]]
	// belongs to position i.
	cands := make([]candidate, 0, blkLen*4)
	candStart := make([]int, blkLen)
	candEnd := make([]int, blkLen)

	for i := 0; i < blkLen-zplMinMatch; i++ {
		start := len(cands)
		added := 0

		for cur := z.chain[i]; cur != math.MaxUint32 && added < zplMaxCands; cur = z.chain[cur] {
			added++
			dist := uint32(i) - cur
			if dist == 0 || int(dist) > maxDist {
				continue
			}
			if int(cur)+4 > blkLen {
				continue
			}
			if binary.LittleEndian.Uint32(blk[cur:]) != binary.LittleEndian.Uint32(blk[i:]) {
				continue
			}
			end := extendMatchBoundedZpl(blk, int(cur)+4, i+4, zplLenMax)
			ml := end - i
			if ml < zplMinMatch {
				continue
			}
			cands = append(cands, candidate{length: uint16(ml), dist: uint16(dist)})
		}

		// Keep only the best zplMaxCands candidates at each position,
		// sorted by score descending. Deduplicate by distance (keep longest).
		if len(cands)-start > 1 {
			best := cands[start:]
			slices.SortFunc(best, func(a, b candidate) int {
				sa := candScore(int(a.length), int(a.dist))
				sb := candScore(int(b.length), int(b.dist))
				return sb - sa
			})
			// Deduplicate: keep longest for each unique distance.
			write := start
			for read := start; read < len(cands); read++ {
				dup := false
				for w := start; w < write; w++ {
					if cands[w].dist == cands[read].dist {
						dup = true
						break
					}
				}
				if !dup && write-start < zplMaxCands {
					cands[write] = cands[read]
					write++
				}
			}
			cands = cands[:write]
		}

		candStart[i] = start
		candEnd[i] = len(cands)
	}

	// Phase 2: DP optimal parse.
	cost := make([]float64, blkLen+1)
	dec := make([]zplStep, blkLen+1)
	for i := 1; i <= blkLen; i++ {
		cost[i] = math.MaxFloat64
	}

	for i := 0; i < blkLen; i++ {
		if cost[i] == math.MaxFloat64 {
			continue
		}

		// Option A: one literal byte.
		c1 := cost[i] + costLiteral()
		if c1 < cost[i+1] {
			cost[i+1] = c1
			dec[i+1] = zplStep{isLiteral: true}
		}

		// Option B: try each candidate match.
		for ci := candStart[i]; ci < candEnd[i]; ci++ {
			c := cands[ci]
			ml := int(c.length)
			if i+ml > blkLen {
				ml = blkLen - i
			}
			base := cost[i] + costMatchBase() + costDistExtra(int(c.dist))
			for l := zplMinMatch; l <= ml; l++ {
				cc := base + costLenExtra(l)
				if cc < cost[i+l] {
					cost[i+l] = cc
					dec[i+l] = zplStep{isLiteral: false, dist: c.dist, length: uint16(l)}
				}
			}
		}
	}

	// Phase 3: trace back from blkLen to 0.
	var steps []zplStep
	pos := blkLen
	for pos > 0 {
		s := dec[pos]
		if !s.isLiteral && s.length == 0 {
			// Guard: force literal if no valid match decision.
			s.isLiteral = true
		}
		steps = append(steps, s)
		if s.isLiteral {
			pos--
		} else {
			nl := int(s.length)
			if nl < zplMinMatch {
				nl = zplMinMatch
			}
			if pos-nl < 0 {
				nl = pos
			}
			pos -= nl
		}
	}
	// Reverse to get forward order.
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}

	// Phase 4: emit in forward order.
	var pendingLits int
	for _, s := range steps {
		if s.isLiteral {
			pendingLits++
			continue
		}
		// Safety: distance must be >= 1.
		if s.dist == 0 {
			pendingLits += int(s.length)
			continue
		}
		e.Dst = append(e.Dst, Match{
			Unmatched: pendingLits,
			Length:    int(s.length),
			Distance:  int(s.dist),
		})
		pendingLits = 0
	}
	if pendingLits > 0 {
		e.Dst = append(e.Dst, Match{Unmatched: pendingLits})
	}
	e.NextEmit += blkLen

	return e
}

func hashZpl(data []byte, i int) uint64 {
	v := binary.LittleEndian.Uint32(data[i:])
	return uint64(v) * 0x1E35A7BD1E35A7BD
}

func extendMatchBoundedZpl(data []byte, cur, pos, limit int) int {
	maxEnd := min(pos+limit-(pos-cur), len(data))
	for pos+8 <= maxEnd {
		x := binary.LittleEndian.Uint64(data[cur:]) ^ binary.LittleEndian.Uint64(data[pos:])
		if x != 0 {
			return pos + (bits.TrailingZeros64(x) >> 3)
		}
		cur += 8
		pos += 8
	}
	for ; pos < maxEnd && data[cur] == data[pos]; cur, pos = cur+1, pos+1 {
	}
	return pos
}

func costLiteral() float64              { return 3.2 }
func costMatchBase() float64            { return 4.5 }
func costLenExtra(n int) float64        { return float64(bits.Len32(uint32(n))) * 0.7 }
func costDistExtra(n int) float64 {
	if n <= 1 {
		return 0
	}
	return float64(bits.Len32(uint32(n))) * 0.5
}
