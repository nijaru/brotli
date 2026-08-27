// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package brotli_test

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/nijaru/brotli"
)

func ExampleWriter_Reset() {
	proverbs := []string{
		"Don't communicate by sharing memory, share memory by communicating.\n",
		"Concurrency is not parallelism.\n",
		"The bigger the interface, the weaker the abstraction.\n",
		"Documentation is for users.\n",
	}

	var b bytes.Buffer

	bw := brotli.NewWriter(nil)
	br := brotli.NewReader(nil)

	for _, s := range proverbs {
		b.Reset()

		// Reset the compressor and encode from some input stream.
		bw.Reset(&b)
		if _, err := io.WriteString(bw, s); err != nil {
			log.Fatal(err)
		}
		if err := bw.Close(); err != nil {
			log.Fatal(err)
		}

		// Reset the decompressor and decode to some output stream.
		if err := br.Reset(&b); err != nil {
			log.Fatal(err)
		}
		if _, err := io.Copy(os.Stdout, br); err != nil {
			log.Fatal(err)
		}
	}

	// Output:
	// Don't communicate by sharing memory, share memory by communicating.
	// Concurrency is not parallelism.
	// The bigger the interface, the weaker the abstraction.
	// Documentation is for users.
}

func ExampleEncode() {
	src := []byte("Hello, Brotli block compression!")

	// Compress using in-memory block API
	compressed := brotli.Encode(nil, src, brotli.DefaultCompression)

	// Decompress into pre-allocated slice
	decompressed, err := brotli.Decode(nil, compressed)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(decompressed))

	// Output:
	// Hello, Brotli block compression!
}

func ExampleReader_Lines() {
	text := "first line\nsecond line\nthird line"
	compressed := brotli.Encode(nil, []byte(text), brotli.DefaultCompression)

	r := brotli.NewReader(bytes.NewReader(compressed))

	// Stream line by line using Go 1.23+ range iterators
	for line, err := range r.Lines() {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(line)
	}

	// Output:
	// first line
	// second line
	// third line
}

func ExampleReader_Chunks() {
	text := strings.Repeat("A", 16)
	compressed := brotli.Encode(nil, []byte(text), brotli.DefaultCompression)

	r := brotli.NewReader(bytes.NewReader(compressed))

	// Yield decompressed chunks up to 8 bytes each
	var totalBytes int
	for chunk, err := range r.Chunks(8) {
		if err != nil {
			log.Fatal(err)
		}
		totalBytes += len(chunk)
	}

	fmt.Println("Total bytes decompressed:", totalBytes)

	// Output:
	// Total bytes decompressed: 16
}

func ExampleWriterPool() {
	pool := brotli.NewWriterPool(brotli.BestSpeed)

	var buf bytes.Buffer
	w := pool.Get(&buf)

	if _, err := w.Write([]byte("Pooled compression")); err != nil {
		log.Fatal(err)
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	pool.Put(w)

	decompressed, err := brotli.Decode(nil, buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(decompressed))

	// Output:
	// Pooled compression
}
