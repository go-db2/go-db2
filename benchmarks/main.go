package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-db2/go-db2/benchmarks/suite"

	// Drivers under test
	_ "github.com/go-db2/go-db2"
	_ "github.com/ibmdb/go_ibm_db"
)

func main() {
	host := getEnv("DB2_HOST", "127.0.0.1")
	port := getEnv("DB2_PORT", "50000")
	database := getEnv("DB2_DATABASE", "TESTDB")
	user := getEnv("DB2_USER", "db2inst1")
	password := getEnv("DB2_PASSWORD", "MinhaSenhaForte123")

	fmt.Println("==========================================================================================================")
	fmt.Println("🚀 INICIANDO SUÍTE COMPARATIVA: go-db2 (Pure Go) VS go_ibm_db (CGO / clidriver)")
	fmt.Printf("   Destino: %s:%s / %s (Usuário: %s)\n", host, port, database, user)
	fmt.Println("==========================================================================================================")

	// 1. Conectar via go-db2 (Pure Go)
	pureGoDSN := fmt.Sprintf("db2://%s:%s@%s:%s/%s?ssl=false", user, password, host, port, database)
	pgDB, err := sql.Open("db2", pureGoDSN)
	if err != nil {
		log.Fatalf("Erro ao abrir conexão com go-db2: %v", err)
	}
	defer pgDB.Close()

	// 2. Conectar via go_ibm_db (CGO)
	cgoDSN := fmt.Sprintf("HOSTNAME=%s;DATABASE=%s;PORT=%s;UID=%s;PWD=%s", host, database, port, user, password)
	cgoDB, err := sql.Open("go_ibm_db", cgoDSN)
	if err != nil {
		log.Fatalf("Erro ao abrir conexão com go_ibm_db: %v", err)
	}
	defer cgoDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Println("\n🔍 Testando conectividade dos dois drivers...")
	if err := pgDB.PingContext(ctx); err != nil {
		log.Fatalf("Falha no Ping do go-db2 (Pure Go): %v", err)
	}
	fmt.Println("   ✅ go-db2 (Pure Go) conectado com sucesso!")

	if err := cgoDB.PingContext(ctx); err != nil {
		log.Fatalf("Falha no Ping do go_ibm_db (CGO): %v", err)
	}
	fmt.Println("   ✅ go_ibm_db (CGO) conectado com sucesso!")

	// 3. Executar Suíte de Conformance (Validação Semântica)
	fmt.Println("\n🧪 Executando testes de Conformance Semântica (Validação de Igualdade de Resultados)...")
	conformanceResults := suite.RunConformanceSuite(ctx, pgDB, cgoDB)

	// 4. Executar Suíte de Performance (Benchmarks de Latência e Memória)
	fmt.Println("\n⚡ Executando testes de Performance e Alocação de Memória...")
	benchmarkResults := suite.RunPerformanceSuite(ctx, pgDB, cgoDB)

	// 5. Gerar Relatórios
	report := suite.ReportGenerator{
		Conformance: conformanceResults,
		Benchmarks:  benchmarkResults,
	}

	report.PrintConsole()

	outputMD := "RESULTS.md"
	if err := report.SaveMarkdown(outputMD); err != nil {
		log.Printf("Aviso: Falha ao salvar relatório Markdown: %v", err)
	} else {
		fmt.Printf("📄 Relatório detalhado exportado com sucesso para: %s\n\n", outputMD)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
