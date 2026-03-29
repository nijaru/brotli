package hasher

import (
	"encoding/binary"
	"fmt"
	"github.com/nijaru/brotli/internal/common"
	"github.com/nijaru/brotli/internal/dictionary"
)

type hasherCommon struct {
	params           common.HasherParams
	Is_prepared_     bool
	dict_num_lookups uint
	dict_num_matches uint
}

func (h *hasherCommon) Common() *hasherCommon {
	return h
}

type Handle interface {
	Common() *hasherCommon
	Initialize(params *common.EncoderParams)
	Prepare(one_shot bool, input_size uint, data []byte)
	StitchToPreviousBlock(num_bytes uint, position uint, ringbuffer []byte, ringbuffer_mask uint)
	HashTypeLength() uint
	StoreLookahead() uint
	PrepareDistanceCache(distance_cache []int)
	FindLongestMatch(dictionary *common.EncoderDictionary, data []byte, ring_buffer_mask uint, distance_cache []int, cur_ix uint, max_length uint, max_backward uint, gap uint, max_distance uint, out *SearchResult)
	StoreRange(data []byte, mask uint, ix_start uint, ix_end uint)
	Store(data []byte, mask uint, ix uint)
}

const KCutoffTransformsCount uint32 = 10

/*   0,  12,   27,    23,    42,    63,    56,    48,    59,    64 */
/* 0+0, 4+8, 8+19, 12+11, 16+26, 20+43, 24+32, 28+20, 32+27, 36+28 */
const KCutoffTransforms uint64 = 0x071B520ADA2D3200

type SearchResult struct {
	Len            uint
	Distance       uint
	Score          uint
	Len_code_delta int
}

const KHashMul32 uint32 = 0x1E35A7BD
const kHashMul64 uint64 = 0x1E35A7BD1E35A7BD
const kHashMul64Long uint64 = 0x1FE35A7BD3579BD3

func hash14(data []byte) uint32 {
	var h uint32 = binary.LittleEndian.Uint32(data) * KHashMul32
	return h >> (32 - 14)
}

func prepareDistanceCache(distance_cache []int, num_distances int) {
	if num_distances > 4 {
		var last_distance int = distance_cache[0]
		distance_cache[4] = last_distance - 1
		distance_cache[5] = last_distance + 1
		distance_cache[6] = last_distance - 2
		distance_cache[7] = last_distance + 2
		distance_cache[8] = last_distance - 3
		distance_cache[9] = last_distance + 3
		if num_distances > 10 {
			var next_last_distance int = distance_cache[1]
			distance_cache[10] = next_last_distance - 1
			distance_cache[11] = next_last_distance + 1
			distance_cache[12] = next_last_distance - 2
			distance_cache[13] = next_last_distance + 2
			distance_cache[14] = next_last_distance - 3
			distance_cache[15] = next_last_distance + 3
		}
	}
}

const literalByteScore = 135
const distanceBitPenalty = 30
const ScoreBase = (distanceBitPenalty * 8 * 8)

func backwardReferenceScore(copy_length uint, backward_reference_offset uint) uint {
	return ScoreBase + literalByteScore*uint(copy_length) - distanceBitPenalty*uint(common.Log2FloorNonZero(backward_reference_offset))
}

func backwardReferenceScoreUsingLastDistance(copy_length uint) uint {
	return literalByteScore*uint(copy_length) + ScoreBase + 15
}

func backwardReferencePenaltyUsingLastDistance(distance_short_code uint) uint {
	return uint(39) + ((0x1CA10 >> (distance_short_code & 0xE)) & 0xE)
}

func testStaticDictionaryItem(dict *common.EncoderDictionary, item uint, data []byte, max_length uint, max_backward uint, max_distance uint, out *SearchResult) bool {
	var len uint
	var word_idx uint
	var offset uint
	var matchlen uint
	var backward uint
	var score uint
	len = item & 0x1F
	word_idx = item >> 5

	words := dict.Words.(*dictionary.Dictionary)
	offset = uint(words.Offsets_by_length[len]) + len*word_idx
	if len > max_length {
		return false
	}

	matchlen = common.FindMatchLengthWithLimit(data, words.Data[offset:], uint(len))
	if matchlen+uint(dict.CutoffTransformsCount) <= len || matchlen == 0 {
		return false
	}
	{
		var cut uint = len - matchlen
		var transform_id uint = (cut << 2) + uint((dict.CutoffTransforms>>(cut*6))&0x3F)
		backward = max_backward + 1 + word_idx + (transform_id << words.Size_bits_by_length[len])
	}

	if backward > max_distance {
		return false
	}

	score = backwardReferenceScore(matchlen, backward)
	if score < out.Score {
		return false
	}

	out.Len = matchlen
	out.Len_code_delta = int(len) - int(matchlen)
	out.Distance = backward
	out.Score = score
	return true
}

