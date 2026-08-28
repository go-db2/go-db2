package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-db2/go-db2"
	"github.com/go-db2/go-db2/network/security"
)

func main() {
	fmt.Println("1. Demonstração de Autenticação Kerberos / GSSAPI (SECMEC 7/11) no go-db2...")

	// 1. Validar módulo de geração de tokens GSSAPI/Kerberos
	spn := security.FormatServicePrincipal("", "db2server.corp.local", "CORP.LOCAL")
	fmt.Printf("   📌 SPN de Serviço Resolvido: %s\n", spn)

	// 1.1 Demonstração de Keytab Binário (.keytab)
	fmt.Println("\n   🔑 Validando leitura e autenticação via arquivo Keytab (.keytab)...")
	tmpKeytab, err := os.CreateTemp("", "demo_db2_*.keytab")
	if err != nil {
		log.Fatalf("Erro ao criar temp keytab: %v", err)
	}
	defer os.Remove(tmpKeytab.Name())

	entries := []security.KeytabEntry{
		{
			Principal: "db2/db2server.corp.local@CORP.LOCAL",
			KeyType:   18, // AES256-CTS-HMAC-SHA1-96
			KVNO:      1,
			Key:       []byte("0123456789abcdef0123456789abcdef"),
		},
		{
			Principal: "db2appservice@CORP.LOCAL",
			KeyType:   18,
			KVNO:      2,
			Key:       []byte("fedcba9876543210fedcba9876543210"),
		},
	}
	keytabBytes, err := security.BuildKeytab(entries...)
	if err != nil {
		log.Fatalf("Erro ao gerar keytab binário: %v", err)
	}
	if _, err := tmpKeytab.Write(keytabBytes); err != nil {
		log.Fatalf("Erro ao salvar keytab: %v", err)
	}
	_ = tmpKeytab.Close()

	// Carregar e validar token a partir do arquivo .keytab
	keytabKrbCfg := security.KerberosConfig{
		KeytabFile:       tmpKeytab.Name(),
		ServicePrincipal: spn,
		Host:             "db2server.corp.local",
	}
	keytabToken, err := security.AcquireKerberosToken(keytabKrbCfg)
	if err != nil {
		log.Fatalf("Erro ao adquirir token via keytab: %v", err)
	}
	fmt.Printf("   ✅ Arquivo Keytab validado com sucesso! Token GSSAPI gerado (%d bytes, header=0x%02X)\n", len(keytabToken), keytabToken[0])

	// 2. Testar conexão com o banco Db2 local via DSN
	connStr := "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false&security_mechanism=dh_encrypted_password"
	fmt.Println("\n2. Conectando ao IBM Db2 local com mecanismo de autenticação negociado...")
	db, err := sql.Open("db2", connStr)
	if err != nil {
		log.Fatalf("Erro no sql.Open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Erro no Ping: %v", err)
	}
	fmt.Println("   ✅ Conexão estabelecida com sucesso!")

	// 3. Consultar informações da sessão
	var currentUser, currentServer string
	err = db.QueryRowContext(ctx, "SELECT CURRENT USER, CURRENT SERVER FROM SYSIBM.SYSDUMMY1").Scan(&currentUser, &currentServer)
	if err != nil {
		log.Fatalf("Erro na consulta: %v", err)
	}
	fmt.Printf("   👤 Usuário Autenticado: %s\n", currentUser)
	fmt.Printf("   🏢 Servidor Db2: %s\n", currentServer)

	// 4. Demonstrar DSN formatada para Kerberos SSO corporativo
	fmt.Println("\n3. Formatos de DSN suportados para Kerberos SSO (Active Directory / MIT Kerberos):")
	fmt.Println("   • Via Ticket Cache (ccache / kinit):")
	fmt.Println("     db2://db2host:50000/MYDB?security_mechanism=kerberos&spn=db2/db2host@CORP.LOCAL")
	fmt.Println("   • Via Keytab (Serviços e Background Jobs):")
	fmt.Println("     db2://db2host:50000/MYDB?security_mechanism=kerberos&krb5_keytab=/etc/db2.keytab&spn=db2/db2host@CORP.LOCAL")
	fmt.Println("   • Key-Value DSN:")
	fmt.Println("     host=db2host;port=50000;database=MYDB;secmec=kerberos;spn=db2/db2host@CORP.LOCAL;")

	fmt.Println("\n🎉 DEMONSTRAÇÃO DE AUTENTICAÇÃO KERBEROS / GSSAPI CONCLUÍDA COM 100% DE SUCESSO!")
	_ = os.Stdout.Sync()
}
