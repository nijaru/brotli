# brotli

A high-performance pure Go implementation of the Brotli compression format (RFC 7932) for Go 1.26+.

Originally derived from Google Brotli and `andybalholm/brotli`, this library has been modernized into a clean, modular architecture. It maintains full drop-in compatibility with the original streaming API while adding zero-allocation block operations, Go 1.23+ range iterators, and concurrency-safe resource pooling.

---

## Features

- **Classic Streaming API**: 100% drop-in replacement for `github.com/andybalholm/brotli` and standard library `compress/*` patterns.
- **Zero-Allocation Block API**: In-memory `Encode` and `Decode` that operate directly on reusable buffer capacity without intermediate allocations.
- **Go 1.23+ Range Iterators**: Stream decompression directly in `for...range` loops using `Lines()` and `Chunks()`.
- **Low Memory Footprint**: 0 heap allocations in steady-state streaming across Q0–Q8, and 99.998% fewer allocations in Q10–Q11 Zopfli modes.
- **Pre-Shared Custom Dictionaries**: Sliding window seeding via `WriterOptions.CustomDict` and `Reader.SetCustomDictionary()`.
- **HTTP Compression Middleware**: Thread-safe pooled HTTP compressor for web services and APIs.

---

## Installation

```bash
go get github.com/nijaru/brotli
```

---

## Usage Examples

### 1. Standard Streaming (Drop-In Replacement)

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

	// Compress
	w := brotli.NewWriter(&compressed)
	if _, err := w.Write([]byte("Brotli compression in pure Go.")); err != nil {
		log.Fatal(err)
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}

	// Decompress
	r := brotli.NewReader(&compressed)
	decompressed, err := io.ReadAll(r)
	if err != nil {
		log.Fatal(err)
	}

	println(string(decompressed))
}
```

### 2. Zero-Allocation Block API (Snappy / Zstd Style)

For in-memory payloads, reusing destination slice capacity executes with zero heap allocations:

```go
// Compress block (DefaultCompression is 6; quality ranges from 0 to 11)
compressed := brotli.Encode(nil, payload, brotli.DefaultCompression)

// Decompress block into pre-allocated slice capacity
decompressed, err := brotli.Decode(dst[:0], compressed)
if err != nil {
	log.Fatal(err)
}
```

### 3. Go 1.23+ Range Iterators

Decompress text line-by-line or byte-by-byte in standard `for...range` loops:

```go
// Stream lines
for line, err := range reader.Lines() {
	if err != nil {
		log.Fatal(err)
	}
	processLine(line)
}

// Zero-allocation chunk iterator (yields up to 8KB per step)
for chunk, err := range reader.Chunks(8192) {
	if err != nil {
		log.Fatal(err)
	}
	processChunk(chunk) // Valid only within the loop iteration
}
```

### 4. Pooled Streaming

For high-throughput services, use package-level pools to eliminate allocation churn:

```go
w := brotli.GetWriter(destination)
w.Write(payload)
w.Close()
brotli.PutWriter(w) // Return writer after Close
```

---

## Performance (Apple M3 Max)

| Quality Level | Throughput | Allocations (Stream Reset) | Memory Footprint |
| :--- | :--- | :--- | :--- |
| **Q0 (Fast)** | **282 MB/s** | **0 allocs/op** | ~2.3 KB |
| **Q1** | **200 MB/s** | **0 allocs/op** | Hasher table reuse |
| **Q6 (Default)** | **39 MB/s** | **0 allocs/op** | ~160 KB (20–25% lower than upstream) |
| **Q9 (High)** | **18 MB/s** | 1 alloc/op | ~1.2 MB |
| **Q10 (Zopfli)** | 1.1 MB/s | **61 allocs/op** | 5.9 MB (-80% vs baseline) |
| **Q11 (Max)** | 0.8 MB/s | **63 allocs/op** | 6.8 MB (-92% vs baseline) |
| **Decompression** | **220–346 MB/s** | **0 allocs/op** | Direct-to-slice decoding |

---

## Verification & Parity

Correctness and interoperability are continuously tested against the canonical specifications:

- **C Reference Parity:** Verified byte-for-byte against the Google Brotli C encoder (`brotli -q 0 -w 22`) and cross-tested across levels Q0–Q11.
- **Cross-Decoder Round-Trip:** Bi-directional round-trip validation with `andybalholm/brotli`.
- **Fuzzing & Robustness:** Native Go fuzzing harness (`FuzzDecode`) and mutation testing to ensure corrupted or adversarial bitstreams fail gracefully without panics.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