func searchInStaticDictionary(dict *common.EncoderDictionary, handle Handle, data []byte, max_length uint, max_backward uint, max_distance uint, out *SearchResult, shallow bool) {
	var key uint
	var i uint
	var self *hasherCommon = handle.Common()
	if self.dict_num_matches < self.dict_num_lookups>>7 {
		return
	}

	key = uint(hash14(data) << 1)
	for i = 0; ; (func() { i++; key++ })() {
		var tmp uint
		if shallow {
			tmp = 1
		} else {
			tmp = 2
		}
		if i >= tmp {
			break
		}
		var item uint = uint(dict.Hash_table[key])
		self.dict_num_lookups++
		if item != 0 {
			var item_matches bool = testStaticDictionaryItem(dict, item, data, max_length, max_backward, max_distance, out)
			if item_matches {
				self.dict_num_matches++
			}
		}
	}
}

type BackwardMatch struct {
	Distance        uint32
	Length          uint32
	distance        uint32
	length_and_code uint32
}

func initBackwardMatch(self *BackwardMatch, dist uint, len uint) {
	self.Distance = uint32(dist)
	self.Length = uint32(len)
	self.distance = uint32(dist)
	self.length_and_code = uint32(len << 5)
}

func initDictionaryBackwardMatch(self *BackwardMatch, dist uint, len uint, len_code uint) {
	self.Distance = uint32(dist)
	self.Length = uint32(len)
	self.distance = uint32(dist)
	var tmp uint
	if len == len_code {
		tmp = 0
	} else {
		tmp = len_code
	}
	self.length_and_code = uint32(len<<5 | tmp)
}

func BackwardMatchLength(self *BackwardMatch) uint {
	return uint(self.length_and_code >> 5)
}

func BackwardMatchLengthCode(self *BackwardMatch) uint {
	var code uint = uint(self.length_and_code) & 31
	if code != 0 {
		return code
	} else {
		return BackwardMatchLength(self)
	}
}

func HasherReset(handle Handle) {
	if handle == nil {
		return
	}
	handle.Common().Is_prepared_ = false
}

func HasherSetup(handle *Handle, params *common.EncoderParams, data []byte, position uint, input_size uint, is_last bool) {
	var self Handle = nil
	var common_ptr *hasherCommon = nil
	var one_shot bool = (position == 0 && is_last)
	if *handle == nil {
		// chooseHasher(params, &params.Hasher) // This should be moved or handled
		self = NewHasher(params.Hasher.Type_)

		*handle = self
		common_ptr = self.Common()
		common_ptr.params = params.Hasher
		self.Initialize(params)
	}

	self = *handle
	common_ptr = self.Common()
	if !common_ptr.Is_prepared_ {
		self.Prepare(one_shot, input_size, data)

		if position == 0 {
			common_ptr.dict_num_lookups = 0
			common_ptr.dict_num_matches = 0
		}

		common_ptr.Is_prepared_ = true
	}
}

func InitOrStitchToPreviousBlock(handle *Handle, data []byte, mask uint, params *common.EncoderParams, position uint, input_size uint, is_last bool) {
	HasherSetup(handle, params, data, position, input_size, is_last)
	self := *handle
	self.StitchToPreviousBlock(input_size, position, data, mask)
}

func NewHasher(typ int) Handle {
	switch typ {
	case 2:
		return &hashLongestMatchQuickly{
			bucketBits:    16,
			bucketSweep:   1,
			hashLen:       5,
			useDictionary: true,
		}
	case 3:
		return &hashLongestMatchQuickly{
			bucketBits:    16,
			bucketSweep:   2,
			hashLen:       5,
			useDictionary: false,
		}
	case 4:
		return &hashLongestMatchQuickly{
			bucketBits:    17,
			bucketSweep:   4,
			hashLen:       5,
			useDictionary: true,
		}
	case 5:
		return new(h5)
	case 6:
		return new(h6)
	case 10:
		return new(H10)
	case 35:
		return &hashComposite{
			ha: NewHasher(3),
			hb: &hashRolling{jump: 4},
		}
	case 40:
		return &hashForgetfulChain{
			bucketBits:              15,
			numBanks:                1,
			bankBits:                16,
			numLastDistancesToCheck: 4,
		}
	case 41:
		return &hashForgetfulChain{
			bucketBits:              15,
			numBanks:                1,
			bankBits:                16,
			numLastDistancesToCheck: 10,
		}
	case 42:
		return &hashForgetfulChain{
			bucketBits:              15,
			numBanks:                512,
			bankBits:                9,
			numLastDistancesToCheck: 16,
		}
	case 54:
		return &hashLongestMatchQuickly{
			bucketBits:    20,
			bucketSweep:   4,
			hashLen:       7,
			useDictionary: false,
		}
	case 55:
		return &hashComposite{
			ha: NewHasher(54),
			hb: &hashRolling{jump: 4},
		}
	case 65:
		return &hashComposite{
			ha: NewHasher(6),
			hb: &hashRolling{jump: 1},
		}
	}

	panic(fmt.Sprintf("unknown hasher type: %d", typ))
}
