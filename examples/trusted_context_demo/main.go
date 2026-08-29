package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-db2/go-db2"
)

func main() {
	fmt.Println("1. Conectando ao IBM Db2 para demonstração de Trusted Context & User Switching...")

	connStr := "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"
	db, err := sql.Open("db2", connStr)
	if err != nil {
		log.Fatalf("Erro ao abrir banco: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro no Ping: %v", err)
	}
	fmt.Println("   ✅ Conexão base estabelecida!")

	// 2. Obter uma conexão dedicada do pool
	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatalf("Erro ao obter conexão dedicada: %v", err)
	}
	defer conn.Close()

	// 3. Consultar usuário inicial
	var currUser, sessUser string
	err = conn.QueryRowContext(ctx, "SELECT CURRENT USER, SESSION_USER FROM SYSIBM.SYSDUMMY1").Scan(&currUser, &sessUser)
	if err != nil {
		log.Fatalf("Erro ao consultar usuário inicial: %v", err)
	}
	fmt.Printf("   👤 Usuário Base Inicial: CURRENT USER=%s, SESSION_USER=%s\n", currUser, sessUser)

	// 4. Alternar identidade para Tenant / Usuário Secundário (ex: DB2INST1 ou usuário de contexto)
	fmt.Println("\n2. Executando comutação de identidade de usuário (SwitchUser)...")
	err = db2.SwitchUser(ctx, conn, "DB2INST1")
	if err != nil {
		log.Fatalf("Erro no SwitchUser: %v", err)
	}

	err = conn.QueryRowContext(ctx, "SELECT CURRENT USER, SESSION_USER FROM SYSIBM.SYSDUMMY1").Scan(&currUser, &sessUser)
	if err != nil {
		log.Fatalf("Erro ao consultar usuário pós-switch: %v", err)
	}
	fmt.Printf("   ✅ Identidade comutada com sucesso: CURRENT USER=%s, SESSION_USER=%s\n", currUser, sessUser)

	// 5. Testar injeção automática de usuário via WithUser no context
	fmt.Println("\n3. Executando consulta com usuário injetado via WithUser no context...")
	tenantCtx := db2.WithUser(ctx, "DB2INST1")
	var tenantUser string
	err = conn.QueryRowContext(tenantCtx, "SELECT CURRENT USER FROM SYSIBM.SYSDUMMY1").Scan(&tenantUser)
	if err != nil {
		log.Fatalf("Erro na consulta com context user: %v", err)
	}
	fmt.Printf("   ✅ Consulta executada sob o contexto do usuário: %s\n", tenantUser)

	fmt.Println("\n🎉 DEMONSTRAÇÃO DE TRUSTED CONTEXT (MULTI-TENANT SWITCH) CONCLUÍDA COM 100% DE SUCESSO!")
	_ = os.Stdout.Sync()
}
