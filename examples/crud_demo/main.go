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

	fmt.Println("1. Conectando ao IBM Db2 via Pure Go driver...")
	db, err := sql.Open("db2", connStr)
	if err != nil {
		log.Fatalf("Erro no sql.Open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro no Ping: %v", err)
	}
	fmt.Println("   ✅ Ping e Handshake OK!")

	// 2. Query do Dummy System Table
	fmt.Println("\n2. Executando consulta básica (SELECT 1 FROM SYSIBM.SYSDUMMY1)...")
	var dummyVal int
	err = db.QueryRowContext(ctx, "SELECT 1 FROM SYSIBM.SYSDUMMY1").Scan(&dummyVal)
	if err != nil {
		log.Fatalf("Erro no QueryRow SYSDUMMY1: %v", err)
	}
	fmt.Printf("   ✅ Resultado do SYSDUMMY1: %d\n", dummyVal)

	// 3. Limpeza de tabela anterior se existir
	fmt.Println("\n3. Preparando tabela de teste (DROP TABLE / CREATE TABLE)...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_users")

	// 4. Criação da Tabela
	_, err = db.ExecContext(ctx, "CREATE TABLE test_users (id INT NOT NULL, name VARCHAR(50), age INT, PRIMARY KEY(id))")
	if err != nil {
		log.Fatalf("Erro no CREATE TABLE: %v", err)
	}
	fmt.Println("   ✅ Tabela 'test_users' criada com sucesso!")

	// 5. Inserções DML
	fmt.Println("\n4. Inserindo registros (INSERT INTO test_users)...")
	inserts := []string{
		"INSERT INTO test_users (id, name, age) VALUES (1, 'Alice', 30)",
		"INSERT INTO test_users (id, name, age) VALUES (2, 'Bob', 25)",
		"INSERT INTO test_users (id, name, age) VALUES (3, 'Charlie', NULL)",
	}

	for _, ins := range inserts {
		_, err := db.ExecContext(ctx, ins)
		if err != nil {
			log.Fatalf("Erro ao inserir: %s -> %v", ins, err)
		}
	}
	fmt.Println("   ✅ 3 registros inseridos com sucesso!")

	// 6. Consulta de Registros (SELECT)
	fmt.Println("\n5. Consultando registros da tabela (SELECT id, name, age FROM test_users ORDER BY id)...")
	rows, err := db.QueryContext(ctx, "SELECT id, name, age FROM test_users ORDER BY id")
	if err != nil {
		log.Fatalf("Erro no Query: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		log.Fatalf("Erro ao obter colunas: %v", err)
	}
	fmt.Printf("   Colunas retornadas: %v\n", cols)

	fmt.Println("   Linhas encontradas:")
	count := 0
	for rows.Next() {
		count++
		var id int
		var name string
		var age sql.NullInt64

		if err := rows.Scan(&id, &name, &age); err != nil {
			log.Fatalf("Erro no Scan da linha %d: %v", count, err)
		}

		ageStr := "NULL"
		if age.Valid {
			ageStr = fmt.Sprintf("%d", age.Int64)
		}
		fmt.Printf("     [%d] id: %d | name: %-10s | age: %s\n", count, id, name, ageStr)
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Erro na iteração dos rows: %v", err)
	}
	fmt.Printf("   ✅ Total de %d linhas lidas com sucesso!\n", count)

	// 7. Limpeza (DROP TABLE)
	fmt.Println("\n6. Limpando tabela de teste (DROP TABLE)...")
	_, err = db.ExecContext(ctx, "DROP TABLE test_users")
	if err != nil {
		log.Fatalf("Erro no DROP TABLE final: %v", err)
	}
	fmt.Println("   ✅ Tabela removida com sucesso!")

	fmt.Println("\n🎉 TODOS OS TESTES DA FASE 2 (DDL, DML, DQL) FORAM EXECUTADOS COM ÊXITO!")
}
