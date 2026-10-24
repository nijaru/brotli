package brotli

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
)

// ErrNoAcceptableEncoding is returned by HTTPCompressorWithError when the
// request rejects identity and none of the supported content encodings are
// acceptable.
var ErrNoAcceptableEncoding = errors.New("brotli: no acceptable content encoding")

// HTTPCompressor chooses a compression method (brotli, gzip, or none) based on
// the Accept-Encoding header, sets the Content-Encoding header, and returns a
// WriteCloser that implements that compression. The Close method must be called
// before the current HTTP handler returns. Callers that need to reject requests
// with 406 Not Acceptable should use HTTPCompressorWithError instead.
func HTTPCompressor(w http.ResponseWriter, r *http.Request) io.WriteCloser {
	return HTTPCompressorWithLevel(w, r, 4)
}

type pooledWriterWrapper struct {
	w    *Writer
	pool *WriterPool
}

func (pw *pooledWriterWrapper) Write(p []byte) (n int, err error) {
	if pw.w == nil {
		return 0, errWriterClosed
	}
	return pw.w.Write(p)
}

func (pw *pooledWriterWrapper) Close() error {
	if pw.w == nil {
		return nil
	}
	err := pw.w.Close()
	pw.pool.Put(pw.w)
	pw.w = nil
	return err
}

// HTTPCompressorWithLevel is the upstream-compatible compressor. It preserves
// upstream's Accept-Encoding parsing, identity selection, and Vary behavior.
func HTTPCompressorWithLevel(w http.ResponseWriter, r *http.Request, level int) io.WriteCloser {
	if w.Header().Get("Vary") == "" {
		w.Header().Set("Vary", "Accept-Encoding")
	}

	wc, ok := compressorForEncoding(w, level, negotiateContentEncodingUpstream(r, []string{"br", "gzip"}))
	if !ok {
		return nopCloser{w}
	}
	return wc
}

// HTTPCompressorWithError is the strict, RFC-correct form of
// HTTPCompressorWithLevel. It returns ErrNoAcceptableEncoding when the request
// rejects identity and none of the supported content encodings are acceptable.
// Callers should respond with 406 Not Acceptable in that case.
func HTTPCompressorWithError(w http.ResponseWriter, r *http.Request, level int) (io.WriteCloser, error) {
	addVaryAcceptEncoding(w)

	wc, ok := compressorForEncoding(w, level, negotiateContentEncoding(r, []string{"br", "gzip"}))
	if !ok {
		return nil, ErrNoAcceptableEncoding
	}
	return wc, nil
}

func compressorForEncoding(w http.ResponseWriter, level int, encoding string) (io.WriteCloser, bool) {
	switch encoding {
	case "br":
		w.Header().Set("Content-Encoding", "br")
		pool := getWriterPool(level)
		return &pooledWriterWrapper{
			w:    pool.Get(w),
			pool: pool,
		}, true
	case "gzip":
		w.Header().Set("Content-Encoding", "gzip")
		gw, err := gzip.NewWriterLevel(w, gzipLevelForBrotliLevel(level))
		if err != nil {
			return gzip.NewWriter(w), true
		}
		return gw, true
	case "identity":
		return nopCloser{w}, true
	default:
		return nil, false
	}
}

func addVaryAcceptEncoding(w http.ResponseWriter) {
	vary := w.Header().Get("Vary")
	if vary == "" {
		w.Header().Set("Vary", "Accept-Encoding")
	} else if !strings.Contains(strings.ToLower(vary), "accept-encoding") {
		w.Header().Add("Vary", "Accept-Encoding")
	}
}

func gzipLevelForBrotliLevel(level int) int {
	switch {
	case level <= BestSpeed:
		return gzip.BestSpeed
	case level >= 9:
		return gzip.BestCompression
	default:
		return level
	}
}

// negotiateContentEncodingUpstream mirrors the upstream helper's selection
// algorithm for the upstream-compatible HTTP functions.
func negotiateContentEncodingUpstream(r *http.Request, offers []string) string {
	bestOffer := "identity"
	bestQ := -1.0
	specs := parseAcceptUpstream(r.Header, "Accept-Encoding")
	for _, offer := range offers {
		for _, spec := range specs {
			if spec.Q > bestQ && (spec.Value == "*" || spec.Value == offer) {
				bestQ = spec.Q
				bestOffer = offer
			}
		}
	}
	if bestQ == 0 {
		bestOffer = ""
	}
	return bestOffer
}

