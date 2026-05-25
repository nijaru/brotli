# brotli

Package `brotli` implements the Brotli compression format (RFC 7932) in pure Go.

This library is an optimized, allocation-conscious fork of the `c2go` port (`github.com/andybalholm/brotli`). It requires Go 1.26 or later and keeps the standard reader, writer, and HTTP helper APIs compatible with the `c2go` implementation, allowing it to be used as a direct, high-performance replacement.

---

## Features & Optimizations

* **Zero-Allocation Block API**: Provides Snappy-style block `Encode` and `Decode` functions with automatic pool recycling. Reusing destination slices delivers absolute zero heap allocations in the steady state (including a direct-to-slice decompression path that bypasses double-buffering).
* **Go 1.23+ Range Iterators**: First-in-class native decompressor iterators (`Lines()` and `Chunks()`) for clean, modern `for...range` loops.
* **Zero-Allocation Resets**: Reusing a writer via `Reset()` or decompressor via `Reset()` performs no heap allocations in the steady state.
* **Fast State Clearing**: Uses Go builtins like `clear()` in hot decoder paths for faster memory operations.
* **Lower Memory Footprint**: Reduces memory allocation by ~9 KB per writer instance at levels Q1–Q11 by removing redundant tables from the generic compression state, and leverages a 45x memory reduction for Q10-Q11 inside the splitters.

---

## Correctness & Interoperability

The test suite verifies compliance and compatibility:

| Test Target | Verification Type | Description |
|:---|:---|:---|
| **C Reference Library** | Differential Round-Trip | When the `brotli` CLI is installed, this package &rarr; C Reference Decoder and C Reference Encoder &rarr; This package across all quality levels ($Q0\text{–}Q11$) |
| **c2go Port** | Cross-Decoder Round-Trip | This package &rarr; `c2go` Decoder and `c2go` Encoder &rarr; This package across all quality levels ($Q0\text{–}Q11$) |
| **Direct Bitstream Parity** | Byte-Identity Verification | $Q0$ compressed byte-identity of this package vs. official C encoder (`brotli -q 0 -w 22`) |
| **Mutation Coverage** | Decoder Robustness | Corrupted-stream tests verify decoder termination and error handling |

---

## Performance

This library focuses on minimizing memory allocations during reuse. For repeated
one-shot compression, prefer `GetWriter`/`PutWriter` or a `WriterPool` for the
chosen quality level:

* **Pooled lifecycle:** Reuses encoder state across independent streams.
* **Manual `Reset`:** Best when one goroutine owns a long-lived writer.
* **Standard constructors:** Keep c2go-compatible drop-in behavior for simple
  calls and one-off compression.

Recent local benchmarks on an Apple M3 Max show pooled and manual-reset writer
lifecycles at `0 allocs/op` for Q0, Q6, and Q9 across 512 B, 8 KiB, and 64 KiB
payloads. Constructing a fresh writer each time still pays the setup cost, from
small Q0 allocations to multi-megabyte setup at higher quality levels. Q10-Q11
remain correctness-first dense modes with higher allocation cost, although
pooling still avoids most construction overhead.

To run the benchmark suite locally:

```bash
go test -bench=BenchmarkEncodeLevelsReset -benchmem .
go test -bench=BenchmarkWriterLifecycle -benchmem .
```

---

## Installation

This package requires **Go 1.26 or later**.

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

### 2. Zero-Allocation Block API (Snappy-Style)
For in-memory block operations, use the `Encode` and `Decode` facade. Reusing a slice capacity in `dst` delivers absolute zero heap allocations in the steady state:

```go
package main

import (
	"fmt"
	"log"

	"github.com/nijaru/brotli"
)

func main() {
	original := []byte("high-performance block data")

	// 1. Compress block (quality level ranges from 0 to 11)
	compressed := brotli.Encode(nil, original, brotli.DefaultCompression)

	// 2. Decompress block (reusing a destination slice to prevent allocation)
	var dst []byte
	var err error
	dst, err = brotli.Decode(dst, compressed)
	if err != nil {
		log.Fatalf("Decode failed: %v", err)
	}

	fmt.Println(string(dst))
}
```

### 3. Modern Go 1.23+ Range Iterators
Decompress streams line-by-line or chunk-by-chunk using standard `for...range` loops:

```go
// Line iterator
for line, err := range reader.Lines() {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(line)
}

// Zero-allocation chunk iterator (yielding chunks up to 8KB)
for chunk, err := range reader.Chunks(8192) {
	if err != nil {
		log.Fatal(err)
	}
	// Process chunk (valid only for the current iteration)
	process(chunk)
}
```

### 4. Pooled Streaming Reuse
```go
var compressed bytes.Buffer

writer := brotli.GetWriter(&compressed)
if _, err := writer.Write(original); err != nil {
	log.Fatalf("Compression failed: %v", err)
}
if err := writer.Close(); err != nil {
	log.Fatalf("Close failed: %v", err)
}
brotli.PutWriter(writer)
```

Call `PutWriter` only after `Close`, and do not use the writer after returning
it to the pool.

### 5. Manual Reset Reuse
```go
// Discard the writer's state and re-use the underlying buffers for a new target
writer.Reset(newDestination)
```

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
Some components are derived from Google's reference Brotli C library (licensed under the MIT License).
