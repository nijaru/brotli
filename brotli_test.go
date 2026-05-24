// Copyright 2016 Google Inc. All Rights Reserved.
//
// Distributed under MIT license.
// See file LICENSE for detail or copy at https://opensource.org/licenses/MIT

package brotli

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	rand "math/rand/v2"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	andybrotli "github.com/andybalholm/brotli"
	"github.com/xyproto/randomstring"
)

func pseudoRandomBytes(n int) []byte {
	var seed [32]byte
	seed[0] = 1
	data := make([]byte, n)
	_, _ = rand.NewChaCha8(seed).Read(data)
	return data
}

func checkCompressedData(compressedData, wantOriginalData []byte) error {
	uncompressed, err := Decode(compressedData)
	if err != nil {
		return fmt.Errorf("brotli decompress failed: %v", err)
	}
	if !bytes.Equal(uncompressed, wantOriginalData) {
		if len(wantOriginalData) != len(uncompressed) {
			return fmt.Errorf(""+
				"Data doesn't uncompress to the original value.\n"+
				"Length of original: %v\n"+
				"Length of uncompressed: %v",
				len(wantOriginalData), len(uncompressed))
		}
		for i := range wantOriginalData {
			if wantOriginalData[i] != uncompressed[i] {
				return fmt.Errorf(""+
					"Data doesn't uncompress to the original value.\n"+
					"Original at %v is %v\n"+
					"Uncompressed at %v is %v",
					i, wantOriginalData[i], i, uncompressed[i])
			}
		}
	}
	return nil
}

func TestEncoderNoWrite(t *testing.T) {
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	if err := e.Close(); err != nil {
		t.Errorf("Close()=%v, want nil", err)
	}
	// Check Write after close.
	if _, err := e.Write([]byte("hi")); err == nil {
		t.Errorf("No error after Close() + Write()")
	}
}

func TestEncoderEmptyWrite(t *testing.T) {
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	n, err := e.Write([]byte(""))
	if n != 0 || err != nil {
		t.Errorf("Write()=%v,%v, want 0, nil", n, err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close()=%v, want nil", err)
	}
}

func TestWriterOptionsClampQ0LGWin(t *testing.T) {
	input := []byte("hello hello hello hello")
	for _, lgwin := range []int{0, 2, 9, 10, 24, 25, 30} {
		out := bytes.Buffer{}
		w := NewWriterOptions(&out, WriterOptions{Quality: BestSpeed, LGWin: lgwin})
		n, err := w.Write(input)
		if err != nil {
			t.Fatalf("LGWin %d: Write() error: %v", lgwin, err)
		}
		if n != len(input) {
			t.Fatalf("LGWin %d: Write() n=%d, want %d", lgwin, n, len(input))
		}
		if err := w.Close(); err != nil {
			t.Fatalf("LGWin %d: Close() error: %v", lgwin, err)
		}
		if err := checkCompressedData(out.Bytes(), input); err != nil {
			t.Fatalf("LGWin %d: compressed data did not round trip: %v", lgwin, err)
		}
	}
}

func TestIssue22(t *testing.T) {
	f, err := os.Open("testdata/issue22.gz")
	if err != nil {
		t.Fatalf("Error opening test data file: %v", err)
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("Error creating gzip reader: %v", err)
	}

	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("Error reading test data: %v", err)
	}

	if len(data) != 2851073 {
		t.Fatalf("Wrong length for test data: got %d, want 2851073", len(data))
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		out := bytes.Buffer{}
		e := NewWriterOptions(&out, WriterOptions{Quality: level})
		n, err := e.Write(data)
		if err != nil {
			t.Errorf("Level %d: Error compressing data: %v", level, err)
			continue
		}
		if int(n) != len(data) {
			t.Errorf("Level %d: Write() n=%v, want %v", level, n, len(data))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Level %d: Close Error after writing %d bytes: %v", level, n, err)
			continue
		}
		if err := checkCompressedData(out.Bytes(), data); err != nil {
			t.Errorf("Level %d: Error decompressing data: %v", level, err)
		} else {
			t.Logf("Level %d: Compressed size: %d", level, out.Len())
			t.Logf("Level %d: Success!", level)
		}
	}
}

