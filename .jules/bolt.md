## 2026-08-26 - Direct Array Lookup and Pre-calculated UTF-8 Tables for CP500/EBCDIC Performance

**Learning:** Replacing Go `map[rune]byte` lookups for ASCII/Latin-1 characters with direct `[256]byte` array indexing in string encoding loops eliminates map hashing overhead (~10-15ns per char), speeding up string conversion by ~3.3x. Pre-calculating UTF-8 byte representations for EBCDIC bytes eliminates `[]rune` slice heap allocations during string decoding (~2.5x speedup, 75% memory reduction).
**Action:** When working with fixed 256-byte character sets or byte-to-rune translation in Go, always prefer pre-computed array lookups `[256]byte` / `[256][2]byte` over maps or `[]rune` conversions.

## 2026-08-27 - Pre-allocated Static Descriptors for DRDA Parameter Encoding and Go Escape Analysis on Interface Calls

**Learning:** Returning slice literals `[]byte{...}` from functions like `FDODSC` forces Go compiler escape analysis to heap-allocate new slices on every call (2 extra allocs per parameter in `BuildSQLDTA`). Using package-level static `[]byte` vars eliminates these allocations. Conversely, attempting stack allocation for local byte arrays (`var stackBuf [64]byte`) when passing subslices to interface methods (e.g., `io.ReadFull(r, buf)`) causes Go escape analysis to move `stackBuf` to the heap (`moved to heap: stackBuf`), increasing memory usage compared to `make([]byte, len)`.
**Action:** Use pre-allocated package-level variables for constant slice returns. Avoid declaring stack arrays to pass to interface methods (`io.Reader`, `io.Writer`) as Go escape analysis will force them onto the heap.

## 2026-08-28 - Stack Array Buffer for String Formatting in Non-Interface Value Decoders

**Learning:** When generating formatted string outputs from raw bytes (such as `DecodePackedDecimal`), using `make([]byte, outLen)` allocates a heap byte slice before `string(out)` allocates the returned string (2 allocs total). Switching to a stack-allocated byte array `var stackOut [64]byte` when `outLen <= 64` keeps the slice buffer on the stack (0 heap allocs for `out`), reducing heap allocations from 2 to 1 and speeding up conversion by ~28% with 50% memory reduction.
**Action:** For string formatting functions that construct intermediate byte slices without passing them to `io.Reader`/`io.Writer` interfaces, use a local fixed stack array `var stack [64]byte` to eliminate intermediate heap allocations.
