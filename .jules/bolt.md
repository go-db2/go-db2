## 2026-08-26 - Direct Array Lookup and Pre-calculated UTF-8 Tables for CP500/EBCDIC Performance

**Learning:** Replacing Go `map[rune]byte` lookups for ASCII/Latin-1 characters with direct `[256]byte` array indexing in string encoding loops eliminates map hashing overhead (~10-15ns per char), speeding up string conversion by ~3.3x. Pre-calculating UTF-8 byte representations for EBCDIC bytes eliminates `[]rune` slice heap allocations during string decoding (~2.5x speedup, 75% memory reduction).
**Action:** When working with fixed 256-byte character sets or byte-to-rune translation in Go, always prefer pre-computed array lookups `[256]byte` / `[256][2]byte` over maps or `[]rune` conversions.

## 2026-08-27 - Pre-allocated Static Descriptors for DRDA Parameter Encoding and Go Escape Analysis on Interface Calls

**Learning:** Returning slice literals `[]byte{...}` from functions like `FDODSC` forces Go compiler escape analysis to heap-allocate new slices on every call (2 extra allocs per parameter in `BuildSQLDTA`). Using package-level static `[]byte` vars eliminates these allocations. Conversely, attempting stack allocation for local byte arrays (`var stackBuf [64]byte`) when passing subslices to interface methods (e.g., `io.ReadFull(r, buf)`) causes Go escape analysis to move `stackBuf` to the heap (`moved to heap: stackBuf`), increasing memory usage compared to `make([]byte, len)`.
**Action:** Use pre-allocated package-level variables for constant slice returns. Avoid declaring stack arrays to pass to interface methods (`io.Reader`, `io.Writer`) as Go escape analysis will force them onto the heap.

## 2026-08-28 - Single Buffer Allocation for Fixed-Header Protocol DDM Packing

**Learning:** Allocating a payload slice separately and then passing it to a wrapper function like `PackDDMObject` causes double memory allocations and slice copies. Allocating a single `make([]byte, 4+payloadLen)` buffer and writing the DDM header (`Length` + `CodePoint`) directly into `buf[0:4]` eliminates an entire allocation and slice copy, speeding up DDM packing by ~30% and reducing memory allocations by 50%.
**Action:** When constructing binary protocol structures with fixed-length headers, allocate the full buffer including the header upfront rather than packing payload and header in separate allocation steps.

## 2026-08-28 - Stack Array Buffer for String Formatting in Non-Interface Value Decoders

**Learning:** When generating formatted string outputs from raw bytes (such as `DecodePackedDecimal`), using `make([]byte, outLen)` allocates a heap byte slice before `string(out)` allocates the returned string (2 allocs total). Switching to a stack-allocated byte array `var stackOut [64]byte` when `outLen <= 64` keeps the slice buffer on the stack (0 heap allocs for `out`), reducing heap allocations from 2 to 1 and speeding up conversion by ~28% with 50% memory reduction.
**Action:** For string formatting functions that construct intermediate byte slices without passing them to `io.Reader`/`io.Writer` interfaces, use a local fixed stack array `var stack [64]byte` to eliminate intermediate heap allocations.

## 2026-09-19 - Single-Buffer Allocation and Static Byte Descriptors for DRDA Command Packet Framing

**Learning:** Composing DRDA command objects by nesting helper calls (`PackPKGNAMCSN` -> `PackBytes` -> `append` -> `PackDDMObject`) causes 3 to 4 independent heap allocations and multiple intermediate memory copies per SQL query/statement execution. Calculating total frame size upfront and writing the outer command header, embedded `PKGNAMCSN`, and invariant parameter descriptors (e.g. `paramRDBCMTOK`, `paramRTNSQLDA`, `paramTYPSQLDA`, `paramQRYCLSIMP`) directly into a single allocated buffer reduces allocations from 3-4 down to 1 alloc/op, cuts memory consumption by ~74-77% (from 304-416 B/op to 80-96 B/op), and increases packing throughput by 2.2x-2.7x.
**Action:** When building nested DRDA protocol commands where inner components have deterministically calculable sizes and static trailing options, assemble them directly into a single contiguous buffer rather than using slice concatenation and intermediate wrapping.

