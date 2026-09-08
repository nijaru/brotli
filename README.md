# brotli

This package is a brotli compressor and decompressor implemented in Go.

It is a fork of `andybalholm/brotli` (itself translated from the reference C implementation at https://github.com/google/brotli). The streaming API is unchanged, so it can replace the upstream package without code changes. It is optimized for low allocation, and adds a block API, custom dictionaries, large-window support, range iterators, and HTTP middleware.

## Installation

```sh
go get github.com/nijaru/brotli
```

Requires Go 1.26 or later.

## Usage

Block API. Pass a `dst` slice to reuse across calls.

```go
src := []byte("Hello, Brotli block compression.")

var encBuf []byte // reused across calls; first call sizes it
compressed := brotli.Encode(encBuf, src, brotli.DefaultCompression)

var decBuf []byte
decompressed, err := brotli.Decode(decBuf, compressed)
if err != nil {
    log.Fatal(err)
}
fmt.Println(string(decompressed))
```

Streaming API. This matches the upstream package.

```go
var b bytes.Buffer

bw := brotli.NewWriter(&b)
if _, err := io.WriteString(bw, "some text to compress\n"); err != nil {
    log.Fatal(err)
}
if err := bw.Close(); err != nil {
    log.Fatal(err)
}

br := brotli.NewReader(&b)
if _, err := io.Copy(os.Stdout, br); err != nil {
    log.Fatal(err)
}
```

Both types can be reset and reused. Call `bw.Reset(&b)` for the next stream and `br.Reset(&b)` before decoding it.

Range iterators over decoded output:

```go
r := brotli.NewReader(bytes.NewReader(compressed))

for line, err := range r.Lines() {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(line)
}

for chunk, err := range r.Chunks(4096) {
    if err != nil {
        log.Fatal(err)
    }
    _ = chunk // up to 4096 decompressed bytes
}
```

Other entry points: `NewWriterLevel`, `NewWriterOptions` with `WriterOptions{Quality, LGWin, CustomDict, LargeWindow}`, `NewWriterPool`, `GetReader`, `GetWriter`, and `HTTPCompressor`.

## Features

* Pure Go, no cgo.
* Same `NewWriter`, `NewWriterLevel`, `NewWriterOptions`, `NewReader`, `Reader`, `Writer`, and `WriterOptions` names as upstream.
* Block API: `Encode(dst, src, quality)` and `Decode(dst, src)` on caller reused slices, with 0 allocs per op at quality 0-9.
* Pre-shared custom dictionaries.
* RFC 9841 large windows, up to 30 bits (about 1 GB).
* Multi-member concatenated stream decoding.
* Range iterators: `Reader.Lines()` and `Reader.Chunks(n)`.
* Pooled HTTP middleware that picks brotli, gzip, or none from `Accept-Encoding`.
* SIMD match length kernel under Go 1.27 with scalar fallback on older Go.

Upstream's experimental matchfinder API (`NewWriterV2`, `Encoder`, `FastEncoder`) is not ported.

## Performance

Measured with the streaming reset benchmarks (`BenchmarkEncodeLevelsReset`) except where noted:

| Op | Throughput | Allocs | Bytes per op |
| --- | --- | --- | --- |
| Q0 encode | 290 MB/s | 0 | 0 |
| Q1 encode | 204 MB/s | 0 | 0 |
| Q6 encode | 40 MB/s | 0 | 0 |
| Q9 encode | 19 MB/s | 0 | 0 |
| Q10 encode | 1.16 MB/s | about 62 | about 6.7 MB |
| Q11 encode | 0.84 MB/s | about 50 | 4-6 MB |
| Block decode (`BenchmarkBlockDecode`, reused dst) | 234-348 MB/s | 0 | 0 |

Conditions: Apple M3 Max, Go 1.27.1, `GOEXPERIMENT=simd` build, 553 KB `testdata/Isaac.Newton-Opticks.txt` corpus. B/op is cumulative bytes allocated per op, not peak heap or RAM.

A freshly constructed (never reset) Q6 encoder allocates 11.5 MB/op (`BenchmarkEncodeLevels`). Once reset and reused, encoders at Q0-Q9 allocate nothing.

The SIMD match length kernel runs about 2.5x faster than scalar on its microbenchmark (`BenchmarkFindMatchLength`, 26 against 10 GB/s). The end to end effect is small, a couple percent at Q0.

## Compatibility and testing

Output is byte identical to the reference C encoder at Q0 on tested vectors (`TestDirectBitstreamParity`, needs the `brotli` CLI). At higher levels, C, upstream, and this library emit different but valid bytes, so tests check decode compatibility in both directions at every level Q0-Q11 (`TestDifferentialCBinary`). Round trips both ways with `andybalholm/brotli` pass at every level (`TestCrossDecoderCompatibility`). Fuzz targets `FuzzDecode` and `FuzzRoundTrip` run in CI, 60 seconds each per push.

## Acknowledgements

Google for the Brotli spec and the C reference encoder and decoder. Andy Balholm for the Go implementation this builds on.

## License

Licensed under the [MIT License](LICENSE).
