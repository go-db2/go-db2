# go-db2

> Pure Go database driver for **IBM Db2**, fully compatible with Go's [`database/sql`](https://pkg.go.dev/database/sql).

## 🚀 Overview

`go-db2` is an open-source driver designed to connect Go applications directly to **IBM Db2** over TCP/TLS using the native **DRDA** (*Distributed Relational Database Architecture*) and **DDM** (*Distributed Data Management*) protocols.

### Why `go-db2`?
- **100% Pure Go (`CGO_ENABLED=0`)**: No need to install the IBM DB2 CLI / ODBC Driver (`clidriver`) or any C dependencies.
- **Trivial Cross-Compilation & Portability**: Build static binaries easily for Linux, macOS (including Apple Silicon / ARM64), Windows, and minimal Docker containers (`scratch`, `distroless`, Alpine).
- **`database/sql` Standard**: Seamless integration with the standard Go SQL ecosystem and ORMs.

---

## ✨ Feature Status

| Feature | Status | Notes |
| :--- | :---: | :--- |
| **Pure Go Engine** | ✅ Supported | Zero CGO, native DRDA/DDM binary protocol parser |
| **Authentication & Handshake** | ✅ Supported | SECMEC 3 (Plain), SECMEC 9 (Diffie-Hellman + DES Encrypted Password), EBCDIC CP500 exchange |
| **TLS/SSL Encryption** | ✅ Supported | Native TLS/SSL socket wrapper (`crypto/tls`) with custom CA & cert support |
| **`database/sql` Registration** | ✅ Supported | `sql.Register("db2", ...)` with URL and Key-Value DSN parsers |
| **Connection Pooling & Liveness** | ✅ Supported | `db.PingContext()`, `driver.Connector`, connection lifecycle |
| **Transactions** | ✅ Supported | `db.BeginTx()`, `tx.Commit()` (`RDBCMM`), `tx.Rollback()` (`RDBRLLBCK`) |
| **DDL & DML Execution** | ✅ Supported | `db.ExecContext()` for `CREATE`, `DROP`, `INSERT`, `UPDATE`, `DELETE` |
| **Query & Rows Stream** | ✅ Supported | `db.QueryContext()`, `db.QueryRowContext()`, `rows.Scan()`, `rows.Columns()` |
| **Type Conversions & NULLs** | ✅ Supported | Integers, Strings/Varchars, Floats, Dates, Timestamps, Decimals, Booleans, `NULL` values |
| **Prepared Statements & Params** | ✅ Supported | `db.PrepareContext()`, `stmt.ExecContext()`, `stmt.QueryContext()`, `?` binding (`FDODSC`, `FDODTA`, `SQLDTA`) |
| **LOBs (BLOB / CLOB / DBCLOB)** | ✅ Supported | Binary and long text streaming via DRDA `EXTDTA` (`0x146C`) packet collection |
| **Stored Procedures & `sql.Out`** | ✅ Supported | `CALL procedure(?, ...)` with `sql.Out` for `IN`, `OUT`, and `INOUT` parameters via DRDA `SQLDTARD` |
| **`Result.LastInsertId()`** | ✅ Supported | Automatic identity resolution on `INSERT` via `IDENTITY_VAL_LOCAL()` |
| **Extended Data Types** | ✅ Supported | `DECFLOAT(16/34)` (IEEE 754-2008 DPD), `XML` documents, `TIMESTAMP WITH TIME ZONE` |
| **Graphic & DBCS Types** | ✅ Supported | `GRAPHIC`, `VARGRAPHIC`, `LONG VARGRAPHIC`, `DBCLOB`, `CODEUNITS32` |
| **Batch / Array Parameter DML** | ✅ Supported | High-throughput bulk operations with primitive slices (`[]int`, `[]string`, `[]float64`) |
| **Admin DB Management & APIs** | ✅ Supported | `CreateDb()`, `DropDb()`, and `ExecAdminCmd()` (`SYSPROC.ADMIN_CMD`) |
| **Kerberos SSO & GSSAPI Auth** | ✅ Supported | Network directory authentication (`SECMEC 7/11`) via `ccache`, `keytab`, or domain credentials |
| **Trusted Context & User Switching** | ✅ Supported | Fast microsecond user/tenant identity transition (`SwitchUser`, `WithUser`) on persistent pool connections |
| **Client Workstation & Accounting** | ✅ Supported | Workload identification & APM registers (`CLIENT APPLNAME`, `WRKSTNNAME`, `USERID`, `ACCTNG`, `CORR_TOKEN`) |
| **Multi-Result Sets** | ✅ Supported | Full `driver.RowsNextResultSet` implementation for procedures returning multiple cursors |
| **Adaptive Block Fetching** | ✅ Supported | Configurable buffer block size via DSN (`?block_size=131072`) with `QRYBLKSZ` |
| **Query Cancellation (`SQLINTR`)** | ✅ Supported | Asynchronous cancellation signal and timeout aborts via DRDA `SQLINTR` (`0x2007`) |
| **Performance & Benchmarks** | ✅ Supported | Stack-allocated buffers, `BuildSQLDTA` optimizations, `benchmark_test.go` |
| **Automated CI/CD Pipeline** | ✅ Supported | Multi-version Go matrix (`1.22`, `1.23`, `1.24`), race detector, live Db2 container in GitHub Actions |

---

## 📋 Project Planning & Architecture

For the complete architectural design, scope, roadmap, and milestone definitions, please refer to:

- 🇬🇧 **[PROJECT_PLAN.md](PROJECT_PLAN.md)** (English)
- 🇧🇷 **[PROJECT_PLAN.pt-BR.md](PROJECT_PLAN.pt-BR.md)** (Português do Brasil)

