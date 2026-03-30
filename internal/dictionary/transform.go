package dictionary

const (
	TransformIdentity       = 0
	TransformOmitLast1      = 1
	TransformOmitLast2      = 2
	TransformOmitLast3      = 3
	TransformOmitLast4      = 4
	TransformOmitLast5      = 5
	TransformOmitLast6      = 6
	TransformOmitLast7      = 7
	TransformOmitLast8      = 8
	TransformOmitLast9      = 9
	TransformUppercaseFirst = 10
	TransformUppercaseAll   = 11
	TransformOmitFirst1     = 12
	TransformOmitFirst2     = 13
	TransformOmitFirst3     = 14
	TransformOmitFirst4     = 15
	TransformOmitFirst5     = 16
	TransformOmitFirst6     = 17
	TransformOmitFirst7     = 18
	TransformOmitFirst8     = 19
	TransformOmitFirst9     = 20
	TransformShiftFirst     = 21
	TransformShiftAll       = 22 + iota - 22
	NumTransformTypes
)

const TransformsMaxCutOff = TransformOmitLast9

type Transforms struct {
	PrefixSuffixSize uint16
	PrefixSuffix      []byte
	PrefixSuffix_map  []uint16
	NumTransforms     uint32
	Transforms         []byte
	Params             []byte
	CutoffTransforms   [TransformsMaxCutOff + 1]int16
}

func transformPrefixId(t *Transforms, I int) byte {
	return t.Transforms[(I*3)+0]
}

func transformType(t *Transforms, I int) byte {
	return t.Transforms[(I*3)+1]
}

func transformSuffixId(t *Transforms, I int) byte {
	return t.Transforms[(I*3)+2]
}

func transformPrefix(t *Transforms, I int) []byte {
	return t.PrefixSuffix[t.PrefixSuffix_map[transformPrefixId(t, I)]:]
}

func transformSuffix(t *Transforms, I int) []byte {
	return t.PrefixSuffix[t.PrefixSuffix_map[transformSuffixId(t, I)]:]
}

/* RFC 7932 transforms string data */
const kPrefixSuffix string = "\001 \002, \010 of the \004 of \002s \001.\005 and \004 " + "in \001\"\004 to \002\">\001\n\002. \001]\005 for \003 a \006 " + "that \001'\006 with \006 from \004 by \001(\006. T" + "he \004 on \004 as \004 is \004ing \002\n\t\001:\003ed " + "\002=\"\004 at \003ly \001,\002='\005.com/\007. This \005" + " not \003er \003al \004ful \004ive \005less \004es" + "t \004ize \002\xc2\xa0\004ous \005 the \002e \000"

var kPrefixSuffixMap = [50]uint16{
	0x00,
	0x02,
	0x05,
	0x0E,
	0x13,
	0x16,
	0x18,
	0x1E,
	0x23,
	0x25,
	0x2A,
	0x2D,
	0x2F,
	0x32,
	0x34,
	0x3A,
	0x3E,
	0x45,
	0x47,
	0x4E,
	0x55,
	0x5A,
	0x5C,
	0x63,
	0x68,
	0x6D,
	0x72,
	0x77,
	0x7A,
	0x7C,
	0x80,
	0x83,
	0x88,
	0x8C,
	0x8E,
	0x91,
	0x97,
	0x9F,
	0xA5,
	0xA9,
	0xAD,
	0xB2,
	0xB7,
	0xBD,
	0xC2,
	0xC7,
	0xCA,
	0xCF,
	0xD5,
	0xD8,
}

