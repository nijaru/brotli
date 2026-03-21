package common

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Functions to estimate the bit cost of Huffman trees. */
func ShannonEntropy(population []uint32, size uint, total *uint) float64 {
	var sum uint = 0
	var retval float64 = 0
	var population_end []uint32 = population[size:]
	var p uint
	for -cap(population) < -cap(population_end) {
		p = uint(population[0])
		population = population[1:]
		sum += p
		if p != 0 {
			retval -= float64(p) * FastLog2(p)
		}
	}

	if sum != 0 {
		retval += float64(sum) * FastLog2(sum)
	}
	*total = sum
	return retval
}

func BitsEntropy(population []uint32, size uint) float64 {
	var sum uint
	var retval float64 = ShannonEntropy(population, size, &sum)
	if retval < float64(sum) {
		/* At least one bit per literal is needed. */
		retval = float64(sum)
	}

	return retval
}

const KOneSymbolHistogramCost float64 = 12
const KTwoSymbolHistogramCost float64 = 20
const KThreeSymbolHistogramCost float64 = 28
const KFourSymbolHistogramCost float64 = 37

func PopulationCostLiteral(histogram *HistogramLiteral) float64 {
	var data_size uint = HistogramDataSizeLiteral()
	var count int = 0
	var s [5]uint
	var i uint
	if histogram.Total_count_ == 0 {
		return KOneSymbolHistogramCost
	}

	for i = 0; i < data_size; i++ {
		if histogram.Data_[i] > 0 {
			s[count] = i
			count++
			if count > 4 {
				break
			}
		}
	}

	if count == 1 {
		return KOneSymbolHistogramCost
	}

	if count == 2 {
		return KTwoSymbolHistogramCost + float64(histogram.Total_count_)
	}

	if count == 3 {
		var histo0 uint32 = histogram.Data_[s[0]]
		var histo1 uint32 = histogram.Data_[s[1]]
		var histo2 uint32 = histogram.Data_[s[2]]
		var histomax uint32 = histo0
		if histo1 > histomax {
			histomax = histo1
		}
		if histo2 > histomax {
			histomax = histo2
		}
		return KThreeSymbolHistogramCost + 2*(float64(histo0)+float64(histo1)+float64(histo2)) - float64(histomax)
	}

	if count == 4 {
		var histo0 uint32 = histogram.Data_[s[0]]
		var histo1 uint32 = histogram.Data_[s[1]]
		var histo2 uint32 = histogram.Data_[s[2]]
		var histo3 uint32 = histogram.Data_[s[3]]
		var histomax uint32 = histo0
		if histo1 > histomax {
			histomax = histo1
		}
		if histo2 > histomax {
			histomax = histo2
		}
		if histo3 > histomax {
			histomax = histo3
		}
		return KFourSymbolHistogramCost + 3*(float64(histo0)+float64(histo1)+float64(histo2)+float64(histo3)) - 2*float64(histomax)
	}

	var max_bits float64 = float64(Log2FloorNonZero(data_size-1) + 1)
	var hist_bits float64 = ShannonEntropy(histogram.Data_[:], data_size, &i)
	/* This approximation is too \`optimistic\` for small histograms,
	   especially for those with many zeros. */
	if hist_bits < 2.0*float64(histogram.Total_count_) {
		hist_bits += 0.5 * float64(histogram.Total_count_)
	}

	return hist_bits + max_bits*float64(count)
}

func PopulationCostCommand(histogram *HistogramCommand) float64 {
	var data_size uint = HistogramDataSizeCommand()
	var count int = 0
	var i uint
	if histogram.Total_count_ == 0 {
		return KOneSymbolHistogramCost
	}

	for i = 0; i < data_size; i++ {
		if histogram.Data_[i] > 0 {
			count++
			if count > 4 {
				break
			}
		}
	}

	if count == 1 {
		return KOneSymbolHistogramCost
	}

	if count == 2 {
		return KTwoSymbolHistogramCost + float64(histogram.Total_count_)
	}

	if count == 3 {
		return KThreeSymbolHistogramCost + 2*float64(histogram.Total_count_)
	}

	if count == 4 {
		return KFourSymbolHistogramCost + 3*float64(histogram.Total_count_)
	}

	var max_bits float64 = float64(Log2FloorNonZero(data_size-1) + 1)
	var hist_bits float64 = ShannonEntropy(histogram.Data_[:], data_size, &i)
	return hist_bits + max_bits*float64(count)
}

func PopulationCostDistance(histogram *HistogramDistance) float64 {
	var data_size uint = HistogramDataSizeDistance()
	var count int = 0
	var i uint
	if histogram.Total_count_ == 0 {
		return KOneSymbolHistogramCost
	}

	for i = 0; i < data_size; i++ {
		if histogram.Data_[i] > 0 {
			count++
			if count > 4 {
				break
			}
		}
	}

	if count == 1 {
		return KOneSymbolHistogramCost
	}

	if count == 2 {
		return KTwoSymbolHistogramCost + float64(histogram.Total_count_)
	}

	if count == 3 {
		return KThreeSymbolHistogramCost + 2*float64(histogram.Total_count_)
	}

	if count == 4 {
		return KFourSymbolHistogramCost + 3*float64(histogram.Total_count_)
	}

	var max_bits float64 = float64(Log2FloorNonZero(data_size-1) + 1)
	var hist_bits float64 = ShannonEntropy(histogram.Data_[:], data_size, &i)
	return hist_bits + max_bits*float64(count)
}
