package brotli

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestDecodeLimitBoundary(t *testing.T) {
	source := bytes.Repeat([]byte("brutli output limit "), 2048)
	compressed := Encode(nil, source, 5)

	got, err := DecodeLimit(nil, compressed, len(source))
	if err != nil {
		t.Fatalf("DecodeLimit at exact size: %v", err)
	}
	if !bytes.Equal(got, source) {
		t.Fatal("DecodeLimit output did not match the source")
	}

	got, err = DecodeLimit(nil, compressed, len(source)-1)
	if !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("DecodeLimit over limit returned %v, want %v", err, ErrOutputLimitExceeded)
	}
	if got != nil {
		t.Fatalf("DecodeLimit over limit returned %d bytes, want nil", len(got))
	}
}

func TestDecodeLimitZero(t *testing.T) {
	empty := Encode(nil, nil, 5)
	got, err := DecodeLimit(nil, empty, 0)
	if err != nil {
		t.Fatalf("DecodeLimit empty stream: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("DecodeLimit empty stream returned %d bytes", len(got))
	}

	compressed := Encode(nil, []byte("x"), 5)
	if _, err := DecodeLimit(nil, compressed, 0); !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("DecodeLimit zero limit returned %v, want %v", err, ErrOutputLimitExceeded)
	}
}

func TestDecodeLimitRejectsNegative(t *testing.T) {
	if _, err := DecodeLimit(nil, nil, -1); !errors.Is(err, ErrInvalidOutputLimit) {
		t.Fatalf("DecodeLimit negative limit returned %v, want %v", err, ErrInvalidOutputLimit)
	}
	if _, err := NewReaderLimit(bytes.NewReader(nil), -1); !errors.Is(err, ErrInvalidOutputLimit) {
		t.Fatalf("NewReaderLimit negative limit returned %v, want %v", err, ErrInvalidOutputLimit)
	}
}

func TestReaderLimit(t *testing.T) {
	source := bytes.Repeat([]byte("reader limit "), 1024)
	compressed := Encode(nil, source, 5)

	r, err := NewReaderLimit(bytes.NewReader(compressed), len(source))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("Reader at exact size: %v", err)
	}
	if !bytes.Equal(got, source) {
		t.Fatal("Reader output did not match the source")
	}

	r, err = NewReaderLimit(bytes.NewReader(compressed), len(source)-1)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(r)
	if !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("Reader over limit returned %v, want %v", err, ErrOutputLimitExceeded)
	}
	if len(got) > len(source)-1 {
		t.Fatalf("Reader over limit returned %d bytes, want at most %d", len(got), len(source)-1)
	}

	r, err = NewReaderLimit(bytes.NewReader(compressed), len(source)-1)
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, r)
	if !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("Reader.WriteTo over limit returned %v, want %v", err, ErrOutputLimitExceeded)
	}
	if n > int64(len(source)-1) {
		t.Fatalf("Reader.WriteTo over limit wrote %d bytes, want at most %d", n, len(source)-1)
	}
}

func TestReaderLimitSurvivesReset(t *testing.T) {
	source := bytes.Repeat([]byte("reset limit "), 512)
	compressed := Encode(nil, source, 5)

	r, err := NewReaderLimit(bytes.NewReader(compressed), len(source)-1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("first read returned %v, want %v", err, ErrOutputLimitExceeded)
	}

	if err := r.Reset(bytes.NewReader(compressed)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("read after Reset returned %v, want %v", err, ErrOutputLimitExceeded)
	}
}

func TestReaderPoolClearsLimit(t *testing.T) {
	source := bytes.Repeat([]byte("pooled limit "), 512)
	compressed := Encode(nil, source, 5)
	pool := NewReaderPool()

	r, err := NewReaderLimit(bytes.NewReader(compressed), 0)
	if err != nil {
		t.Fatal(err)
	}
	pool.Put(r)

	got, err := io.ReadAll(pool.GetBytes(compressed))
	if err != nil {
		t.Fatalf("pooled reader retained a limit: %v", err)
	}
	if !bytes.Equal(got, source) {
		t.Fatal("pooled reader output did not match the source")
	}
}

func TestDecodeLimitReusesExactCapacity(t *testing.T) {
	source := bytes.Repeat([]byte("exact capacity "), 1024)
	compressed := Encode(nil, source, 5)
	dst := make([]byte, 0, len(source))

	got, err := DecodeLimit(dst, compressed, len(source))
	if err != nil {
		t.Fatalf("DecodeLimit at exact capacity: %v", err)
	}
	if cap(got) != len(source) {
		t.Fatalf("DecodeLimit grew exact-capacity dst from %d to %d", len(source), cap(got))
	}
	if !bytes.Equal(got, source) {
		t.Fatal("DecodeLimit output did not match the source")
	}
}

func TestReaderLimitZeroLengthRead(t *testing.T) {
	source := []byte("xy")
	compressed := Encode(nil, source, 5)

	r, err := NewReaderLimit(bytes.NewReader(compressed), len(source))
	if err != nil {
		t.Fatal(err)
	}
	first := make([]byte, len(source))
	if _, err := io.ReadFull(r, first); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(nil); !errors.Is(err, io.EOF) {
		t.Fatalf("Read(nil) at exact limit returned %v, want EOF", err)
	}

	r, err = NewReaderLimit(bytes.NewReader(compressed), len(source)-1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(r, first[:len(source)-1]); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(nil); !errors.Is(err, ErrOutputLimitExceeded) {
		t.Fatalf("Read(nil) over limit returned %v, want %v", err, ErrOutputLimitExceeded)
	}
}
