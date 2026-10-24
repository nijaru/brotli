# brotli

Pure-Go Brotli compression and decompression. It is a fork of
[`github.com/andybalholm/brotli`](https://github.com/andybalholm/brotli) with a
block API, bounded decoding, reusable pools, large windows, and HTTP helpers.

## Compatibility

The stable upstream streaming API is drop-in compatible: `NewReader`,
`NewWriter`, `NewWriterLevel`, `NewWriterOptions`, `WriterOptions`,
`HTTPCompressor`, and `HTTPCompressorWithLevel` keep their signatures.
`WriterOptions` matches upstream exactly (`Quality`, `LGWin`). Use
`WriterConfig` for custom dictionaries and RFC 9841 large windows.

`HTTPCompressor` and `HTTPCompressorWithLevel` preserve upstream behavior,
including its `Accept-Encoding` parsing and identity fallback. Use
`HTTPCompressorWithError` for RFC-correct negotiation, correct `Vary`
handling, and a 406 error when no encoding is acceptable.

Upstream's experimental matchfinder API (`NewWriterV2`, `Encoder`,
`FastEncoder`) is not ported.

## Install

```sh
go get github.com/nijaru/brotli
```

Requires Go 1.26 or later.

## Usage

### Streaming

```go
var buf bytes.Buffer

w := brotli.NewWriter(&buf)
if _, err := io.WriteString(w, "some text to compress\n"); err != nil {
    log.Fatal(err)
}
if err := w.Close(); err != nil {
    log.Fatal(err)
}

r := brotli.NewReader(&buf)
if _, err := io.Copy(os.Stdout, r); err != nil {
    log.Fatal(err)
}
```

Writers and readers can be reset and reused with `Reset`.

### Block API

Pass a destination slice to reuse its capacity across calls.

```go
src := []byte("Hello, Brotli block compression.")

var encBuf []byte
compressed := brotli.Encode(encBuf, src, brotli.DefaultCompression)

var decBuf []byte
decompressed, err := brotli.Decode(decBuf, compressed)
if err != nil {
    log.Fatal(err)
}
fmt.Println(string(decompressed))
```

### Bounded decoding

`DecodeLimit` and `NewReaderLimit` cap decoded output. Use
`DecodeWithLimits` or `NewReaderWithLimits` to also reject streams that request
a larger window than the caller allows.

```go
const (
    maxOutput     = 16 << 20
    maxWindowBits = 20
)

decoded, err := brotli.DecodeWithLimits(nil, compressed, maxOutput, maxWindowBits)
if errors.Is(err, brotli.ErrOutputLimitExceeded) {
    // Decoded output exceeded maxOutput.
}
if errors.Is(err, brotli.ErrWindowBitsLimitExceeded) {
    // The stream requests a window larger than maxWindowBits.
}

r, err := brotli.NewReaderWithLimits(bytes.NewReader(compressed), maxOutput, maxWindowBits)
```

### Writer configuration

Use `WriterConfig` for the first-party extensions:

```go
w := brotli.NewWriterConfig(dst, brotli.WriterConfig{
    Quality:     brotli.DefaultCompression,
    LGWin:       26,
    LargeWindow: true,
})
```

`CustomDict` can be set the same way.

### HTTP

```go
w, err := brotli.HTTPCompressorWithError(rw, req, brotli.DefaultCompression)
if errors.Is(err, brotli.ErrNoAcceptableEncoding) {
    http.Error(rw, "no acceptable content encoding", http.StatusNotAcceptable)
    return
}
defer w.Close()
```

`HTTPCompressor` and `HTTPCompressorWithLevel` remain the upstream-compatible
variants.

### Pools and iterators

For repeated streams, `WriterPool`, `ReaderPool`, `GetWriter`, and `GetReader`
reduce allocation overhead. `Reader.Lines` and `Reader.Chunks` provide range
iterators over decoded output.

## Testing

```sh
go test ./...
go test -race ./...
```

The differential C tests run when the `brotli` CLI is installed. Fuzz targets
include `FuzzDecode`, `FuzzDecodeLimit`, and `FuzzRoundTrip`.

## Performance

The block API avoids allocations when the destination capacity is sufficient.
Streaming writers reuse their internal state after `Reset`; at qualities 0-9
the reset benchmarks report zero allocations per operation. Run
`go test -bench . -benchmem` for current numbers on your machine.

## License

MIT
