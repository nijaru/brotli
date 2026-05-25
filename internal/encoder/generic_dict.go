package encoder

import (
	"github.com/nijaru/brotli/internal/common")

func initEncoderDictionary(dict *common.EncoderDictionary) {
	dict.Words = common.GetDictionary()
	dict.Hash_table = common.StaticDictionaryHash[:]
	dict.Buckets = common.StaticDictionaryBuckets[:]
	dict.CutoffTransformsCount = KCutoffTransformsCount
	dict.CutoffTransforms = KCutoffTransforms
}
