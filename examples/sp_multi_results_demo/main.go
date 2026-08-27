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

	fmt.Println("1. Conectando ao IBM Db2 para testes de Multi-Result Sets em Stored Procedures...")
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
	fmt.Println("   ✅ Conectado com sucesso!")

	fmt.Println("\n2. Preparando tabela de teste 'test_users'...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_users")
	if _, err := db.ExecContext(ctx, "CREATE TABLE test_users (id INT NOT NULL, name VARCHAR(50), age INT)"); err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO test_users VALUES (1, 'Alice', 30), (2, 'Bob', 25), (3, 'Charlie', 35), (4, 'Diana', 22)"); err != nil {
		log.Fatalf("Erro ao inserir dados: %v", err)
	}
	fmt.Println("   ✅ Tabela 'test_users' populada com 4 registros!")

	fmt.Println("\n3. Criando Stored Procedure 'sp_multi_results' com 2 Result Sets dinâmicos...")
	createSP := `CREATE OR REPLACE PROCEDURE sp_multi_results (IN min_age INT)
LANGUAGE SQL
DYNAMIC RESULT SETS 2
BEGIN
    DECLARE cur_users CURSOR WITH RETURN TO CALLER FOR
        SELECT id, name, age FROM test_users WHERE age >= min_age ORDER BY id;
    DECLARE cur_summary CURSOR WITH RETURN TO CALLER FOR
        SELECT COUNT(*) AS total_users, AVG(age) AS avg_age FROM test_users WHERE age >= min_age;
    OPEN cur_users;
    OPEN cur_summary;
END`

	if _, err := db.ExecContext(ctx, createSP); err != nil {
		log.Fatalf("Erro ao criar procedure: %v", err)
	}
	fmt.Println("   ✅ Stored Procedure 'sp_multi_results' criada com sucesso!")

	fmt.Println("\n4. Executando CALL sp_multi_results(?) via db.QueryContext...")
	rows, err := db.QueryContext(ctx, "CALL sp_multi_results(?)", 25)
	if err != nil {
		log.Fatalf("Erro ao chamar procedure: %v", err)
	}
	defer rows.Close()

	fmt.Println("\n5. Lendo 1º Result Set (Lista de Usuários com age >= 25)...")
	cols, _ := rows.Columns()
	fmt.Printf("   Colunas do 1º Result Set: %v\n", cols)
	userCount := 0
	for rows.Next() {
		var id, age int
		var name string
		if err := rows.Scan(&id, &name, &age); err != nil {
			log.Fatalf("Erro ao ler linha do 1º result set: %v", err)
		}
		userCount++
		fmt.Printf("     [Usuário #%d] ID: %d | Nome: %s | Idade: %d\n", userCount, id, name, age)
	}

	fmt.Println("\n6. Avançando para o 2º Result Set (rows.NextResultSet())...")
	if rows.NextResultSet() {
		cols2, _ := rows.Columns()
		fmt.Printf("   Colunas do 2º Result Set: %v\n", cols2)
		if rows.Next() {
			var totalUsers int
			var avgAge float64
			if err := rows.Scan(&totalUsers, &avgAge); err != nil {
				log.Fatalf("Erro ao ler linha do 2º result set: %v", err)
			}
			fmt.Printf("     [Resumo] Total Usuários: %d | Média de Idade: %.1f\n", totalUsers, avgAge)
		}
	} else {
		fmt.Println("   (NextResultSet retornou false - ainda precisa de implementação)")
	}

	fmt.Println("\n7. Limpando objetos de teste...")
	_, _ = db.ExecContext(ctx, "DROP PROCEDURE sp_multi_results")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_users")
	fmt.Println("   ✅ Limpeza concluída com sucesso!")
}
