# brotli

Package `brotli` implements the Brotli compression format (RFC 7932) in pure Go.

This library is an optimized, allocation-free fork of the `c2go` port (`github.com/andybalholm/brotli`). It requires Go 1.26 or later and maintains complete API compatibility with the `c2go` implementation, allowing it to be used as a direct, high-performance replacement.

---

## Optimizations

* **Zero-Allocation Resets**: Reusing a writer via `Reset()` or flushing via `Flush()` performs no heap allocations by using concrete structural routing inside `brotli.Writer`.
* **Fast State Clearing**: Uses Go builtins like `clear()` in hot decoder paths for faster memory operations.
* **Lower Memory Footprint**: Reduces memory allocation by ~9 KB per writer instance at levels Q1–Q11 by removing redundant tables from the generic compression state.

---

## Correctness & Interoperability

The test suite automatically verifies compliance and compatibility:

| Test Target | Verification Type | Description |
|:---|:---|:---|
| **C Reference Library** | Differential Round-Trip | This package &rarr; C Reference Decoder and C Reference Encoder &rarr; This package across all quality levels ($Q0\text{–}Q11$) |
| **c2go Port** | Cross-Decoder Round-Trip | This package &rarr; `c2go` Decoder and `c2go` Encoder &rarr; This package across all quality levels ($Q0\text{–}Q11$) |
| **Direct Bitstream Parity** | Byte-Identity Verification | $Q0$ compressed byte-identity of this package vs. official C encoder (`brotli -q 0 -w 22`) |
| **Native Fuzzing** | Memory Safety & Fuzzing | Continuous random mutation testing with `go test -fuzz` |

---

## Performance

This library focuses on minimizing memory allocations during reuse. Reusing a writer via `Reset()` completely eliminates heap allocations at standard compression levels:

* **Levels Q0–Q9**: `0 allocs/op` during buffer reuse.
* **Throughput**: Up to 300 MB/s at level Q0 depending on hardware configuration.

To run the benchmark suite locally:

```bash
go test -bench=BenchmarkEncodeLevelsReset -benchmem .
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

### 2. High-Performance Zero-Allocation Re-use
```go
// Discard the writer's state and re-use the underlying buffers for a new target
writer.Reset(newDestination)
```

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
Some components are derived from Google's reference Brotli C library (licensed under the MIT License).