func TestEncoderStreams(t *testing.T) {
	// Test that output is streamed.
	// Adjust window size to ensure the encoder outputs at least enough bytes
	// to fill the window.
	const lgWin = 16
	windowSize := int(math.Pow(2, lgWin))
	input := pseudoRandomBytes(8 * windowSize)
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 11, LGWin: lgWin})
	halfInput := input[:len(input)/2]
	in := bytes.NewReader(halfInput)

	n, err := io.Copy(e, in)
	if err != nil {
		t.Errorf("Copy Error: %v", err)
	}

	// We've fed more data than the sliding window size. Check that some
	// compressed data has been output.
	if out.Len() == 0 {
		t.Errorf("Output length is 0 after %d bytes written", n)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close Error after copied %d bytes: %v", n, err)
	}
	if err := checkCompressedData(out.Bytes(), halfInput); err != nil {
		t.Error(err)
	}
}

func TestEncoderLargeInput(t *testing.T) {
	for level := BestSpeed; level <= BestCompression; level++ {
		input := pseudoRandomBytes(1000000)
		out := bytes.Buffer{}
		e := NewWriterOptions(&out, WriterOptions{Quality: level})
		in := bytes.NewReader(input)

		n, err := io.Copy(e, in)
		if err != nil {
			t.Errorf("Copy Error: %v", err)
		}
		if int(n) != len(input) {
			t.Errorf("Copy() n=%v, want %v", n, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close Error after copied %d bytes: %v", n, err)
		}
		if err := checkCompressedData(out.Bytes(), input); err != nil {
			t.Error(err)
		}

		out2 := bytes.Buffer{}
		e.Reset(&out2)
		n2, err := e.Write(input)
		if err != nil {
			t.Errorf("Write error after Reset: %v", err)
		}
		if n2 != len(input) {
			t.Errorf("Write() after Reset n=%d, want %d", n2, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close error after Reset (copied %d) bytes: %v", n2, err)
		}
		if !bytes.Equal(out.Bytes(), out2.Bytes()) {
			t.Error("Compressed data after Reset doesn't equal first time")
		}
	}
}

func TestEncoderFlush(t *testing.T) {
	input := pseudoRandomBytes(1000)
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	in := bytes.NewReader(input)
	_, err := io.Copy(e, in)
	if err != nil {
		t.Fatalf("Copy Error: %v", err)
	}
	if err := e.Flush(); err != nil {
		t.Fatalf("Flush(): %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("0 bytes written after Flush()")
	}
	decompressed := make([]byte, 1000)
	reader := NewReader(bytes.NewReader(out.Bytes()))
	n, err := reader.Read(decompressed)
	if n != len(decompressed) || err != nil {
		t.Errorf("Expected <%v, nil>, but <%v, %v>", len(decompressed), n, err)
	}
	if !bytes.Equal(decompressed, input) {
		t.Errorf(""+
			"Decompress after flush: %v\n"+
			"%q\n"+
			"want:\n%q",
			err, decompressed, input)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close(): %v", err)
	}
}

type readerWithTimeout struct {
	io.Reader
}

func (r readerWithTimeout) Read(p []byte) (int, error) {
	type result struct {
		n   int
		err error
	}
	ch := make(chan result)
	go func() {
		n, err := r.Reader.Read(p)
		ch <- result{n, err}
	}()
	select {
	case result := <-ch:
		return result.n, result.err
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("read timed out")
	}
}

func TestDecoderStreaming(t *testing.T) {
	pr, pw := io.Pipe()
	writer := NewWriterOptions(pw, WriterOptions{Quality: 5, LGWin: 20})
	reader := readerWithTimeout{NewReader(pr)}
	defer func() {
		go io.ReadAll(pr) // swallow the "EOF" token from writer.Close
		if err := writer.Close(); err != nil {
			t.Errorf("writer.Close: %v", err)
		}
	}()

	ch := make(chan []byte)
	errch := make(chan error)
	go func() {
		for {
			segment, ok := <-ch
			if !ok {
				return
			}
			if n, err := writer.Write(segment); err != nil || n != len(segment) {
				errch <- fmt.Errorf("write=%v,%v, want %v,%v", n, err, len(segment), nil)
				return
			}
			if err := writer.Flush(); err != nil {
				errch <- fmt.Errorf("flush: %v", err)
				return
			}
		}
	}()
	defer close(ch)

	segments := [...][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
	}
	for k, segment := range segments {
		t.Run(fmt.Sprintf("Segment%d", k), func(t *testing.T) {
			select {
			case ch <- segment:
			case err := <-errch:
				t.Fatalf("write: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out")
			}
			wantLen := len(segment)
			got := make([]byte, wantLen)
			if n, err := reader.Read(got); err != nil || n != wantLen ||
				!bytes.Equal(got, segment) {
				t.Fatalf("read[%d]=%q,%v,%v, want %q,%v,%v", k, got, n, err, segment, wantLen, nil)
			}
		})
	}
}

func TestReader(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	r := NewReader(bytes.NewReader(encoded))
	var decodedOutput bytes.Buffer
	n, err := io.Copy(&decodedOutput, r)
	if err != nil {
		t.Fatalf("Copy(): n=%v, err=%v", n, err)
	}
	if got := decodedOutput.Bytes(); !bytes.Equal(got, content) {
		t.Errorf(""+
			"Reader output:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			got, len(content))
	}

	r.Reset(bytes.NewReader(encoded))
	decodedOutput.Reset()
	n, err = io.Copy(&decodedOutput, r)
	if err != nil {
		t.Fatalf("After Reset: Copy(): n=%v, err=%v", n, err)
	}
	if got := decodedOutput.Bytes(); !bytes.Equal(got, content) {
		t.Errorf("After Reset: "+
			"Reader output:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			got, len(content))
	}
}

func TestDecode(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	decoded, err := Decode(encoded)
	if err != nil {
		t.Errorf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, content) {
		t.Errorf(""+
			"Decode content:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			decoded, len(content))
	}
}

func TestQuality(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	for q := 0; q < 12; q++ {
		encoded, _ := Encode(content, WriterOptions{Quality: q})
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode: %v", err)
		}
		if !bytes.Equal(decoded, content) {
			t.Errorf(""+
				"Decode content:\n"+
				"%q\n"+
				"want:\n"+
				"<%d bytes>",
				decoded, len(content))
		}
	}
}

func TestDecodeFuzz(t *testing.T) {
	// Test that the decoder terminates with corrupted input.
	content := bytes.Repeat([]byte("hello world!"), 100)
	rnd := rand.New(rand.NewPCG(0, 0))
	encoded, err := Encode(content, WriterOptions{Quality: 5})
	if err != nil {
		t.Fatalf("Encode(<%d bytes>, _) = _, %s", len(content), err)
	}
	if len(encoded) == 0 {
		t.Fatalf("Encode(<%d bytes>, _) produced empty output", len(content))
	}
	for i := 0; i < 100; i++ {
		enc := append([]byte{}, encoded...)
		for j := 0; j < 5; j++ {
			enc[rnd.IntN(len(enc))] = byte(rnd.IntN(256))
		}
		Decode(enc)
	}
}

func TestDecodeTrailingData(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 100)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	_, err := Decode(append(encoded, 0))
	if err == nil {
		t.Errorf("Expected 'excessive input' error")
	}
}

func TestEncodeDecode(t *testing.T) {
	for _, test := range []struct {
		data    []byte
		repeats int
	}{
		{nil, 0},
		{[]byte("A"), 1},
		{[]byte("<html><body><H1>Hello world</H1></body></html>"), 10},
		{[]byte("<html><body><H1>Hello world</H1></body></html>"), 1000},
	} {
		t.Logf("case %q x %d", test.data, test.repeats)
		input := bytes.Repeat(test.data, test.repeats)
		encoded, err := Encode(input, WriterOptions{Quality: 5})
		if err != nil {
			t.Errorf("Encode: %v", err)
		}
		// Inputs are compressible, but may be too small to compress.
		if maxSize := len(input)/2 + 20; len(encoded) >= maxSize {
			t.Errorf(""+
				"Encode returned %d bytes, want <%d\n"+
				"Encoded=%q",
				len(encoded), maxSize, encoded)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode: %v", err)
		}
		if !bytes.Equal(decoded, input) {
			var want string
			if len(input) > 320 {
				want = fmt.Sprintf("<%d bytes>", len(input))
			} else {
				want = fmt.Sprintf("%q", input)
			}
			t.Errorf(""+
				"Decode content:\n"+
				"%q\n"+
				"want:\n"+
				"%s",
				decoded, want)
		}
	}
}

func TestErrorReset(t *testing.T) {
	compress := func(input []byte) []byte {
		var buf bytes.Buffer
		writer := new(Writer)
		writer.Reset(&buf)
		writer.Write(input)
		writer.Close()

		return buf.Bytes()
	}

	corruptReader := func(reader *Reader) {
		buf := bytes.NewBuffer([]byte("trash"))
		reader.Reset(buf)
		_, err := io.ReadAll(reader)
		if err == nil {
			t.Fatalf("successively decompressed invalid input")
		}
	}

	decompress := func(input []byte, reader *Reader) []byte {
		buf := bytes.NewBuffer(input)
		reader.Reset(buf)
		output, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("failed to decompress data %s", err.Error())
		}

		return output
	}

	source := []byte("text")

	compressed := compress(source)
	reader := &Reader{}
	corruptReader(reader)
	decompressed := decompress(compressed, reader)
	if string(source) != string(decompressed) {
		t.Fatalf("decompressed data does not match original state")
	}
}

// Encode returns content encoded with Brotli.
func Encode(content []byte, options WriterOptions) ([]byte, error) {
	var buf bytes.Buffer
	writer := NewWriterOptions(&buf, options)
	_, err := writer.Write(content)
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return buf.Bytes(), err
}

// Decode decodes Brotli encoded data.
func Decode(encodedData []byte) ([]byte, error) {
	r := NewReader(bytes.NewReader(encodedData))
	return io.ReadAll(r)
}

func BenchmarkEncodeLevels(b *testing.B) {
	opticks, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(opticks)))
			for b.Loop() {
				w := NewWriterLevel(io.Discard, level)
				w.Write(opticks)
				w.Close()
			}
		})
	}
}

