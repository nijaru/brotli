# brotli

Pure Go Brotli compression (RFC 7932 and RFC 9841).

A fork of `andybalholm/brotli` with the same streaming API, so existing code switches over untouched. A Q11 encode allocates about 4 MB per op here versus about 40 MB upstream, at roughly the same speed (upstream's Q10 is faster). New in this fork: a zero-allocation block API, custom dictionaries, large windows, range iterators, and HTTP middleware.

---

## Features

- **Drop-in replacement.** Same `NewWriter`, `NewWriterLevel`, `NewWriterOptions`, and `NewReader` API as `andybalholm/brotli`. Upstream's newer experimental API (`NewWriterV2`, `Encoder`, `FastEncoder`, `matchfinder`) is not included.
- **Zero-allocation block API.** `Encode` and `Decode` work directly on slices you reuse, with no per-op allocations at Q0-Q9. The Zopfli levels Q10-Q11 still allocate per op; see the table below.
- **Large windows and concatenated streams (RFC 9841).** Sliding windows up to 30 bits (about 1 GB), and multi-member concatenated streams decode without any extra work.
- **Less allocation in Zopfli modes.** A reset Q11 encoder allocates about 4 MB per op here versus about 40 MB upstream.
- **SIMD match finding on Go 1.27.** The match-length kernel runs about 2.5x faster under Go 1.27 (26 GB/s versus 10 GB/s scalar on a microbenchmark). The real-world effect is small, a couple percent at Q0. Go 1.26 builds use a scalar fallback.
- **Range iterators.** Decompress inside `for...range` loops with `Lines()` and `Chunks()`. Needs Go 1.23+.
- **Custom dictionaries.** Seed the compressor with a pre-shared dictionary. Useful for small payloads like JSON or RPC bodies. Upstream doesn't support these.
- **HTTP middleware.** Pooled middleware for serving Brotli from web services and reverse proxies.

---

## Installation

```bash
go get github.com/nijaru/brotli
```

---

## Quickstart

### 1. In-memory block compression

For byte slices directly in memory. Reuse `dst` across calls and there are no per-op allocations at Q0-Q9:

```go
// Compress (Quality: 0 = Fastest, 6 = Default, 11 = Best)
var encBuf []byte // reused across calls; first call sizes it
compressed := brotli.Encode(encBuf, payload, brotli.DefaultCompression)

// Decompress into a reused slice
var decBuf []byte
decompressed, err := brotli.Decode(decBuf, compressed)
if err != nil {
    log.Fatal(err)
}
```

### 2. Standard streaming

If you've used `compress/flate` or `compress/gzip`, this looks familiar:

```go
// Compress a stream
w := brotli.NewWriter(dst)
w.Write(data)
w.Close()

// Decompress a stream
r := brotli.NewReader(src)
io.Copy(dst, r)
```

### 3. Range iterators

```go
// Line by line
for line, err := range r.Lines() {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(line)
}

// Fixed-size chunks, no per-chunk allocation
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
| **Q1** | **204 MB/s** | **0 allocs** |
| **Q6 (Default)** | **40 MB/s** | **0 allocs** |
| **Q9 (High)** | **19 MB/s** | **0 allocs** |
| **Q10 (Zopfli)** | 1.16 MB/s | ~62 allocs (~6.7 MB) |
| **Q11 (Max)** | 0.84 MB/s | ~50 allocs (~4-6 MB, ~90% fewer allocated bytes than upstream) |
| **Decompress (block API)** | 234-348 MB/s (varies by quality; Q6-compressed ~343-348) | **0 allocs** (reused `dst`) |

These numbers come from an Apple M3 Max, Go 1.27.1, SIMD build, compressing the 553 KB `testdata/Isaac.Newton-Opticks.txt` file. The allocation columns are `B/op` and `allocs/op` from `go test -benchmem` (`BenchmarkEncodeLevelsReset` and `BenchmarkBlockDecode`). Upstream means `andybalholm/brotli` v1.2.3.

Two caveats. `B/op` counts every byte allocated during the operation, so it is not peak heap or RAM use. And per-op alloc counts can wobble by one, because a bucketed 24-48 KB hash table allocation sometimes lands inside the timed loop and sometimes outside it.

Two related measurements:

- A freshly constructed (never reset) Q6 encoder allocates 11.5 MB/op here versus 15.3 MB/op upstream, about 25% less (`BenchmarkEncodeLevels`). Once reset and reused, both allocate nothing.
- The "84 MB down to 4 MB" Q11 improvement was this repo's own before/after for packing the Zopfli match buffer (`BackwardMatch` from 16 bytes to 8), not a comparison against upstream.

---

## Compatibility

- **Against the C library.** Output is byte-identical to the reference C encoder at Q0 on the tested vectors (`TestDirectBitstreamParity`; needs the `brotli` binary, which CI installs). Higher levels are deliberately not compared byte-for-byte: C, upstream, and this library all emit different but equally valid bitstreams there, so the tests check decode compatibility instead. At every level Q0-Q11, our decoder reads C's output and C reads ours (`TestDifferentialCBinary`).
- **Against upstream.** Round-trips both directions with `andybalholm/brotli` at every level, on every CI run (`TestCrossDecoderCompatibility`).
- **Fuzzing.** `FuzzDecode` and `FuzzRoundTrip` run in CI, 60 seconds each on every push. Corrupt input has to fail cleanly, never panic.

---

## Acknowledgements

- [Google Brotli](https://github.com/google/brotli) for the original specification and the reference C implementation (RFC 7932).
- [Andy Balholm](https://github.com/andybalholm/brotli) for the pure Go implementation this builds on.

---

## License

Licensed under the [MIT License](LICENSE).
