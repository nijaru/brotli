# brotli

A high-performance, pure Go implementation of the Brotli compression format (RFC 7932 & RFC 9841).

`nijaru/brotli` is a drop-in replacement for `andybalholm/brotli` and standard library `compress/*` packages. It provides faster throughput, up to ~90% fewer allocated bytes per stream in Zopfli modes, zero-allocation in-memory block compression, and native Go 1.23+ range iterators.

---

## Features

- **100% Drop-In Replacement**
  - Swap existing `andybalholm/brotli` imports with zero code changes.
- **Zero-Allocation Block API**
  - In-memory `Encode` and `Decode` operate directly on reusable slices without stream wrapping overhead.
- **RFC 9841 Large Window & Multi-Stream Framing**
  - Supports sliding windows up to 30 bits (~1 GB) and transparent multi-member concatenated stream decoding.
- **~90% Fewer Allocated Bytes (Zopfli Modes)**
  - Q11 reset-stream encoding allocates ~4 MB/op vs ~40.6 MB/op for `andybalholm/brotli` (see benchmark conditions below). An earlier internal baseline of 84 MB/op refers to this repo's pre-optimization code, not upstream.
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

| Quality Level | Throughput | Reset-Stream Allocations |
| :--- | :--- | :--- |
| **Q0 (Fastest)** | **290 MB/s** | **0 allocs** |
| **Q1** | **204 MB/s** | **0 allocs** (hasher reuse) |
| **Q6 (Default)** | **40 MB/s** | **0 allocs** |
| **Q9 (High)** | **19 MB/s** | **0 allocs** |
| **Q10 (Zopfli)** | 1.16 MB/s | ~62 allocs (~6.7 MB) |
| **Q11 (Max)** | 0.84 MB/s | ~50 allocs (~4–6 MB, ~90% fewer allocated bytes than upstream) |
| **Decompress** | **310–346 MB/s** | **0 allocs** |

**Benchmark conditions.** Apple M3 Max, Go 1.27.1, 553 KB `testdata/Isaac.Newton-Opticks.txt` corpus. Allocation figures are `B/op` and `allocs/op` from `go test -benchmem` on `BenchmarkEncodeLevelsReset` (upstream comparison at `andybalholm/brotli` commit `0675b24`, v1.2.0+21). `B/op` is cumulative bytes allocated per operation — not peak heap or RAM usage, which these benchmarks do not measure. Two related measurements often quoted alongside this table:

- **Q6 fresh construction:** ~25% fewer allocated bytes than upstream (11.5 vs 15.3 MB/op, `BenchmarkEncodeLevels`); steady-state resets are 0-alloc in both libraries.
- **Q11 84 MB → 4 MB:** an internal before/after comparison of this repo's own Zopfli match-buffer optimization (`BackwardMatch` 16B→8B packing), not an upstream comparison.

---

## Compatibility & Verification

- **Google C Brotli Parity:** Verified byte-for-byte against canonical Google C Brotli across all quality levels (Q0–Q11).
- **Cross-Decoder Compatibility:** Full bi-directional round-trip compatibility tested with `andybalholm/brotli`.
- **Fuzzing & Safety:** Continuously validated with native Go fuzzing (`FuzzDecode`) to ensure corrupted or adversarial bitstreams fail cleanly without panics.

---

## Acknowledgements

- [Google Brotli](https://github.com/google/brotli) for the original Brotli compression specification and canonical C reference implementation (RFC 7932).
- [Andy Balholm](https://github.com/andybalholm/brotli) for the foundational pure Go implementation.

---

## License

MIT License. See [LICENSE](LICENSE) for details.
