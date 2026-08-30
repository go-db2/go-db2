# IBM Db2 Driver Benchmark & Conformance Report

> Generated at: 2026-08-30 02:33:16 UTC

Comparison between **`go-db2`** (Pure Go DRDA implementation) and **`go_ibm_db`** (IBM Official CGO / clidriver wrapper).

## 1. Semantic Conformance (Result Parity)

| Status | Test Scenario | `go-db2` (Pure Go) Output | `go_ibm_db` (CGO) Output |
| :---: | :--- | :--- | :--- |
| ✅ PASS | **Basic Scalar (Int, String, Float)** | `i=42, s="Pure Go Db2 Driver", f=123.45` | `i=42, s="Pure Go Db2 Driver", f=123.45` |
| ✅ PASS | **Null Handling (sql.Null*)** | `ni.Valid=false, ns.Valid=false` | `ni.Valid=false, ns.Valid=false` |
| ✅ PASS | **Temporal (Current Date Format)** | `2026-08-30` | `2026-08-30` |
| ✅ PASS | **Numeric Precision (Smallint, Int, Bigint)** | `s=32767, i=2147483647, b=922337203685...` | `s=32767, i=2147483647, b=922337203685...` |
| ✅ PASS | **High Precision Decimal & DECFLOAT** | `dec=1234567.89, decfloat=123456789012...` | `dec=1234567.89, decfloat=123456789012...` |
| ✅ PASS | **BLOB Binary Integrity (SHA-256)** | `326e228882b798cecdd5fb44fddc9a7f84301...` | `326e228882b798cecdd5fb44fddc9a7f84301...` |
| ✅ PASS | **CLOB Text Streaming** | `len=1200, prefix=Pure-Go-Db2-CLOB-Tes...` | `len=1200, prefix=Pure-Go-Db2-CLOB-Tes...` |
| ✅ PASS | **VARGRAPHIC International Strings** | `Unicode / 日本語 / Português` | `Unicode / 日本語 / Português` |

## 2. Performance & Resource Allocation

| Scenario | `go-db2` Avg Latency | `go_ibm_db` Avg Latency | Latency Ratio | `go-db2` Mem/Op | `go_ibm_db` Mem/Op | `go-db2` Allocs/Op | `go_ibm_db` Allocs/Op |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **Ping (Round-trip Latency)** | 6.715µs | 402ns | 16.70x slower | 521 B | 0 B | 18 | 0 |
| **Simple Single-Row SELECT** | 8.369µs | 259.841µs | **31.05x faster** 🚀 | 1323 B | 1907 B | 35 | 56 |
| **Fetch 1,000 Rows Scan** | 16.37µs | 5.34967ms | **326.80x faster** 🚀 | 1340 B | 97908 B | 34 | 8735 |
| **Prepared Statement INSERT** | 14.9µs | 18.741131ms | **1257.79x faster** 🚀 | 2424 B | 11470 B | 64 | 321 |
| **Read 500KB BLOB Payload** | 29.123µs | 1.943707ms | **66.74x faster** 🚀 | 1375 B | 1051515 B | 35 | 62 |

## 3. Key Observations

- **CGO Bridge Overhead**: `go_ibm_db` incurs a context switch overhead across the CGO boundary for every row fetch (`rows.Next()` / `rows.Scan()`).
- **Pure Go Memory Management**: `go-db2` uses stack buffers and pre-allocated slices, significantly lowering heap memory allocations.
- **Zero External C Dependencies**: `go-db2` compiles statically (`CGO_ENABLED=0`) across all operating systems and architectures.
