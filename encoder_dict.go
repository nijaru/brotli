package brotli

import "github.com/nijaru/brotli/internal/dictionary"

/* Dictionary data (words and transforms) for 1 possible context */
type encoderDictionary struct {
	words                 *dictionary.Dictionary
	cutoffTransformsCount uint32
	cutoffTransforms      uint64
	hash_table            []uint16
	buckets               []uint16
	dict_words            []dictionary.DictWord
}

func initEncoderDictionary(dict *encoderDictionary) {
	dict.words = dictionary.GetDictionary()

	dict.hash_table = dictionary.KStaticDictionaryHash[:]
	dict.buckets = dictionary.KStaticDictionaryBuckets[:]
	dict.dict_words = dictionary.KStaticDictionaryWords[:]

	dict.cutoffTransformsCount = dictionary.KCutoffTransformsCount
	dict.cutoffTransforms = dictionary.KCutoffTransforms
}
