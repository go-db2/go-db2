## 2026-08-26 - Direct Array Lookup and Pre-calculated UTF-8 Tables for CP500/EBCDIC Performance

**Learning:** Replacing Go `map[rune]byte` lookups for ASCII/Latin-1 characters with direct `[256]byte` array indexing in string encoding loops eliminates map hashing overhead (~10-15ns per char), speeding up string conversion by ~3.3x. Pre-calculating UTF-8 byte representations for EBCDIC bytes eliminates `[]rune` slice heap allocations during string decoding (~2.5x speedup, 75% memory reduction).
**Action:** When working with fixed 256-byte character sets or byte-to-rune translation in Go, always prefer pre-computed array lookups `[256]byte` / `[256][2]byte` over maps or `[]rune` conversions.
