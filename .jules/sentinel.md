# Sentinel Security Journal

## 2026-03-30 - Slice Index Bounds Protection in Wire Decoders
**Vulnerability:** Zero-length boolean parameter scale metadata (`ps`) caused `DecodeField` to allocate a zero-length slice and access `buf[len(buf)-1]` (`buf[-1]`), triggering an unhandled runtime panic and application crash. Additionally, `DecodePackedDecimal` used a fixed-size 64-byte stack array, panicking when decoding packed decimal payloads exceeding 32 bytes (>64 digits).
**Learning:** Binary protocol decoders handling length fields from remote network frames or parameter metadata must always validate length boundaries before slice index operations and avoid fixed-size arrays for variable-length payload buffers.
**Prevention:** Always perform explicit slice length validation (`len(buf) == 0`) before negative/end-relative indexing and use dynamic slice allocations (`make([]byte, len(b)*2)`) when parsing wire protocol structures.
