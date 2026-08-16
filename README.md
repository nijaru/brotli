# brotli

A high-performance, idiomatic pure Go implementation of the Brotli compression format (RFC 7932).

Originally derived from Google Brotli and the `andybalholm/brotli` Go port, this library has been completely modernized into a modular Go architecture requiring Go 1.26+. It provides two cohesive API tiers: a **Classic Streaming API** for 100% drop-in compatibility, and a **Modern High-Performance API** featuring zero-allocation block operations, Go 1.23+ range iterators, and concurrency pools.

---

## Two API Tiers

### 1. Classic Streaming API (100% Drop-In Compatible)
* Drop-in replacement for `github.com/andybalholm/brotli` and Go standard library `compress/*` patterns.
* Streaming [`NewWriter`](file:///Users/nick/github/nijaru/brotli/writer.go#L56), [`NewWriterLevel`](file:///Users/nick/github/nijaru/brotli/writer.go#L64), [`NewReader`](file:///Users/nick/github/nijaru/brotli/reader.go#L20), and buffer-reusing `Reset()` methods.

### 2. Modern High-Performance API
* **Zero-Alloc Block API ([`Encode`](file:///Users/nick/github/nijaru/brotli/encode.go#L9)/[`Decode`](file:///Users/nick/github/nijaru/brotli/decode.go#L11))**: In-memory block compression and decompression with direct-to-slice buffer reuse.
* **Go 1.23+ Range Iterators**: Decompress streams directly in `for...range` loops via [`Lines()`](file:///Users/nick/github/nijaru/brotli/iter.go#L13) and [`Chunks(size)`](file:///Users/nick/github/nijaru/brotli/iter.go#L39).
* **High-Churn Resource Pools**: Concurrency-safe [`GetWriter`](file:///Users/nick/github/nijaru/brotli/pool.go#L37)/[`PutWriter`](file:///Users/nick/github/nijaru/brotli/pool.go#L43) and quality-specific [`WriterPool`](file:///Users/nick/github/nijaru/brotli/pool.go#L73)/[`ReaderPool`](file:///Users/nick/github/nijaru/brotli/pool.go#L113).
* **Modern I/O Fast Paths**: Zero-alloc `io.WriterTo`, `io.ReaderFrom`, `io.StringWriter`, and `io.ByteWriter`.
* **Pre-Shared Custom Dictionaries**: Sliding window pre-seeding via `WriterOptions.CustomDict` and `Reader.SetCustomDictionary()`.
* **HTTP Compression Middleware**: Thread-safe pooled HTTP compressor in [`http.go`](file:///Users/nick/github/nijaru/brotli/http.go).

---

## Verification

The test suite verifies correctness and interoperability:

| Test Target | Verification Type | Description |
| :--- | :--- | :--- |
| **C Reference Library** | Differential Round-Trip | Round-trip tests comparing this package against the C reference encoder and decoder across levels Q0–Q11. |
| **c2go Port** | Cross-Decoder Round-Trip | Verifies compatibility by encoding with this package and decoding with `c2go` (and vice versa) across all levels. |
| **Direct Bitstream Parity** | Byte-Identity | Ensures Q0 output matches the official C encoder (`brotli -q 0 -w 22`) byte-for-byte. |
| **Robustness** | Mutation Testing | Corrupts streams to verify the decoder exits safely with diagnostic errors instead of panicking or looping. |

---

## Performance

### Benchmarks (Apple M3 Max)
* **Pooled & Reset Streams (Q0–Q8):** `0 allocs/op` across all payload sizes.
* **Q10–Q11 Zopfli Optimizations:** Struct compaction & escape elimination reduced Q10 allocations from 1.2M to 61 allocs/op and Q11 allocations from 4.8M to 63 allocs/op (-99.998%), dropping memory footprint by 92–95%.
* **Block Decompression:** `0 allocs/op` when reusing destination slice capacity, completely bypassing double-buffering.

To run benchmarks locally:
```bash
go test -bench=BenchmarkEncodeLevelsReset -benchmem .
go test -bench=BenchmarkCompareUpstreamBrotli -benchmem .
go test -bench=BenchmarkBlock -benchmem .
```

---

## Installation

```bash
go get github.com/nijaru/brotli
```

---

## Usage Examples

### 1. Standard Reader & Writer
```go
package main

import (
	"bytes"
	"io"
	"log"

	"github.com/nijaru/brotli"
)

func main() {
	var compressed bytes.Buffer
	original := []byte("Brotli compression.")

	// Compress
	writer := brotli.NewWriter(&compressed)
	if _, err := writer.Write(original); err != nil {
		log.Fatalf("Compression failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		log.Fatalf("Close failed: %v", err)
	}

	// Decompress
	reader := brotli.NewReader(&compressed)
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		log.Fatalf("Decompression failed: %v", err)
	}

	println(string(decompressed))
}
```

### 2. Block API (Snappy-Style)
Reusing a slice capacity in `dst` executes with zero heap allocations:

```go
package main

import (
	"fmt"
	"log"

	"github.com/nijaru/brotli"
)

func main() {
	original := []byte("high-performance block data")

	// Compress block (DefaultCompression is 6; quality ranges from 0 to 11)
	compressed := brotli.Encode(nil, original, brotli.DefaultCompression)

	// Decompress block (reusing dst slice capacity)
	var dst []byte
	var err error
	dst, err = brotli.Decode(dst, compressed)
	if err != nil {
		log.Fatalf("Decode failed: %v", err)
	}

	fmt.Println(string(dst))
}
```

### 3. Go 1.23+ Range Iterators
Decompress text line-by-line or slice data chunk-by-chunk using standard `for...range` loops:

```go
// Line iterator
for line, err := range reader.Lines() {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(line)
}

// Zero-allocation chunk iterator (yields slices up to 8KB)
for chunk, err := range reader.Chunks(8192) {
	if err != nil {
		log.Fatal(err)
	}
	process(chunk) // Slice is only valid during this iteration
}
```

### 4. Pooled Streaming
```go
var compressed bytes.Buffer

writer := brotli.GetWriter(&compressed)
if _, err := writer.Write(original); err != nil {
	log.Fatalf("Compression failed: %v", err)
}
if err := writer.Close(); err != nil {
	log.Fatalf("Close failed: %v", err)
}
brotli.PutWriter(writer) // Always Put after Close
```

### 5. Manual Reset
```go
// Discard the writer's state and target a new destination
writer.Reset(newDestination)
```

---

## License

MIT License. See [LICENSE](LICENSE) for details.
