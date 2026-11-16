package db2_test

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-db2/go-db2"
)

func ExampleParseDSN() {
	cfg, err := db2.ParseDSN("db2s://appuser:secret@db2.example.com/PRODDB?conn_timeout=10s&block_size=131072")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Host=%s Port=%d Database=%s UseSSL=%t Timeout=%v BlockSize=%d\n",
		cfg.Host, cfg.Port, cfg.Database, cfg.UseSSL, cfg.Timeout, cfg.BlockSize)
	// Output:
	// Host=db2.example.com Port=50001 Database=PRODDB UseSSL=true Timeout=10s BlockSize=131072
}

func ExampleParseDSN_plaintextGuard() {
	_, err := db2.ParseDSN("db2://user:secret@localhost:50000/SAMPLE")
	fmt.Printf("error: %t\n", err != nil)
	fmt.Printf("leaked: %t\n", strings.Contains(err.Error(), "secret"))
	fmt.Printf("msg: %s\n", err)
	// Output:
	// error: true
	// leaked: false
	// msg: db2: plaintext authentication over unencrypted connection is disabled by default; set allow_plaintext_auth=true to override
}

func ExampleNewConnector() {
	cfg, err := db2.ParseDSN("db2s://appuser:secret@db2.example.com:50001/PRODDB")
	if err != nil {
		log.Fatal(err)
	}
	cfg.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	connector := db2.NewConnector(cfg)
	db := sql.OpenDB(connector)
	defer db.Close()

	fmt.Println(cfg)
	// Output:
	// Config{Host:db2.example.com, Port:50001, Database:PRODDB, User:appuser, Password:"******", UseSSL:true, Timeout:30s}
}

func ExampleConfig_String() {
	cfg := db2.Config{
		Host:     "db2.internal",
		Port:     50001,
		Database: "FINANCE",
		User:     "svc_app",
		Password: "SuperSecretPassword",
		UseSSL:   true,
		Timeout:  15 * time.Second,
	}
	fmt.Printf("%v\n", cfg)
	fmt.Printf("%#v\n", cfg)
	// Output:
	// Config{Host:db2.internal, Port:50001, Database:FINANCE, User:svc_app, Password:"******", UseSSL:true, Timeout:15s}
	// Config{Host:db2.internal, Port:50001, Database:FINANCE, User:svc_app, Password:"******", UseSSL:true, Timeout:15s}
}

func ExampleWithClientInfo() {
	ctx := db2.WithClientInfo(context.Background(), db2.ClientInfo{
		ApplicationName:  "orders-api",
		WorkstationName:  "pod-01",
		UserID:           "svc",
		Accounting:       "billing",
		CorrelationToken: "trace-1",
	})
	info := db2.ClientInfoFromContext(ctx)
	fmt.Printf("%s %s %s %s %s\n",
		info.ApplicationName, info.WorkstationName, info.UserID, info.Accounting, info.CorrelationToken)
	// Output:
	// orders-api pod-01 svc billing trace-1
}

func ExampleWithUser() {
	ctx := db2.WithUser(context.Background(), "tenant_a")
	fmt.Println(db2.UserFromContext(ctx))
	// Output:
	// tenant_a
}

func ExampleParseDSN_invalidError() {
	_, err := db2.ParseDSN("")
	fmt.Println(errors.Is(err, db2.ErrInvalidConnectionStr))
	// Output:
	// true
}

func ExampleError() {
	var err error = &db2.Error{
		SQLCode:  -204,
		SQLState: "42704",
		Message:  "undefined name",
	}
	var dbErr *db2.Error
	if errors.As(err, &dbErr) {
		fmt.Printf("SQLCODE=%d SQLSTATE=%s Message=%s\n", dbErr.SQLCode, dbErr.SQLState, dbErr.Message)
	}
	fmt.Println(err)
	// Output:
	// SQLCODE=-204 SQLSTATE=42704 Message=undefined name
	// db2: SQLCODE=-204, SQLSTATE=42704: undefined name
}

func Example_openAndQuery() {
	db, err := sql.Open("db2", "db2s://appuser:secret@db2.example.com:50001/PRODDB")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	var greeting string
	if err := db.QueryRowContext(ctx, "SELECT 'Hello from Db2' FROM SYSIBM.SYSDUMMY1").Scan(&greeting); err != nil {
		log.Fatal(err)
	}
	fmt.Println(greeting)
}

func Example_transactions() {
	db, err := sql.Open("db2", "db2s://appuser:secret@db2.example.com:50001/PRODDB")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "UPDATE accounts SET balance = balance - ? WHERE id = ?", 100, 1); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
}

func Example_storedProcedureOut() {
	db, err := sql.Open("db2", "db2s://appuser:secret@db2.example.com:50001/PRODDB")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var status string
	if _, err := db.ExecContext(ctx, "CALL PROCESS_ORDER(?, ?)", 1001, sql.Out{Dest: &status}); err != nil {
		log.Fatal(err)
	}
	fmt.Println(status)

	rows, err := db.QueryContext(ctx, "CALL GET_CUSTOMER_AND_ORDERS(?)", 42)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Fatal(err)
		}
		fmt.Println(name)
	}
	if rows.NextResultSet() {
		for rows.Next() {
			var orderID int
			if err := rows.Scan(&orderID); err != nil {
				log.Fatal(err)
			}
			fmt.Println(orderID)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
}

func Example_errorInspection() {
	db, err := sql.Open("db2", "db2s://appuser:secret@db2.example.com:50001/PRODDB")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SELECT id FROM missing_table")
	if err != nil {
		var dbErr *db2.Error
		if errors.As(err, &dbErr) {
			fmt.Printf("SQLCODE=%d SQLSTATE=%s Message=%s\n", dbErr.SQLCode, dbErr.SQLState, dbErr.Message)
		}
		if errors.Is(err, db2.ErrAuthenticationFailed) || errors.Is(err, db2.ErrDatabaseNotFound) {
			log.Fatal(err)
		}
		return
	}
	defer rows.Close()
}

func Example_finalTableIdentity() {
	db, err := sql.Open("db2", "db2s://appuser:secret@db2.example.com:50001/PRODDB")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var orderID int64
	if err := db.QueryRowContext(
		ctx,
		"SELECT order_id FROM FINAL TABLE (INSERT INTO orders (customer) VALUES (?))",
		"acme-corp",
	).Scan(&orderID); err != nil {
		log.Fatal(err)
	}
	fmt.Println(orderID)
}