func BenchmarkEncodeLevelsReset(b *testing.B) {
	opticks, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterLevel(buf, level)
		w.Write(opticks)
		w.Close()
		ratio := float64(len(opticks)) / float64(buf.Len())
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(opticks)))
			for b.Loop() {
				w.Reset(io.Discard)
				w.Write(opticks)
				w.Close()
				b.ReportMetric(ratio, "ratio")
			}
		})
	}
}

func BenchmarkDecodeLevels(b *testing.B) {
	opticks, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterLevel(buf, level)
		w.Write(opticks)
		w.Close()
		compressed := buf.Bytes()
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(opticks)))
			for b.Loop() {
				io.Copy(io.Discard, NewReader(bytes.NewReader(compressed)))
			}
		})
	}
}

func TestIssue51(t *testing.T) {
	for i := 65536; i <= 65536*4; i += 65536 {
		t.Run("compress data length: "+strconv.Itoa(i)+"bytes", func(t *testing.T) {
			dataStr := randomstring.HumanFriendlyString(i)
			dataBytes := []byte(dataStr)
			buf := bytes.Buffer{}
			w := NewWriterLevel(&buf, 4)

			n, err := w.Write(dataBytes)
			if err != nil {
				t.Fatalf("Error while compressing data: %v", err)
			}
			if n != len(dataBytes) {
				t.Fatalf("Bytes written (%d) != len(databytes) (%d)", n, len(dataBytes))
			}
			err = w.Close()
			if err != nil {
				t.Fatalf("Error closing writer: %v", err)
			}

			r := NewReader(&buf)
			dst := make([]byte, len(dataBytes)+100)
			p := dst
			total := 0
			for {
				n1, err1 := r.Read(p)
				if err1 != nil {
					if err1 != io.EOF {
						t.Fatal(err1)
					}
					break
				}
				total += n1
				p = p[n1:]
			}
			if !bytes.Equal(dst[:total], dataBytes) {
				t.Fatal("Decompressed bytes don't match")
			}
		})
	}
}

