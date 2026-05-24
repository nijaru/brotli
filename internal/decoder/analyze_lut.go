package decoder

import "fmt"

func AnalyzeLut() {
	printSymbolRange("depth[0..7] -> cmd_depth[0..7]", []int{0, 1, 2, 3, 4, 5, 6, 7})
	printSymbolRange("depth[8..15] -> cmd_depth[64..71]", []int{64, 65, 66, 67, 68, 69, 70, 71})
	printSymbolRange(
		"depth[16..23] -> cmd_depth[128..135]",
		[]int{128, 129, 130, 131, 132, 133, 134, 135},
	)
	printSymbolRange(
		"depth[24..31] -> cmd_depth[192..199]",
		[]int{192, 193, 194, 195, 196, 197, 198, 199},
	)
	printSymbolRange(
		"depth[32..39] -> cmd_depth[384..391]",
		[]int{384, 385, 386, 387, 388, 389, 390, 391},
	)

	range5 := []int{}
	range6 := []int{}
	range7 := []int{}
	for i := 0; i < 8; i++ {
		range5 = append(range5, 128+8*i)
		range6 = append(range6, 256+8*i)
		range7 = append(range7, 448+8*i)
	}
	printSymbolRange("depth[40..47] -> cmd_depth[128, 136, ...]", range5)
	printSymbolRange("depth[48..55] -> cmd_depth[256, 264, ...]", range6)
	printSymbolRange("depth[56..63] -> cmd_depth[448, 456, ...]", range7)
}

func printSymbolRange(name string, symbols []int) {
	fmt.Printf("\n=== %s ===\n", name)
	fmt.Println("Symbol | InsertLenOffset | CopyLenOffset | DistanceCode | InsertExtra | CopyExtra")
	fmt.Println(
		"----------------------------------------------------------------------------------",
	)
	for _, i := range symbols {
		entry := kCmdLut[i]
		fmt.Printf(
			"%6d | %15d | %13d | %12d | %11d | %9d\n",
			i,
			entry.InsertLenOffset,
			entry.CopyLenOffset,
			entry.DistanceCode,
			entry.InsertLenExtraBits,
			entry.CopyLenExtraBits,
		)
	}
}
