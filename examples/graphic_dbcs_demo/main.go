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

	fmt.Println("1. Conectando ao IBM Db2 para demonstração de Tipos Gráficos & DBCS (GRAPHIC, VARGRAPHIC, CODEUNITS32, DBCLOB)...")
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

	fmt.Println("\n2. Criando tabela de teste 'test_graphic_dbcs' com colunas GRAPHIC, VARGRAPHIC, CODEUNITS32 e DBCLOB...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_graphic_dbcs")

	createTable := `CREATE TABLE test_graphic_dbcs (
		id INT NOT NULL PRIMARY KEY,
		g_code GRAPHIC(10),
		vg_desc VARGRAPHIC(100),
		vg_intl VARGRAPHIC(50 CODEUNITS32),
		dbclob_doc DBCLOB(64K)
	)`
	if _, err := db.ExecContext(ctx, createTable); err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}
	fmt.Println("   ✅ Tabela 'test_graphic_dbcs' criada!")

	fmt.Println("\n3. Inserindo dados gráficos diretos com ideogramas e caracteres internacionais...")
	insertSQL1 := `INSERT INTO test_graphic_dbcs (id, g_code, vg_desc, vg_intl, dbclob_doc) VALUES (
		1,
		'JP-01',
		'Descrição com caracteres gráficos em Japonês: 日本語 (Japanese)',
		'Texto chinês: 数据库系统 (Database Systems)',
		'Documento DBCS: Grande volume de texto gráfico com acentuação e ideogramas: 東京 / 大阪 / 北京'
	)`
	if _, err := db.ExecContext(ctx, insertSQL1); err != nil {
		log.Fatalf("Erro ao inserir registro #1: %v", err)
	}
	fmt.Println("   ✅ Registro #1 inserido com sucesso!")

	fmt.Println("\n4. Inserindo registro #2 parametrizado com Prepared Statement...")
	stmt, err := db.PrepareContext(ctx, "INSERT INTO test_graphic_dbcs (id, g_code, vg_desc, vg_intl, dbclob_doc) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("Erro ao preparar statement: %v", err)
	}
	defer stmt.Close()

	gCode2 := "GLOBAL-02"
	vgDesc2 := "Multi-idioma: São Paulo, Köln, München, Tokyo (東京)"
	vgIntl2 := "Coreano & Árabe: 한국어 (Korean) & العربية"
	dbclob2 := "DBCLOB Documento completo: pure-go DRDA driver com suporte nativo a DBCS e CODEUNITS32."

	if _, err := stmt.ExecContext(ctx, 2, gCode2, vgDesc2, vgIntl2, dbclob2); err != nil {
		log.Fatalf("Erro ao inserir registro #2 via Prepared Statement: %v", err)
	}
	fmt.Println("   ✅ Registro #2 inserido via Prepared Statement!")

	fmt.Println("\n5. Consultando e validando os dados gráficos e DBCS...")
	rows, err := db.QueryContext(ctx, "SELECT id, g_code, vg_desc, vg_intl, dbclob_doc FROM test_graphic_dbcs ORDER BY id")
	if err != nil {
		log.Fatalf("Erro ao consultar registros: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var gCode, vgDesc, vgIntl, dbclob string
		if err := rows.Scan(&id, &gCode, &vgDesc, &vgIntl, &dbclob); err != nil {
			log.Fatalf("Erro ao escanear linha: %v", err)
		}
		fmt.Printf("   📌 Registro #%d:\n", id)
		fmt.Printf("      - GRAPHIC(10):      %q\n", strings.TrimSpace(gCode))
		fmt.Printf("      - VARGRAPHIC(100):  %q\n", vgDesc)
		fmt.Printf("      - CODEUNITS32:      %q\n", vgIntl)
		fmt.Printf("      - DBCLOB(64K):      %q\n", dbclob)
	}

	fmt.Println("\n6. Limpando tabela de teste...")
	if _, err := db.ExecContext(ctx, "DROP TABLE test_graphic_dbcs"); err != nil {
		log.Fatalf("Erro ao limpar tabela: %v", err)
	}
	fmt.Println("   ✅ Limpeza concluída com sucesso!")

	fmt.Println("\n🎉 DEMONSTRAÇÃO DE TIPOS GRÁFICOS & DBCS CONCLUÍDA COM 100% DE SUCESSO!")
}
