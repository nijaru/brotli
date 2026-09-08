# brotli

A high-performance, pure Go implementation of the Brotli compression format (RFC 7932 & RFC 9841).

`nijaru/brotli` is a drop-in replacement for the classic streaming API of `andybalholm/brotli`, with a familiar `io`-based interface in the style of standard library `compress/*` packages. It provides competitive throughput (faster at Q11, equivalent at Q0/Q1/Q6/Q9; upstream's Q10 encoder is faster), up to ~90% fewer allocated bytes per stream in Zopfli modes, zero-allocation in-memory block compression with reused buffers, and native Go 1.23+ range iterators.

---

## Features

- **Drop-In Replacement (Classic Streaming API)**
  - Swap existing `andybalholm/brotli` imports with zero code changes for the classic streaming API (`NewWriter`/`NewWriterLevel`/`NewWriterOptions`/`NewReader`). Upstream's experimental `matchfinder`-based API (`NewWriterV2`, `Encoder`, `FastEncoder`) is not ported.
- **Zero-Allocation Block API**
  - In-memory `Encode` and `Decode` operate directly on caller-reused slices with 0 allocs/op at Q0–Q9 and no stream wrapping overhead (verified by `BenchmarkBlockEncode`/`BenchmarkBlockDecode`; Zopfli levels Q10–Q11 allocate per-op — see table).
- **RFC 9841 Large Window & Multi-Stream Framing**
  - Supports sliding windows up to 30 bits (~1 GB) and transparent multi-member concatenated stream decoding.
- **~90% Fewer Allocated Bytes (Zopfli Modes)**
  - Q11 reset-stream encoding allocates ~4 MB/op vs ~40.6 MB/op for `andybalholm/brotli` (see benchmark conditions below). An earlier internal baseline of 84 MB/op refers to this repo's pre-optimization code, not upstream.
- **Go 1.27 SIMD Vector Acceleration**
  - ~2.5x faster match-length kernel on Go 1.27+ (26.4 GB/s vs 10.5 GB/s scalar at 1024B identical input; microbenchmark — end-to-end effect is small, e.g. Q0 +2–3%) with transparent fallback on Go 1.26.
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

### 1. In-Memory Block Compression (Reusable Buffers)

For compressing and decompressing byte slices directly in memory. Reuse `dst` across calls for 0 allocs/op at Q0–Q9 (`BenchmarkBlockEncode`/`BenchmarkBlockDecode`):

```go
// Compress (Quality: 0 = Fastest, 6 = Default, 11 = Best)
var encBuf []byte // reused across calls; first call sizes it
compressed := brotli.Encode(encBuf, payload, brotli.DefaultCompression)

// Decompress into reused slice
var decBuf []byte
decompressed, err := brotli.Decode(decBuf, compressed)
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
for line, err := range r.Lines() {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(line)
}

// Read fixed-size zero-alloc byte chunks
for chunk, err := range r.Chunks(8192) {
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
| **Decompress (block API)** | 234–348 MB/s (varies by quality; Q6-compressed ~343–348) | **0 allocs** (reused `dst`) |

**Benchmark conditions.** Apple M3 Max, Go 1.27.1, 553 KB `testdata/Isaac.Newton-Opticks.txt` corpus, `GOEXPERIMENT=simd` build. Allocation figures are `B/op` and `allocs/op` from `go test -benchmem` on `BenchmarkEncodeLevelsReset` / `BenchmarkBlockDecode` (upstream comparison at `andybalholm/brotli` `v1.2.3`, latest as of 2026-09-08; Q11/Q6 numbers identical to `v1.2.1`/`v1.2.2`; per-op allocation counts fluctuate by one when a bucketed ~24–48 KB hash table allocation lands inside vs outside the timed loop). `B/op` is cumulative bytes allocated per operation — not peak heap or RAM usage, which these benchmarks do not measure. Two related measurements often quoted alongside this table:

- **Q6 fresh construction:** ~25% fewer allocated bytes than upstream (11.5 vs 15.3 MB/op, `BenchmarkEncodeLevels`); steady-state resets are 0-alloc in both libraries.
- **Q11 84 MB → 4 MB:** an internal before/after comparison of this repo's own Zopfli match-buffer optimization (`BackwardMatch` 16B→8B packing), not an upstream comparison.

---

## Compatibility & Verification

- **Google C Brotli Interop:** Strict byte-identity vs the reference C encoder is enforced at Q0 on reference vectors (`TestDirectBitstreamParity`, requires the `brotli` CLI; CI installs it). Encode-identity is intentionally *not* asserted at higher levels — C, upstream, and this library all emit different-but-valid bitstreams there (verified: Q1 Opticks prefix compresses to three different sizes across the three encoders). What is enforced at all levels Q0–Q11: bidirectional decode-acceptance with the C CLI (`TestDifferentialCBinary`) — our decoder reads C output, C reads ours.
- **Cross-Decoder Compatibility:** Full bi-directional round-trip compatibility tested with `andybalholm/brotli` across all levels (`TestCrossDecoderCompatibility`, always runs in CI).
- **Fuzzing & Safety:** Validated with native Go fuzzing (`FuzzDecode`, `FuzzRoundTrip`); CI runs a bounded 60s fuzz on every push (see `ci.yml`). Corrupted or adversarial bitstreams must fail cleanly without panics.

---

## Acknowledgements

- [Google Brotli](https://github.com/google/brotli) for the original Brotli compression specification and canonical C reference implementation (RFC 7932).
- [Andy Balholm](https://github.com/andybalholm/brotli) for the foundational pure Go implementation.

---

## License

[MIT License](LICENSE). See [LICENSE](LICENSE) for details.
