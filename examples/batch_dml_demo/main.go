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

	fmt.Println("1. Conectando ao IBM Db2 para demonstração de Batch / Array Parameter Binding (DML)...")
	db, err := sql.Open("db2", dsn)
	if err != nil {
		log.Fatalf("Erro ao abrir driver: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro ao conectar ao Db2: %v", err)
	}
	fmt.Println("   ✅ Conexão estabelecida com sucesso!")

	fmt.Println("\n2. Criando tabela de teste 'test_batch_products'...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_batch_products")

	createTable := `CREATE TABLE test_batch_products (
		id INT NOT NULL,
		category VARCHAR(30) NOT NULL,
		sku VARCHAR(50) NOT NULL,
		price DECIMAL(10, 2) NOT NULL,
		in_stock BOOLEAN NOT NULL,
		PRIMARY KEY (id)
	)`
	if _, err := db.ExecContext(ctx, createTable); err != nil {
		log.Fatalf("Erro ao criar tabela de teste: %v", err)
	}
	fmt.Println("   ✅ Tabela 'test_batch_products' criada!")

	fmt.Println("\n3. Preparando matrizes de parâmetros para inserção em lote (Bulk Insert de 100 registros)...")
	numItems := 100
	ids := make([]int, numItems)
	category := "Eletrônicos" // Escalar reutilizado para todo o lote
	skus := make([]string, numItems)
	prices := make([]float64, numItems)
	inStock := make([]bool, numItems)

	for i := 0; i < numItems; i++ {
		ids[i] = 1000 + i
		skus[i] = fmt.Sprintf("SKU-PROD-%04d", 1000+i)
		prices[i] = 49.90 + float64(i)*2.50
		inStock[i] = (i%2 == 0)
	}

	stmt, err := db.PrepareContext(ctx, "INSERT INTO test_batch_products (id, category, sku, price, in_stock) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("Erro ao preparar statement de inserção: %v", err)
	}
	defer stmt.Close()

	start := time.Now()
	res, err := stmt.ExecContext(ctx, ids, category, skus, prices, inStock)
	if err != nil {
		log.Fatalf("Erro ao executar inserção em lote: %v", err)
	}
	elapsed := time.Since(start)

	affected, err := res.RowsAffected()
	if err != nil {
		log.Fatalf("Erro ao obter RowsAffected: %v", err)
	}
	fmt.Printf("   🚀 %d produtos inseridos em lote com sucesso em %v! (RowsAffected: %d)\n", numItems, elapsed, affected)
	if affected != int64(numItems) {
		log.Fatalf("Esperava RowsAffected = %d, obteve %d", numItems, affected)
	}

	fmt.Println("\n4. Validando contagem total e amostra dos registros inseridos...")
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_batch_products").Scan(&count); err != nil {
		log.Fatalf("Erro ao contar registros: %v", err)
	}
	fmt.Printf("   📊 Contagem total na tabela: %d registros confirmados!\n", count)

	rows, err := db.QueryContext(ctx, "SELECT id, category, sku, price, in_stock FROM test_batch_products WHERE id IN (1000, 1050, 1099) ORDER BY id")
	if err != nil {
		log.Fatalf("Erro ao consultar amostra: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var cat, sku string
		var price float64
		var stock bool
		if err := rows.Scan(&id, &cat, &sku, &price, &stock); err != nil {
			log.Fatalf("Erro ao escanear linha: %v", err)
		}
		fmt.Printf("     📦 Produto #%d | Categoria: %-12s | SKU: %-15s | Preço: R$ %6.2f | Em estoque: %t\n", id, cat, sku, price, stock)
	}

	fmt.Println("\n5. Executando atualização em lote (Bulk Update)...")
	updateIDs := []int{1000, 1001, 1002}
	newPrices := []float64{999.99, 888.88, 777.77}
	updateStmt, err := db.PrepareContext(ctx, "UPDATE test_batch_products SET price = ? WHERE id = ?")
	if err != nil {
		log.Fatalf("Erro ao preparar update: %v", err)
	}
	defer updateStmt.Close()

	updRes, err := updateStmt.ExecContext(ctx, newPrices, updateIDs)
	if err != nil {
		log.Fatalf("Erro ao executar update em lote: %v", err)
	}
	updAffected, _ := updRes.RowsAffected()
	fmt.Printf("   🔄 %d registros atualizados em lote! (RowsAffected: %d)\n", len(updateIDs), updAffected)

	fmt.Println("\n6. Limpando tabela de teste...")
	if _, err := db.ExecContext(ctx, "DROP TABLE test_batch_products"); err != nil {
		log.Fatalf("Erro ao remover tabela: %v", err)
	}
	fmt.Println("   ✅ Limpeza concluída com sucesso!")

	fmt.Println("\n🎉 DEMONSTRAÇÃO DE BATCH / ARRAY PARAMETER BINDING CONCLUÍDA COM 100% DE SUCESSO!")
}
