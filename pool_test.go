package brotli

import (
	"bytes"
	"io"
	"testing"
)

func TestPoolCorrectness(t *testing.T) {
	wp := NewWriterPool(DefaultCompression)
	rp := NewReaderPool()

	input := []byte("Hello, pool-based compression!")

	// 1. Get Writer and compress
	var buf bytes.Buffer
	w := wp.Get(&buf)
	if _, err := w.Write(input); err != nil {
		t.Fatalf("pool writer write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("pool writer close failed: %v", err)
	}
	wp.Put(w)

	// 2. Get Reader and decompress
	r := rp.Get(&buf)
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("pool reader read failed: %v", err)
	}
	rp.Put(r)

	if !bytes.Equal(input, decompressed) {
		t.Errorf("expected %q, got %q", input, decompressed)
	}
}

func BenchmarkPoolUsage(b *testing.B) {
	input := []byte("Benchmark data for pool-based compression. Let's make it relatively short.")
	wp := NewWriterPool(DefaultCompression)
	rp := NewReaderPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		w := wp.Get(&buf)
		w.Write(input)
		w.Close()
		wp.Put(w)

		r := rp.Get(&buf)
		io.Copy(io.Discard, r)
		rp.Put(r)
	}
}

func BenchmarkNoPoolUsage(b *testing.B) {
	input := []byte("Benchmark data for pool-based compression. Let's make it relatively short.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		w := NewWriterLevel(&buf, DefaultCompression)
		w.Write(input)
		w.Close()

		r := NewReader(&buf)
		io.Copy(io.Discard, r)
	}
}