## 2026-09-21 - Type Assert io.ByteReader to Bypass Escape Analysis on Single-Byte Reader Slices

**Learning:** Passing a local stack array slice (`var nullIndicator [1]byte; io.ReadFull(r, nullIndicator[:])`) forces Go's escape analysis to allocate `nullIndicator` on the heap whenever `r` is passed as an `io.Reader` interface parameter. Type asserting `r` to `io.ByteReader` and calling `br.ReadByte()` when reading 1-byte headers or indicators bypasses passing slice references through `io.Reader`, eliminating 1 heap allocation per nullable field decode and speeding up field decoding by ~17%.
**Action:** When reading single bytes from an `io.Reader` interface in hot execution loops, check if the reader implements `io.ByteReader` and call `ReadByte()` directly instead of `io.ReadFull(r, slice[:])`.

## 2026-09-20 - Direct Nibble Bitwise Operations for Packed Decimal Parameter Encoding

**Learning:** Encoding packed decimal parameters by formatting strings (`fmt.Sprint`), splitting (`strings.Split`), padding (`strings.Repeat`), and hex decoding (`hex.DecodeString`) introduces 5+ heap allocations per parameter. Calculating BCD nibble positions and bitwise shifting (`<< 4` / `|=`) directly into a single pre-allocated byte slice (`make([]byte, 1+byteLen)`) cuts execution time by 55% (278 ns vs 618 ns) and reduces heap allocations from 5 to 2 (or 1 for strings).
**Action:** When encoding BCD/packed decimal binary wire formats, avoid intermediate string/hex manipulation routines and directly populate nibbles into a pre-allocated byte buffer using bitwise operations.

## 2026-09-22 - Single-Buffer Direct Assembly for SQLDTA Parameter Blocks

**Learning:** Constructing SQLDTA parameter blocks using `bytes.Buffer` and returning temporary byte slices from functions like `FDODTA` and `FDODSC` introduces multiple heap allocations per parameter. Calculating `FDODSC` length upfront and appending parameter wire descriptors and payloads directly into a single pre-allocated contiguous buffer (`sqldta`) using helper functions (`appendFDODTA`, `appendFDODSC`) eliminates per-parameter heap allocations. This reduces allocations by 80% (10 -> 2 allocs/op), cuts memory consumption by 55% (408 -> 184 B/op), and increases `BuildSQLDTA` throughput by 40%.
**Action:** When assembling composite binary wire objects with static header layouts and dynamic parameter lists, construct the entire payload directly in a single pre-allocated slice using `append` helpers rather than using `bytes.Buffer` or returning intermediate `[]byte` slices.

## 2026-09-23 - Zero-Allocation Scalar Byte-Reader Helpers and Inlined Case Stack Buffers for DRDA Field Decoding

**Learning:** Returning subslice references from helper functions like `readBuffer(r, ln, stackBuf)` causes Go escape analysis to force `stackBuf` onto the heap because the returned slice escapes the helper frame. Inlining local stack buffers (`var stackBuf [64]byte`) directly within each switch case in `DecodeField` allows short string and numeric payloads to be read into stack memory without heap allocations. Furthermore, replacing `io.ReadFull(r, buf[:])` for fixed multi-byte primitives (16-bit, 32-bit, 64-bit integers and floats) with scalar helper functions (`read2Bytes`, `read4Bytes`, `read8Bytes`) that call `io.ByteReader.ReadByte()` returns values directly by value without passing stack slice references into interface methods. This speeds up field decoding by 37.6% (305.5 ns -> 190.6 ns) and reduces allocations per decoded column by 40% (5 -> 3 allocs/op).
**Action:** When decoding binary protocol primitive types or short strings from an `io.Reader`, use scalar helper functions returning primitive values by value via `io.ByteReader`, and inline stack slice buffers directly in caller functions rather than passing stack slice references into subslice-returning helper functions.
