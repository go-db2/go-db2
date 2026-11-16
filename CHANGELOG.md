# Changelog

All notable changes to `go-db2` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-16

### Added
- 100% Pure Go (`CGO_ENABLED=0`, zero external dependencies in root `go.mod`) DRDA/DDM wire engine implementing Go's `database/sql/driver` (`Driver`, `Connector`, `NewConnector`, `Conn`, `Stmt`, `Tx`, `Result`).
- URL (`db2://` and `db2s://`) and Key-Value `ParseDSN` parsers with strict parameter validation and `conn_timeout` alias support.
- On-demand cursor streaming (`QueryStream`) over DRDA `OPNQRY`, `CNTQRY`, and `CLSQRY` with multi-block row spanning.
- Stored procedure support for `IN`, `OUT`, and `INOUT` parameters (`sql.Out` via `SQLDTARD`) and multiple dynamic result sets (`driver.RowsNextResultSet`).
- Dedicated DRDA section bitmap manager (`sectionManager`, 512-bit bitmap supporting up to 499 concurrent prepared statements in sections `2..500`, with section `1` reserved for direct commands).
- LOB streaming (`BLOB`, `CLOB`, `DBCLOB`, `io.Reader`) via DRDA `EXTDTA` with DoS size guards (`MaxLOBStreamSize`, `MaxDSSPayloadSize`, `MaxReplyChainSize`).
- Data type codecs for `DECFLOAT(16/34)`, `DECIMAL`, `GRAPHIC`/`VARGRAPHIC`/`DBCLOB`, `DATE`/`TIME`/`TIMESTAMP` (including timezone variants), `XML`, and `BOOLEAN`.
- Atomic transactional batch DML execution (`ExecBatchWithParams`) using typed parameter slices with automatic rollback on failure.
- Context-aware session identity and telemetry APIs (`WithUser`, `SwitchUser`, `WithClientInfo`, `SetClientInfo`, `CURRENT CLIENT_CORR_TOKEN`) and database management helpers (`CreateDB`, `DropDB`, `ExecAdminCmd`).
- Native Go fuzz targets (`Fuzz*`) across `internal/drda`, `internal/codec`, `internal/auth`, and `ParseDSN`, plus `pkg.go.dev` testable examples in `example_test.go`.
- Project governance and contributor docs: `CONTRIBUTING.md`, `CHANGELOG.md`, and `.github/dependabot.yml`.

### Changed
- Encapsulated protocol, codec, authentication, and validation internals under `internal/drda`, `internal/codec`, `internal/auth`, and `internal/validate`, exposing only `package db2` publicly.
- Standardized structured `*db2.Error` (`SQLCode`, `SQLState`, `Message`, `Unwrap`) and sentinel errors (`ErrConnectionClosed`, `ErrDatabaseNotFound`, `ErrAuthenticationFailed`, `ErrInvalidConnectionStr`).
- Updated `CheckNamedValue` to delegate standard scalar types and `driver.Valuer` via `driver.ErrSkip` and normalize `[16]byte` / fixed byte arrays; made `ColumnTypeScanType` zero-allocation.
- Updated `Result.LastInsertId()` to return an explicit error directing callers to `SELECT ... FROM FINAL TABLE (INSERT ...)`.
- Migrated `.golangci.yml` to schema `version: "2"` and aligned `README.md`, `SECURITY.md`, and `doc.go` with runtime behavior.

### Fixed
- `Ping` inside active transactions uses `EXCSAT` instead of `RDBCMM` to avoid committing open work.
- Explicit DSN ports are preserved when `ssl=true` is set.
- Fixed credential leakage in `parseURL` error paths when `!hasStructuralPwd`.
- Added defensive `scale` clamping (`[-1024, 1024]`) and `outLen < 0` guard in `DecodePackedDecimal`.
- Added `context.Context` propagation to `CreateDB` and `DropDB`, fixed error propagation in `benchmarks/suite/perf.go`, and added `DB2_DSN` support to `examples/client_accounting_demo/main.go`.

### Security
- Enforced TLS 1.2+ floor with strict certificate verification and defensive `Config.TLSConfig` cloning.
- Enforced `SECMEC 9` anti-downgrade and `SECTKN` validation during `ACCSEC`/`SECCHK` negotiation.
- Blocked plaintext password authentication (`SECMEC 3`) over unencrypted TCP unless `allow_plaintext_auth=true` is explicitly set.
- Implemented credential redaction (`String()`, `GoString()`, `redactDSNPassword`) and log-injection control-character sanitization (`sanitizeLogString`).
- Disabled runtime Kerberos `ccache`/`keytab` ticket exchange (`SECMEC 7/11`) until a compliant RFC 4120 `AP-REQ` subpackage is integrated.
- Hardened CI with `permissions: contents: read`, strict `govulncheck`, `CGO_ENABLED=0` verification, and ephemeral masked integration credentials.

### Removed
- Removed internal `.jules/` notes (`bolt.md`, `sentinel.md`) from the public `./go-db2` repository.
