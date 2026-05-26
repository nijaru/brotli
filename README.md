# brotli

A pure Go implementation of the Brotli compression format (RFC 7932).

This package is an optimized fork of the `c2go` port (`github.com/andybalholm/brotli`). It requires Go 1.26 or later and maintains API compatibility with the original package to act as a drop-in replacement.

---

## Features

* **Block API (`Encode`/`Decode`)**: In-memory block compression and decompression. Reusing destination slice capacity executes with zero heap allocations (including direct-to-slice decompression).
* **Go 1.23+ Range Iterators**: Decompress streams using native `Lines()` and `Chunks()` loops.
* **Stream Reuse**: Call `Reset()` on streaming writers and readers to clear state and reuse underlying buffers without new heap allocations.
* **Low Memory Footprint**: Reduces writer memory usage by ~9 KB at levels Q1–Q11 by removing redundant tables, and uses a 45x memory reduction for Q10–Q11 metablock splitting.

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

The library minimizes heap allocations through resource reuse. For repeated stream operations, use the package-level `GetWriter`/`PutWriter` pools to recycle resources and avoid garbage collection overhead.

### Benchmarks (Apple M3 Max)
* **Pooled & Reset Streams:** `0 allocs/op` for levels Q0, Q6, and Q9 across 512 B, 8 KiB, and 64 KiB payloads.
* **Block Decompression:** `1 alloc/op` (the `bytes.Reader` interface escape boundary) and `4,463 B/op` when reusing the destination buffer, completely bypassing intermediate double-buffering.

To run benchmarks locally:
```bash
go test -bench=BenchmarkEncodeLevelsReset -benchmem .
go test -bench=BenchmarkWriterLifecycle -benchmem .
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
