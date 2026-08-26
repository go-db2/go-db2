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
	connStr := "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"

	fmt.Println("Conectando ao IBM Db2 via Pure Go driver...")
	db, err := sql.Open("db2", connStr)
	if err != nil {
		log.Fatalf("Erro no sql.Open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Enviando Ping para o Db2 (Handshake DRDA: EXCSAT -> ACCSEC -> SECCHK -> ACCRDB)...")
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Falha no Ping/Handshake: %v", err)
	}

	fmt.Println("✅ SUCESSO! Conexão e Handshake DRDA com o IBM Db2 estabelecidos com sucesso!")

	fmt.Println("Testando transação BeginTx -> Commit...")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("Erro no BeginTx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("Erro no Commit: %v", err)
	}

	fmt.Println("✅ Transação (RDBCMM) confirmada com sucesso!")
}
