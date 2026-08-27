# Modernization & Rewrite Target Rankings

A prioritized roadmap of high-impact compression libraries in the **Go** and **Rust** ecosystems ripe for modernization or ground-up rewrites.

---

## Evaluation Metrics

| Metric | Description | Weight |
| :--- | :--- | :--- |
| **Ecosystem Leverage** | Total adoption across databases, web servers, message queues, and storage engines. | 30% |
| **Current Flaws / Debt** | Severity of C-to-Go/C-to-Rust porting debt, excessive heap allocations, or CGO/FFI reliance. | 25% |
| **SIMD Acceleration** | Potential throughput multiplier using Go 1.27 `simd` or Rust `std::simd`. | 25% |
| **Standalone Value** | Strategic advantage of a clean, zero-dependency ground-up implementation over legacy forks. | 20% |

---

## Ranked Targets

### 1. Pure Rust Zstandard (`zstd-rs`) — Score: 96/100
* **Target:** Create a high-performance, 100% safe, pure-Rust implementation of Zstandard (RFC 8878).
* **Current Ecosystem Problem:** The entire Rust ecosystem currently relies on the [`zstd`](https://crates.io/crates/zstd) crate, which wraps C `libzstd` via C FFI (`zstd-sys`). Existing pure-Rust attempts (like `rzstd`) are incomplete, unmaintained, or slow.
* **Why Ground-Up:**
  - Eliminates C toolchains, `cc` build dependencies, and C FFI overhead in Rust services.
  - Zstandard's FSE (Finite State Entropy) and Huff0 entropy coding map cleanly to Rust SIMD registers.
  - Massive demand across Parquet, Arrow, DataFusion, Turborepo, and embedded storage engines.

---

### 2. Modern Rust Brotli (`brotli-rs`) — Score: 92/100
* **Target:** Ground-up, safe, idiomatic Rust Brotli encoder & decoder (RFC 7932 + RFC 9841 ready).
* **Current Ecosystem Problem:** The dominant crate ([`dropbox/rust-brotli`](https://crates.io/crates/brotli)) is an automated C-to-Rust translation from 2016. It uses a virtual memory allocator layer (`AllocatedStackMemory`, `CustomAlloc`), thousands of unsafe C-style macros, and is notoriously difficult to read, compile, and maintain.
* **Why Ground-Up:**
  - Build an idiomatic safe Rust crate implementing standard `std::io::Read` and `std::io::Write`.
  - Leverage `std::simd` or `wide` for 25+ GB/s vector match finding (mirroring `nijaru/brotli`).
  - Zero-allocation block compression for async runtimes (Tokio / Actix / Hyper).
* **RFC 9841 (September 2025) Specification Roadmap:**
  - **v0.x Target:** Standard RFC 7932 Brotli baseline.
  - **Forward-Compatibility Design:** Avoid architectural assumptions that make RFC 9841 painful later:
    1. *Large Windows:* Design ring buffers and window types to accommodate up to 30-bit (1 GB) sliding windows rather than hardcoding 24-bit (16 MB) limits.
    2. *Shared Dictionaries:* Structure dictionary interfaces for arbitrary external pre-shared dictionaries.
    3. *Framing:* Maintain clean separation between the LZ77 entropy core and outer container framing/metadata blocks.

---

### 3. Modern Go Snappy (`snappy`) — Score: 88/100
* **Target:** Next-generation Go 1.27 SIMD-accelerated Snappy engine.
* **Current Ecosystem Problem:** [`golang/snappy`](https://github.com/golang/snappy) is maintained conservatively by the Go team and has not adopted modern vectorization, remaining on scalar 64-bit word loops.
* **Why Ground-Up / Standalone:**
  - Snappy's LZ77 format is intentionally lightweight and simple.
  - Applying Go 1.27 portable SIMD (`archsimd.Uint8x16`) and branchless offset tables will push throughput to **3.5–5.0 GB/s** in pure Go.
  - Critical infrastructure dependency for Prometheus, CockroachDB, LevelDB, BadgerDB, and gRPC.

---

### 4. Zero-Allocation Go LZ4 (`lz4`) — Score: 84/100
* **Target:** Ultra-lean, zero-allocation Go LZ4 block and frame compressor.
* **Current Ecosystem Problem:** [`pierrec/lz4`](https://github.com/pierrec/lz4) has undergone significant API churn (v1 through v4), carries heavy internal state structs, and allocates during streaming.
* **Why Ground-Up / Standalone:**
  - High demand in Kafka client drivers, ClickHouse Go connectors, and high-frequency message buses.
  - Provide zero-alloc block APIs (`Encode`/`Decode`) and Go 1.23+ range iterators (`Lines()`, `Chunks()`).
  - Hardware SIMD match finding for multi-gigabyte-per-second streaming.

---

### 5. Pure Go Portable SIMD Hashing (`xxhash` / `xxh3`) — Score: 78/100
* **Target:** Pure Go implementation of XXH3 / xxHash using Go 1.27 `simd` vector intrinsics.
* **Current Ecosystem Problem:** Existing top-speed Go hashing libraries ([`cespare/xxhash`](https://github.com/cespare/xxhash), `zeebo/xxh3`) rely heavily on hand-written Go assembly (`.s` files). Assembly is brittle, cannot be inlined by the Go compiler, and requires separate implementations per CPU architecture.
* **Why Ground-Up:**
  - Pure Go with `archsimd.Uint8x16` compiles down to native hardware vector instructions without `.s` assembly files.
  - Fully inlinable by the Go compiler SSA backend.

---

## Comparison Matrix

| Rank | Project | Language | Core Advantage | Strategy |
| :---: | :--- | :---: | :--- | :--- |
| **#1** | **Zstandard (`zstd`)** | Rust | Eliminates C FFI / `zstd-sys` across Rust | Ground-Up Clean Architecture |
| **#2** | **Brotli (`brotli`)** | Rust | Replaces Dropbox C-transpiled spaghetti with safe idiomatic Rust | Ground-Up Clean Architecture |
| **#3** | **Snappy (`snappy`)** | Go | 3–5 GB/s vector matching via Go 1.27 SIMD | Standalone / Fork Modernization |
| **#4** | **LZ4 (`lz4`)** | Go | Zero-alloc block API + Go 1.23 iterators for Kafka/ClickHouse | Ground-Up Clean Architecture |
| **#5** | **XXH3 / Fast Hash** | Go | Pure Go SIMD replacing fragile `.s` assembly | Ground-Up Clean Architecture |
