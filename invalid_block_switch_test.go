package brotli

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// invalidBlockSwitchVector is the Node.js regression case for CVE-2026-111497:
// a stream that declares one block type and then attempts a block switch.
const invalidBlockSwitchVector = "200010414141fcffff07002228f00b60f7f41f000000"

func TestInvalidBlockSwitchFailsClosed(t *testing.T) {
	input, err := hex.DecodeString(invalidBlockSwitchVector)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Decode(nil, input); err == nil || !strings.Contains(err.Error(), "BLOCK_SWITCH") {
		t.Fatalf("Decode returned %v, want BLOCK_SWITCH", err)
	}

	readers := map[string]io.Reader{
		"buffered":         bytes.NewReader(input),
		"single-byte":      iotest.OneByteReader(bytes.NewReader(input)),
		"trailing-padding": bytes.NewReader(append(append([]byte(nil), input...), make([]byte, 64)...)),
	}
	for name, src := range readers {
		t.Run(name, func(t *testing.T) {
			n, err := io.Copy(io.Discard, NewReader(src))
			if err == nil || !strings.Contains(err.Error(), "BLOCK_SWITCH") {
				t.Fatalf("Reader returned %v after %d bytes, want BLOCK_SWITCH", err, n)
			}
		})
	}
}
