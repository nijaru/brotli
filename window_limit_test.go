package brotli

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func encodeWindowStream(t *testing.T, source []byte, lgwin int, large bool) []byte {
	t.Helper()

	var compressed bytes.Buffer
	w := NewWriterConfig(&compressed, WriterConfig{
		Quality:     6,
		LGWin:       lgwin,
		LargeWindow: large,
	})
	if _, err := w.Write(source); err != nil {
		t.Fatalf("encode lgwin=%d: %v", lgwin, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close lgwin=%d: %v", lgwin, err)
	}
	return compressed.Bytes()
}

func TestReaderWindowBitsLimit(t *testing.T) {
	source := bytes.Repeat([]byte("window-bits-limit "), 4096)
	standard := encodeWindowStream(t, source, 22, false)
	large := encodeWindowStream(t, source, 26, true)

	tests := []struct {
		name    string
		stream  []byte
		maxBits int
	}{
		{"standard too large", standard, 21},
		{"large too large", large, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewReaderWithLimits(bytes.NewReader(tt.stream), len(source), tt.maxBits)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(r)
			if !errors.Is(err, ErrWindowBitsLimitExceeded) {
				t.Fatalf("ReadAll returned %v, want %v", err, ErrWindowBitsLimitExceeded)
			}
			if len(got) != 0 {
				t.Fatalf("ReadAll returned %d bytes before rejecting the window", len(got))
			}
		})
	}

	tests = []struct {
		name    string
		stream  []byte
		maxBits int
	}{
		{"standard exact", standard, 22},
		{"large exact", large, 26},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewReaderWithLimits(bytes.NewReader(tt.stream), len(source), tt.maxBits)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, source) {
				t.Fatal("decoded output did not match the source")
			}
		})
	}
}

func TestDecodeWithLimits(t *testing.T) {
	source := bytes.Repeat([]byte("decode-with-limits "), 2048)
	compressed := encodeWindowStream(t, source, 22, false)

	got, err := DecodeWithLimits(nil, compressed, len(source), 22)
	if err != nil {
		t.Fatalf("DecodeWithLimits at exact window: %v", err)
	}
	if !bytes.Equal(got, source) {
		t.Fatal("DecodeWithLimits output did not match the source")
	}

	if _, err := DecodeWithLimits(nil, compressed, len(source), 21); !errors.Is(err, ErrWindowBitsLimitExceeded) {
		t.Fatalf("DecodeWithLimits returned %v, want %v", err, ErrWindowBitsLimitExceeded)
	}

	if _, err := DecodeWithLimits(nil, compressed, 1, 22); !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("DecodeWithLimits output limit returned %v, want %v", err, ErrOutputLimitExceeded)
	}
}

func TestInvalidWindowBitsLimit(t *testing.T) {
	for _, maxBits := range []int{0, 9, 31, 100} {
		if _, err := NewReaderWithLimits(bytes.NewReader(nil), 0, maxBits); !errors.Is(err, ErrInvalidWindowBitsLimit) {
			t.Errorf("NewReaderWithLimits max=%d returned %v, want %v", maxBits, err, ErrInvalidWindowBitsLimit)
		}
		if _, err := DecodeWithLimits(nil, nil, 0, maxBits); !errors.Is(err, ErrInvalidWindowBitsLimit) {
			t.Errorf("DecodeWithLimits max=%d returned %v, want %v", maxBits, err, ErrInvalidWindowBitsLimit)
		}
	}
}

func TestReaderWindowBitsLimitSurvivesReset(t *testing.T) {
	source := bytes.Repeat([]byte("reset window limit "), 2048)
	compressed := encodeWindowStream(t, source, 22, false)
	smaller := encodeWindowStream(t, source, 20, false)

	r, err := NewReaderWithLimits(bytes.NewReader(compressed), len(source), 21)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); !errors.Is(err, ErrWindowBitsLimitExceeded) {
		t.Fatalf("first read returned %v, want %v", err, ErrWindowBitsLimitExceeded)
	}

	if err := r.Reset(bytes.NewReader(compressed)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); !errors.Is(err, ErrWindowBitsLimitExceeded) {
		t.Fatalf("read after Reset returned %v, want %v", err, ErrWindowBitsLimitExceeded)
	}

	if err := r.Reset(bytes.NewReader(smaller)); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read of a permitted stream after Reset: %v", err)
	}
	if !bytes.Equal(got, source) {
		t.Fatal("output after Reset did not match the source")
	}
}

func TestReaderWindowBitsLimitAppliesToEveryMember(t *testing.T) {
	source := bytes.Repeat([]byte("concatenated window limit "), 1024)
	first := encodeWindowStream(t, source, 20, false)
	second := encodeWindowStream(t, source, 22, false)
	combined := append(append([]byte(nil), first...), second...)

	r, err := NewReaderWithLimits(bytes.NewReader(combined), 2*len(source), 21)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); !errors.Is(err, ErrWindowBitsLimitExceeded) {
		t.Fatalf("concatenated read returned %v, want %v", err, ErrWindowBitsLimitExceeded)
	}
}

func TestReaderPoolClearsWindowBitsLimit(t *testing.T) {
	source := bytes.Repeat([]byte("pooled window limit "), 1024)
	compressed := encodeWindowStream(t, source, 22, false)
	pool := NewReaderPool()

	r, err := NewReaderWithLimits(bytes.NewReader(compressed), len(source), 21)
	if err != nil {
		t.Fatal(err)
	}
	pool.Put(r)

	got, err := io.ReadAll(pool.GetBytes(compressed))
	if err != nil {
		t.Fatalf("pooled reader retained a window limit: %v", err)
	}
	if !bytes.Equal(got, source) {
		t.Fatal("pooled reader output did not match the source")
	}
}
