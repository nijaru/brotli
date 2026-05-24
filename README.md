# brotli

Package `brotli` implements the Brotli compression format (RFC 7932) in pure Go.

This library is an optimized, allocation-free fork of `github.com/andybalholm/brotli` that retains complete API compatibility.

---

## Optimizations

* **Zero-Allocation Resets**: Reusing a writer via `Reset()` or flushing via `Flush()` performs no heap allocations by using concrete structural routing inside `brotli.Writer`.
* **Fast State Clearing**: Uses Go builtins like `clear()` in hot decoder paths for faster memory operations.
* **Lower Memory Footprint**: Reduces memory allocation by ~9 KB per writer instance at levels Q1–Q11 by removing redundant tables from the generic compression state.
* **Parallel Encoding**: Includes a multi-threaded block encoder (`NewParallelWriter`) and alternative matchfinders (`NewWriterV2`) for high-concurrency systems.

---

## Correctness & Interoperability

The test suite automatically verifies compliance and compatibility:

| Test Target | Verification Type | Description |
|:---|:---|:---|
| **C Reference Library** | Differential Round-Trip | Go Encoder &rarr; C Decoder and C Encoder &rarr; Go Decoder across all quality levels ($Q0\text{–}Q11$) |
| **Standard Go Brotli** | Cross-Decoder Round-Trip | Go Encoder &rarr; Standard Decoder and Standard Encoder &rarr; Go Decoder across $Q0\text{–}Q11$ |
| **Direct Bitstream Parity** | Byte-Identity Verification | $Q0$ compressed byte-identity against `brotli -q 0 -w 22` |
| **Native Fuzzing** | Memory Safety & Fuzzing | Continuous random mutation testing with `go test -fuzz` |

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
	original := []byte("Brotli compression in Go.")

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

### 2. High-Performance Zero-Allocation Re-use
```go
// Discard the writer's state and re-use the underlying buffers for a new target
writer.Reset(newDestination)
```

### 3. Multithreaded Parallel Compression
```go
// Create a writer that compresses blocks in parallel across 4 goroutines
writer := brotli.NewParallelWriter(destination, brotli.DefaultCompression, 4)
```

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
Some components are derived from Google's reference Brotli C library (licensed under the MIT License).
