package brotli

import (
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/dictionary"
	"github.com/nijaru/brotli/internal/hasher"
)

func initEncoderDictionary(dict *common.EncoderDictionary) {
	dict.Words = dictionary.GetDictionary()
	dict.Hash_table = dictionary.StaticDictionaryHash[:]
	dict.Buckets = dictionary.StaticDictionaryBuckets[:]
	dict.CutoffTransformsCount = hasher.KCutoffTransformsCount
	dict.CutoffTransforms = hasher.KCutoffTransforms
}
