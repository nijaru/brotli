#!/bin/bash
set -euo pipefail

OUR_DIR="/Users/nick/github/nijaru/brotli"
UPSTREAM_DIR="/Users/nick/github/andybalholm/brotli"
CROSS_DIR="/tmp/crosscheck"

LEVELS="${1:-0 5 9 10 11}"
COUNT="${COUNT:-6}"

# Build bench pattern
PATTERN=""
for lvl in $LEVELS; do
	PATTERN="${PATTERN:+$PATTERN|}BenchmarkEncodeLevels/${lvl}$|BenchmarkDecodeLevels/${lvl}$|BenchmarkEncodeLevelsReset/${lvl}$"
done

OUTDIR=$(mktemp -d)
trap "rm -rf $OUTDIR" EXIT

# Extract benchmark lines only (benchstat needs clean input)
extract() { grep '^Benchmark' "$1" || true; }

# Run benchmarks
cd "$UPSTREAM_DIR"
GOWORK=off go test -bench="$PATTERN" -benchmem -count="$COUNT" -run='^$' ./... >"$OUTDIR/upstream_raw.txt" 2>&1
extract "$OUTDIR/upstream_raw.txt" >"$OUTDIR/upstream.txt"

cd "$OUR_DIR"
GOWORK=off go test -bench="$PATTERN" -benchmem -count="$COUNT" -run='^$' ./... >"$OUTDIR/ours_raw.txt" 2>&1
extract "$OUTDIR/ours_raw.txt" >"$OUTDIR/ours.txt"

# Cross-compatibility
cd "$CROSS_DIR"
if GOWORK=off go test -count=1 ./... >"$OUTDIR/cross.txt" 2>&1; then
	CROSS_OK=true
else
	CROSS_OK=false
fi

# Compare
echo "=== upstream vs ours ==="
benchstat "$OUTDIR/upstream.txt" "$OUTDIR/ours.txt"

echo ""
if $CROSS_OK; then
	echo "Cross-compatibility: PASS"
else
	echo "Cross-compatibility: FAIL"
	cat "$OUTDIR/cross.txt"
fi