// negotiateContentEncoding returns the best offered content encoding for the
// request's Accept-Encoding header. If two offers match with equal weight and
// then the offer earlier in the list is preferred. If no offers are
// acceptable, then "" is returned.
func negotiateContentEncoding(r *http.Request, offers []string) string {
	specs := parseAccept(r.Header, "Accept-Encoding")
	if len(specs) == 0 {
		return "identity"
	}

	// quality returns the best explicit q-value for coding, falling back to a
	// wildcard. Explicit entries win over the wildcard by RFC 9110.
	quality := func(coding string) (float64, bool) {
		q := -1.0
		wildcard := -1.0
		for _, spec := range specs {
			switch spec.Value {
			case coding:
				if spec.Q > q {
					q = spec.Q
				}
			case "*":
				if spec.Q > wildcard {
					wildcard = spec.Q
				}
			}
		}
		if q >= 0 {
			return q, true
		}
		if wildcard >= 0 {
			return wildcard, true
		}
		return 0, false
	}

	bestOffer := ""
	bestQ := -1.0
	for _, offer := range offers {
		q, ok := quality(offer)
		if !ok || q <= 0 {
			continue
		}
		// Keep the first offer on ties between offered codings.
		if q > bestQ {
			bestOffer, bestQ = offer, q
		}
	}

	// Identity is the fallback when no offered coding is acceptable. An
	// explicit identity q-value can override an offer or exclude it.
	identityQ, identityKnown := quality("identity")
	identityExplicit := false
	for _, spec := range specs {
		if spec.Value == "identity" {
			identityExplicit = true
			break
		}
	}
	if identityExplicit && identityQ > bestQ {
		bestOffer, bestQ = "identity", identityQ
	}
	if bestOffer == "" || bestQ <= 0 {
		if identityKnown && identityQ <= 0 {
			return ""
		}
		return "identity"
	}

	return bestOffer
}

// acceptSpec describes an Accept* header.
type acceptSpec struct {
	Value string
	Q     float64
}

// parseAcceptUpstream mirrors the upstream parser for compatibility.
func parseAcceptUpstream(header http.Header, key string) (specs []acceptSpec) {
loop:
	for _, s := range header[key] {
		for {
			var spec acceptSpec
			spec.Value, s = expectTokenSlash(s)
			if spec.Value == "" {
				continue loop
			}
			spec.Q = 1.0
			s = skipSpace(s)
			if strings.HasPrefix(s, ";") {
				s = skipSpace(s[1:])
				if !strings.HasPrefix(s, "q=") {
					continue loop
				}
				spec.Q, s = expectQualityUpstream(s[2:])
				if spec.Q < 0.0 {
					continue loop
				}
			}
			specs = append(specs, spec)
			s = skipSpace(s)
			if !strings.HasPrefix(s, ",") {
				continue loop
			}
			s = skipSpace(s[1:])
		}
	}
	return
}

// parseAccept parses Accept* headers.
func parseAccept(header http.Header, key string) (specs []acceptSpec) {
	for _, line := range header[key] {
		s := line
		for {
			s = skipSpace(s)
			if s == "" {
				break
			}

			value, rest := expectTokenSlash(s)
			if value == "" {
				s = skipToNextAcceptElement(s)
				continue
			}

			spec := acceptSpec{Value: strings.ToLower(value), Q: 1.0}
			s = skipSpace(rest)
			if strings.HasPrefix(s, ";") {
				s = skipSpace(s[1:])
				if !strings.HasPrefix(s, "q=") {
					s = skipToNextAcceptElement(s)
					continue
				}
				qualityText := s[2:]
				spec.Q, s = expectQuality(qualityText)
				if spec.Q < 0.0 {
					s = skipToNextAcceptElement(qualityText)
					continue
				}
			}

			specs = append(specs, spec)
			s = skipSpace(s)
			if !strings.HasPrefix(s, ",") {
				break
			}
			s = skipSpace(s[1:])
		}
	}
	return
}

