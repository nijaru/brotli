# brotli

A high-performance, pure Go implementation of the Brotli compression format (RFC 7932).

`nijaru/brotli` is a drop-in replacement for `andybalholm/brotli` and standard library `compress/*` packages. It provides faster throughput, up to 95% lower memory footprint, zero-allocation in-memory block compression, and native Go 1.23+ range iterators.

---

## Features

- **100% Drop-In Replacement**
  - Swap existing `andybalholm/brotli` imports with zero code changes.
- **Zero-Allocation Block API**
  - In-memory `Encode` and `Decode` operate directly on reusable slices without stream wrapping overhead.
- **95% Lower Memory Footprint**
  - Reduces peak heap memory in archival Zopfli modes from 84 MB down to 3.8 MB.
- **Go 1.27 SIMD Vector Acceleration**
  - Up to 3x faster vector match finding on Go 1.27+ with transparent fallback on Go 1.26.
- **Go 1.23+ Range Iterators**
  - Stream decompression directly inside `for...range` loops using `Lines()` and `Chunks()`.
- **Pre-Shared Custom Dictionaries**
  - Seed compression with pre-shared dictionaries for small payloads (JSON, RPCs).
- **HTTP Compression Middleware**
  - Thread-safe pooled middleware for web services and reverse proxies.

---

## Installation

```bash
go get github.com/nijaru/brotli
```

---

## Quickstart

### 1. In-Memory Block Compression (Zero-Allocation)

For compressing and decompressing byte slices directly in memory:

```go
// Compress (Quality: 0 = Fastest, 6 = Default, 11 = Best)
compressed := brotli.Encode(nil, payload, brotli.DefaultCompression)

// Decompress into pre-allocated slice
decompressed, err := brotli.Decode(nil, compressed)
if err != nil {
    log.Fatal(err)
}
```

### 2. Standard Streaming (Drop-In Replacement)

```go
// Compress a stream
w := brotli.NewWriter(dst)
w.Write(data)
w.Close()

// Decompress a stream
r := brotli.NewReader(src)
io.Copy(dst, r)
```

### 3. Go 1.23+ Range Iterators

Stream lines or byte chunks directly inside `for...range` loops:

```go
// Read line by line
for line, err := range reader.Lines() {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(line)
}

// Read fixed-size zero-alloc byte chunks
for chunk, err := range reader.Chunks(8192) {
    if err != nil {
        log.Fatal(err)
    }
    process(chunk)
}
```

---

## Performance (Apple M3 Max)

| Quality Level | Throughput | Stream Reset Memory |
| :--- | :--- | :--- |
| **Q0 (Fastest)** | **290 MB/s** | **0 allocs** (~2.3 KB) |
| **Q1** | **204 MB/s** | **0 allocs** (Hasher reuse) |
| **Q6 (Default)** | **40 MB/s** | **0 allocs** (25% less RAM than upstream) |
| **Q9 (High)** | **19 MB/s** | 1 alloc (~1.2 MB) |
| **Q10 (Zopfli)** | 1.16 MB/s | **61 allocs** (5.9 MB) |
| **Q11 (Max)** | 0.84 MB/s | **63 allocs** (**3.8 MB** — *95% lower memory vs baseline*) |
| **Decompress** | **310–346 MB/s** | **0 allocs** |

---

## Compatibility & Verification

- **Google C Brotli Parity:** Verified byte-for-byte against canonical Google C Brotli across all quality levels (Q0–Q11).
- **Cross-Decoder Compatibility:** Full bi-directional round-trip compatibility tested with `andybalholm/brotli`.
- **Fuzzing & Safety:** Continuously validated with native Go fuzzing (`FuzzDecode`) to ensure corrupted or adversarial bitstreams fail cleanly without panics.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