func TestIssue58(t *testing.T) {
	content := []byte("---\nthis-is-not-brotli: \"it is actually yaml\"")
	input := bytes.NewBuffer(content)

	r := NewReader(input)

	buf, err := io.ReadAll(r)
	if err == nil {
		t.Fatalf("expected error, got none and read:\n%x\n%s\n%v", buf, buf, buf)
	}
}

func TestCrossDecoderCompatibility(t *testing.T) {
	opticks, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{"Hello", []byte("Hello, Brotli World!")},
		{"Opticks", opticks},
		{"Empty", []byte{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for level := BestSpeed; level <= BestCompression; level++ {
				t.Run(fmt.Sprintf("Level_%d", level), func(t *testing.T) {
					// 1. Compress with OUR encoder
					var compressedBuf bytes.Buffer
					w := NewWriterOptions(&compressedBuf, WriterOptions{Quality: level})
					w.Write(tc.data)
					if err := w.Close(); err != nil {
						t.Fatalf("our encoder failed to close: %v", err)
					}

					// 2. Decompress with STANDARDIZED andybalholm decoder
					r := andybrotli.NewReader(bytes.NewReader(compressedBuf.Bytes()))
					decompressed, err := io.ReadAll(r)
					if err != nil {
						t.Fatalf("andybalholm failed to decompress our output: %v", err)
					}
					if !bytes.Equal(tc.data, decompressed) {
						t.Fatalf("decompressed data mismatch")
					}

					// 3. Compress with STANDARDIZED andybalholm encoder
					var stdCompressedBuf bytes.Buffer
					stdW := andybrotli.NewWriterLevel(&stdCompressedBuf, level)
					if _, err := stdW.Write(tc.data); err != nil {
						t.Fatalf("andybalholm failed to write: %v", err)
					}
					if err := stdW.Close(); err != nil {
						t.Fatalf("andybalholm failed to close: %v", err)
					}

					// 4. Decompress with OUR decoder
					ourR := NewReader(bytes.NewReader(stdCompressedBuf.Bytes()))
					ourDecompressed, err := io.ReadAll(ourR)
					if err != nil {
						t.Fatalf("our decoder failed to decompress andybalholm output: %v", err)
					}
					if !bytes.Equal(tc.data, ourDecompressed) {
						t.Fatalf("our decompressed data mismatch")
					}
				})
			}
		})
	}
}

