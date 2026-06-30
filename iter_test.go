package brotli

import (
	"bytes"
	"slices"
	"testing"
)

func TestReaderLines(t *testing.T) {
	linesInput := "line 1\nline 2\r\nline 3\nline 4"
	var buf bytes.Buffer
	bw := NewWriter(&buf)
	if _, err := bw.Write([]byte(linesInput)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	br := NewReader(&buf)
	var decodedLines []string
	for line, err := range br.Lines() {
		if err != nil {
			t.Fatalf("unexpected lines error: %v", err)
		}
		decodedLines = append(decodedLines, line)
	}

	expectedLines := []string{"line 1", "line 2", "line 3", "line 4"}
	if !slices.Equal(decodedLines, expectedLines) {
		t.Errorf("expected lines %v, got %v", expectedLines, decodedLines)
	}
}

func TestReaderChunks(t *testing.T) {
	text := "abcdefghijklmnopqrstuvwxyz"
	var buf bytes.Buffer
	bw := NewWriter(&buf)
	if _, err := bw.Write([]byte(text)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Test zero-allocation chunk size 5
	br := NewReader(bytes.NewReader(buf.Bytes()))
	var chunks [][]byte
	for chunk, err := range br.Chunks(5) {
		if err != nil {
			t.Fatalf("unexpected chunk error: %v", err)
		}
		// Since the iterator yields a reused buffer directly for zero-allocation,
		// we copy the bytes to verify correctness over all iterations.
		copied := make([]byte, len(chunk))
		copy(copied, chunk)
		chunks = append(chunks, copied)
	}

	expectedChunks := [][]byte{
		[]byte("abcde"),
		[]byte("fghij"),
		[]byte("klmno"),
		[]byte("pqrst"),
		[]byte("uvwxy"),
		[]byte("z"),
	}

	if len(chunks) != len(expectedChunks) {
		t.Fatalf("expected %d chunks, got %d", len(expectedChunks), len(chunks))
	}
	for i, chunk := range chunks {
		if !bytes.Equal(chunk, expectedChunks[i]) {
			t.Errorf("chunk %d: expected %q, got %q", i, expectedChunks[i], chunk)
		}
	}

	// Test invalid chunk size
	brInvalid := NewReader(bytes.NewReader(buf.Bytes()))
	var errorCount int
	for _, err := range brInvalid.Chunks(0) {
		if err != nil {
			errorCount++
		}
	}
	if errorCount != 1 {
		t.Errorf("expected exactly 1 error for chunk size 0, got %d", errorCount)
	}
}
