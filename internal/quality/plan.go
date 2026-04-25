package quality

// Quality thresholds.
const (
	Q0                                        = 0
	Q1                                        = 1
	QZopfli                                   = 10
	QHQZopfli                                 = 11
	MinQualityForBlockSplit                   = 4
	MinQualityForNonzeroDistanceParams        = 4
	MinQualityForOptimizeHistograms           = 4
	MinQualityForExtensiveReferenceSearch     = 5
	MinQualityForContextModeling              = 5
	MinQualityForHqContextModeling            = 7
	MinQualityForHqBlockSplitting             = 10
	MaxNumDelayedSymbols                      = 0x2FFF
	MaxQualityForStaticEntropyCodes           = 2
	MinWindowBits                             = 10
	MaxWindowBits                             = 24
	LargeMaxWindowBits                        = 30
	MinInputBlockBits                         = 16
	MaxInputBlockBits                         = 24
	MinQuality                                = 0
	MaxQuality                                = 11
	MaxZopfliLenQuality10                     = 150
	MaxZopfliLenQuality11                     = 325
	LongCopyQuickStep                         = 16384
	MaxZopfliCandidatesQ10                    = 1
	MaxZopfliCandidatesQ11                    = 5
	SparseSearchSpreeLow                      = 64
	SparseSearchSpreeHigh                     = 512
)

// Tier identifies the encoder strategy for a quality level.
type Tier int

const (
	TierQ0     Tier = iota // quality 0: single-pass fast
	TierQ1                 // quality 1: two-pass fast
	TierGeneric            // quality 2-9: generic metablock
	TierZopfli             // quality 10-11: zopfli optimization
)

// Plan carries all quality-derived parameters, computed once at writer creation.
type Plan struct {
	Tier             Tier
	StaticEntropy    bool    // quality <= 2: use precomputed Huffman codes
	BlockSplit       bool    // quality >= 4: use block splitting
	ContextModeling  bool    // quality >= 5: use context-aware literal encoding
	HQContext        bool    // quality >= 7: high-quality context modeling
	HQBlockSplit     bool    // quality >= 10: high-quality block splitting
	DistanceParams   bool    // quality >= 4: nonzero distance parameters
	OptimizeHisto    bool    // quality >= 4: optimize histogram clusters
	ExtensiveRef     bool    // quality >= 5: extensive distance reference search
	Lgblock          int     // input block log size (computed from quality + lgwin)
	MaxZopfliLen     int     // 150 for Q10, 325 for Q11
	ZopfliCandidates int     // 1 for Q10, 5 for Q11
	SparseSearchSpree int    // 64 for Q<9, 512 for Q>=9
	LargeWindow      bool    // enable large window (lgwin > 24)
}

// NewPlan computes all quality-derived parameters.
func NewPlan(quality, lgwin, lgblock int, sizeHint uint64, largeWindow bool) Plan {
	quality = clamp(quality, 0, 11)

	// Tier selection
	var tier Tier
	switch {
	case quality == 0:
		tier = TierQ0
	case quality == 1:
		tier = TierQ1
	case quality >= 10:
		tier = TierZopfli
	default:
		tier = TierGeneric
	}

	// Large window only for quality > 2
	if quality <= 2 {
		largeWindow = false
	}

	// Lgwin bounds
	minLgwin, maxLgwin := 10, 24
	if largeWindow {
		maxLgwin = 30
	}
	lgwin = max(minLgwin, min(maxLgwin, lgwin))

	// Lgblock computation
	switch {
	case tier <= TierQ1:
		lgblock = lgwin // fast paths use full window
	case quality < 4:
		lgblock = 14
	default:
		if lgblock == 0 {
			lgblock = 16
			if quality >= 9 && lgwin > 16 {
				lgblock = min(18, lgwin)
			}
		} else {
			lgblock = max(16, min(24, lgblock))
		}
	}

	return Plan{
		Tier:            tier,
		StaticEntropy:   quality <= 2,
		BlockSplit:      quality >= 4,
		ContextModeling: quality >= 5,
		HQContext:       quality >= 7,
		HQBlockSplit:    quality >= 10,
		DistanceParams:  quality >= 4,
		OptimizeHisto:   quality >= 4,
		ExtensiveRef:    quality >= 5,
		Lgblock:         lgblock,
		MaxZopfliLen:    zopfliLen(quality),
		ZopfliCandidates: zopfliCandidates(quality),
		SparseSearchSpree: sparseSearchSpree(quality),
		LargeWindow:     largeWindow,
	}
}

// HasherType returns the hasher type for this plan.
func (p Plan) HasherType(lgwin int, sizeHint uint64) int {
	switch {
	case p.Tier == TierZopfli:
		return 10
	case p.Tier == TierGeneric && lgwin <= 16:
		switch {
		case p.ContextModeling && !p.HQContext: // Q5-6
			return 40
		case p.HQContext && p.Tier != TierZopfli: // Q7-8
			return 41
		default: // Q9
			return 42
		}
	case p.Tier == TierGeneric && sizeHint >= 1<<20 && lgwin >= 19:
		return 6
	case p.Tier == TierGeneric && !p.LargeWindow:
		return 5
	case p.Tier <= TierQ1:
		// Fast paths use quality as hasher type
		return 0
	default:
		return 5
	}
}

// HasherParams returns composite hasher parameters.
func (p Plan) HasherParams() (blockBits, bucketBits, numDistCheck int) {
	if p.Tier <= TierQ1 {
		return 0, 0, 0
	}
	blockBits = 0
	switch {
	case p.HQContext && !p.HQBlockSplit: // Q7-8
		blockBits = 7
		bucketBits = 15
		numDistCheck = 10
	case p.HQBlockSplit: // Q9+
		blockBits = 8
		bucketBits = 15
		numDistCheck = 16
	default: // Q5-6
		blockBits = 5
		bucketBits = 14
		numDistCheck = 4
	}
	return blockBits, bucketBits, numDistCheck
}

// HasherTypeForLargeWindow adjusts hasher type for large window mode.
func (p Plan) HasherTypeForLargeWindow(lgwin int, sizeHint uint64) int {
	base := p.HasherType(lgwin, sizeHint)
	switch base {
	case 3:
		return 35
	case 54:
		return 55
	case 6:
		return 65
	default:
		return base
	}
}

func zopfliLen(quality int) int {
	switch {
	case quality <= 10:
		return 150
	default:
		return 325
	}
}

func zopfliCandidates(quality int) int {
	switch {
	case quality <= 10:
		return 1
	default:
		return 5
	}
}

func sparseSearchSpree(quality int) int {
	if quality < 9 {
		return 64
	}
	return 512
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
