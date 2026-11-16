# Contributing to `go-db2`

Thank you for contributing to `go-db2`! Before opening a pull request, please review the architectural invariants, local verification workflow, and security guidelines below.

## Architectural Invariants

1. **100% Pure Go (`CGO_ENABLED=0`) & Zero Runtime Dependencies**:
   - The root module (`go.mod`) must remain stdlib-only with zero external dependencies.
   - Do not introduce CGO, C wrapper libraries (such as IBM `clidriver`/ODBC), or `unsafe`.
2. **Internal Encapsulation**:
   - Keep low-level DRDA/DDM framing, codecs, authentication helpers, and SQL validation inside `internal/drda`, `internal/codec`, `internal/auth`, and `internal/validate`.
   - Never expose `internal/*` types in the exported signatures of `package db2`.
3. **`database/sql/driver` Compliance**:
   - Preserve `NamedValueChecker` delegation via `driver.ErrSkip` for standard scalar types and `driver.Valuer`.
   - Return structured `*db2.Error` values compatible with `errors.As` and sentinel errors compatible with `errors.Is`.
   - Keep `ColumnTypeScanType` and critical decoding paths low-allocation (verify with benchmarks).

## Development Workflow & Local Checks

Run the full validation suite from the repository root (`./go-db2`):

- **Format & Vet**:
  ```bash
  gofmt -s -w .
  test -z "$(gofmt -l .)"
  go vet ./...
  ```
- **Pure Go Build & Unit Tests (`CGO_ENABLED=0`)**:
  ```bash
  CGO_ENABLED=0 go build ./...
  CGO_ENABLED=0 go test -count=1 ./...
  ```
- **Race Detector & Testable Examples**:
  ```bash
  go test -v -race -timeout 5m -count=1 ./...
  go test -v -run '^Example' ./...
  ```
- **Lint & Vulnerability Scan**:
  ```bash
  golangci-lint run --timeout=5m
  govulncheck ./...
  ```
- **Fuzzing**:
  ```bash
  go test -run='^$' -fuzz=FuzzReadDSS -fuzztime=30s ./internal/drda
  go test -run='^$' -fuzz=FuzzParseDSN -fuzztime=30s .
  ```
  Additional `Fuzz*` targets live in `./internal/drda`, `./internal/codec`, `./internal/auth`, and `.`.
- **Benchmarks**:
  ```bash
  go test -bench=. -benchmem ./...
  ```
  For head-to-head live comparisons against `go_ibm_db`, run `./benchmarks/run_benchmark.sh` against a local IBM Db2 container.
- **Live Integration Demos & Tests**:
  The 15 integration demos under `examples/*/main.go` (and any `TestLive_*` tests) accept `DB2_DSN` pointing at a running IBM Db2 instance (see [README.md](README.md#testing)).

## Security Guidelines

- Always redact passwords and key material in `fmt.Stringer`, `fmt.GoStringer`, and DSN error messages (`redactDSNPassword`, `sanitizeLogString`).
- Enforce strict DSN parameter validation, TLS 1.2+ with server certificate verification (`InsecureSkipVerify: false`), and SQL identifier validation (`internal/validate`).
- Do **not** disclose security vulnerabilities in public GitHub issues. Follow the private reporting process in [SECURITY.md](SECURITY.md).
