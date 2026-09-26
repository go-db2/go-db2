# Sentinel Security Journal

## 2026-03-30 - Legacy Encryption Algorithm (DES) Protocol Mandate in SECMEC 9
**Vulnerability:** Static analysis highlighted the use of weak cryptographic algorithm DES (`des.NewCipher`) in DRDA SECMEC 9 password encryption (`network/security/secmec9.go`).
**Learning:** IBM DRDA Level 5 wire specification mandates DES-CBC with Diffie-Hellman key exchange specifically for SECMEC 9 (USRENCPWD). Replacing the cipher would break wire protocol compatibility with legacy IBM Db2 database servers negotiating SECMEC 9.
**Prevention:** Retain DES for wire specification compliance in SECMEC 9 with `#nosec G401,G502` linter annotations and doc comments, while encouraging the use of TLS 1.2+ (`ssl=true`) or Kerberos/GSSAPI (SECMEC 7/11) in production environments.

## 2026-03-30 - Slice Index Bounds Protection in Wire Decoders
**Vulnerability:** Zero-length boolean parameter scale metadata (`ps`) caused `DecodeField` to allocate a zero-length slice and access `buf[len(buf)-1]` (`buf[-1]`), triggering an unhandled runtime panic and application crash. Additionally, `DecodePackedDecimal` used a fixed-size 64-byte stack array, panicking when decoding packed decimal payloads exceeding 32 bytes (>64 digits).
**Prevention:** Always perform explicit slice length validation (`len(buf) == 0`) before negative/end-relative indexing and use dynamic slice allocations (`make([]byte, len(b)*2)`) when parsing wire protocol structures.

## 2026-09-19 - Context Identity & Auditing Isolation in Prepared Statements
**Vulnerability:** Multi-tenant identity transitions (`WithUser`) and client audit registers (`WithClientInfo`) were only applied in direct query executions (`Conn.ExecContext`, `Conn.QueryContext`), but were bypassed in prepared statements (`Conn.PrepareContext`, `Stmt.ExecContext`, `Stmt.QueryContext`), allowing pooled prepared statements to execute under stale user privileges and without audit registers.
**Learning:** `database/sql` directs prepared statement invocations directly to `driver.Stmt` methods rather than `driver.Conn`, so context metadata handlers must be applied across both `driver.Conn` and `driver.Stmt` execution paths to prevent multi-tenant boundary escapes.
**Prevention:** Centralize context metadata resolution into an `applyContextMetadata(ctx)` helper and invoke it at all statement entry points (`PrepareContext`, `execContextLocked`, `queryContextLocked`, `ExecContext`, `QueryContext`).

## 2026-09-20 - Unpropagated Client Correlation Token Register in Session Audit Context
**Vulnerability:** `SetClientInfo` stored `ClientInfo.CorrelationToken` in local session config but failed to execute `SET CLIENT CORR_TOKEN` on the Db2 server, leaving database security audit logs without distributed tracing correlation tokens.
**Learning:** Db2 special registers require explicit `SET CLIENT CORR_TOKEN` (or `SET CLIENT PROGRAMID` fallback) execution via `EXCSQLSET` to persist correlation tokens into Db2 server-side session registers (`CURRENT CLIENT_CORR_TOKEN`).
**Prevention:** Ensure all `ClientInfo` struct attributes (`ApplicationName`, `WorkstationName`, `UserID`, `Accounting`, and `CorrelationToken`) map directly to corresponding server-side special register SQL statements in `SetClientInfo`.

## 2026-09-21 - Multi-Group FDODSC Descriptor Parsing in Wire Decoders
**Vulnerability:** `ParseSQLDTARD` evaluated only the first FDODSC descriptor group, truncating output parameter definitions when procedures returned > 84 parameters across multiple descriptor chunks, and was vulnerable to out-of-bounds slicing on malformed payload lengths.
**Learning:** DRDA wire protocol chunks parameter descriptors into multiple triplet groups of up to 84 parameters each. Parser loops must iterate over all descriptor groups in sequence while enforcing strict `groupLen` bounds checks.
**Prevention:** Always loop over descriptor blocks sequentially with `groupLen` boundary checks (`pos + groupLen <= len(buf)` and `groupLen >= 3`) before parsing field descriptors.

## 2026-09-22 - Session State & Switched Identity Pollution in Connection Pooling
**Vulnerability:** `ResetSession` in `Conn` did not restore initial connection user identity, client audit info, or auto-commit mode when pooled connections were returned to `database/sql` connection pool, allowing subsequent tenant requests assigned the same pooled connection to execute under stale user privileges and audit registers.
**Learning:** In connection pooling, any session-level mutation (switched user identity, audit registers, auto-commit mode) persists across requests unless `ResetSession` explicitly resets session user, client info registers, and auto-commit back to the initial connection configuration (`Config`).
**Prevention:** Always implement `ResetSession` in `driver.SessionResetter` to restore base connection configuration (`c.cfg.User`, client audit registers, and auto-commit mode) before returning connections to the pool, or return `driver.ErrBadConn` to discard contaminated connections if restoration fails.
