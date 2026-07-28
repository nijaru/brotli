package common

import (
	"bytes"
	"testing"
)

func TestBitReader(t *testing.T) {
	data := []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x00, 0x00, 0x00, 0x00}
	var br BitReader
	br.Init()
	br.Input = data
	br.InputLen = uint(len(data))
	br.BytePos = 0

	if !br.Warmup() {
		t.Fatal("Warmup failed")
	}

	bits := br.ReadBits(8)
	if bits != 0x12 {
		t.Errorf("ReadBits(8) = %x, want 0x12", bits)
	}

	bits16 := br.ReadBits(16)
	if bits16 != 0x5634 {
		t.Errorf("ReadBits(16) = %x, want 0x5634", bits16)
	}
}

func TestBitMask(t *testing.T) {
	if got := BitMask(0); got != 0 {
		t.Errorf("BitMask(0) = %x, want 0", got)
	}
	if got := BitMask(8); got != 0xFF {
		t.Errorf("BitMask(8) = %x, want 0xFF", got)
	}
	if got := BitMask(32); got != 0xFFFFFFFF {
		t.Errorf("BitMask(32) = %x, want 0xFFFFFFFF", got)
	}
}

func TestRingBuffer(t *testing.T) {
	var rb RingBuffer
	rb.Size_ = 1024
	rb.Mask_ = 1023
	rb.Tail_size_ = 256
	rb.Total_size_ = rb.Size_ + rb.Tail_size_

	RingBufferInitBuffer(rb.Total_size_, &rb)
	RingBufferInit(&rb)

	input := []byte("hello ringbuffer test")
	RingBufferWrite(input, uint(len(input)), &rb)

	if rb.Pos_ != uint32(len(input)) {
		t.Errorf("rb.Pos_ = %d, want %d", rb.Pos_, len(input))
	}
}

func TestDictionaryAndTransforms(t *testing.T) {
	dict := GetDictionary()
	if dict == nil {
		t.Fatal("GetDictionary() returned nil")
	}
	if len(dict.Data) == 0 {
		t.Fatal("Dictionary data is empty")
	}

	transforms := GetTransforms()
	if transforms == nil {
		t.Fatal("GetTransforms() returned nil")
	}

	// Test transform 0 (identity)
	var dst [128]byte
	word := []byte("test")
	written := TransformDictionaryWord(dst[:], word, len(word), transforms, 0)
	if written <= 0 {
		t.Errorf("TransformDictionaryWord returned %d", written)
	}
	if !bytes.Equal(dst[:written], word) {
		t.Errorf("Transform result %q, want %q", dst[:written], word)
	}
}
