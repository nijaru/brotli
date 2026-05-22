package match

func adjustTableOffsets(tbl []tableEntry, delta int) {
	for i := range tbl {
		v := tbl[i].offset - int32(delta)
		if v < 0 {
			tbl[i] = tableEntry{}
		} else {
			tbl[i].offset = v
		}
	}
}

func rebaseTableOffsets(tbl []tableEntry, minOffset, newBase int32) {
	for i := range tbl {
		v := tbl[i].offset
		if v < minOffset {
			tbl[i].offset = 0
		} else {
			tbl[i].offset = v - minOffset + newBase
		}
	}
}
