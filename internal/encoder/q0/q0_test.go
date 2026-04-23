package q0

import (
	"bytes"
	"github.com/nijaru/brotli"
	"io"
	"os"
	"testing"
)

func Decode(encodedData []byte) ([]byte, error) {
	r := brotli.NewReader(bytes.NewReader(encodedData))
	return io.ReadAll(r)
}

func testParity(t *testing.T, data []byte) {
	// Reference implementation (current Q0)
	var refBuf bytes.Buffer
	w := brotli.NewWriterLevel(&refBuf, 0)
	w.Write(data)
	w.Close()
	refEncoded := refBuf.Bytes()

	// New implementation
	e := &Encoder{}
	encoded := e.Encode(nil, data, nil, true)

	if bytes.Equal(refEncoded, encoded) {
		t.Logf("Size %d: Encoded bytes are identical (%d bytes)!", len(data), len(encoded))
	} else {
		t.Errorf("Size %d: Encoded bytes differ. Ref: %d, New: %d", len(data), len(refEncoded), len(encoded))
		// Find first difference
		for i := 0; i < len(refEncoded) && i < len(encoded); i++ {
			if refEncoded[i] != encoded[i] {
				t.Logf("First difference at byte %d: ref=0x%02x, new=0x%02x", i, refEncoded[i], encoded[i])
				break
			}
		}
		
		// Still check if it decompresses correctly
		newDec, err := Decode(encoded)
		if err != nil {
			t.Errorf("New decode error: %v", err)
		} else if !bytes.Equal(newDec, data) {
			t.Error("New decode data mismatch")
		}
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
	t.Run("Opticks", func(t *testing.T) {
		data, err := os.ReadFile("../../../testdata/Isaac.Newton-Opticks.txt")
		if err != nil {
			t.Fatal(err)
		}
		testParity(t, data)
	})
}
