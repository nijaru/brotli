package encoder

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"testing"

	"github.com/nijaru/brotli/internal/decoder")

func Decode(encodedData []byte) ([]byte, error) {
	r := decoder.NewReader(bytes.NewReader(encodedData))
	return io.ReadAll(r)
}

func testParity(t *testing.T, data []byte) {
	// New implementation
	e := &Encoder{}
	encoded := e.Encode(nil, data, true)

	// Check if it decompresses correctly
	newDec, err := Decode(encoded)
	if err != nil {
		t.Errorf("Decode error: %v", err)
	} else if !bytes.Equal(newDec, data) {
		t.Error("Decode data mismatch")
	} else {
		t.Logf("Size %d: Compressed to %d bytes and decompressed successfully!", len(data), len(encoded))
	}
}

func TestEncoderParity(t *testing.T) {
	t.Run("Hello", func(t *testing.T) { testParity(t, []byte("hello")) })
	t.Run("1KB", func(t *testing.T) {
		data := make([]byte, 1024)
		for i := range data {
			data[i] = byte(i)
		}
		testParity(t, data)
	})
	t.Run("32KB", func(t *testing.T) {
		data := make([]byte, 32*1024)
		for i := range data {
			data[i] = byte(i % 251)
		}
		testParity(t, data)
	})
	t.Run("128KB", func(t *testing.T) {
		data := make([]byte, 128*1024)
		for i := range data {
			data[i] = byte(i % 251)
		}
		testParity(t, data)
	})
	t.Run("1MB", func(t *testing.T) {
		data := make([]byte, 1024*1024)
		for i := range data {
			data[i] = byte(i % 251)
		}
		testParity(t, data)
	})
	t.Run("Isaac.Newton-Opticks.txt", func(t *testing.T) {
		data, err := os.ReadFile("../../testdata/Isaac.Newton-Opticks.txt")
		if err != nil {
			t.Fatal(err)
		}
		testParity(t, data)
	})
	t.Run("Issue22", func(t *testing.T) {
		f, err := os.Open("../../testdata/issue22.gz")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		zr, err := gzip.NewReader(f)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		testParity(t, data)
	})
}

func TestStreamingParity(t *testing.T) {
	data, err := os.ReadFile("../../testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		t.Fatal(err)
	}

	// One-shot
	e1 := &Encoder{}
	oneShot := e1.Encode(nil, data, true)

	// Two-shot
	e2 := &Encoder{}
	half := len(data) / 2
	part1Raw := e2.Encode(nil, data[:half], false)
	part1 := make([]byte, len(part1Raw))
	copy(part1, part1Raw)
	part2 := e2.Encode(nil, data[half:], true)
	twoShot := append(part1, part2...)

	t.Logf(
		"One-shot length: %d, Two-shot length: %d (part1=%d, part2=%d)",
		len(oneShot),
		len(twoShot),
		len(part1),
		len(part2),
	)

	// Find where one-shot and two-shot diverge
	minLen := len(oneShot)
	if len(twoShot) < minLen {
		minLen = len(twoShot)
	}
	divergeIdx := -1
	for i := 0; i < minLen; i++ {
		if oneShot[i] != twoShot[i] {
			divergeIdx = i
			break
		}
	}
	if divergeIdx != -1 {
		t.Logf(
			"Divergence at byte %d: oneShot=0x%02x, twoShot=0x%02x",
			divergeIdx,
			oneShot[divergeIdx],
			twoShot[divergeIdx],
		)
		start := divergeIdx - 10
		if start < 0 {
			start = 0
		}
		end := divergeIdx + 10
		if end > minLen {
			end = minLen
		}
		t.Logf("Bytes around divergence in oneShot: %x", oneShot[start:end])
		t.Logf("Bytes around divergence in twoShot: %x", twoShot[start:end])
	} else {
		t.Logf("No divergence found up to length %d", minLen)
	}

	// Decompress two-shot
	dec, err := Decode(twoShot)
	if err != nil {
		t.Fatalf("Decompress two-shot error: %v", err)
	}
	if !bytes.Equal(dec, data) {
		t.Fatal("Decompress two-shot mismatch")
	}

	// Check decompress one-shot
	dec1, err := Decode(oneShot)
	if err != nil {
		t.Fatalf("Decompress one-shot error: %v", err)
	}
	if !bytes.Equal(dec1, data) {
		t.Fatal("Decompress one-shot mismatch")
	}
}
