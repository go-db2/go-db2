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

// Product represents a business entity mapped to a Db2 relational table.
type Product struct {
	ID        int64     `db:"id"`
	SKU       string    `db:"sku"`
	Name      string    `db:"name"`
	Price     float64   `db:"price"`
	Stock     int       `db:"stock"`
	IsActive  bool      `db:"is_active"`
	CreatedAt time.Time `db:"created_at"`
}

func main() {
	dsn := os.Getenv("DB2_DSN")
	if dsn == "" {
		dsn = "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false&block_size=65535"
	}

	fmt.Println("1. Conectando ao IBM Db2 via Pure Go driver (go-db2)...")
	db, err := sql.Open("db2", dsn)
	if err != nil {
		log.Fatalf("Erro ao abrir driver: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro no ping: %v", err)
	}
	fmt.Println("   ✅ Conectado com sucesso ao IBM Db2!")

	fmt.Println("\n2. Criando tabela de produtos 'store_products'...")
	_, _ = db.ExecContext(ctx, "DROP TABLE store_products")
	createTable := `CREATE TABLE store_products (
		id INT NOT NULL,
		sku VARCHAR(30) NOT NULL,
		name VARCHAR(100) NOT NULL,
		price DECIMAL(10, 2) NOT NULL,
		stock INT NOT NULL,
		is_active SMALLINT NOT NULL,
		PRIMARY KEY (id)
	)`
	if _, err := db.ExecContext(ctx, createTable); err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}
	fmt.Println("   ✅ Tabela 'store_products' criada com sucesso!")

	fmt.Println("\n3. Inserindo produtos em lote...")
	insertSQL := "INSERT INTO store_products (id, sku, name, price, stock, is_active) VALUES (?, ?, ?, ?, ?, ?)"
	stmt, err := db.PrepareContext(ctx, insertSQL)
	if err != nil {
		log.Fatalf("Erro ao preparar insert: %v", err)
	}
	defer stmt.Close()

	productsToInsert := []Product{
		{ID: 1, SKU: "TECH-001", Name: "Laptop ThinkPad", Price: 4899.90, Stock: 12, IsActive: true},
		{ID: 2, SKU: "TECH-002", Name: "Monitor Dell 27 4K", Price: 1999.00, Stock: 25, IsActive: true},
		{ID: 3, SKU: "TECH-003", Name: "Teclado Mecânico RGB", Price: 349.50, Stock: 40, IsActive: true},
		{ID: 4, SKU: "TECH-004", Name: "Mouse Ergonômico", Price: 129.90, Stock: 80, IsActive: true},
		{ID: 5, SKU: "TECH-005", Name: "Cabo HDMI 2.1 (Inativo)", Price: 49.90, Stock: 0, IsActive: false},
	}

	for _, p := range productsToInsert {
		activeInt := 0
		if p.IsActive {
			activeInt = 1
		}
		if _, err := stmt.ExecContext(ctx, p.ID, p.SKU, p.Name, p.Price, p.Stock, activeInt); err != nil {
			log.Fatalf("Erro ao inserir produto %s: %v", p.SKU, err)
		}
		fmt.Printf("   ➕ Inserido: [%s] %s - R$ %.2f\n", p.SKU, p.Name, p.Price)
	}

	fmt.Println("\n4. Consultando produtos com mapeamento automático para Structs...")
	querySQL := "SELECT id, sku, name, price, stock, is_active FROM store_products WHERE is_active = ? AND price >= ? ORDER BY id"
	rows, err := db.QueryContext(ctx, querySQL, 1, 300.00)
	if err != nil {
		log.Fatalf("Erro na consulta: %v", err)
	}
	defer rows.Close()

	var catalog []Product
	for rows.Next() {
		var p Product
		var activeInt int
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &activeInt); err != nil {
			log.Fatalf("Erro ao escanear linha: %v", err)
		}
		p.IsActive = (activeInt == 1)
		catalog = append(catalog, p)
	}

	fmt.Printf("   Encontrados %d produtos ativos com preço >= R$ 300.00:\n", len(catalog))
	for _, p := range catalog {
		fmt.Printf("     📦 ID #%d | SKU: %-10s | %-22s | Preço: R$ %8.2f | Estoque: %3d unid\n",
			p.ID, p.SKU, p.Name, p.Price, p.Stock)
	}

	if len(catalog) != 3 {
		log.Fatalf("Esperava 3 produtos, obteve %d", len(catalog))
	}
	fmt.Println("   ✅ Validação dos registros concluída com sucesso!")

	fmt.Println("\n5. Limpando tabela de teste...")
	if _, err := db.ExecContext(ctx, "DROP TABLE store_products"); err != nil {
		log.Fatalf("Erro ao remover tabela: %v", err)
	}
	fmt.Println("   ✅ Limpeza concluída com sucesso!")

	fmt.Println("\n🎉 DEMO DE STRUCT MAPPING E ORM CONCLUÍDA COM SUCESSO!")
}
