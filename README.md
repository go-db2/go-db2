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

- **Performance Benchmarks (Latency & Memory Allocs):**
  ```bash
  go test -bench=. -benchmem -run=^$ ./...
  ```

---

## 📄 References & Inspiration
- [pydrda](https://github.com/nakagami/pydrda): Pure Python DRDA database driver for Db2.
- [go-ora](https://github.com/sijms/go-ora): Pure Go Oracle database driver.

---

## 📜 License
Apache License 2.0
