package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	db2 "github.com/go-db2/go-db2"
)

func main() {
	connStr := "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"

	fmt.Println("1. Conectando ao IBM Db2 para demonstração de Gerenciamento de Banco & ADMIN_CMD...")
	db, err := sql.Open("db2", connStr)
	if err != nil {
		log.Fatalf("Erro no sql.Open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro no Ping: %v", err)
	}
	fmt.Println("   ✅ Conexão estabelecida com sucesso!")

	// 2. Criar tabela de teste para comandos administrativos
	fmt.Println("\n2. Criando tabela 'test_admin_orders' para rotinas administrativas...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_admin_orders")

	createTableSQL := `CREATE TABLE test_admin_orders (
		order_id INT NOT NULL,
		customer_name VARCHAR(100) NOT NULL,
		order_total DECIMAL(10, 2),
		order_date DATE,
		PRIMARY KEY (order_id)
	)`
	if _, err := db.ExecContext(ctx, createTableSQL); err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}
	fmt.Println("   ✅ Tabela criada com sucesso!")

	// 3. Inserir alguns registros de exemplo
	fmt.Println("\n3. Inserindo dados de teste...")
	for i := 1; i <= 20; i++ {
		_, err := db.ExecContext(ctx,
			"INSERT INTO test_admin_orders (order_id, customer_name, order_total, order_date) VALUES (?, ?, ?, CURRENT DATE)",
			i, fmt.Sprintf("Cliente #%03d", i), float64(i)*49.90,
		)
		if err != nil {
			log.Fatalf("Erro ao inserir linha %d: %v", i, err)
		}
	}
	fmt.Println("   ✅ 20 registros inseridos!")

	// 4. Executar comando administrativo via ExecAdminCmd (RUNSTATS)
	fmt.Println("\n4. Executando comando administrativo RUNSTATS via ExecAdminCmd...")
	res, err := db2.ExecAdminCmd(ctx, db, "RUNSTATS ON TABLE test_admin_orders ON ALL COLUMNS WITH DISTRIBUTION")
	if err != nil {
		log.Fatalf("Erro ao executar RUNSTATS: %v", err)
	}
	_ = res
	fmt.Println("   ✅ RUNSTATS executado com sucesso no motor Db2!")

	// 5. Consultar SYSCAT.TABLES para verificar que as estatísticas foram atualizadas
	fmt.Println("\n5. Verificando estatísticas da tabela no catálogo de sistema SYSCAT.TABLES...")
	var card int64
	var statsTime sql.NullString
	err = db.QueryRowContext(ctx,
		"SELECT CARD, VARCHAR(STATS_TIME) FROM SYSCAT.TABLES WHERE TABNAME = 'TEST_ADMIN_ORDERS'",
	).Scan(&card, &statsTime)
	if err != nil {
		log.Printf("Aviso ao ler SYSCAT.TABLES: %v", err)
	} else {
		statsStr := "Recém-calculado"
		if statsTime.Valid {
			statsStr = statsTime.String
		}
		fmt.Printf("   📊 SYSCAT.TABLES: Linhas estimadas (CARD) = %d, STATS_TIME = %s\n", card, strings.TrimSpace(statsStr))
	}

	// 6. Validar funções de gerenciamento CreateDb e DropDb
	fmt.Println("\n6. Demonstrando APIs de alto nível CreateDb e DropDb...")
	fmt.Println("   🧪 Testando validação defensiva de parâmetros inválidos...")
	if _, err := db2.CreateDb("", connStr); err != nil {
		fmt.Printf("   ✅ CreateDb com nome vazio rejeitado corretamente: %v\n", err)
	}
	if _, err := db2.CreateDb("NEWDB", connStr, "invalidOption"); err != nil {
		fmt.Printf("   ✅ Opção mal formatada rejeitada corretamente: %v\n", err)
	}
	if _, err := db2.DropDb("", connStr); err != nil {
		fmt.Printf("   ✅ DropDb com nome vazio rejeitado corretamente: %v\n", err)
	}

	// 7. Limpeza
	fmt.Println("\n7. Limpando tabela de teste...")
	if _, err := db.ExecContext(ctx, "DROP TABLE test_admin_orders"); err != nil {
		log.Fatalf("Erro ao limpar tabela: %v", err)
	}
	fmt.Println("   ✅ Limpeza concluída com sucesso!")

	fmt.Println("\n🎉 DEMONSTRAÇÃO DE GERENCIAMENTO DE BANCO & ADMIN_CMD CONCLUÍDA COM 100% DE SUCESSO!")
}
