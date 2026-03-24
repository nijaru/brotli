package brotli

import (
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/dictionary"
)

func initEncoderDictionary(dict *common.EncoderDictionary) {
	dict.Words = dictionary.GetDictionary()
	dict.Hash_table = dictionary.StaticDictionaryHash[:]
	dict.Buckets = dictionary.StaticDictionaryBuckets[:]
	dict_words := make([]any, len(dictionary.StaticDictionaryWords))
	for i, w := range dictionary.StaticDictionaryWords {
		dict_words[i] = w
	}
	dict.Dict_words = dict_words
}