/* RFC 7932 transforms */
var kTransformsData = []byte{
	49,
	TransformIdentity,
	49,
	49,
	TransformIdentity,
	0,
	0,
	TransformIdentity,
	0,
	49,
	TransformOmitFirst1,
	49,
	49,
	TransformUppercaseFirst,
	0,
	49,
	TransformIdentity,
	47,
	0,
	TransformIdentity,
	49,
	4,
	TransformIdentity,
	0,
	49,
	TransformIdentity,
	3,
	49,
	TransformUppercaseFirst,
	49,
	49,
	TransformIdentity,
	6,
	49,
	TransformOmitFirst2,
	49,
	49,
	TransformOmitLast1,
	49,
	1,
	TransformIdentity,
	0,
	49,
	TransformIdentity,
	1,
	0,
	TransformUppercaseFirst,
	0,
	49,
	TransformIdentity,
	7,
	49,
	TransformIdentity,
	9,
	48,
	TransformIdentity,
	0,
	49,
	TransformIdentity,
	8,
	49,
	TransformIdentity,
	5,
	49,
	TransformIdentity,
	10,
	49,
	TransformIdentity,
	11,
	49,
	TransformOmitLast3,
	49,
	49,
	TransformIdentity,
	13,
	49,
	TransformIdentity,
	14,
	49,
	TransformOmitFirst3,
	49,
	49,
	TransformOmitLast2,
	49,
	49,
	TransformIdentity,
	15,
	49,
	TransformIdentity,
	16,
	0,
	TransformUppercaseFirst,
	49,
	49,
	TransformIdentity,
	12,
	5,
	TransformIdentity,
	49,
	0,
	TransformIdentity,
	1,
	49,
	TransformOmitFirst4,
	49,
	49,
	TransformIdentity,
	18,
	49,
	TransformIdentity,
	17,
	49,
	TransformIdentity,
	19,
	49,
	TransformIdentity,
	20,
	49,
	TransformOmitFirst5,
	49,
	49,
	TransformOmitFirst6,
	49,
	47,
	TransformIdentity,
	49,
	49,
	TransformOmitLast4,
	49,
	49,
	TransformIdentity,
	22,
	49,
	TransformUppercaseAll,
	49,
	49,
	TransformIdentity,
	23,
	49,
	TransformIdentity,
	24,
	49,
	TransformIdentity,
	25,
	49,
	TransformOmitLast7,
	49,
	49,
	TransformOmitLast1,
	26,
	49,
	TransformIdentity,
	27,
	49,
	TransformIdentity,
	28,
	0,
	TransformIdentity,
	12,
	49,
	TransformIdentity,
	29,
	49,
	TransformOmitFirst9,
	49,
	49,
	TransformOmitFirst7,
	49,
	49,
	TransformOmitLast6,
	49,
	49,
	TransformIdentity,
	21,
	49,
	TransformUppercaseFirst,
	1,
	49,
	TransformOmitLast8,
	49,
	49,
	TransformIdentity,
	31,
	49,
	TransformIdentity,
	32,
	47,
	TransformIdentity,
	3,
	49,
	TransformOmitLast5,
	49,
	49,
	TransformOmitLast9,
	49,
	0,
	TransformUppercaseFirst,
	1,
	49,
	TransformUppercaseFirst,
	8,
	5,
	TransformIdentity,
	21,
	49,
	TransformUppercaseAll,
	0,
	49,
	TransformUppercaseFirst,
	10,
	49,
	TransformIdentity,
	30,
	0,
	TransformIdentity,
	5,
	35,
	TransformIdentity,
	49,
	47,
	TransformIdentity,
	2,
	49,
	TransformUppercaseFirst,
	17,
	49,
	TransformIdentity,
	36,
	49,
	TransformIdentity,
	33,
	5,
	TransformIdentity,
	0,
	49,
	TransformUppercaseFirst,
	21,
	49,
	TransformUppercaseFirst,
	5,
	49,
	TransformIdentity,
	37,
	0,
	TransformIdentity,
	30,
	49,
	TransformIdentity,
	38,
	0,
	TransformUppercaseAll,
	0,
	49,
	TransformIdentity,
	39,
	0,
	TransformUppercaseAll,
	49,
	49,
	TransformIdentity,
	34,
	49,
	TransformUppercaseAll,
	8,
	49,
	TransformUppercaseFirst,
	12,
	0,
	TransformIdentity,
	21,
	49,
	TransformIdentity,
	40,
	0,
	TransformUppercaseFirst,
	12,
	49,
	TransformIdentity,
	41,
	49,
	TransformIdentity,
	42,
	49,
	TransformUppercaseAll,
	17,
	49,
	TransformIdentity,
	43,
	0,
	TransformUppercaseFirst,
	5,
	49,
	TransformUppercaseAll,
	10,
	0,
	TransformIdentity,
	34,
	49,
	TransformUppercaseFirst,
	33,
	49,
	TransformIdentity,
	44,
	49,
	TransformUppercaseAll,
	5,
	45,
	TransformIdentity,
	49,
	0,
	TransformIdentity,
	33,
	49,
	TransformUppercaseFirst,
	30,
	49,
	TransformUppercaseAll,
	30,
	49,
	TransformIdentity,
	46,
	49,
	TransformUppercaseAll,
	1,
	49,
	TransformUppercaseFirst,
	34,
	0,
	TransformUppercaseFirst,
	33,
	0,
	TransformUppercaseAll,
	30,
	0,
	TransformUppercaseAll,
	1,
	49,
	TransformUppercaseAll,
	33,
	49,
	TransformUppercaseAll,
	21,
	49,
	TransformUppercaseAll,
	12,
	0,
	TransformUppercaseAll,
	5,
	49,
	TransformUppercaseAll,
	34,
	0,
	TransformUppercaseAll,
	12,
	0,
	TransformUppercaseFirst,
	30,
	0,
	TransformUppercaseAll,
	34,
	0,
	TransformUppercaseFirst,
	34,
}

var kBrotliTransforms = Transforms{
	217,
	[]byte(kPrefixSuffix),
	kPrefixSuffixMap[:],
	121,
	kTransformsData,
	nil, /* no extra parameters */
	[TransformsMaxCutOff + 1]int16{0, 12, 27, 23, 42, 63, 56, 48, 59, 64},
}

func GetTransforms() *Transforms {
	return &kBrotliTransforms
}

