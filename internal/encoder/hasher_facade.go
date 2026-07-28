package encoder

import "github.com/nijaru/brotli/internal/encoder/hash"

type (
	Handle        = hash.Handle
	SearchResult  = hash.SearchResult
	HasherHandle  = hash.Handle
	BackwardMatch = hash.BackwardMatch
	H10           = hash.H10
)

const (
	ScoreBase              = hash.ScoreBase
	MaxNumMatchesH10       = hash.MaxNumMatchesH10
	KHashMul32             = hash.KHashMul32
	KCutoffTransformsCount = hash.KCutoffTransformsCount
	KCutoffTransforms      = hash.KCutoffTransforms
)

var (
	NewHasher                   = hash.NewHasher
	HasherSetup                 = hash.HasherSetup
	HasherReset                 = hash.HasherReset
	InitOrStitchToPreviousBlock  = hash.InitOrStitchToPreviousBlock
	BackwardMatchLength         = hash.BackwardMatchLength
	BackwardMatchLengthCode     = hash.BackwardMatchLengthCode
	FindAllMatchesH10           = hash.FindAllMatchesH10
)
