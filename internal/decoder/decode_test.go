package decoder

import "testing"

func TestRejectsBlockSwitchWhenOneTypeIsDeclared(t *testing.T) {
	var s Reader
	s.numBlockTypes = [3]uint32{1, 1, 1}

	for _, safe := range []int{0, 1} {
		for treeType := 0; treeType < 3; treeType++ {
			if got := decodeBlockTypeAndLength(safe, &s, treeType); got != decoderErrorFormatBlockSwitch {
				t.Fatalf("decodeBlockTypeAndLength(safe=%d, tree=%d) = %d, want %d",
					safe, treeType, got, decoderErrorFormatBlockSwitch)
			}
		}
	}
}

func TestBlockSwitchErrorPropagates(t *testing.T) {
	var s Reader
	s.numBlockTypes = [3]uint32{1, 1, 1}

	for name, decode := range map[string]func(*Reader) int{
		"literal":   decodeLiteralBlockSwitch,
		"command":   decodeCommandBlockSwitch,
		"distance":  decodeDistanceBlockSwitch,
		"safe-lit":  safeDecodeLiteralBlockSwitch,
		"safe-cmd":  safeDecodeCommandBlockSwitch,
		"safe-dist": safeDecodeDistanceBlockSwitch,
	} {
		if got := decode(&s); got != decoderErrorFormatBlockSwitch {
			t.Fatalf("%s returned %d, want %d", name, got, decoderErrorFormatBlockSwitch)
		}
	}
}
