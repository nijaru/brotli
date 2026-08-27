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
	if uint(len(p)) < n {
		n = uint(len(p))
	}
	for i := uint(0); i < n; i++ {
		self.Data_[p[i]]++
	}
}

func HistogramAddHistogramLiteral(self *HistogramLiteral, v *HistogramLiteral) {
	self.Total_count_ += v.Total_count_
	for i := 0; i < NumLiteralSymbols; i += 8 {
		self.Data_[i+0] += v.Data_[i+0]
		self.Data_[i+1] += v.Data_[i+1]
		self.Data_[i+2] += v.Data_[i+2]
		self.Data_[i+3] += v.Data_[i+3]
		self.Data_[i+4] += v.Data_[i+4]
		self.Data_[i+5] += v.Data_[i+5]
		self.Data_[i+6] += v.Data_[i+6]
		self.Data_[i+7] += v.Data_[i+7]
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
	self.Total_count_ += v.Total_count_
	for i := 0; i < NumCommandSymbols; i += 8 {
		self.Data_[i+0] += v.Data_[i+0]
		self.Data_[i+1] += v.Data_[i+1]
		self.Data_[i+2] += v.Data_[i+2]
		self.Data_[i+3] += v.Data_[i+3]
		self.Data_[i+4] += v.Data_[i+4]
		self.Data_[i+5] += v.Data_[i+5]
		self.Data_[i+6] += v.Data_[i+6]
		self.Data_[i+7] += v.Data_[i+7]
	}
}

func HistogramAddVectorCommand(self *HistogramCommand, p []uint16, n uint) {
	self.Total_count_ += n
	if uint(len(p)) < n {
		n = uint(len(p))
	}
	for i := uint(0); i < n; i++ {
		self.Data_[p[i]]++
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
	self.Total_count_ += v.Total_count_
	for i := 0; i < NumDistanceSymbols; i += 8 {
		self.Data_[i+0] += v.Data_[i+0]
		self.Data_[i+1] += v.Data_[i+1]
		self.Data_[i+2] += v.Data_[i+2]
		self.Data_[i+3] += v.Data_[i+3]
		self.Data_[i+4] += v.Data_[i+4]
		self.Data_[i+5] += v.Data_[i+5]
		self.Data_[i+6] += v.Data_[i+6]
		self.Data_[i+7] += v.Data_[i+7]
	}
}

func HistogramAddVectorDistance(self *HistogramDistance, p []uint16, n uint) {
	self.Total_count_ += n
	if uint(len(p)) < n {
		n = uint(len(p))
	}
	for i := uint(0); i < n; i++ {
		self.Data_[p[i]]++
	}
}

func HistogramDataSizeDistance() uint {
	return NumDistanceSymbols
}
