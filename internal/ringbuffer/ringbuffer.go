package ringbuffer

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

import (
	"github.com/nijaru/brotli/internal/common"
)

type RingBuffer struct {
	Size_       uint32
	Mask_       uint32
	Tail_size_  uint32
	Total_size_ uint32
	Cur_size_   uint32
	Pos_        uint32
	Data_       []byte
	Buffer_     []byte
}

func RingBufferInit(rb *RingBuffer) {
	rb.Pos_ = 0
}

func RingBufferSetup(params *common.EncoderParams, rb *RingBuffer) {
	window_bits := common.ComputeRbBits(params)
	tail_bits := params.Lgblock
	rb.Size_ = 1 << uint(window_bits)
	rb.Mask_ = (1 << uint(window_bits)) - 1
	rb.Tail_size_ = 1 << uint(tail_bits)
	rb.Total_size_ = rb.Size_ + rb.Tail_size_
}

const kSlackForEightByteHashingEverywhere uint = 7

/* Allocates or re-allocates data_ to the given length + plus some slack
   region before and after. Fills the slack regions with zeros. */
func RingBufferInitBuffer(buflen uint32, rb *RingBuffer) {
	var new_data []byte
	var i uint
	size := 2 + int(buflen) + int(kSlackForEightByteHashingEverywhere)
	if cap(rb.Data_) < size {
		new_data = make([]byte, size)
	} else {
		new_data = rb.Data_[:size]
	}
	if rb.Data_ != nil {
		copy(new_data, rb.Data_[:2+rb.Cur_size_+uint32(kSlackForEightByteHashingEverywhere)])
	}

	rb.Data_ = new_data
	rb.Cur_size_ = buflen
	rb.Buffer_ = rb.Data_[2:]
	rb.Data_[1] = 0
	rb.Data_[0] = rb.Data_[1]
	for i = 0; i < kSlackForEightByteHashingEverywhere; i++ {
		rb.Buffer_[rb.Cur_size_+uint32(i)] = 0
	}
}

func RingBufferWriteTail(bytes []byte, n uint, rb *RingBuffer) {
	var masked_pos uint = uint(rb.Pos_ & rb.Mask_)
	if uint32(masked_pos) < rb.Tail_size_ {
		/* Just fill the tail buffer with the beginning data. */
		var p uint = uint(rb.Size_ + uint32(masked_pos))
		copy(rb.Buffer_[p:], bytes[:common.BrotliMinSizeT(n, uint(rb.Tail_size_-uint32(masked_pos)))])
	}
}

/* Push bytes into the ring buffer. */
func RingBufferWrite(bytes []byte, n uint, rb *RingBuffer) {
	if rb.Pos_ == 0 && uint32(n) < rb.Tail_size_ {
		/* Special case for the first write: to process the first block, we don't
		   need to allocate the whole ring-buffer and we don't need the tail
		   either. However, we do this memory usage optimization only if the
		   first write is less than the tail size, which is also the input block
		   size, otherwise it is likely that other blocks will follow and we
		   will need to reallocate to the full size anyway. */
		rb.Pos_ = uint32(n)

		RingBufferInitBuffer(rb.Pos_, rb)
		copy(rb.Buffer_, bytes[:n])
		return
	}

	if rb.Cur_size_ < rb.Total_size_ {
		/* Lazily allocate the full buffer. */
		RingBufferInitBuffer(rb.Total_size_, rb)

		/* Initialize the last two bytes to zero, so that we don't have to worry
		   later when we copy the last two bytes to the first two positions. */
		rb.Buffer_[rb.Size_-2] = 0

		rb.Buffer_[rb.Size_-1] = 0
	}
	{
		var masked_pos uint = uint(rb.Pos_ & rb.Mask_)

		/* The length of the writes is limited so that we do not need to worry
		   about a write */
		RingBufferWriteTail(bytes, n, rb)

		if uint32(masked_pos+n) <= rb.Size_ {
			/* A single write fits. */
			copy(rb.Buffer_[masked_pos:], bytes[:n])
		} else {
			/* Split into two writes.
			   Copy into the end of the buffer, including the tail buffer. */
			copy(rb.Buffer_[masked_pos:], bytes[:common.BrotliMinSizeT(n, uint(rb.Total_size_-uint32(masked_pos)))])

			/* Copy into the beginning of the buffer */
			copy(rb.Buffer_, bytes[rb.Size_-uint32(masked_pos):][:uint32(n)-(rb.Size_-uint32(masked_pos))])
		}
	}
	{
		var not_first_lap bool = rb.Pos_&(1<<31) != 0
		var rb_pos_mask uint32 = (1 << 31) - 1
		rb.Data_[0] = rb.Buffer_[rb.Size_-2]
		rb.Data_[1] = rb.Buffer_[rb.Size_-1]
		rb.Pos_ = (rb.Pos_ & rb_pos_mask) + uint32(uint32(n)&rb_pos_mask)
		if not_first_lap {
			/* Wrap, but preserve not-a-first-lap feature. */
			rb.Pos_ |= 1 << 31
		}
	}
}
