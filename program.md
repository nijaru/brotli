# Brotli Autoresearch Optimization

This repo runs autonomous experiments against a fixed benchmark harness to optimize Brotli compression performance.

## Setup

Work with the user to:
1. Agree on a run tag and create a fresh branch for the run.
2. Read the in-scope files.
3. Initialize the untracked `results.tsv` log if it does not exist.
4. Confirm setup before starting the loop.

## In-Scope Files

- **Immutable files**: `brotli_test.go`, `flate/flate_test.go`, `program.md`
- **Mutable files**: `*.go` excluding `*_test.go`
- **Optional docs**: `AGENTS.md`, `ai/STATUS.md`

## Experimentation Rules

You may:
- edit only the mutable files
- run the approved experiment command
- record results in the untracked log

You may not:
- modify the benchmark harness
- modify data prep
- add external dependencies

## Goal

Optimize the core `BenchmarkEncodeLevels` metric.

Comparison rule:
- `MB/s` (higher is better)
- `allocs/op` (lower is better)

Time budget:
- Fast incremental benchmarks (~10-20 seconds per run)
- Timeout: 1 hour total per session.

Complexity rule:
- prefer simpler wins (e.g., better buffer management, less allocations)
- revert tiny gains that add brittle complexity

## Output Extraction

Run experiments and extract the target metric:

```bash
go test -bench '^BenchmarkEncodeLevels$' -benchtime=1s -benchmem . | grep -E '^BenchmarkEncodeLevels' > run.log
```

If extraction fails, inspect the tail of the log:

```bash
tail -n 10 run.log
```

## Results Log

Keep an untracked tab-separated log (`results.tsv`) with a schema like:

```tsv
commit	mb_s	allocs_op	status	description
```

Statuses:
- `keep`
- `discard`
- `crash`

## Loop

1. Check current branch and commit.
2. Make one experimental change (e.g., inline a fast path, remove an allocation, test SIMD).
3. Commit the change.
4. Run the experiment.
5. Extract the metric.
6. Log the result.
7. Keep the commit only if it improves the metric enough to justify the complexity.
8. Revert failed or non-improving runs (`git reset --hard HEAD~1`).
9. Continue until the user stops the run.
