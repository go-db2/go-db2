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

	fmt.Println("1. Conectando ao IBM Db2 para testes da Fase 3 (Prepared Statements)...")
	db, err := sql.Open("db2", connStr)
	if err != nil {
		log.Fatalf("Erro no sql.Open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro no Ping: %v", err)
	}
	fmt.Println("   ✅ Conectado com sucesso!")

	// 2. Tabela simples test_users
	fmt.Println("\n2. Testando Prepared Statement com test_users (INT, VARCHAR)...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_users")
	_, err = db.ExecContext(ctx, "CREATE TABLE test_users (id INT NOT NULL, name VARCHAR(50) NOT NULL, age INT, PRIMARY KEY(id))")
	if err != nil {
		log.Fatalf("Erro ao criar test_users: %v", err)
	}

	userStmt, err := db.PrepareContext(ctx, "INSERT INTO test_users (id, name, age) VALUES (?, ?, ?)")
	if err != nil {
		log.Fatalf("Erro ao preparar insert test_users: %v", err)
	}
	defer userStmt.Close()

	_, err = userStmt.ExecContext(ctx, 1, "Alice", 30)
	if err != nil {
		log.Fatalf("Erro ao inserir Alice: %v", err)
	}
	fmt.Println("   ✅ Inserido usuário Alice com sucesso!")

	var uName string
	var uAge int
	err = db.QueryRowContext(ctx, "SELECT name, age FROM test_users WHERE id = ?", 1).Scan(&uName, &uAge)
	if err != nil {
		log.Fatalf("Erro ao consultar Alice: %v", err)
	}
	fmt.Printf("   ✅ Consulta parametrizada retornou: name=%s, age=%d\n", uName, uAge)

	// 3. Preparar tabela de produtos
	fmt.Println("\n3. Criando tabela 'test_products' com múltiplos tipos...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_products")

	createSQL := `CREATE TABLE test_products (
		id INT NOT NULL,
		title VARCHAR(100) NOT NULL,
		price FLOAT,
		is_active BOOLEAN,
		created_at DATE,
		PRIMARY KEY(id)
	)`
	_, err = db.ExecContext(ctx, createSQL)
	if err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}
	fmt.Println("   ✅ Tabela criada com sucesso!")

	// 4. Teste de INSERT com Prepared Statement (db.Prepare)
	fmt.Println("\n4. Preparando Statement de INSERT (db.Prepare)...")
	insertStmt, err := db.PrepareContext(ctx, "INSERT INTO test_products (id, title, price, is_active, created_at) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("Erro no db.Prepare: %v", err)
	}
	defer insertStmt.Close()

	fmt.Println("   ✅ Statement preparado com sucesso!")

	now := time.Now()
	testData := []struct {
		id       int
		title    string
		price    any
		isActive any
		date     any
	}{
		{1, "Teclado Mecânico", 250.50, true, now},
		{2, "Mouse Gamer", 120.00, true, now},
		{3, "Monitor 4K", 1850.75, false, now},
		{4, "Cabo USB-C", nil, nil, now}, // Teste de NULLs
	}

	fmt.Println("\n4. Executando inserções parametrizadas...")
	for _, item := range testData {
		_, err := insertStmt.ExecContext(ctx, item.id, item.title, item.price, item.isActive, item.date)
		if err != nil {
			log.Fatalf("Erro ao executar insert para item %d: %v", item.id, err)
		}
		fmt.Printf("   ✅ Inserido: id=%d, title='%s'\n", item.id, item.title)
	}

	// 5. Teste de SELECT com Prepared Statement e Filtros
	fmt.Println("\n5. Executando SELECT parametrizado (WHERE is_active = ? AND price > ?)...")
	queryStmt, err := db.PrepareContext(ctx, "SELECT id, title, price FROM test_products WHERE is_active = ? AND price > ? ORDER BY id")
	if err != nil {
		log.Fatalf("Erro ao preparar SELECT: %v", err)
	}
	defer queryStmt.Close()

	rows, err := queryStmt.QueryContext(ctx, true, 100.0)
	if err != nil {
		log.Fatalf("Erro no QueryContext: %v", err)
	}
	defer rows.Close()

	fmt.Println("   Resultados encontrados:")
	for rows.Next() {
		var id int
		var title string
		var price float64
		if err := rows.Scan(&id, &title, &price); err != nil {
			log.Fatalf("Erro no Scan: %v", err)
		}
		fmt.Printf("     [ID: %d] %-20s - R$ %.2f\n", id, title, price)
	}

	// 6. Teste de QueryRow direta com parâmetro
	fmt.Println("\n6. Testando db.QueryRowContext direto com parâmetro...")
	var singleTitle string
	err = db.QueryRowContext(ctx, "SELECT title FROM test_products WHERE id = ?", 2).Scan(&singleTitle)
	if err != nil {
		log.Fatalf("Erro no QueryRowContext: %v", err)
	}
	fmt.Printf("   ✅ Produto com ID=2: '%s'\n", singleTitle)

	// 7. Limpeza
	fmt.Println("\n7. Limpando tabela de teste...")
	_, err = db.ExecContext(ctx, "DROP TABLE test_products")
	if err != nil {
		log.Fatalf("Erro no DROP TABLE: %v", err)
	}
	fmt.Println("   ✅ Tabela removida com sucesso!")

	fmt.Println("\n🎉 TODOS OS TESTES DA FASE 3 (PREPARED STATEMENTS & PARAMETER BINDING) FORAM CONCLUÍDOS COM SUCESSO!")
}
