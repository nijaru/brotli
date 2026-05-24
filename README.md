# Modernized Brotli for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/nijaru/brotli.svg)](https://pkg.go.dev/github.com/nijaru/brotli)
[![Go Report Card](https://goreportcard.com/badge/github.com/nijaru/brotli)](https://goreportcard.com/report/github.com/nijaru/brotli)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A highly optimized, modernized, and idiomatic Go package for Brotli compression and decompression. This library is a production-ready fork and comprehensive rewrite of the original `c2go`-translated port (`github.com/andybalholm/brotli`), bringing modern Go performance and memory safety characteristics without altering the standard API.

---

## Optimizations

* **Zero-Allocation Resets**: Uses concrete structural routing in `brotli.Writer` (separating Q0 and Generic paths) to ensure that writer reuse via `Reset()` and stream flushes via `Flush()` do not allocate heap memory.
* **Go 1.26 Modernizations**: Replaces manual loops with Go 1.26 builtins (such as `clear()`) in hot paths of the decoder state machine.
* **Reduced Memory Usage**: Prunes obsolete tables and duplicate structures from the generic compression state to reduce individual `brotli.Writer` memory allocations by approximately 9 KB per instance at levels Q1–Q11.
* **Block-Level Parallelism & Alternative Matchfinders**: Adds a multi-threaded block encoder (`NewParallelWriter`) and optimized matchfinders (`NewWriterV2`) to support higher throughput on multi-core systems.

---

## Correctness & Interoperability

To guarantee that this library can serve as a drop-in, zero-regression replacement for standard implementations, we maintain a comprehensive, automated verification suite:

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
	original := []byte("Modernized Brotli compression for Go 1.26!")

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