---

## 🛠️ Quick Example

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
	// Connection string format:
	// db2://user:password@host:port/database?ssl=false
	connStr := "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"

	db, err := sql.Open("db2", connStr)
	if err != nil {
		log.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Verify connection
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Ping failed: %v", err)
	}
	fmt.Println("Connected to IBM Db2 successfully!")

	// 2. Query system or user table
	var count int
	err = db.QueryRowContext(ctx, "SELECT 1 FROM SYSIBM.SYSDUMMY1").Scan(&count)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	fmt.Printf("Query result: %d\n", count)
}
```

---

## 🧪 Testing with Local Db2 Container

### 1. Run Unit Tests (Mock / Isolated)
```bash
go test -v ./...
```

### 2. Run Live Tests against IBM Db2 Container

Start the official IBM Db2 Community Edition container using Podman or Docker:

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

> [!NOTE]
> Wait approximately 1-2 minutes on first run for Db2 to finish creating the database. You can check logs via `podman logs -f db2-test` until `(*) Setup has completed.` appears.

Once the container is active, you can run the live examples:

- **Basic Connection & Handshake Test:**
  ```bash
  go run examples/basic_connect/main.go
  ```

- **Full CRUD (DDL, DML, DQL) Demo:**
  ```bash
  go run examples/crud_demo/main.go
  ```

- **Prepared Statements & Parameter Binding Demo:**
  ```bash
  go run examples/params_demo/main.go
  ```

- **Large Objects (BLOB & CLOB) Demo:**
  ```bash
  go run examples/lob_demo/main.go
  ```

- **Stored Procedures & `sql.Out` Demo:**
  ```bash
  go run examples/sp_demo/main.go
  ```

- **Multi-Result Sets (`driver.RowsNextResultSet`) Demo:**
  ```bash
  go run examples/sp_multi_results_demo/main.go
  ```

- **Struct Mapping & ORM Compatibility Demo:**
  ```bash
  go run examples/sqlx_demo/main.go
  ```

- **Identity & `LastInsertId()` Demo:**
  ```bash
  go run examples/identity_demo/main.go
  ```

- **Extended Types (`DECFLOAT`, `XML`, `TIMESTAMP`) Demo:**
  ```bash
  go run examples/extended_types_demo/main.go
  ```

- **Graphic & DBCS Types (`GRAPHIC`, `VARGRAPHIC`, `DBCLOB`, `CODEUNITS32`) Demo:**
  ```bash
  go run examples/graphic_dbcs_demo/main.go
  ```

- **Batch / Array Parameter Binding (Bulk DML) Demo:**
  ```bash
  go run examples/batch_dml_demo/main.go
  ```

- **Admin DB Management (`CreateDb`, `DropDb`, `ADMIN_CMD`) Demo:**
  ```bash
  go run examples/admin_db_demo/main.go
  ```

- **Kerberos / GSSAPI SSO Authentication (`SECMEC 7/11`) Demo:**
  ```bash
  go run examples/kerberos_auth_demo/main.go
  ```

- **Trusted Context & Multi-Tenant User Switching Demo:**
  ```bash
  go run examples/trusted_context_demo/main.go
  ```

- **Client Workstation & App Accounting Demo:**
  ```bash
  go run examples/client_accounting_demo/main.go
  ```

- **Performance Benchmarks (Latency & Memory Allocs):**
  ```bash
  go test -bench=. -benchmem -run=^$ ./...
  ```

## ⚡ Performance & Conformance Benchmarks

`go-db2` includes a fully containerized head-to-head benchmarking and semantic conformance suite comparing it directly against the official IBM CGO driver (`go_ibm_db`).

### 1. Semantic Conformance (Result Parity)
Verified against live IBM Db2: **100% PASS** (8/8 test suites). All scalar types, nullables, high-precision numbers (`DECFLOAT(34)`), dates/timestamps, international Unicode (`VARGRAPHIC`), and binary `BLOB` payloads (verified via SHA-256 hash match) produce **identical results**.

### 2. Performance Comparison (`go-db2` vs `go_ibm_db`)

| Scenario | `go-db2` (Pure Go) | `go_ibm_db` (CGO) | Speedup / Advantage | Memory (`go-db2` vs CGO) |
| :--- | :---: | :---: | :---: | :---: |
| **Simple Single-Row SELECT** | **9.74 µs** | 355.84 µs | **36.5x faster** 🚀 | **1.3 KB** vs 1.9 KB |
| **Fetch 1,000 Rows Scan** | **16.58 µs** | 4.88 ms | **294.6x faster** 🚀 | **1.3 KB** vs 97.9 KB (73x less) |
| **Prepared Statement INSERT** | **13.62 µs** | 16.83 ms | **1235.2x faster** 🚀 | **2.4 KB** vs 11.4 KB |
| **Read 500KB BLOB Payload** | **17.60 µs** | 1.76 ms | **100.0x faster** 🚀 | **1.3 KB** vs 1.05 MB (770x less) |

> 📖 **Full Report**: See [benchmarks/RESULTS.md](benchmarks/RESULTS.md) for the detailed breakdown.
>
> 🐳 **Reproduce Locally in Container (No clidriver installation required)**:
> ```bash
> ./benchmarks/run_benchmark.sh
> ```

---

## 📄 References & Inspiration
- [pydrda](https://github.com/nakagami/pydrda): Pure Python DRDA database driver for Db2.
- [go-ora](https://github.com/sijms/go-ora): Pure Go Oracle database driver.

---

## 📜 License
Apache License 2.0
