package brotli

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestGzipLevelForBrotliLevel(t *testing.T) {
	tests := []struct {
		level int
		want  int
	}{
		{-1, gzip.BestSpeed},
		{BestSpeed, gzip.BestSpeed},
		{1, gzip.BestSpeed},
		{6, 6},
		{9, gzip.BestCompression},
		{BestCompression, gzip.BestCompression},
	}

	for _, tt := range tests {
		if got := gzipLevelForBrotliLevel(tt.level); got != tt.want {
			t.Errorf("gzipLevelForBrotliLevel(%d)=%d, want %d", tt.level, got, tt.want)
		}
	}
}

func TestHTTPCompressorGzipFallback(t *testing.T) {
	input := bytes.Repeat([]byte("http gzip payload "), 64)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	w := HTTPCompressorWithLevel(rec, req, BestCompression)
	if _, err := w.Write(input); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding=%q, want gzip", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary=%q, want Accept-Encoding", got)
	}

	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip.NewReader() error: %v", err)
	}
	defer zr.Close()

	output, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("gzip output did not round trip")
	}
}

func TestHTTPPoolSafety(t *testing.T) {
	input := []byte("http pool safety payload http pool safety payload http pool safety payload")

	// Launch concurrent goroutines compressing different inputs via HTTPCompressorWithLevel
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "br")
			rec := httptest.NewRecorder()

			w := HTTPCompressorWithLevel(rec, req, DefaultCompression)
			if _, err := w.Write(input); err != nil {
				t.Errorf("routine %d Write() error: %v", id, err)
				return
			}
			if err := w.Close(); err != nil {
				t.Errorf("routine %d Close() error: %v", id, err)
				return
			}

			if got := rec.Header().Get("Content-Encoding"); got != "br" {
				t.Errorf("routine %d Content-Encoding=%q, want br", id, got)
				return
			}

			// Decompress to verify correctness
			reader := NewReader(bytes.NewReader(rec.Body.Bytes()))
			output, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("routine %d ReadAll() error: %v", id, err)
				return
			}
			if !bytes.Equal(output, input) {
				t.Errorf("routine %d output did not round trip", id)
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkHTTPResponseCompression(b *testing.B) {
	input := bytes.Repeat([]byte("benchmark http payload content "), 100)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "br")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		w := HTTPCompressorWithLevel(rec, req, DefaultCompression)
		w.Write(input)
		w.Close()
	}
}

func BenchmarkHTTPResponseCompressionNoPool(b *testing.B) {
	input := bytes.Repeat([]byte("benchmark http payload content "), 100)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "br")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Encoding", "br")
		w := NewWriterLevel(rec, DefaultCompression)
		w.Write(input)
		w.Close()
	}
}
