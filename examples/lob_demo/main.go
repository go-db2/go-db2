package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-db2/go-db2"
)

func main() {
	connStr := "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"

	fmt.Println("1. Conectando ao IBM Db2 para testes da Fase 4 (LOBs - BLOB e CLOB)...")
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
	fmt.Println("   ✅ Conectado com sucesso!")

	// 2. Criar tabela com colunas LOB (BLOB e CLOB)
	fmt.Println("\n2. Criando tabela 'test_lobs' com BLOB(1M) e CLOB(1M)...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_lobs")

	createSQL := `CREATE TABLE test_lobs (
		id INT NOT NULL,
		title VARCHAR(100) NOT NULL,
		image_data BLOB(1M),
		long_notes CLOB(1M),
		PRIMARY KEY(id)
	)`
	_, err = db.ExecContext(ctx, createSQL)
	if err != nil {
		log.Fatalf("Erro ao criar tabela: %v", err)
	}
	fmt.Println("   ✅ Tabela criada com sucesso!")

	// 3. Gerar dados de teste (BLOB binário e CLOB texto longo)
	blobSample1 := make([]byte, 1024*4) // 4 KB de bytes binários
	_, _ = rand.Read(blobSample1)

	clobSample1 := strings.Repeat("O driver Pure Go go-db2 suporta objetos grandes CLOB e BLOB de forma nativa! ", 30)

	blobSample2 := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R'}
	clobSample2 := "Documento de especificações técnicas do protocolo DRDA / IBM Db2."

	// 4. Inserir registros via Prepared Statement com parâmetros BLOB e CLOB
	fmt.Println("\n3. Inserindo registros com BLOB e CLOB via Prepared Statement...")
	insertStmt, err := db.PrepareContext(ctx, "INSERT INTO test_lobs (id, title, image_data, long_notes) VALUES (?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("Erro no db.Prepare: %v", err)
	}
	defer insertStmt.Close()

	_, err = insertStmt.ExecContext(ctx, 1, "Relatório 16KB", blobSample1, clobSample1)
	if err != nil {
		log.Fatalf("Erro ao inserir registro 1: %v", err)
	}
	fmt.Printf("   ✅ Inserido ID 1: BLOB (%d bytes), CLOB (%d caracteres)\n", len(blobSample1), len(clobSample1))

	_, err = insertStmt.ExecContext(ctx, 2, "Imagem PNG Pequena", blobSample2, clobSample2)
	if err != nil {
		log.Fatalf("Erro ao inserir registro 2: %v", err)
	}
	fmt.Printf("   ✅ Inserido ID 2: BLOB (%d bytes), CLOB (%d caracteres)\n", len(blobSample2), len(clobSample2))

	// Inserir registro com LOBs nulos
	_, err = insertStmt.ExecContext(ctx, 3, "Registro Sem LOBs", nil, nil)
	if err != nil {
		log.Fatalf("Erro ao inserir registro 3: %v", err)
	}
	fmt.Println("   ✅ Inserido ID 3: BLOB (NULL), CLOB (NULL)")

	// 5. Consultar e verificar integridade dos dados LOB recuperados
	fmt.Println("\n4. Consultando e validando integridade dos dados LOB...")
	rows, err := db.QueryContext(ctx, "SELECT id, title, image_data, long_notes FROM test_lobs ORDER BY id")
	if err != nil {
		log.Fatalf("Erro no db.Query: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int
		var title string
		var blobVal []byte
		var clobVal *string

		if err := rows.Scan(&id, &title, &blobVal, &clobVal); err != nil {
			log.Fatalf("Erro no rows.Scan: %v", err)
		}

		clobStr := "NULL"
		if clobVal != nil {
			if len(*clobVal) > 40 {
				clobStr = fmt.Sprintf("'%s...' (%d chars)", (*clobVal)[:40], len(*clobVal))
			} else {
				clobStr = fmt.Sprintf("'%s'", *clobVal)
			}
		}

		fmt.Printf("   [ID: %d] Title: '%-20s' | BLOB: %d bytes | CLOB: %s\n", id, title, len(blobVal), clobStr)

		// Validações de integridade
		if id == 1 {
			if !bytes.Equal(blobVal, blobSample1) {
				log.Fatalf("❌ Erro de integridade: BLOB do ID 1 difere dos bytes inseridos!")
			}
			if clobVal == nil || *clobVal != clobSample1 {
				log.Fatalf("❌ Erro de integridade: CLOB do ID 1 difere do texto inserido!")
			}
		} else if id == 2 {
			if !bytes.Equal(blobVal, blobSample2) {
				log.Fatalf("❌ Erro de integridade: BLOB do ID 2 difere dos bytes inseridos!")
			}
			if clobVal == nil || *clobVal != clobSample2 {
				log.Fatalf("❌ Erro de integridade: CLOB do ID 2 difere do texto inserido!")
			}
		} else if id == 3 {
			if blobVal != nil && len(blobVal) > 0 {
				log.Fatalf("❌ Esperado BLOB NULL para ID 3, obtido %d bytes", len(blobVal))
			}
			if clobVal != nil {
				log.Fatalf("❌ Esperado CLOB NULL para ID 3, obtido texto")
			}
		}
		count++
	}

	if count != 3 {
		log.Fatalf("Esperado 3 registros, obtido %d", count)
	}
	fmt.Println("   ✅ Integridade dos dados binários (BLOB) e textuais (CLOB) validada bit a bit!")

	// 6. Limpeza
	fmt.Println("\n5. Limpando tabela de teste...")
	_, _ = db.ExecContext(ctx, "DROP TABLE test_lobs")
	fmt.Println("   ✅ Tabela removida com sucesso!")

	fmt.Println("\n🎉 TODOS OS TESTES DA FASE 4 (LOBS, BLOB, CLOB & SEGURANÇA) FORAM CONCLUÍDOS COM SUCESSO!")
}
