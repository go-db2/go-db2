# Plano de Projeto: `go-db2` (Pure Go Driver para IBM Db2)

> **Documento em Português do Brasil** | *Available in [English](PROJECT_PLAN.md)*

---

## 1. Visão Geral e Motivação

### 1.1 O Problema
Atualmente, a principal forma de conectar aplicações em Go ao **IBM Db2** é por meio do driver `ibmdb/go_ibm_db`. No entanto, esse driver é um wrapper CGO sobre o **IBM DB2 CLI / ODBC Driver (`clidriver`)**, trazendo sérias desvantagens:
- **Dependência de CGO:** Desabilita compilação estática (`CGO_ENABLED=0`), tornando *cross-compilation* difícil.
- **Ambientes de Deploy Complexos:** Imagens Docker tornam-se pesadas e frágeis; requer instalação manual de pacotes e bibliotecas C, incompatível com imagens mínimas (*distroless*, *scratch*) e com o ecossistema Alpine Linux (`musl` vs `glibc`).
- **Incompatibilidade Arquitetural:** O `clidriver` da IBM tem suporte limitado e burocrático a certas arquiteturas (como Apple Silicon / ARM64 no macOS e Linux ARM).
- **Ambientes Serverless / Edge:** Dificuldade extrema de execução em AWS Lambda, Cloud Run ou Kubernetes otimizado.

