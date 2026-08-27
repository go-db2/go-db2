# Project Plan: `go-db2` (Pure Go Driver for IBM Db2)

> *Também disponível em [Português do Brasil](PROJECT_PLAN.pt-BR.md)*

---

## 1. Overview and Motivation

### 1.1 The Problem
Currently, the primary way to connect Go applications to **IBM Db2** is via the `ibmdb/go_ibm_db` driver. However, that driver is a CGO wrapper around the **IBM DB2 CLI / ODBC Driver (`clidriver`)**, which introduces significant drawbacks:
- **CGO Dependency:** Disables static compilation (`CGO_ENABLED=0`), making cross-compilation cumbersome.
- **Complex Deployment Environments:** Docker images become heavy and fragile; requires manual installation of C packages and shared libraries, incompatible with minimal base images (*distroless*, *scratch*) and Alpine Linux (`musl` vs `glibc`).
- **Architectural Incompatibility:** IBM's `clidriver` has limited and bureaucratic support for certain architectures (such as Apple Silicon / ARM64 on macOS and Linux ARM).
- **Serverless / Edge Environments:** Extremely difficult to run in AWS Lambda, Cloud Run, or resource-constrained Kubernetes environments.

### 1.2 The Solution
Build **`go-db2`**: a database driver for IBM Db2 written **100% in Pure Go**, which:
1. Communicates directly over TCP/TLS using the native **DRDA** (*Distributed Relational Database Architecture*) and **DDM** (*Distributed Data Management*) protocols.
2. Is fully compliant with the standard Go [`database/sql`](https://pkg.go.dev/database/sql) and [`database/sql/driver`](https://pkg.go.dev/database/sql/driver) packages.
3. Requires zero installation of `clidriver` or any proprietary IBM C libraries on the host system.

---

## 2. Learnings and Base References

The project will draw key insights from two proven open-source implementations:

```
┌────────────────────────────────────────┐       ┌────────────────────────────────────────┐
│               pydrda                   │       │                go-ora                  │
│     (DRDA Protocol Reference)          │       │       (Go Driver Architecture)         │
├────────────────────────────────────────┤       ├────────────────────────────────────────┤
│ • DRDA Handshake (EXCSAT, ACCSEC, etc) │       │ • database/sql/driver architecture     │
│ • DDM Codepoints and commands          │       │ • Packet management / network session  │
│ • Db2 SQL data type mappings           │       │ • Context support (Cancel/Timeout)     │
│ • Auth encryption (SECMEC 3, 7, 9)     │       │ • Robust type conversions (Go <-> DB)  │
│ • Binary/EBCDIC wire data formats      │       │ • Connection String & DSN parsing      │
└────────────────────────────────────────┘       └────────────────────────────────────────┘
                                     │               │
                                     ▼               ▼
                        ┌────────────────────────────────────────┐
                        │                go-db2                  │
                        │      Pure Go Driver for IBM Db2        │
                        └────────────────────────────────────────┘
```

### 2.1 Insights from `pydrda` (Python):
- **Network Protocol Flow:** Exact message ordering for DRDA communication:
  - `EXCSAT` (*Exchange Server Attributes*)
  - `ACCSEC` (*Access Security*)
  - `SECCHK` (*Security Check*)
  - `ACCRDB` (*Access Relational Database*)
  - `PRPSQLSTT` (*Prepare SQL Statement*)
  - `EXCSQLSTT` (*Execute SQL Statement*)
  - `OPNQRY` (*Open Query*) / `CNTQRY` (*Continue Query*) / `CLSQRY` (*Close Query*)
  - `RDBCMM` (*RDB Commit*) / `RDBRLLBCK` (*RDB Rollback*)
- **Codepoint Dictionary:** Full catalog of hexadecimal codes representing DRDA commands and parameters.
- **Data Types:** Type constants (e.g., `DB2_SQLTYPE_VARCHAR`, `DB2_SQLTYPE_TIMESTAMP`, `DB2_SQLTYPE_BLOB`, etc.) and parameter serialization with format descriptors.

### 2.2 Insights from `go-ora` (Go):
- **Idiomatic Go Driver Design:**
  - Implementation of `driver.Driver`, `driver.DriverContext`, `driver.Connector`, `driver.Conn`, `driver.Stmt`, `driver.Tx`, `driver.Rows`, `driver.Result` interfaces.
- **Network Layer (`network/`):**
  - Read/write buffer management using `sync.Pool` for minimal allocations.
  - Native TCP and TLS handling via `crypto/tls`.
- **Type System and Converters (`converters/`):**
  - Safe bidirectional mappings between native Go types (`int64`, `float64`, `string`, `time.Time`, `[]byte`, `driver.Valuer`) and database wire formats.
- **Context Support:**
  - Query cancellation and deadline/timeout enforcement via `context.Context`.

---

## 3. Proposed Architecture

```
go-db2/
├── go.mod
├── README.md
├── PROJECT_PLAN.md           # English version
├── PROJECT_PLAN.pt-BR.md     # Brazilian Portuguese version
├── driver.go                 # Driver registration (sql.Register("db2", ...))
├── connector.go              # driver.Connector implementation
├── connection.go             # driver.Conn, Ping, ExecDirect implementation
├── statement.go              # driver.Stmt and parameterized query implementation
├── transaction.go            # driver.Tx implementation (Commit and Rollback)
├── rows.go                   # driver.Rows and column metadata implementation
├── config.go                 # DSN / Connection String parser (URL and Key-Value)
├── errors.go                 # Db2 SQLCODE and SQLSTATE error handling
├── network/                  # Network communication and protocol layer
│   ├── session.go            # TCP/TLS session, buffer pooling, and flow control
│   ├── dss.go                # Data Stream Structure (Request/Reply/Object DSS)
│   ├── ddm.go                # DDM command and object parser/encoder
│   ├── codepoint.go          # DRDA Codepoint constants table
│   └── security/             # Authentication mechanisms (SECMEC)
│       ├── secmec.go         # Security mechanism interface
│       ├── plain.go          # SECMEC 3 (USRIDPWD - Plain text)
│       ├── des.go            # SECMEC 9 (USRENCPWD - DES encryption)
│       └── aes.go            # SECMEC 7 (USRENCPWD - AES encryption)
├── converters/               # Go Type <-> Db2 Wire Format conversions
│   ├── encoders.go           # Go Types -> Db2 Wire Format
│   ├── decoders.go           # Db2 Wire Format -> Go Types
│   ├── ebcdic.go             # EBCDIC / ASCII / UTF-8 conversion utilities
│   └── numeric.go            # Integer, float, and Packed Decimal conversions
├── types/                    # Rich Db2-specific types
│   ├── null_types.go         # Custom nullable types (if needed)
│   ├── lob.go                # BLOB, CLOB, DBCLOB handling
│   └── decimal.go            # High-precision decimal support
└── examples/                 # Practical usage examples
    ├── basic_query/
    └── prepared_statement/
```

---

## 4. Feature Scope

### 4.1 Core Features (Essential)
- [x] **Connection String / DSN:**
  - URL Format: `db2://user:pass@host:port/dbname?ssl=true&timeout=30s`
  - DSN Format: `host=localhost;port=50000;database=testdb;user=db2inst1;password=secret;`
- [x] **DRDA Handshake:**
  - `EXCSAT` (Exchange server/client attributes).
  - `ACCSEC` and security authentication with `SECCHK`.
  - `ACCRDB` (Access and bind to relational database).
- [x] **Basic Data Type Support:**
  - Integers: `SMALLINT` (int16), `INTEGER` (int32), `BIGINT` (int64).
  - Floating Point: `REAL` (float32), `DOUBLE` (float64).
  - Strings: `CHAR`, `VARCHAR`, `LONG VARCHAR`.
  - Nulls: Proper `NULL` column detection (`sql.NullString`, `sql.NullInt64`, etc.).
  - Temporal & Booleans: `DATE`, `TIME`, `TIMESTAMP`, `BOOLEAN`.
- [x] **SQL Statement Execution:**
  - `Exec` / `ExecContext` for DDL and DML (`CREATE`, `INSERT`, `UPDATE`, `DELETE`).
  - `Query` / `QueryContext` for DQL (`SELECT`).
  - Column metadata support (`Rows.Columns()`, `Rows.ColumnTypeScanType()`, `Rows.ColumnTypeDatabaseTypeName()`, `Rows.ColumnTypeNullable()`, `Rows.ColumnTypeLength()`, `Rows.ColumnTypePrecisionScale()`).
- [x] **Transactions:**
  - `Begin` / `BeginTx` with `Commit` (`RDBCMM`) and `Rollback` (`RDBRLLBCK`).
- [x] **Prepared Statements:**
  - `Prepare` / `PrepareContext` with positional `?` parameter markers.
  - Parameter encoding (`FDODSC`, `FDODTA`, `SQLDTA`) with typed parameters & null values.
  - Statement execution (`stmt.ExecContext`) and parameterized queries (`stmt.QueryContext`, `db.QueryRowContext`).

### 4.2 Advanced Features
- [x] **Security and Encryption:**
  - Native SSL/TLS encrypted connection (`crypto/tls`).
  - Support for multiple DRDA authentication mechanisms:
    - `SECMEC 3` (Plain text User/Password).
    - `SECMEC 9` (Diffie-Hellman + DES Encrypted Password).
- [x] **Advanced Db2 Data Types:**
  - Temporal: `DATE`, `TIME`, `TIMESTAMP` (mapped to `time.Time`).
  - High Precision: `DECIMAL` / `NUMERIC` (Packed Decimal format).
  - Binaries and LOBs: `BLOB`, `CLOB`, `DBCLOB` (via DRDA `EXTDTA` streaming).
- [x] **Performance and Optimization:**
  - Stack-allocated DSS headers and zero-allocation header serialization.
  - Pre-allocated parameter descriptor buffers in `BuildSQLDTA`.
  - Comprehensive benchmark suite (`benchmark_test.go`) measuring time, memory (`B/op`), and allocs (`allocs/op`).
- [x] **Stored Procedures:**
  - Invocation of stored procedures via `CALL procedure(?, ...)` using standard Go `sql.Out`.
  - Full support for `IN`, `OUT`, and `INOUT` parameters parsed from DRDA `SQLDTARD` (`0x2413`).

---

## 5. Development Roadmap

```mermaid
gantt
    title go-db2 Development Roadmap
    dateFormat  YYYY-MM-DD
    section Phase 1: Foundation & Handshake
    Module setup & DSN Parser                :done, f1_1, 2026-09-01, 7d
    Network Layer (DSS & DDM Encoders)       :done, f1_2, after f1_1, 10d
    DRDA Handshake & Basic Auth (SECMEC 3)   :done, f1_3, after f1_2, 10d
    section Phase 2: CRUD & database/sql
    driver.Driver & Conn implementation      :done, f2_1, after f1_3, 7d
    Exec Direct (DDL/DML) & Simple Select    :done, f2_2, after f2_1, 10d
    Primitive Types Mapping                  :done, f2_3, after f2_2, 7d
    Transactions (Commit / Rollback)         :done, f2_4, after f2_3, 5d
    section Phase 3: Complete Types & Params
    Prepared Statements (?)                  :done, f3_1, after f2_4, 10d
    Temporal Types (Date/Time/Timestamp)     :done, f3_2, after f3_1, 7d
    Decimals (Packed Decimal) & LOBs         :done, f3_3, after f3_2, 10d
    section Phase 4: Security & LOBs
    TLS/SSL & SECMEC 9 Encrypted Auth        :done, f4_1, after f3_3, 10d
    LOB Streaming (EXTDTA)                   :done, f4_2, after f4_1, 7d
    section Phase 5: Quality, Stored Procedures & Release
    Stored Procedures (CALL & sql.Out)       :done, f5_1, after f4_2, 5d
    Performance Optimizations & Benchmarks   :done, f5_2, after f5_1, 5d
    CI/CD Automation with GitHub Actions     :done, f5_3, after f5_2, 5d
    Release v0.1.0 (Public MVP)              :done, milestone, after f5_3, 0d
```

### Milestone Breakdown

| Milestone | Status | Objective | Deliverables |
| :--- | :---: | :--- | :--- |
| **M1 - Protocol Foundation** | ✅ Supported | Establish basic socket connection with Db2 | DSS/DDM packet layer, dispatching `EXCSAT`, `ACCSEC`, `SECCHK`, `ACCRDB`, and clean session termination. |
| **M2 - Minimal `database/sql`** | ✅ Supported | Execute simple SQL commands and receive results | Driver registered in Go; support for `db.Ping()`, `db.Exec("CREATE TABLE...")`, and `db.Query("SELECT 1 FROM ...")`. |
| **M3 - Types & Statements** | ✅ Supported | Full support for everyday CRUD operations | `db.Prepare()`, `?` placeholders, conversions for integers, text, dates, decimals, and booleans. |
| **M4 - Security & LOBs** | ✅ Supported | Enterprise and cloud readiness | SSL/TLS support, encrypted password authentication (SECMEC 9), `BLOB`/`CLOB` reading and streaming via `EXTDTA`. |
| **M5 - Quality, SPs & Release** | ✅ Supported | Production quality, Stored Procedures & CI | `CALL` with `sql.Out` (`IN`/`OUT`/`INOUT`), memory benchmarks (`benchmark_test.go`), and GitHub Actions CI workflow. |

---

## 6. Testing Strategy and Development Environment

To ensure continuous validation and local testing:

1. **Local Environment with Podman / Docker:**
   Launch the official IBM Db2 Community Edition container using verified flags:
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
   > Wait for the message `(*) Setup has completed.` in container logs (`podman logs -f db2-test`).

2. **Unit & Mock Tests (Isolated):**
   - Run unit and mock TCP server test suites without external database dependencies:
     ```bash
     go test -v ./...
     ```

3. **Live Driver Integration Test:**
   - Run the live handshake, ping, and transaction test against the active container:
     ```bash
     go run examples/basic_connect/main.go
     ```

4. **Automated Integration Tests (Future):**
   - Integration with `testcontainers-go` to automatically launch Db2 containers during `go test ./...`.

5. **CI/CD with GitHub Actions:**
   - Automated pipeline for linting (`golangci-lint`), unit testing, and containerized integration tests.

---

## 7. Immediate Next Steps

1. Initialize the Go module inside `go-db2`:
   ```bash
   go mod init github.com/go-db2/go-db2
   ```
2. Create the initial package directory structure (`network/`, `converters/`, `types/`).
3. Port and catalog the **DRDA Codepoints** table from `pydrda/drda/codepoint.py` to Go (`network/codepoint.go`).
4. Implement the **DSS (Data Stream Structure)** packet format and the first handshake test (`EXCSAT`).
