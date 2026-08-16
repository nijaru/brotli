# brotli

Pure Go implementation of the Brotli compression format (RFC 7932) for Go 1.26+.

Provides a drop-in replacement for `github.com/andybalholm/brotli` alongside zero-allocation block APIs, range iterators, and concurrency pools.

## Installation

```bash
go get github.com/nijaru/brotli
```

## Quickstart

### 1. Streaming (Drop-In API)

```go
package main

import (
	"bytes"
	"io"

	"github.com/nijaru/brotli"
)

func main() {
	var buf bytes.Buffer

	// Compress
	w := brotli.NewWriter(&buf)
	w.Write([]byte("Hello, Brotli!"))
	w.Close()

	// Decompress
	r := brotli.NewReader(&buf)
	decompressed, _ := io.ReadAll(r)
	println(string(decompressed))
}
```

### 2. Block API (Zero Allocations)

Compress and decompress in-memory slices without streaming wrapper overhead:

```go
// Compress (DefaultCompression is 6; quality 0 to 11)
compressed := brotli.Encode(nil, data, brotli.DefaultCompression)

// Decompress (reusing dst slice capacity)
decompressed, err := brotli.Decode(dst[:0], compressed)
```

### 3. Iterators (Go 1.23+)

```go
// Line-by-line streaming
for line, err := range reader.Lines() {
	if err != nil {
		break
	}
	_ = line
}

// Zero-allocation chunk streaming (up to chunkSize bytes per slice)
for chunk, err := range reader.Chunks(8192) {
	if err != nil {
		break
	}
	_ = chunk
}
```

### 4. Pooled Streaming

```go
w := brotli.GetWriter(&buf)
w.Write(data)
w.Close()
brotli.PutWriter(w)
```

## Verification

- **Parity:** Verified against the C reference library and cross-tested across levels Q0–Q11.
- **Robustness:** Native Go fuzzing harness (`FuzzDecode`) with mutation tests.

## License

MIT License. See [LICENSE](LICENSE).
