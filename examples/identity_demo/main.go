package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-db2/go-db2"
)

func main() {
	dsn := os.Getenv("DB2_DSN")
	if dsn == "" {
		dsn = "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"
	}

	fmt.Println("1. Conectando ao IBM Db2 para validação de Result.LastInsertId()...")
	db, err := sql.Open("db2", dsn)
	if err != nil {
		log.Fatalf("Erro ao abrir driver: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro ao conectar ao Db2: %v", err)
	}
	fmt.Println("   ✅ Conexão estabelecida com sucesso!")

	fmt.Println("\n2. Criando tabela de teste 'test_orders' com coluna IDENTITY...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_orders")
	createTable := `CREATE TABLE test_orders (
		order_id INT GENERATED ALWAYS AS IDENTITY (START WITH 100, INCREMENT BY 1),
		customer VARCHAR(50) NOT NULL,
		amount DECIMAL(10, 2) NOT NULL,
		PRIMARY KEY (order_id)
	)`
	if _, err := db.ExecContext(ctx, createTable); err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}
	fmt.Println("   ✅ Tabela 'test_orders' criada com IDENTITY iniciando em 100!")

	fmt.Println("\n3. Executando inserções diretas (db.ExecContext) e testando LastInsertId()...")
	res1, err := db.ExecContext(ctx, "INSERT INTO test_orders (customer, amount) VALUES ('Alice Santos', 150.75)")
	if err != nil {
		log.Fatalf("Erro ao inserir pedido 1: %v", err)
	}
	id1, err := res1.LastInsertId()
	if err != nil {
		log.Fatalf("Erro ao obter LastInsertId para pedido 1: %v", err)
	}
	fmt.Printf("   ➕ Pedido #1 inserido! LastInsertId = %d (esperado: 100)\n", id1)
	if id1 != 100 {
		log.Fatalf("Falha na validação do ID: obteve %d, esperava 100", id1)
	}

	res2, err := db.ExecContext(ctx, "INSERT INTO test_orders (customer, amount) VALUES ('Bruno Lima', 320.00)")
	if err != nil {
		log.Fatalf("Erro ao inserir pedido 2: %v", err)
	}
	id2, err := res2.LastInsertId()
	if err != nil {
		log.Fatalf("Erro ao obter LastInsertId para pedido 2: %v", err)
	}
	fmt.Printf("   ➕ Pedido #2 inserido! LastInsertId = %d (esperado: 101)\n", id2)
	if id2 != 101 {
		log.Fatalf("Falha na validação do ID: obteve %d, esperava 101", id2)
	}

	fmt.Println("\n4. Executando inserção parametrizada (stmt.ExecContext) e testando LastInsertId()...")
	stmt, err := db.PrepareContext(ctx, "INSERT INTO test_orders (customer, amount) VALUES (?, ?)")
	if err != nil {
		log.Fatalf("Erro ao preparar statement: %v", err)
	}
	defer stmt.Close()

	res3, err := stmt.ExecContext(ctx, "Carla Dias", 890.50)
	if err != nil {
		log.Fatalf("Erro ao executar prepared insert: %v", err)
	}
	id3, err := res3.LastInsertId()
	if err != nil {
		log.Fatalf("Erro ao obter LastInsertId via Prepared Statement: %v", err)
	}
	fmt.Printf("   ➕ Pedido #3 inserido via Prepared Statement! LastInsertId = %d (esperado: 102)\n", id3)
	if id3 != 102 {
		log.Fatalf("Falha na validação do ID: obteve %d, esperava 102", id3)
	}

	fmt.Println("\n5. Validando todos os registros inseridos na tabela...")
	rows, err := db.QueryContext(ctx, "SELECT order_id, customer, amount FROM test_orders ORDER BY order_id")
	if err != nil {
		log.Fatalf("Erro ao consultar pedidos: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var orderID int64
		var customer string
		var amount float64
		if err := rows.Scan(&orderID, &customer, &amount); err != nil {
			log.Fatalf("Erro ao escanear linha: %v", err)
		}
		fmt.Printf("     📦 Pedido ID: %d | Cliente: %-15s | Valor: R$ %7.2f\n", orderID, customer, amount)
	}

	fmt.Println("\n6. Limpando tabela de teste...")
	if _, err := db.ExecContext(ctx, "DROP TABLE test_orders"); err != nil {
		log.Fatalf("Erro ao remover tabela: %v", err)
	}
	fmt.Println("   ✅ Limpeza concluída com sucesso!")

	fmt.Println("\n🎉 DEMO DE RESULT.LASTINSERTID() CONCLUÍDA COM 100% DE SUCESSO!")
}