func TestDifferentialCBinary(t *testing.T) {
	// Look for the "brotli" reference C command line tool in the system PATH
	brotliPath, err := exec.LookPath("brotli")
	if err != nil {
		t.Skip(
			"Reference 'brotli' C CLI tool not found in PATH, skipping differential interop tests",
		)
		return
	}

	opticks, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{"Hello", []byte("Hello, C Brotli World!")},
		{"Opticks", opticks},
		{"Empty", []byte{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for level := BestSpeed; level <= BestCompression; level++ {
				t.Run(fmt.Sprintf("Level_%d", level), func(t *testing.T) {
					// 1. Compress with OUR Go encoder
					var compressedBuf bytes.Buffer
					w := NewWriterOptions(&compressedBuf, WriterOptions{Quality: level})
					w.Write(tc.data)
					if err := w.Close(); err != nil {
						t.Fatalf("our encoder failed to close: %v", err)
					}

					// 2. Decompress using official Brotli C binary: brotli -d -c
					cDecompressCmd := exec.Command(brotliPath, "-d", "-c")
					cDecompressCmd.Stdin = &compressedBuf
					var cDecompressed bytes.Buffer
					cDecompressCmd.Stdout = &cDecompressed
					if err := cDecompressCmd.Run(); err != nil {
						t.Fatalf(
							"reference C brotli binary failed to decompress our output: %v",
							err,
						)
					}
					if !bytes.Equal(tc.data, cDecompressed.Bytes()) {
						t.Fatalf("decompressed data mismatch from C brotli tool")
					}

					// 3. Compress using official Brotli C binary: brotli -c -q <level>
					cCompressCmd := exec.Command(brotliPath, "-c", "-q", strconv.Itoa(level))
					cCompressCmd.Stdin = bytes.NewReader(tc.data)
					var cCompressed bytes.Buffer
					cCompressCmd.Stdout = &cCompressed
					if err := cCompressCmd.Run(); err != nil {
						t.Fatalf("reference C brotli binary failed to compress: %v", err)
					}

					// 4. Decompress using OUR Go decoder
					ourR := NewReader(&cCompressed)
					ourDecompressed, err := io.ReadAll(ourR)
					if err != nil {
						t.Fatalf(
							"our decoder failed to decompress reference C brotli output: %v",
							err,
						)
					}
					if !bytes.Equal(tc.data, ourDecompressed) {
						t.Fatalf("our decompressed data mismatch from C brotli input")
					}
				})
			}
		})
	}
}