// skipToNextAcceptElement discards the current malformed element and returns
// the remainder after its comma, if any.
func skipToNextAcceptElement(s string) string {
	if comma := strings.IndexByte(s, ','); comma >= 0 {
		return s[comma+1:]
	}
	return ""
}

func skipSpace(s string) (rest string) {
	i := 0
	for ; i < len(s); i++ {
		if octetTypes[s[i]]&isSpace == 0 {
			break
		}
	}
	return s[i:]
}

func expectTokenSlash(s string) (token, rest string) {
	i := 0
	for ; i < len(s); i++ {
		b := s[i]
		if (octetTypes[b]&isToken == 0) && b != '/' {
			break
		}
	}
	return s[:i], s[i:]
}

func expectQualityUpstream(s string) (q float64, rest string) {
	switch {
	case len(s) == 0:
		return -1, ""
	case s[0] == '0':
		q = 0
	case s[0] == '1':
		q = 1
	default:
		return -1, ""
	}
	s = s[1:]
	if !strings.HasPrefix(s, ".") {
		return q, s
	}
	s = s[1:]
	i := 0
	n := 0
	d := 1
	for ; i < len(s); i++ {
		b := s[i]
		if b < '0' || b > '9' {
			break
		}
		n = n*10 + int(b) - '0'
		d *= 10
	}
	return q + float64(n)/float64(d), s[i:]
}

func expectQuality(s string) (q float64, rest string) {
	switch {
	case len(s) == 0:
		return -1, ""
	case s[0] == '0':
		q = 0
	case s[0] == '1':
		q = 1
	default:
		return -1, ""
	}
	s = s[1:]
	if !strings.HasPrefix(s, ".") {
		if len(s) != 0 && !isQualityTerminator(s[0]) {
			return -1, ""
		}
		return q, s
	}
	s = s[1:]
	i := 0
	n := 0
	d := 1
	for ; i < len(s) && i < 3; i++ {
		b := s[i]
		if b < '0' || b > '9' {
			break
		}
		n = n*10 + int(b) - '0'
		d *= 10
	}
	if i < len(s) && s[i] >= '0' && s[i] <= '9' {
		return -1, ""
	}
	if i < len(s) && !isQualityTerminator(s[i]) {
		return -1, ""
	}
	q += float64(n) / float64(d)
	if q > 1 {
		return -1, ""
	}
	return q, s[i:]
}

func isQualityTerminator(b byte) bool {
	return b == ',' || b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// Octet types from RFC 2616.
var octetTypes [256]octetType

type octetType byte

const (
	isToken octetType = 1 << iota
	isSpace
)

func init() {
	// OCTET      = <any 8-bit sequence of data>
	// CHAR       = <any US-ASCII character (octets 0 - 127)>
	// CTL        = <any US-ASCII control character (octets 0 - 31) and DEL (127)>
	// CR         = <US-ASCII CR, carriage return (13)>
	// LF         = <US-ASCII LF, linefeed (10)>
	// SP         = <US-ASCII SP, space (32)>
	// HT         = <US-ASCII HT, horizontal-tab (9)>
	// <">        = <US-ASCII double-quote mark (34)>
	// CRLF       = CR LF
	// LWS        = [CRLF] 1*( SP | HT )
	// TEXT       = <any OCTET except CTLs, but including LWS>
	// separators = "(" | ")" | "<" | ">" | "@" | "," | ";" | ":" | "\" | <">
	//              | "/" | "[" | "]" | "?" | "=" | "{" | "}" | SP | HT
	// token      = 1*<any CHAR except CTLs or separators>
	// qdtext     = <any TEXT except <">>

	for c := 0; c < 256; c++ {
		var t octetType
		isCtl := c <= 31 || c == 127
		isChar := 0 <= c && c <= 127
		isSeparator := strings.ContainsRune(" \t\"(),/:;<=>?@[]\\{}", rune(c))
		if strings.ContainsRune(" \t\r\n", rune(c)) {
			t |= isSpace
		}
		if isChar && !isCtl && !isSeparator {
			t |= isToken
		}
		octetTypes[c] = t
	}
}
