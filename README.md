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

Once the container is active, execute the live connection test:

```bash
go run examples/basic_connect/main.go
```

---

## 📄 References & Inspiration
- [pydrda](https://github.com/nakagami/pydrda): Pure Python DRDA database driver for Db2.
- [go-ora](https://github.com/sijms/go-ora): Pure Go Oracle database driver.

---

## 📜 License
Apache License 2.0
