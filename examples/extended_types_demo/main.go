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

	fmt.Println("1. Conectando ao IBM Db2 para demonstração de Tipos Estendidos (DECFLOAT, XML, TIMESTAMP)...")
	db, err := sql.Open("db2", dsn)
	if err != nil {
		log.Fatalf("Erro ao abrir driver: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro ao conectar ao Db2: %v", err)
	}
	fmt.Println("   ✅ Conexão estabelecida com sucesso!")

	fmt.Println("\n2. Criando tabela 'test_extended_types' com DECFLOAT(16), DECFLOAT(34), XML e TIMESTAMP...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_extended_types")

	createTable := `CREATE TABLE test_extended_types (
		id INT NOT NULL,
		rate16 DECFLOAT(16),
		rate34 DECFLOAT(34),
		xml_doc XML,
		recorded_at TIMESTAMP,
		PRIMARY KEY (id)
	)`
	if _, err := db.ExecContext(ctx, createTable); err != nil {
		log.Fatalf("Erro ao criar tabela de tipos estendidos: %v", err)
	}
	fmt.Println("   ✅ Tabela 'test_extended_types' criada!")

	fmt.Println("\n3. Inserindo dados de alta precisão e documento XML...")
	insertSQL := `INSERT INTO test_extended_types (id, rate16, rate34, xml_doc, recorded_at) VALUES (
		1,
		123.456,
		DECFLOAT('98765432109876543210.12345678901234'),
		XMLPARSE(DOCUMENT '<customer id="42"><name>Alice</name><tier>Gold</tier></customer>'),
		'2026-08-27-23.50.00.123456'
	)`
	if _, err := db.ExecContext(ctx, insertSQL); err != nil {
		log.Fatalf("Erro ao inserir registro direto: %v", err)
	}
	fmt.Println("   ✅ Registro #1 inserido com sucesso!")

	fmt.Println("\n4. Inserindo registro #2 via Prepared Statement...")
	stmt, err := db.PrepareContext(ctx, "INSERT INTO test_extended_types (id, rate16, rate34, xml_doc, recorded_at) VALUES (?, ?, ?, XMLPARSE(DOCUMENT CAST(? AS VARCHAR(1000))), ?)")
	if err != nil {
		log.Fatalf("Erro ao preparar statement: %v", err)
	}
	defer stmt.Close()

	xmlPayload := `<customer id="43"><name>Bruno</name><tier>Platinum</tier></customer>`
	now := time.Date(2026, 8, 27, 23, 55, 0, 0, time.UTC)
	if _, err := stmt.ExecContext(ctx, 2, "999.888", "12345678901234567890.99999999999999", xmlPayload, now); err != nil {
		log.Fatalf("Erro ao executar insert com parâmetros: %v", err)
	}
	fmt.Println("   ✅ Registro #2 inserido via Prepared Statement!")

	fmt.Println("\n5. Consultando e validando os dados estendidos...")
	rows, err := db.QueryContext(ctx, "SELECT id, rate16, rate34, XMLSERIALIZE(xml_doc AS VARCHAR(1000)), recorded_at FROM test_extended_types ORDER BY id")
	if err != nil {
		log.Fatalf("Erro ao consultar tabela: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id         int
			rate16     string
			rate34     string
			xmlDoc     string
			recordedAt time.Time
		)
		if err := rows.Scan(&id, &rate16, &rate34, &xmlDoc, &recordedAt); err != nil {
			log.Fatalf("Erro ao escanear linha: %v", err)
		}
		fmt.Printf("   📌 Linha #%d:\n", id)
		fmt.Printf("      - DECFLOAT(16): %s\n", rate16)
		fmt.Printf("      - DECFLOAT(34): %s\n", rate34)
		fmt.Printf("      - XML Serialized: %s\n", xmlDoc)
		fmt.Printf("      - Timestamp: %s\n", recordedAt.Format("2006-01-02 15:04:05.000000"))
	}

	fmt.Println("\n6. Limpando tabela de teste...")
	if _, err := db.ExecContext(ctx, "DROP TABLE test_extended_types"); err != nil {
		log.Fatalf("Erro ao remover tabela: %v", err)
	}
	fmt.Println("   ✅ Limpeza concluída com sucesso!")

	fmt.Println("\n🎉 DEMONSTRAÇÃO DE TIPOS ESTENDIDOS CONCLUÍDA COM 100% DE SUCESSO!")
}
