# Sentinel Security Journal

## 2026-03-30 - Legacy Encryption Algorithm (DES) Protocol Mandate in SECMEC 9
**Vulnerability:** Static analysis highlighted the use of weak cryptographic algorithm DES (`des.NewCipher`) in DRDA SECMEC 9 password encryption (`network/security/secmec9.go`).
**Learning:** IBM DRDA Level 5 wire specification mandates DES-CBC with Diffie-Hellman key exchange specifically for SECMEC 9 (USRENCPWD). Replacing the cipher would break wire protocol compatibility with legacy IBM Db2 database servers negotiating SECMEC 9.
**Prevention:** Retain DES for wire specification compliance in SECMEC 9 with `#nosec G401,G502` linter annotations and doc comments, while encouraging the use of TLS 1.2+ (`ssl=true`) or Kerberos/GSSAPI (SECMEC 7/11) in production environments.

## 2026-03-30 - Slice Index Bounds Protection in Wire Decoders
**Vulnerability:** Zero-length boolean parameter scale metadata (`ps`) caused `DecodeField` to allocate a zero-length slice and access `buf[len(buf)-1]` (`buf[-1]`), triggering an unhandled runtime panic and application crash. Additionally, `DecodePackedDecimal` used a fixed-size 64-byte stack array, panicking when decoding packed decimal payloads exceeding 32 bytes (>64 digits).
**Learning:** Binary protocol decoders handling length fields from remote network frames or parameter metadata must always validate length boundaries before slice index operations and avoid fixed-size arrays for variable-length payload buffers.
**Prevention:** Always perform explicit slice length validation (`len(buf) == 0`) before negative/end-relative indexing and use dynamic slice allocations (`make([]byte, len(b)*2)`) when parsing wire protocol structures.
