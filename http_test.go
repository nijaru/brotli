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

func TestHTTPCompressorVaryExistingHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "br")
	rec := httptest.NewRecorder()
	rec.Header().Set("Vary", "Accept-Language")

	w := HTTPCompressorWithLevel(rec, req, DefaultCompression)
	w.Write([]byte("hello"))
	w.Close()

	varValues := rec.Header().Values("Vary")
	foundAcceptEncoding := false
	for _, v := range varValues {
		if v == "Accept-Encoding" {
			foundAcceptEncoding = true
		}
	}
	if !foundAcceptEncoding {
		t.Errorf("Vary values %v did not contain Accept-Encoding", varValues)
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

	for b.Loop() {
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

	for b.Loop() {
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Encoding", "br")
		w := NewWriterLevel(rec, DefaultCompression)
		w.Write(input)
		w.Close()
	}
}

func TestNegotiateContentEncoding(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"no header", "", "identity"},
		{"brotli", "br", "br"},
		{"gzip", "gzip", "gzip"},
		{"both prefer brotli", "gzip, br", "br"},
		{"brotli higher q", "gzip;q=0.5, br;q=0.9", "br"},
		{"gzip higher q", "gzip;q=0.9, br;q=0.5", "gzip"},
		{"wildcard", "*", "br"},
		{"identity excluded", "identity;q=0", ""},
		{"identity excluded brotli", "identity;q=0, br;q=0.5", "br"},
		{"identity preferred", "identity;q=0.9, gzip;q=0.5", "identity"},
		{"wildcard skips explicit zero", "br;q=0, *;q=1", "gzip"},
		{"case insensitive", "BR", "br"},
		{"all excluded", "identity;q=0, *;q=0", ""},
		{"offered coding beats implicit identity", "gzip;q=0.5", "gzip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Accept-Encoding", tt.header)
			}
			if got := negotiateContentEncoding(req, []string{"br", "gzip"}); got != tt.want {
				t.Fatalf("negotiateContentEncoding(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestExpectQualityRejectsAboveOne(t *testing.T) {
	if q, _ := expectQuality("1"); q != 1 {
		t.Fatalf("expectQuality(1) = %v, want 1", q)
	}
	if q, _ := expectQuality("1.5"); q >= 0 {
		t.Fatalf("expectQuality(1.5) = %v, want rejection", q)
	}
}
