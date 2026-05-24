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

func TestGlobalPoolCorrectness(t *testing.T) {
	input := []byte("Hello from package-level default pools!")

	// 1. Get default Writer and compress
	var buf bytes.Buffer
	w := GetWriter(&buf)
	if _, err := w.Write(input); err != nil {
		t.Fatalf("global writer write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("global writer close failed: %v", err)
	}
	PutWriter(w)

	// 2. Get default Reader and decompress
	r := GetReader(&buf)
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("global reader read failed: %v", err)
	}
	PutReader(r)

	if !bytes.Equal(input, decompressed) {
		t.Errorf("expected %q, got %q", input, decompressed)
	}
}

func TestWriterPoolNormalizesReturnedWriterQuality(t *testing.T) {
	p := NewWriterPool(DefaultCompression)
	w := NewWriterLevel(io.Discard, BestCompression)

	p.Put(w)
	if w.options.Quality != DefaultCompression {
		t.Fatalf("Put normalized quality to %d, want %d", w.options.Quality, DefaultCompression)
	}

	var buf bytes.Buffer
	reused := p.Get(&buf)
	if reused.options.Quality != DefaultCompression {
		t.Fatalf("Get returned quality %d, want %d", reused.options.Quality, DefaultCompression)
	}
	p.Put(reused)
}

func BenchmarkPoolUsage(b *testing.B) {
	input := []byte("Benchmark data for pool-based compression. Let's make it relatively short.")
	wp := NewWriterPool(DefaultCompression)
	rp := NewReaderPool()

	b.ResetTimer()
	for b.Loop() {
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

func BenchmarkGlobalPoolUsage(b *testing.B) {
	input := []byte("Benchmark data for pool-based compression. Let's make it relatively short.")

	b.ResetTimer()
	for b.Loop() {
		var buf bytes.Buffer
		w := GetWriter(&buf)
		w.Write(input)
		w.Close()
		PutWriter(w)

		r := GetReader(&buf)
		io.Copy(io.Discard, r)
		PutReader(r)
	}
}

func BenchmarkNoPoolUsage(b *testing.B) {
	input := []byte("Benchmark data for pool-based compression. Let's make it relatively short.")

	b.ResetTimer()
	for b.Loop() {
		var buf bytes.Buffer
		w := NewWriterLevel(&buf, DefaultCompression)
		w.Write(input)
		w.Close()

		r := NewReader(&buf)
		io.Copy(io.Discard, r)
	}
}
