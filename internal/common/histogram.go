package common

import "math"

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* The distance symbols effectively used by "Large Window Brotli" (32-bit). */
const NumHistogramDistanceSymbols = 544

type HistogramLiteral struct {
	Data_        [NumLiteralSymbols]uint32
	Total_count_ uint
	Bit_cost_    float64
}

func HistogramClearLiteral(self *HistogramLiteral) {
	self.Data_ = [NumLiteralSymbols]uint32{}
	self.Total_count_ = 0
	self.Bit_cost_ = math.MaxFloat64
}

func ClearHistogramsLiteral(array []HistogramLiteral, length uint) {
	var i uint
	for i = 0; i < length; i++ {
		HistogramClearLiteral(&array[i:][0])
	}
}

func HistogramAddLiteral(self *HistogramLiteral, val uint) {
	self.Data_[val]++
	self.Total_count_++
}

func HistogramAddVectorLiteral(self *HistogramLiteral, p []byte, n uint) {
	self.Total_count_ += n
	n += 1
	for {
		n--
		if n == 0 {
			break
		}
		self.Data_[p[0]]++
		p = p[1:]
	}
}

func HistogramAddHistogramLiteral(self *HistogramLiteral, v *HistogramLiteral) {
	var i uint
	self.Total_count_ += v.Total_count_
	for i = 0; i < NumLiteralSymbols; i++ {
		self.Data_[i] += v.Data_[i]
	}
}

func HistogramDataSizeLiteral() uint {
	return NumLiteralSymbols
}

type HistogramCommand struct {
	Data_        [NumCommandSymbols]uint32
	Total_count_ uint
	Bit_cost_    float64
}

func HistogramClearCommand(self *HistogramCommand) {
	self.Data_ = [NumCommandSymbols]uint32{}
	self.Total_count_ = 0
	self.Bit_cost_ = math.MaxFloat64
}

func ClearHistogramsCommand(array []HistogramCommand, length uint) {
	var i uint
	for i = 0; i < length; i++ {
		HistogramClearCommand(&array[i:][0])
	}
}

func HistogramAddCommand(self *HistogramCommand, val uint) {
	self.Data_[val]++
	self.Total_count_++
}

func HistogramAddHistogramCommand(self *HistogramCommand, v *HistogramCommand) {
	var i uint
	self.Total_count_ += v.Total_count_
	for i = 0; i < NumCommandSymbols; i++ {
		self.Data_[i] += v.Data_[i]
	}
}

func HistogramAddVectorCommand(self *HistogramCommand, p []uint16, n uint) {
	self.Total_count_ += n
	n += 1
	for {
		n--
		if n == 0 {
			break
		}
		self.Data_[p[0]]++
		p = p[1:]
	}
}

func HistogramDataSizeCommand() uint {
	return NumCommandSymbols
}

type HistogramDistance struct {
	Data_        [NumDistanceSymbols]uint32
	Total_count_ uint
	Bit_cost_    float64
}

func HistogramClearDistance(self *HistogramDistance) {
	self.Data_ = [NumDistanceSymbols]uint32{}
	self.Total_count_ = 0
	self.Bit_cost_ = math.MaxFloat64
}

func ClearHistogramsDistance(array []HistogramDistance, length uint) {
	var i uint
	for i = 0; i < length; i++ {
		HistogramClearDistance(&array[i:][0])
	}
}

func HistogramAddDistance(self *HistogramDistance, val uint) {
	self.Data_[val]++
	self.Total_count_++
}

func HistogramAddHistogramDistance(self *HistogramDistance, v *HistogramDistance) {
	var i uint
	self.Total_count_ += v.Total_count_
	for i = 0; i < NumDistanceSymbols; i++ {
		self.Data_[i] += v.Data_[i]
	}
}

func HistogramAddVectorDistance(self *HistogramDistance, p []uint16, n uint) {
	self.Total_count_ += n
	n += 1
	for {
		n--
		if n == 0 {
			break
		}
		self.Data_[p[0]]++
		p = p[1:]
	}
}

func HistogramDataSizeDistance() uint {
	return NumDistanceSymbols
}
