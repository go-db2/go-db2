# go-db2

> Pure Go database driver for **IBM Db2**, fully compatible with Go's [`database/sql`](https://pkg.go.dev/database/sql).

## 🚀 Overview

`go-db2` is an open-source driver designed to connect Go applications directly to **IBM Db2** over TCP/TLS using the native **DRDA** (*Distributed Relational Database Architecture*) and **DDM** (*Distributed Data Management*) protocols.

### Why `go-db2`?
- **100% Pure Go (`CGO_ENABLED=0`)**: No need to install the IBM DB2 CLI / ODBC Driver (`clidriver`) or any C dependencies.
- **Trivial Cross-Compilation & Portability**: Build static binaries easily for Linux, macOS (including Apple Silicon / ARM64), Windows, and minimal Docker containers (`scratch`, `distroless`, Alpine).
- **`database/sql` Standard**: Seamless integration with the standard Go SQL ecosystem and ORMs.

---

## 📋 Project Planning & Architecture

For the complete architectural design, scope, roadmap, and milestone definitions, please refer to:

- 🇬🇧 **[PROJECT_PLAN.md](PROJECT_PLAN.md)** (English)
- 🇧🇷 **[PROJECT_PLAN.pt-BR.md](PROJECT_PLAN.pt-BR.md)** (Português do Brasil)

---

## 🛠️ Quick Example (Target API)

```go
package main

import (
    "database/sql"
    "fmt"
    "log"

    _ "github.com/go-db2/go-db2"
)

func main() {
    // Connection string via URL format
    connStr := "db2://db2inst1:password@localhost:50000/testdb?ssl=false"
    
    db, err := sql.Open("db2", connStr)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    if err := db.Ping(); err != nil {
        log.Fatal(err)
    }

    var serviceLevel string
    err = db.QueryRow("SELECT service_level FROM sysibmadm.env_inst_info").Scan(&serviceLevel)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Connected to Db2! Version: %s\n", serviceLevel)
}
```

---

## 📄 References & Inspiration
- [pydrda](https://github.com/nakagami/pydrda): Pure Python DRDA database driver for Db2.
- [go-ora](https://github.com/sijms/go-ora): Pure Go Oracle database driver.

---

## 📜 License
Apache License 2.0
