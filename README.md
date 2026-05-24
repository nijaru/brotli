# Modernized Brotli for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/nijaru/brotli.svg)](https://pkg.go.dev/github.com/nijaru/brotli)
[![Go Report Card](https://goreportcard.com/badge/github.com/nijaru/brotli)](https://goreportcard.com/report/github.com/nijaru/brotli)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A highly optimized, modernized, and idiomatic Go package for Brotli compression and decompression. This library is a production-ready fork and comprehensive rewrite of the original `c2go`-translated port (`github.com/andybalholm/brotli`), bringing modern Go performance and memory safety characteristics without altering the standard API.

---

## Key Enhancements

### ⚡ Zero-Allocation Stream Resets & Flushes
The original translated library suffered from performance overheads due to virtual interface dispatch and heap-escaping allocations when reusing writers.
This modernized port implements clean, concrete routing inside `brotli.Writer` (tier-based routing for Q0 vs. Generic engines). Reusing a writer via `Reset()` or invoking `Flush()` executes with **zero heap allocations**, making it ideal for high-throughput HTTP proxies and server runtimes.

### 🧹 Surgical Decoder Modernization
Surgically modernizes the Brotli decompression state machine. It leverages Go 1.26 primitives such as the builtin `clear()` for fast block-zeroing on hot paths, mapping directly to highly optimized CPU vector instructions.

### 📉 Reduced Memory Footprint
By pruning obsolete table representations and fast-path duplicates from the generic compression engines, the individual `brotli.Writer` memory allocation footprint is reduced by **~9 KB per instance** at levels Q1–Q11.

### 🏎️ Parallel Compression & Custom Matchfinders
Includes a robust multithreaded block-level encoder (`NewParallelWriter`) and customized matchfinders (`NewWriterV2`), letting you squeeze out maximum compression speeds and ratios across multi-core systems.

---

## Bulletproof Correctness & Interoperability

To guarantee that this library can serve as a drop-in, zero-regression replacement for standard implementations, we maintain a comprehensive testing system:

* **Official C Library Interop (`TestDifferentialCBinary`)**: Verified round-trip cross-decompression against the official Google `brotli` reference C tool in **both directions** across all 12 quality levels ($Q0\text{–}Q11$).
* **Cross-Decoder Compatibility (`TestCrossDecoderCompatibility`)**: Verified round-trip compatibility in both directions against the standard `github.com/andybalholm/brotli` package.
* **Continuous Native Fuzzing (`FuzzRoundTrip`)**: Runs a native Go fuzzing target testing absolute memory safety and layout robustness under extreme mutations.
* **Strict Bitstream Parity (`TestDirectBitstreamParity`)**: Ensures Q0 bitstream parity matching against the official Google C encoder.

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
