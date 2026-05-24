package brotli

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
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
