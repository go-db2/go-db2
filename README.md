<p align="center">
  <img src="assets/logo.png" alt="go-db2 Logo" width="480" />
</p>

# go-db2

> Pure Go database driver for **IBM Db2**, fully compatible with Go's standard [`database/sql`](https://pkg.go.dev/database/sql).

[![Go Reference](https://pkg.go.dev/badge/github.com/go-db2/go-db2.svg)](https://pkg.go.dev/github.com/go-db2/go-db2)
[![CI/CD Pipeline](https://github.com/go-db2/go-db2/actions/workflows/ci.yml/badge.svg)](https://github.com/go-db2/go-db2/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## Overview

`go-db2` is a high-performance, open-source database driver written **100% in Pure Go** to connect Go applications directly to **IBM Db2** over TCP/TLS using the native **DRDA** (*Distributed Relational Database Architecture*) and **DDM** (*Distributed Data Management*) wire protocols.

> **Disclaimer:** `go-db2` is an independent, community-driven open-source project. It is **not** maintained, endorsed, sponsored, or certified by International Business Machines Corporation (IBM), and it is **not** the official IBM Db2 driver for Go. IBM and Db2 are registered trademarks of International Business Machines Corporation.

### Why `go-db2`?
- **100% Pure Go (`CGO_ENABLED=0`)**: Zero CGO, zero `unsafe`, and zero external C library dependencies. No need to install IBM's heavy `clidriver` or ODBC components.
- **Trivial Cross-Compilation & Portability**: Build small, static binaries for Linux (x86_64, ARM64), macOS (including Apple Silicon M1/M2/M3/M4), Windows, and lightweight container base images (`scratch`, `distroless`, Alpine Linux).
- **Standard `database/sql` & Ecosystem Ready**: Native support for Go's connection pooling, explicit transactions with ACID isolation, contexts/timeouts/cancellations, and seamless compatibility with ORMs and tools such as `sqlx`, `GORM`, `sqlc`, and `golang-migrate`.
- **Low-Allocation Wire Engine**: Stack buffers, pre-computed EBCDIC CP500 and UTF-16BE lookup tables, zero-alloc `ColumnTypeScanType`, and configurable DRDA `QRYBLKSZ` streaming minimize GC pressure.

---

## Feature Matrix

| Feature | Status | Description |
| :--- | :---: | :--- |
| **Pure Go Engine** | Supported | 100% Go implementation (`CGO_ENABLED=0`, zero external dependencies) with native DRDA/DDM binary protocol parser |
| **Authentication & Handshake** | Supported | SECMEC 3 (Plain, requires TLS or `allow_plaintext_auth=true`), SECMEC 4 (User ID only), SECMEC 9 (Diffie-Hellman + DES), EBCDIC CP500 exchange |
| **TLS/SSL & mTLS Encryption** | Supported | Native TLS 1.2+ (`crypto/tls`) with strict verification, `db2s://`, custom Root CAs, mTLS, and in-memory `Config.TLSConfig` |
| **`database/sql` Registration** | Supported | `sql.Register("db2", ...)` (`DriverContext`, `NewConnector`) with URL (`db2://`, `db2s://`) and Key-Value DSN parsers |
| **Connection Pooling & Liveness** | Supported | `db.PingContext()` (via `EXCSAT`), `driver.Validator`, and `driver.SessionResetter` for safe pool lifecycle management |
| **Transactions & ACID Isolation** | Supported | `db.BeginTx()`, `tx.Commit()`, `tx.Rollback()` supporting `UR`, `CS`, `RS`, and `RR` isolation levels |
| **DDL & DML Execution** | Supported | `db.ExecContext()` for `CREATE`, `DROP`, `INSERT`, `UPDATE`, `DELETE` |
| **Query & Streaming Rows** | Supported | On-demand cursor streaming via DRDA `OPNQRY` / `CNTQRY` / `CLSQRY` (`QueryStream`) with row spanning |
| **Type Conversions & NULLs** | Supported | Integers, Varchars, Floats, Dates, Timestamps, Decimals, Booleans, `sql.Null*`, `[16]byte`, `NamedValueChecker` (`driver.ErrSkip`) |
| **Prepared Statements & Params** | Supported | Positional `?` binding (`FDODSC`, `FDODTA`, `SQLDTA`) with dedicated DRDA section allocation (`sectionManager`, up to 499 concurrent prepared statements per connection — sections `2..500`, section `1` reserved for direct commands) |
| **LOBs (BLOB / CLOB / DBCLOB)** | Supported | Large binary and text streaming (`[]byte`, `string`, `io.Reader`) via DRDA `EXTDTA` (`0x146C`) with DoS frame/chain bounds |
| **Stored Procedures & `sql.Out`** | Supported | `CALL procedure(?, ...)` supporting `IN`, `OUT`, and `INOUT` parameters via `SQLDTARD` |
| **Multi-Result Sets** | Supported | Full `driver.RowsNextResultSet` implementation for procedures returning multiple cursors |
| **`Result.LastInsertId()`** | Unsupported (DRDA) | Returns explicit error directing callers to `SELECT ... FROM FINAL TABLE (INSERT ...)` |
| **Extended Data Types** | Supported | `DECFLOAT(16/34)` (IEEE 754-2008 DPD), `XML` documents, `TIMESTAMP WITH TIME ZONE` |
| **Graphic & DBCS Types** | Supported | `GRAPHIC`, `VARGRAPHIC`, `LONG VARGRAPHIC`, `DBCLOB` (`UTF-16BE` codec) |
| **Batch / Array Parameter DML** | Supported | Atomic transactional multi-row execution (`ExecBatchWithParams`) using primitive slices (`[]int`, `[]string`, `[]float64`, etc.) with automatic rollback on failure |
| **Admin DB Management & APIs** | Supported | `CreateDB()`, `DropDB()`, and `ExecAdminCmd()` (`SYSPROC.ADMIN_CMD`) |
| **Kerberos SSO & GSSAPI Auth** | Disabled (Runtime) | `ParseDSN` accepts Kerberos parameters and `internal/auth` implements keytab/GSSAPI token helpers, while runtime `SECMEC 7/11` exchange is disabled (`Session.handshake` / `AcquireKerberosToken`) until a compliant RFC 4120 `AP-REQ` subpackage is integrated |
| **Trusted Context & User Switching** | Supported | Session user identity transition (`SwitchUser`, `WithUser`) on pooled/dedicated connections |
| **Client Workstation & Accounting** | Supported | Workload identification & APM registers (`CURRENT CLIENT_APPLNAME`, `WRKSTNNAME`, `USERID`, `ACCTNG`, `CORR_TOKEN`) |
| **Adaptive Block Fetching** | Supported | Configurable DRDA `QRYBLKSZ` block size (`1024..1048576` bytes, default `65535`) via DSN (`?block_size=131072`) |
| **Context Cancellation & Timeouts** | Supported | Implemented via `Session.watch(ctx)` with `context.AfterFunc`, expiring the socket deadline (`SetDeadline(time.Now())`) and marking the session `broken` for safe eviction by `database/sql` (does not use `SQLINTR` on a separate connection) |
| **Security Hardening & Redaction** | Supported | Sensitive fields redacted in `fmt.Stringer`/`fmt.GoStringer` and DSN errors, strict DSN validation, identifier validation, zero data races |

---

## Installation

```bash
go get github.com/go-db2/go-db2
```

Requires Go **1.22** or higher.

---

## Connection String & DSN Reference

`go-db2` supports both standard **URL format** (`db2://` and `db2s://`) and **Key-Value (ODBC-style) format**.

### 1. URL Format (Recommended)
```text
db2://[username:password@]host[:port]/database[?param1=value1&param2=value2]
db2s://[username:password@]host[:port]/database[?param1=value1&param2=value2]
```
- `db2://` defaults to `ssl=false` and port `50000`.
- `db2s://` enforces `ssl=true` and defaults to port `50001`.

*Examples:*
```text
db2://db2inst1:SecretPass123@127.0.0.1:50000/TESTDB?ssl=false&allow_plaintext_auth=true
db2s://db2inst1:SecretPass123@127.0.0.1:50001/TESTDB?ssl_root_ca=/path/to/ca.pem
```

### 2. Key-Value Format
```text
host=127.0.0.1;port=50000;database=TESTDB;uid=db2inst1;pwd=SecretPass123;ssl=false;allow_plaintext_auth=true;
```

### Supported DSN Parameters

| Parameter | Aliases | Type | Default | Description |
| :--- | :--- | :---: | :---: | :--- |
| `host` | `server`, `hostname` | `string` | `"localhost"` | Db2 server hostname or IP (`host:port` supported). |
| `port` | — | `int` | `50000` (`50001` with SSL) | TCP port (`1..65535`). Explicit port is preserved when `ssl=true`. |
| `database` | `dbname`, `db` | `string` | `""` | Target relational database name (required; enforced in URL format). |
| `user` | `uid`, `username` | `string` | `""` | Authentication username. |
| `password` | `pwd` | `string` | `""` | Authentication password. |
| `ssl` | `use_ssl`, `usessl`, `tls` | `bool` | `false` (`true` for `db2s://`) | Enable TLS 1.2+ with strict server verification (`InsecureSkipVerify: false`). Automatically enabled by `db2s://` or `ssl_root_ca`/`ssl_client_cert_path`/`ssl_client_key_path`. Accepts `true`/`1`/`yes` or `false`/`0`/`no`. |
| `ssl_root_ca` | `ssl_ca`, `sslrootcapath`, `ssl_ca_path` | `string` | `""` | Path to PEM Root CA file (enables `ssl=true`). |
| `ssl_client_cert_path` | `ssl_client_cert`, `sslcert`, `cert` | `string` | `""` | Path to PEM client certificate for mTLS (enables `ssl=true`). |
| `ssl_client_key_path` | `ssl_client_key`, `sslkey`, `key` | `string` | `""` | Path to PEM client private key for mTLS (enables `ssl=true`). |
| `allow_plaintext_auth` | — | `bool` | `false` | Explicitly allow plaintext password auth (`SECMEC 3`) without TLS. Accepts `true`/`1`/`yes` or `false`/`0`/`no`. |
| `security_mechanism` | `secmec`, `auth_mechanism`, `auth` | `string` | `""` (`SECMEC 3` at runtime) | `plain`/`plaintext`/`secmec3`/`3` (SECMEC 3), `encrypted-password`/`encrypted_password`/`dh_encrypted_password`/`secmec9`/`9` (SECMEC 9), `user_only`/`secmec4`/`4` (SECMEC 4, no password), `kerberos`/`gssapi`/`7` and `encrypted_kerberos`/`11` (disabled at runtime). |
| `spn` | `service_principal`, `server_principal` | `string` | `""` | Kerberos Service Principal Name. |
| `realm` | `krb5_realm` | `string` | `""` | Kerberos realm. |
| `krb5_config` | `krb5_conf` | `string` | `""` | Path to `krb5.conf`. |
| `krb5_keytab` | `keytab` | `string` | `""` | Path to Kerberos `.keytab` file. |
| `krb5_ccache` | `ccache` | `string` | `""` | Path to Kerberos credential cache. |
| `client_applname` | `applname`, `application_name`, `appname` | `string` | `""` | Initial `CURRENT CLIENT_APPLNAME` register. |
| `client_wrkstnname` | `wrkstnname`, `workstation_name`, `workstation` | `string` | `""` | Initial `CURRENT CLIENT_WRKSTNNAME` register. |
| `client_userid` | `client_user`, `end_user` | `string` | `""` | Initial `CURRENT CLIENT_USERID` register. |
| `client_acctng` | `acctng`, `accounting` | `string` | `""` | Initial `CURRENT CLIENT_ACCTNG` register. |
| `client_corr_token` | `corr_token`, `correlation_token` | `string` | `""` | Initial `CURRENT CLIENT_CORR_TOKEN` register. |
| `block_size` | `blocksize`, `qryblksz` | `int` | `65535` | DRDA `QRYBLKSZ` block size in bytes (`1024..1048576`). |
| `timeout` | `connect_timeout`, `conn_timeout` | `duration` | `30s` | TCP dial timeout (duration e.g. `10s`, or integer seconds). Per-query timeouts use `context.Context`. |

> **Strict DSN Validation**: Unknown parameter keys as well as invalid boolean, integer, or duration values fail immediately in `ParseDSN`.
>
> **Custom In-Memory TLS Configuration**: There is no DSN parameter to disable server certificate verification. For in-memory Root CA pools, client certificates, or custom `*tls.Config` settings, parse a base DSN with `db2.ParseDSN`, assign `cfg.TLSConfig`, and open the pool via `db2.NewConnector(cfg)` + `sql.OpenDB(connector)`.

---

## Code Examples

### 1. Basic Connection & Query
```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-db2/go-db2"
)

func main() {
	db, err := sql.Open("db2", "db2://db2inst1:SecretPass123@127.0.0.1:50000/TESTDB?ssl=false&allow_plaintext_auth=true")
	if err != nil {
		log.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Ping failed: %v", err)
	}

	var greeting string
	err = db.QueryRowContext(ctx, "SELECT 'Hello from IBM Db2!' FROM SYSIBM.SYSDUMMY1").Scan(&greeting)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	fmt.Println(greeting)
}
```

### 2. Explicit Transactions (ACID Isolation)
```go
tx, err := db.BeginTx(ctx, nil)
if err != nil {
    log.Fatal(err)
}
defer tx.Rollback() // Safe: no-op if committed

_, err = tx.ExecContext(ctx, "INSERT INTO accounts (id, balance) VALUES (?, ?)", 101, 5000.00)
if err != nil {
    return err
}

_, err = tx.ExecContext(ctx, "UPDATE accounts SET balance = balance - ? WHERE id = ?", 200.00, 101)
if err != nil {
    return err
}

if err := tx.Commit(); err != nil {
    log.Fatal(err)
}
```

### 3. Stored Procedures with `sql.Out` & Multi-Result Sets
```go
// Stored procedure with IN and OUT parameters
var outTotal int
var outStatus string

_, err := db.ExecContext(ctx, "CALL PROCESS_ORDER(?, ?, ?)", 
    1001, 
    sql.Out{Dest: &outTotal}, 
    sql.Out{Dest: &outStatus},
)

// Stored procedure returning multiple result sets (cursors)
rows, err := db.QueryContext(ctx, "CALL GET_CUSTOMER_AND_ORDERS(?)", 42)
if err != nil {
    log.Fatal(err)
}
defer rows.Close()

// 1. Process first result set (Customer details)
for rows.Next() {
    var name, email string
    _ = rows.Scan(&name, &email)
}

// 2. Advance to second result set (Customer orders)
if rows.NextResultSet() {
    for rows.Next() {
        var orderID int
        var total float64
        _ = rows.Scan(&orderID, &total)
    }
}
```

### 4. Client Telemetry & APM Tracking (`WithClientInfo`)
```go
import "github.com/go-db2/go-db2"

// Inject telemetry context into any query or transaction
clientCtx := db2.WithClientInfo(ctx, db2.ClientInfo{
    ApplicationName:  "orders-microservice",
    WorkstationName:  "k8s-worker-node-04",
    UserID:           "app_user_42",
    Accounting:       "billing_dept",
    CorrelationToken: "trace-1",
})

// Db2 special registers (CURRENT CLIENT_APPLNAME, etc.) are automatically synchronized
rows, err := db.QueryContext(clientCtx, "SELECT * FROM orders WHERE status = ?", "PENDING")
```

### 5. Atomic Transactional Batch / Bulk DML
```go
// Atomic transactional batch insert (ExecBatchWithParams) with automatic rollback on failure
productIDs := []int{101, 102, 103, 104}
skus := []string{"SKU-A", "SKU-B", "SKU-C", "SKU-D"}
prices := []float64{19.99, 29.50, 49.00, 99.90}

result, err := db.ExecContext(ctx, 
    "INSERT INTO products (id, sku, price) VALUES (?, ?, ?)", 
    productIDs, skus, prices,
)
```

### 6. Structured Error Inspection (`*db2.Error`)
```go
var dbErr *db2.Error
if errors.As(err, &dbErr) {
    fmt.Printf("Db2 SQLCODE: %d, SQLSTATE: %s: %s\n", dbErr.SQLCode, dbErr.SQLState, dbErr.Message)
}

// Or test standard driver sentinels:
if errors.Is(err, db2.ErrAuthenticationFailed) {
    log.Println("Invalid username/password or SECMEC failure")
}
```

---

## Performance & Conformance Benchmarks

`go-db2` includes an isolated containerized benchmark and semantic conformance suite in the [`benchmarks/`](benchmarks/) module comparing `go-db2` directly against IBM's CGO driver (`go_ibm_db`).

### 1. Semantic Conformance (Result Parity)
Verified against live IBM Db2 across 8 test suites (`benchmarks/suite`): scalar types, nullables (`sql.Null*`), high-precision decimals (`DECIMAL` / `DECFLOAT(34)`), dates/timestamps, DBCS strings (`VARGRAPHIC`), and binary `BLOB`/`CLOB` payloads (verified via SHA-256 hash match).

### 2. Running Microbenchmarks & Live Containerized Comparisons
- **Local Go wire/codec microbenchmarks** (no Db2 server required):
  ```bash
  go test -bench=. -benchmem ./...
  ```
- **Live containerized benchmark suite** (`benchmarks/` module with `SPEC-004` error-propagating harness in `benchmarks/suite/perf.go`):
  ```bash
  ./benchmarks/run_benchmark.sh
  ```

> **Note on [benchmarks/RESULTS.md](benchmarks/RESULTS.md)**: The static snapshot generated on `2026-08-30` predates the `SPEC-004` benchmark harness error-propagation fix. Re-run `./benchmarks/run_benchmark.sh` against a live IBM Db2 container to collect fresh comparative measurements.

---

## Testing

### 1. Run Unit Tests, Examples & Race Checks
```bash
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go test -count=1 ./...
go test -v -race -timeout 5m -count=1 ./...
```

### 2. Run Live Tests against Local Db2 Container
Start the official IBM Db2 container:
```bash
podman run -d \
  --name db2-test \
  --privileged \
  -p 50000:50000 \
  -e LICENSE=accept \
  -e DB2INST1_PASSWORD=MinhaSenhaForte123 \
  -e DBNAME=testdb \
  -e PERSISTENT_HOME=false \
  icr.io/db2_community/db2
```

Run all 15 integration examples:
```bash
go run examples/basic_connect/main.go
go run examples/crud_demo/main.go
go run examples/params_demo/main.go
go run examples/lob_demo/main.go
go run examples/sp_demo/main.go
go run examples/sp_multi_results_demo/main.go
go run examples/sqlx_demo/main.go
go run examples/identity_demo/main.go
go run examples/extended_types_demo/main.go
go run examples/batch_dml_demo/main.go
go run examples/admin_db_demo/main.go
go run examples/graphic_dbcs_demo/main.go
go run examples/kerberos_auth_demo/main.go
go run examples/trusted_context_demo/main.go
go run examples/client_accounting_demo/main.go
```

---

## License

Licensed under the MIT License. See [LICENSE](LICENSE) for details.