func toUpperCase(p []byte) int {
	if p[0] < 0xC0 {
		if p[0] >= 'a' && p[0] <= 'z' {
			p[0] ^= 32
		}

		return 1
	}

	/* An overly simplified uppercasing model for UTF-8. */
	if p[0] < 0xE0 {
		p[1] ^= 32
		return 2
	}

	/* An arbitrary transform for three byte characters. */
	p[2] ^= 5

	return 3
}

func shiftTransform(word []byte, word_len int, parameter uint16) int {
	/* Limited sign extension: scalar < (1 << 24). */
	var scalar uint32 = (uint32(parameter) & 0x7FFF) + (0x1000000 - (uint32(parameter) & 0x8000))
	if word[0] < 0x80 {
		/* 1-byte rune / 0sssssss / 7 bit scalar (ASCII). */
		scalar += uint32(word[0])

		word[0] = byte(scalar & 0x7F)
		return 1
	} else if word[0] < 0xC0 {
		/* Continuation / 10AAAAAA. */
		return 1
	} else if word[0] < 0xE0 {
		/* 2-byte rune / 110sssss AAssssss / 11 bit scalar. */
		if word_len < 2 {
			return 1
		}
		scalar += uint32(word[1]&0x3F | (word[0]&0x1F)<<6)
		word[0] = byte(0xC0 | (scalar>>6)&0x1F)
		word[1] = byte(uint32(word[1]&0xC0) | scalar&0x3F)
		return 2
	} else if word[0] < 0xF0 {
		/* 3-byte rune / 1110ssss AAssssss BBssssss / 16 bit scalar. */
		if word_len < 3 {
			return word_len
		}
		scalar += uint32(word[2])&0x3F | uint32(word[1]&0x3F)<<6 | uint32(word[0]&0x0F)<<12
		word[0] = byte(0xE0 | (scalar>>12)&0x0F)
		word[1] = byte(uint32(word[1]&0xC0) | (scalar>>6)&0x3F)
		word[2] = byte(uint32(word[2]&0xC0) | scalar&0x3F)
		return 3
	} else if word[0] < 0xF8 {
		/* 4-byte rune / 11110sss AAssssss BBssssss CCssssss / 21 bit scalar. */
		if word_len < 4 {
			return word_len
		}
		scalar += uint32(word[3])&0x3F | uint32(word[2]&0x3F)<<6 | uint32(word[1]&0x3F)<<12 | uint32(word[0]&0x07)<<18
		word[0] = byte(0xF0 | (scalar>>18)&0x07)
		word[1] = byte(uint32(word[1]&0xC0) | (scalar>>12)&0x3F)
		word[2] = byte(uint32(word[2]&0xC0) | (scalar>>6)&0x3F)
		word[3] = byte(uint32(word[3]&0xC0) | scalar&0x3F)
		return 4
	}

	return 1
}

func TransformDictionaryWord(dst []byte, word []byte, len int, trans *Transforms, transform_idx int) int {
	var idx int = 0
	var prefix []byte = transformPrefix(trans, transform_idx)
	var type_ byte = transformType(trans, transform_idx)
	var suffix []byte = transformSuffix(trans, transform_idx)
	{
		var prefix_len int = int(prefix[0])
		prefix = prefix[1:]
		for {
			tmp1 := prefix_len
			prefix_len--
			if tmp1 == 0 {
				break
			}
			dst[idx] = prefix[0]
			idx++
			prefix = prefix[1:]
		}
	}
	{
		var t int = int(type_)
		var i int = 0
		if t <= TransformOmitLast9 {
			len -= t
		} else if t >= TransformOmitFirst1 && t <= TransformOmitFirst9 {
			var skip int = t - (TransformOmitFirst1 - 1)
			word = word[skip:]
			len -= skip
		}

		for i < len {
			dst[idx] = word[i]
			idx++
			i++
		}
		if t == TransformUppercaseFirst {
			toUpperCase(dst[idx-len:])
		} else if t == TransformUppercaseAll {
			var uppercase []byte = dst
			uppercase = uppercase[idx-len:]
			for len > 0 {
				var step int = toUpperCase(uppercase)
				uppercase = uppercase[step:]
				len -= step
			}
		} else if t == TransformShiftFirst {
			var param uint16 = uint16(trans.Params[transform_idx*2]) + uint16(trans.Params[transform_idx*2+1])<<8
			shiftTransform(dst[idx-len:], int(len), param)
		} else if t == TransformShiftAll {
			var param uint16 = uint16(trans.Params[transform_idx*2]) + uint16(trans.Params[transform_idx*2+1])<<8
			var shift []byte = dst
			shift = shift[idx-len:]
			for len > 0 {
				var step int = shiftTransform(shift, int(len), param)
				shift = shift[step:]
				len -= step
			}
		}
	}
	{
		var suffix_len int = int(suffix[0])
		suffix = suffix[1:]
		for {
			tmp2 := suffix_len
			suffix_len--
			if tmp2 == 0 {
				break
			}
			dst[idx] = suffix[0]
			idx++
			suffix = suffix[1:]
		}
		return idx
	}
}
