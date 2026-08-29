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
	fmt.Println("1. Conectando ao IBM Db2 com registradores de Client Workstation & Accounting via DSN...")

	dsn := "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false" +
		"&client_applname=orders-microservice" +
		"&client_wrkstnname=k8s-node-worker-01" +
		"&client_userid=user_app_77" +
		"&client_acctng=dept_ecommerce" +
		"&client_corr_token=trace-initial-001"

	db, err := sql.Open("db2", dsn)
	if err != nil {
		log.Fatalf("Erro ao abrir banco: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro no Ping: %v", err)
	}
	fmt.Println("   ✅ Conexão estabelecida com metadados de cliente sincronizados!")

	// 2. Obter uma conexão dedicada e consultar registradores especiais do Db2
	conn, err := db.Conn(ctx)
	if err != nil {
		log.Fatalf("Erro ao obter conexão dedicada: %v", err)
	}
	defer conn.Close()

	var applName, wrkstnName, userid, acctng string
	queryRegisters := "SELECT " +
		"COALESCE(CURRENT CLIENT_APPLNAME, ''), " +
		"COALESCE(CURRENT CLIENT_WRKSTNNAME, ''), " +
		"COALESCE(CURRENT CLIENT_USERID, ''), " +
		"COALESCE(CURRENT CLIENT_ACCTNG, '') " +
		"FROM SYSIBM.SYSDUMMY1"

	err = conn.QueryRowContext(ctx, queryRegisters).Scan(&applName, &wrkstnName, &userid, &acctng)
	if err != nil {
		log.Fatalf("Erro ao consultar registradores de cliente: %v", err)
	}

	fmt.Println("\n2. Registradores Especiais do Db2 Ativos na Sessão:")
	fmt.Printf("   📱 CURRENT CLIENT_APPLNAME  : %q (configurado via DSN)\n", applName)
	fmt.Printf("   🖥️  CURRENT CLIENT_WRKSTNNAME: %q\n", wrkstnName)
	fmt.Printf("   👤 CURRENT CLIENT_USERID    : %q\n", userid)
	fmt.Printf("   💳 CURRENT CLIENT_ACCTNG    : %q\n", acctng)

	// 3. Atualizar metadados dinamicamente via SetClientInfo (ex: troca de endpoint/tarefa em lote)
	fmt.Println("\n3. Atualizando registradores dinamicamente via db2.SetClientInfo()...")
	newInfo := db2.ClientInfo{
		ApplicationName:  "batch-billing-job",
		WorkstationName:  "batch-runner-node-09",
		UserID:           "cron_worker_42",
		Accounting:       "billing_invoicing",
		CorrelationToken: "trace-batch-req-999",
	}

	err = db2.SetClientInfo(ctx, conn, newInfo)
	if err != nil {
		log.Fatalf("Erro ao executar SetClientInfo: %v", err)
	}

	err = conn.QueryRowContext(ctx, queryRegisters).Scan(&applName, &wrkstnName, &userid, &acctng)
	if err != nil {
		log.Fatalf("Erro ao consultar registradores pós-update: %v", err)
	}

	fmt.Println("   ✅ Registradores atualizados com sucesso no Db2:")
	fmt.Printf("   📱 CURRENT CLIENT_APPLNAME  : %q\n", applName)
	fmt.Printf("   🖥️  CURRENT CLIENT_WRKSTNNAME: %q\n", wrkstnName)
	fmt.Printf("   👤 CURRENT CLIENT_USERID    : %q\n", userid)
	fmt.Printf("   💳 CURRENT CLIENT_ACCTNG    : %q\n", acctng)

	// 4. Testar injeção de metadados por query via contexto Go
	fmt.Println("\n4. Executando query com injeção automática de metadados via db2.WithClientInfo(ctx)...")
	dynamicCtx := db2.WithClientInfo(ctx, db2.ClientInfo{
		ApplicationName:  "checkout-api",
		WorkstationName:  "k8s-pod-checkout-33",
		UserID:           "customer_api_user",
		Accounting:       "ecommerce_sales",
		CorrelationToken: "trace-otel-span-777",
	})

	var dynamicAppl string
	err = conn.QueryRowContext(dynamicCtx, "SELECT COALESCE(CURRENT CLIENT_APPLNAME, '') FROM SYSIBM.SYSDUMMY1").Scan(&dynamicAppl)
	if err != nil {
		log.Fatalf("Erro na consulta com dynamic context: %v", err)
	}
	fmt.Printf("   ✅ Query executada sob o contexto da aplicação (CLIENT_APPLNAME=%q)\n", dynamicAppl)

	fmt.Println("\n🎉 DEMONSTRAÇÃO DE CLIENT WORKSTATION & APP ACCOUNTING CONCLUÍDA COM 100% DE SUCESSO!")
	_ = os.Stdout.Sync()
}