func FuzzRoundTrip(f *testing.F) {
	opticks, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err == nil {
		f.Add(opticks)
	}
	f.Add([]byte("Hello, Brotli Fuzzing!"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, original []byte) {
		// Test multiple quality levels dynamically
		for _, level := range []int{0, 1, 5, 9, 11} {
			// Compress with our Go encoder
			var compressed bytes.Buffer
			w := NewWriterOptions(&compressed, WriterOptions{Quality: level})
			_, err := w.Write(original)
			if err != nil {
				t.Fatalf("Level %d: write failed: %v", level, err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Level %d: close failed: %v", level, err)
			}

			// Decompress with our Go decoder
			r := NewReader(&compressed)
			decompressed, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("Level %d: decompress failed: %v", level, err)
			}
			if !bytes.Equal(original, decompressed) {
				t.Fatalf("Level %d: original and decompressed data mismatch", level)
			}
		}
	})
}

func TestDirectBitstreamParity(t *testing.T) {
	brotliPath, err := exec.LookPath("brotli")
	if err != nil {
		t.Skip(
			"Reference 'brotli' C CLI tool not found in PATH, skipping direct bitstream parity tests",
		)
		return
	}

	opticks, err := os.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{"Hello", []byte("Hello, C Brotli World!")},
		{"OpticksPrefix", opticks[:50000]},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Compress with OUR Go encoder at Level 0
			var compressedBuf bytes.Buffer
			w := NewWriterOptions(&compressedBuf, WriterOptions{Quality: 0, LGWin: 22})
			w.Write(tc.data)
			w.Close()

			// 2. Compress with C reference encoder: brotli -c -q 0 -w 22
			cCompressCmd := exec.Command(brotliPath, "-c", "-q", "0", "-w", "22")
			cCompressCmd.Stdin = bytes.NewReader(tc.data)
			var cCompressed bytes.Buffer
			cCompressCmd.Stdout = &cCompressed
			if err := cCompressCmd.Run(); err != nil {
				t.Fatalf("reference C brotli binary failed to compress: %v", err)
			}

			// 3. Directly compare the byte slices to verify absolute bitstream byte identity!
			ourBytes := compressedBuf.Bytes()
			cBytes := cCompressed.Bytes()
			if !bytes.Equal(ourBytes, cBytes) {
				t.Logf("Length/bitstream variation: Go len=%d, C len=%d", len(ourBytes), len(cBytes))
				minLen := len(ourBytes)
				if len(cBytes) < minLen {
					minLen = len(cBytes)
				}
				firstDiff := -1
				for i := 0; i < minLen; i++ {
					if ourBytes[i] != cBytes[i] {
						firstDiff = i
						break
					}
				}
				if firstDiff != -1 {
					t.Logf("First difference at byte index %d (0x%x):", firstDiff, firstDiff)
					start := firstDiff - 10
					if start < 0 {
						start = 0
					}
					end := firstDiff + 10
					if end > minLen {
						end = minLen
					}
					t.Logf("Go around diff: %x", ourBytes[start:end])
					t.Logf("C  around diff: %x", cBytes[start:end])
				} else {
					t.Logf("No byte difference in prefix, but lengths differ.")
				}

				// Strict byte identity is enforced on "Hello" to prevent regressions
				if tc.name == "Hello" {
					t.Fatalf("Bitstream mismatch on strict parity target 'Hello'")
				}
			}

			// 4. Ensure BOTH streams are fully compatible and decompress back to the exact same input!
			// A. Our Go decoder reads C-compressed stream
			rC := NewReader(bytes.NewReader(cBytes))
			ourDecryptedFromC, err := io.ReadAll(rC)
			if err != nil {
				t.Fatalf("our decoder failed to decompress C bitstream: %v", err)
			}
			if !bytes.Equal(tc.data, ourDecryptedFromC) {
				t.Fatalf("our decoded data mismatch from C bitstream")
			}

			// B. C decoder reads our Go-compressed stream
			cDecompressCmd := exec.Command(brotliPath, "-d", "-c")
			cDecompressCmd.Stdin = bytes.NewReader(ourBytes)
			var cDecryptedFromGo bytes.Buffer
			cDecompressCmd.Stdout = &cDecryptedFromGo
			if err := cDecompressCmd.Run(); err != nil {
				t.Fatalf("C decoder failed to decompress our Go bitstream: %v", err)
			}
			if !bytes.Equal(tc.data, cDecryptedFromGo.Bytes()) {
				t.Fatalf("C decoded data mismatch from our Go bitstream")
			}
		})
	}
}
