package brotli

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

func TestIncompleteStreams(t *testing.T) {
	payload := bytes.Repeat([]byte("hello "), 100)
	complete := Encode(nil, payload, DefaultCompression)
	reported, err := hex.DecodeString("1b220000244072904598c965f23c5d")
	if err != nil {
		t.Fatal(err)
	}
	type testCase struct {
		name  string
		data  []byte
		limit int
	}
	tests := []testCase{
		{"no member", nil, 0},
		{"header only", complete[:1], len(payload)},
		{"missing tail", complete[:len(complete)-1], len(payload)},
		{"reported truncation", reported, len(payload)},
	}
	for _, quality := range []int{BestSpeed, DefaultCompression} {
		var buf bytes.Buffer
		w := NewWriterLevel(&buf, quality)
		if _, err := w.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		flushed := bytes.Clone(buf.Bytes())
		tests = append(tests, testCase{fmt.Sprintf("flushed Q%d", quality), flushed, len(payload)})
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	tests = append(tests, testCase{"incomplete second member", append(bytes.Clone(complete), reported...), 2 * len(payload)})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoders := map[string]func() error{
				"Decode": func() error {
					_, err := Decode(nil, tt.data)
					return err
				},
				"DecodeLimit": func() error {
					_, err := DecodeLimit(nil, tt.data, tt.limit)
					return err
				},
				"DecodeWithLimits": func() error {
					_, err := DecodeWithLimits(nil, tt.data, tt.limit, 24)
					return err
				},
				"ResetBytes": func() error {
					r := NewReader(bytes.NewReader(complete))
					if _, err := io.ReadAll(r); err != nil {
						return err
					}
					r.ResetBytes(tt.data)
					_, err := io.ReadAll(r)
					return err
				},
				"Reader": func() error {
					_, err := io.ReadAll(NewReader(bytes.NewReader(tt.data)))
					return err
				},
			}
			for name, decode := range decoders {
				t.Run(name, func(t *testing.T) {
					if err := decode(); !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatalf("got %v, want io.ErrUnexpectedEOF", err)
					}
				})
			}
		})
	}
}

func TestDecodeMemberBoundaries(t *testing.T) {
	payload := []byte("hello")
	empty := Encode(nil, nil, DefaultCompression)
	complete := Encode(nil, payload, DefaultCompression)
	for _, tt := range []struct {
		name string
		data []byte
		want []byte
	}{
		{"empty", empty, nil},
		{"exact", complete, payload},
		{"empty members", append(bytes.Clone(empty), empty...), nil},
		{"mixed members", append(append(bytes.Clone(empty), complete...), empty...), payload},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, decode := range []func([]byte, []byte) ([]byte, error){
				Decode,
				func(dst, src []byte) ([]byte, error) { return DecodeLimit(dst, src, len(tt.want)) },
				func(dst, src []byte) ([]byte, error) { return DecodeWithLimits(dst, src, len(tt.want), 24) },
			} {
				got, err := decode(make([]byte, 0, len(tt.want)), tt.data)
				if err != nil || !bytes.Equal(got, tt.want) {
					t.Fatalf("got %q, %v; want %q, nil", got, err, tt.want)
				}
			}
		})
	}
}

type memberReadObserver struct {
	io.Reader
	calls    int
	readMore chan struct{}
}

func (r *memberReadObserver) Read(p []byte) (int, error) {
	r.calls++
	if r.calls == 2 {
		close(r.readMore)
	}
	return r.Reader.Read(p)
}

func TestReaderReturnsOutputBeforeReadingNextMember(t *testing.T) {
	pr, pw := io.Pipe()
	src := &memberReadObserver{Reader: pr, readMore: make(chan struct{})}
	r := NewReader(src)
	var workers sync.WaitGroup
	t.Cleanup(func() {
		pr.Close()
		pw.Close()
		workers.Wait()
	})

	for _, text := range []string{"first member", "second member"} {
		compressed := Encode(nil, []byte(text), DefaultCompression)
		writeDone := make(chan error, 1)
		workers.Add(2)
		go func() {
			defer workers.Done()
			_, err := pw.Write(compressed)
			writeDone <- err
		}()
		type result struct {
			data string
			err  error
		}
		readDone := make(chan result, 1)
		go func() {
			defer workers.Done()
			buf := make([]byte, 1024)
			n, err := r.Read(buf)
			readDone <- result{string(buf[:n]), err}
		}()
		var unexpectedRead <-chan struct{}
		if text == "first member" {
			unexpectedRead = src.readMore
		}
		select {
		case <-unexpectedRead:
			t.Fatal("Read fetched another member before returning output")
		case got := <-readDone:
			if got.data != text || got.err != nil {
				t.Fatalf("Read = %q, %v; want %q, nil", got.data, got.err, text)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Read blocked with a completed member available")
		}
		select {
		case err := <-writeDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("pipe write did not finish")
		}
		if text == "first member" {
			select {
			case <-src.readMore:
				t.Fatal("Read fetched another member before returning output")
			default:
			}
		}
	}
	pw.Close()
	if n, err := r.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatalf("final Read = %d, %v; want 0, EOF", n, err)
	}
}