### 1.2 A Solução
Criar o **`go-db2`**: um driver de banco de dados para IBM Db2 escrito **100% em Go puro (*Pure Go*)**, que:
1. Comunica-se diretamente via TCP/TLS utilizando o protocolo **DRDA** (*Distributed Relational Database Architecture*) / **DDM** (*Distributed Data Management*).
2. É totalmente compatível com a biblioteca padrão [`database/sql`](https://pkg.go.dev/database/sql) e [`database/sql/driver`](https://pkg.go.dev/database/sql/driver).
3. Não requer instalação do `clidriver` nem qualquer biblioteca proprietária da IBM no sistema operacional.

---

## 2. Aprendizados e Referências Base

O projeto utilizará como referências fundamentais dois projetos de sucesso:

```
┌────────────────────────────────────────┐       ┌────────────────────────────────────────┐
│               pydrda                   │       │                go-ora                  │
│    (Referência de Protocolo DRDA)      │       │     (Referência de Arquitetura Go)     │
├────────────────────────────────────────┤       ├────────────────────────────────────────┤
│ • Handshake DRDA (EXCSAT, ACCSEC, etc) │       │ • Arquitetura database/sql/driver Go   │
│ • Codepoints e comandos DDM            │       │ • Gerenciamento de pacotes / network   │
│ • Mapeamento de tipos SQL do Db2       │       │ • Suporte a Context (Cancel/Timeout)   │
│ • Criptografia de Auth (SECMEC 3, 7, 9)│       │ • Conversão robusta de tipos Go <-> DB │
│ • Formato de dados binários/EBCDIC     │       │ • Connection String & DSN Parser       │
└────────────────────────────────────────┘       └────────────────────────────────────────┘
                                     │               │
                                     ▼               ▼
                        ┌────────────────────────────────────────┐
                        │                go-db2                  │
                        │      Driver Pure Go para IBM Db2       │
                        └────────────────────────────────────────┘
```

### 2.1 O que extrair do `pydrda` (Python):
- **Protocolo de Rede:** Compreensão exata da sequência de mensagens DRDA:
  - `EXCSAT` (*Exchange Server Attributes*)
  - `ACCSEC` (*Access Security*)
  - `SECCHK` (*Security Check*)
  - `ACCRDB` (*Access Relational Database*)
  - `PRPSQLSTT` (*Prepare SQL Statement*)
  - `EXCSQLSTT` (*Execute SQL Statement*)
  - `OPNQRY` (*Open Query*) / `CNTQRY` (*Continue Query*) / `CLSQRY` (*Close Query*)
  - `RDBCMM` (*RDB Commit*) / `RDBRLLBCK` (*RDB Rollback*)
- **Dicionário de Codepoints:** Catálogo completo de códigos hexadecimais dos comandos DRDA.
- **Tipos de Dados:** Constantes de tipos (ex: `DB2_SQLTYPE_VARCHAR`, `DB2_SQLTYPE_TIMESTAMP`, `DB2_SQLTYPE_BLOB`, etc.) e serialização de parâmetros com descritores de formato.

### 2.2 O que extrair do `go-ora` (Go):
- **Design de Driver Idiomático em Go:**
  - Implementação das interfaces `driver.Driver`, `driver.DriverContext`, `driver.Connector`, `driver.Conn`, `driver.Stmt`, `driver.Tx`, `driver.Rows`, `driver.Result`.
- **Camada de Rede (`network/`):**
  - Gerenciamento de buffers de leitura e escrita com `sync.Pool` para alocação mínima de memória.
  - Tratamento nativo de conexões TCP e TLS (`crypto/tls`).
- **Sistema de Tipos e Converters (`converters/`):**
  - Mapeamento bidirecional seguro entre tipos nativos do Go (`int64`, `float64`, `string`, `time.Time`, `[]byte`, `driver.Valuer`) e os formatos binários do banco de dados.
- **Suporte a Contextos:**
  - Cancelamento de queries e controle de timeouts via `context.Context`.

---

## 3. Arquitetura Proposta do Projeto

```
go-db2/
├── go.mod
├── README.md
├── PROJECT_PLAN.md           # Versão em inglês
├── PROJECT_PLAN.pt-BR.md     # Versão em português
├── driver.go                 # Registro do driver (sql.Register("db2", ...))
├── connector.go              # Implementação de driver.Connector
├── connection.go             # Implementação de driver.Conn, Ping, ExecDirect
├── statement.go              # Implementação de driver.Stmt e query parametrizada
├── transaction.go            # Implementação de driver.Tx (Commit e Rollback)
├── rows.go                   # Implementação de driver.Rows e metadados de colunas
├── config.go                 # DSN / Connection String parser (URL e Key-Value)
├── errors.go                 # Tratamento de erros, SQLCODE e SQLSTATE do Db2
├── network/                  # Camada de comunicação de rede e protocolo
│   ├── session.go            # Sessão TCP/TLS, buffer pooling e controle de fluxo
│   ├── dss.go                # Data Stream Structure (Request/Reply/Object DSS)
│   ├── ddm.go                # Parser e encoder de comandos e objetos DDM
│   ├── codepoint.go          # Tabela de constantes de Codepoints DRDA
│   └── security/             # Mecanismos de autenticação (SECMEC)
│       ├── secmec.go         # Interface dos mecanismos de segurança
│       ├── plain.go          # SECMEC 3 (USRIDPWD - Texto plano)
│       ├── des.go            # SECMEC 9 (USRENCPWD - Criptografia DES)
│       └── aes.go            # SECMEC 7 (USRENCPWD - Criptografia AES)
├── converters/               # Conversão de tipos Go <-> Formatos Db2
│   ├── encoders.go           # Go Types -> Db2 Wire Format
│   ├── decoders.go           # Db2 Wire Format -> Go Types
│   ├── ebcdic.go             # Utilitários para conversão EBCDIC / ASCII / UTF-8
│   └── numeric.go            # Conversão de inteiros, floats e Packed Decimal
├── types/                    # Tipos ricos específicos do Db2
│   ├── null_types.go         # Tipos nulos customizados (se necessário)
│   ├── lob.go                # Tratamento de BLOB, CLOB, DBCLOB
│   └── decimal.go            # Suporte a decimais de alta precisão
└── examples/                 # Exemplos de uso práticos
    ├── basic_query/
    └── prepared_statement/
```

---

## 4. Escopo de Funcionalidades

### 4.1 Funcionalidades Essenciais (Core)
- [ ] **Connection String / DSN:**
  - Formato URL: `db2://user:pass@host:port/dbname?ssl=true&timeout=30s`
  - Formato DSN: `host=localhost;port=50000;database=testdb;user=db2inst1;password=secret;`
- [ ] **Handshake DRDA:**
  - `EXCSAT` (Troca de atributos cliente/servidor).
  - `ACCSEC` e autenticação com `SECCHK`.
  - `ACCRDB` (Abertura e fixação do banco de dados).
- [ ] **Suporte a Tipos de Dados Básicos:**
  - Inteiros: `SMALLINT` (int16), `INTEGER` (int32), `BIGINT` (int64).
  - Ponto Flutuante: `REAL` (float32), `DOUBLE` (float64).
  - Texto: `CHAR`, `VARCHAR`, `LONG VARCHAR`.
  - Lógicos / Nulos: Detecção de colunas `NULL` (`sql.NullString`, `sql.NullInt64`, etc.).
- [ ] **Execução de Instruções SQL:**
  - `Exec` / `ExecContext` para DDL e DML (`CREATE`, `INSERT`, `UPDATE`, `DELETE`).
  - `Query` / `QueryContext` para DQL (`SELECT`).
  - Retorno de metadados de colunas (`Rows.Columns()`, `Rows.ColumnTypeScanType()`).
- [ ] **Transações:**
  - `Begin` / `BeginTx` com `Commit` e `Rollback`.
- [ ] **Prepared Statements:**
  - `Prepare` / `PrepareContext` com suporte a marcadores posicionais `?`.

### 4.2 Funcionalidades Avançadas
- [ ] **Segurança e Criptografia:**
  - Conexão criptografada via SSL/TLS nativa (`crypto/tls`).
  - Suporte a múltiplos mecanismos de autenticação DRDA:
    - `SECMEC 3` (Plain text User/Password).
    - `SECMEC 7` (AES Encrypted Password).
    - `SECMEC 9` (DES Encrypted Password - compatível com `pydrda`).
- [ ] **Tipos Avançados do Db2:**
  - Temporais: `DATE`, `TIME`, `TIMESTAMP` (mapeados para `time.Time`).
  - Alta Precisão: `DECIMAL` / `NUMERIC` (Packed Decimal format).
  - Binários e LOBs: `BLOB`, `CLOB`, `DBCLOB`.
  - Gráficos / Multibyte: `GRAPHIC`, `VARGRAPHIC`.
- [ ] **Performance e Otimização:**
  - *Block Fetching* (recuperação de múltiplos registros por pacote de rede DRDA).
  - `sync.Pool` para buffers de I/O de rede.
  - Zero alocações em *hot paths* de conversão de dados.
- [ ] **Stored Procedures:**
  - Chamada de Stored Procedures com parâmetros `IN`, `OUT`, `INOUT`.

---

## 5. Roadmap de Desenvolvimento

```mermaid
gantt
    title Roadmap de Desenvolvimento do go-db2
    dateFormat  YYYY-MM-DD
    section Fase 1: Fundação & Handshake
    Estruturação do Módulo & DSN Parser      :f1_1, 2026-09-01, 7d
    Camada de Rede (DSS & DDM Encoders)       :f1_2, after f1_1, 10d
    Handshake DRDA & Auth Básica (SECMEC 3)  :f1_3, after f1_2, 10d
    section Fase 2: CRUD & database/sql
    Implementação driver.Driver & Conn       :f2_1, after f1_3, 7d
    Exec Direct (DDL/DML) & Simple Select    :f2_2, after f2_1, 10d
    Mapeamento de Tipos Primitivos           :f2_3, after f2_2, 7d
    Transações (Commit / Rollback)           :f2_4, after f2_3, 5d
    section Fase 3: Tipos Completos & Params
    Prepared Statements (?)                  :f3_1, after f2_4, 10d
    Tipos Temporais (Date/Time/Timestamp)    :f3_2, after f3_1, 7d
    Decimais (Packed Decimal) & LOBs         :f3_3, after f3_2, 10d
    section Fase 4: Segurança & Produção
    TLS/SSL & SECMEC 9/7 Encrypted Auth      :f4_1, after f3_3, 10d
    Suporte a Context & Cancelamento         :f4_2, after f4_1, 7d
    Testes de Integração & Benchmarks        :f4_3, after f4_2, 10d
    Release v0.1.0 (MVP Público)             :milestone, after f4_3, 0d
```

### Detalhamento dos Milestones

| Milestone | Objetivo | Entregáveis |
| :--- | :--- | :--- |
| **M1 - Protocol Foundation** | Estabelecer conexão básica com o Db2 via socket | Camada de pacotes DSS/DDM, envio de `EXCSAT`, `ACCSEC`, `SECCHK`, `ACCRDB` e fechamento de sessão limpo. |
| **M2 - Minimal `database/sql`** | Executar comandos SQL simples e receber resultados | Driver registrado no Go; suporte a `db.Ping()`, `db.Exec("CREATE TABLE...")` e `db.Query("SELECT 1 FROM ...")`. |
| **M3 - Tipos & Statements** | Suporte completo a operações de CRUD do dia a dia | `db.Prepare()`, placeholders `?`, conversão de tipos inteiros, texto, datas, decimais e booleanos. |
| **M4 - Segurança & LOBs** | Prontidão para ambientes corporativos e nuvem | Suporte a SSL/TLS, senhas criptografadas (SECMEC 9/7), leitura e gravação de `BLOB`/`CLOB`. |
| **M5 - Qualidade & Release** | Testes automatizados robustos e documentação | Testes automatizados com Testcontainers (Docker Db2), benchmarks de alocação de memória e release inicial no GitHub. |

---

## 6. Estratégia de Testes e Ambiente de Desenvolvimento

Para garantir validação contínua e testes locais:

1. **Ambiente Local com Podman / Docker:**
   Inicializar a imagem oficial do IBM Db2 Community Edition com as configurações validadas:
   ```bash
   podman run -d \
     --name db2-test \
     --privileged \
     -p 50000:50000 \
     -e LICENSE=accept \
     -e DB2INST1_PASSWORD=MinhaSenhaForte123 \
     -e DBNAME=testdb \
     -e PERSISTENT_HOME=false \
     icr.io/db2_community/db2
   ```
   > Aguarde a mensagem `(*) Setup has completed.` nos logs (`podman logs -f db2-test`).

2. **Testes Unitários e Mock (Isolados):**
   - Testes isolados com tabelas de bytes e servidor TCP mock sem depender do banco online:
     ```bash
     go test -v ./...
     ```

3. **Execução de Teste Real com o Driver:**
   - Executar o programa de validação de handshake e transações reais contra o contêiner:
     ```bash
     go run examples/basic_connect/main.go
     ```

4. **Testes de Integração Automatizados em Go (Futuro):**
   - Integração com a biblioteca `testcontainers-go` para inicializar automaticamente contêineres Db2 durante a execução da suite `go test ./...`.

5. **CI/CD no GitHub Actions:**
   - Pipeline automatizado de linting (`golangci-lint`), testes de unidade e testes de integração com o serviço de contêiner Db2.

---

## 7. Próximos Passos Imediatos

1. Inicializar o módulo Go no diretório `go-db2`:
   ```bash
   go mod init github.com/go-db2/go-db2
   ```
2. Criar a estrutura inicial de pacotes (`network/`, `converters/`, `types/`).
3. Portar e catalogar a tabela de **Codepoints DRDA** de `pydrda/drda/codepoint.py` para Go (`network/codepoint.go`).
4. Implementar a estrutura de pacotes **DSS (Data Stream Structure)** e o primeiro teste de handshake (`EXCSAT`).
