package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-db2/go-db2"
)

func main() {
	dsn := os.Getenv("DB2_DSN")
	if dsn == "" {
		dsn = "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"
	}

	fmt.Println("1. Conectando ao IBM Db2 para testes de Stored Procedures...")
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

	fmt.Println("\n2. Criando Stored Procedure 'sp_math_ops' com parâmetros IN, OUT e INOUT...")
	createSP := `CREATE OR REPLACE PROCEDURE sp_math_ops (
    IN a INT,
    IN b INT,
    OUT sum_val INT,
    OUT prod_val INT,
    INOUT msg VARCHAR(100)
)
LANGUAGE SQL
BEGIN
    SET sum_val = a + b;
    SET prod_val = a * b;
    SET msg = 'Sucesso: ' || msg;
END`

	if _, err := db.ExecContext(ctx, createSP); err != nil {
		log.Fatalf("Erro ao criar procedure: %v", err)
	}
	fmt.Println("   ✅ Stored Procedure 'sp_math_ops' criada com sucesso!")

	fmt.Println("\n3. Executando Stored Procedure via Prepared Statement com sql.Out...")
	stmt, err := db.PrepareContext(ctx, "CALL sp_math_ops(?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("Erro ao preparar CALL statement: %v", err)
	}
	defer stmt.Close()

	var sumVal int
	var prodVal int
	var msgVal string = "Operação Alpha"

	_, err = stmt.ExecContext(ctx,
		15,
		4,
		sql.Out{Dest: &sumVal},
		sql.Out{Dest: &prodVal},
		sql.Out{Dest: &msgVal, In: true},
	)
	if err != nil {
		log.Fatalf("Erro ao executar procedure: %v", err)
	}

	fmt.Printf("   Retorno dos parâmetros OUT / INOUT:\n")
	fmt.Printf("     - sum_val (OUT) : %d (esperado: 19)\n", sumVal)
	fmt.Printf("     - prod_val (OUT): %d (esperado: 60)\n", prodVal)
	fmt.Printf("     - msg (INOUT)   : '%s' (esperado: 'Sucesso: Operação Alpha')\n", msgVal)

	if sumVal != 19 || prodVal != 60 || !strings.Contains(msgVal, "Sucesso: Operação Alpha") {
		log.Fatalf("❌ Erro na validação dos parâmetros da procedure!")
	}
	fmt.Println("   ✅ Parâmetros IN, OUT e INOUT validados com sucesso!")

	fmt.Println("\n4. Executando segunda chamada com valores diferentes...")
	var sumVal2 int
	var prodVal2 int
	var msgVal2 string = "Cálculo Beta"

	_, err = stmt.ExecContext(ctx,
		100,
		25,
		sql.Out{Dest: &sumVal2},
		sql.Out{Dest: &prodVal2},
		sql.Out{Dest: &msgVal2, In: true},
	)
	if err != nil {
		log.Fatalf("Erro na segunda chamada da procedure: %v", err)
	}

	fmt.Printf("     - sum_val (OUT) : %d (esperado: 125)\n", sumVal2)
	fmt.Printf("     - prod_val (OUT): %d (esperado: 2500)\n", prodVal2)
	fmt.Printf("     - msg (INOUT)   : '%s'\n", msgVal2)

	if sumVal2 != 125 || prodVal2 != 2500 {
		log.Fatalf("❌ Erro na segunda validação dos parâmetros!")
	}
	fmt.Println("   ✅ Segunda execução validada com sucesso!")

	fmt.Println("\n5. Limpando Stored Procedure de teste...")
	if _, err := db.ExecContext(ctx, "DROP PROCEDURE sp_math_ops"); err != nil {
		log.Fatalf("Erro ao remover procedure: %v", err)
	}
	fmt.Println("   ✅ Stored Procedure removida com sucesso!")

	fmt.Println("\n🎉 TODOS OS TESTES DE STORED PROCEDURES COM sql.Out FORAM CONCLUÍDOS COM SUCESSO!")
}
