package db2

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-db2/go-db2/network"
)

func TestSQLHelpers(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		isQuery  bool
		isInsert bool
	}{
		{
			name:     "simple select",
			sql:      "SELECT * FROM USERS",
			isQuery:  true,
			isInsert: false,
		},
		{
			name:     "select with leading comments and whitespace",
			sql:      "  \n -- get users\n /* multi line\ncomment */ \n SELECT id FROM USERS",
			isQuery:  true,
			isInsert: false,
		},
		{
			name:     "cte with query",
			sql:      "WITH cte AS (SELECT 1 FROM SYSIBM.SYSDUMMY1) SELECT * FROM cte",
			isQuery:  true,
			isInsert: false,
		},
		{
			name:     "values expression",
			sql:      "VALUES (1, 'test')",
			isQuery:  true,
			isInsert: false,
		},
		{
			name:     "table expression",
			sql:      "TABLE(SELECT 1 FROM SYSIBM.SYSDUMMY1)",
			isQuery:  true,
			isInsert: false,
		},
		{
			name:     "create table DDL",
			sql:      "CREATE TABLE USERS (ID INT NOT NULL PRIMARY KEY, NAME VARCHAR(100))",
			isQuery:  false,
			isInsert: false,
		},
		{
			name:     "create table with comments",
			sql:      "/* create user table */ CREATE TABLE USERS (ID INT)",
			isQuery:  false,
			isInsert: false,
		},
		{
			name:     "insert statement",
			sql:      "INSERT INTO USERS (ID, NAME) VALUES (1, 'Alice')",
			isQuery:  false,
			isInsert: true,
		},
		{
			name:     "insert statement with comment",
			sql:      "-- insert new user\nINSERT INTO USERS VALUES (1)",
			isQuery:  false,
			isInsert: true,
		},
		{
			name:     "update statement",
			sql:      "UPDATE USERS SET NAME = 'Bob' WHERE ID = 1",
			isQuery:  false,
			isInsert: false,
		},
		{
			name:     "delete statement",
			sql:      "DELETE FROM USERS WHERE ID = 1",
			isQuery:  false,
			isInsert: false,
		},
		{
			name:     "drop table DDL",
			sql:      "DROP TABLE USERS",
			isQuery:  false,
			isInsert: false,
		},
		{
			name:     "alter table DDL",
			sql:      "ALTER TABLE USERS ADD COLUMN AGE INT",
			isQuery:  false,
			isInsert: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isQueryStatement(tt.sql); got != tt.isQuery {
				t.Errorf("isQueryStatement(%q) = %v; want %v", tt.sql, got, tt.isQuery)
			}
			if got := isInsertQuery(tt.sql); got != tt.isInsert {
				t.Errorf("isInsertQuery(%q) = %v; want %v", tt.sql, got, tt.isInsert)
			}
		})
	}
}

func TestQueryContextWithDDLDirect(t *testing.T) {
	// Start a mock Db2 TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Run mock server
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()

				// 1. Read EXCSAT + ACCSEC
				for {
					hdr, cp, _, _, err := network.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == network.CodePointACCSEC || !hdr.Chained {
						break
					}
				}

				// 2. Respond with ACCSECRD
				accsecRdBody := network.PackUint16(network.CodePointSECMEC, network.SecMecUSRIDPWD)
				accsecRdObj := network.PackDDMObject(network.CodePointACCSECRD, accsecRdBody)
				dssHdr := network.BuildDSSHeader(len(accsecRdObj), network.DSSTypeReply, false, false, false, 1)
				if _, err := c.Write(append(dssHdr, accsecRdObj...)); err != nil {
					return
				}

				// 3. Read SECCHK + ACCRDB
				for {
					hdr, cp, _, _, err := network.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == network.CodePointACCRDB || !hdr.Chained {
						break
					}
				}

				// 4. Respond with SECCHKRM + ACCRDBRM
				secchkRmBody := network.PackBytes(network.CodePointSECCHKCD, []byte{0x00})
				secchkRmObj := network.PackDDMObject(network.CodePointSECCHKRM, secchkRmBody)
				accrdbRmObj := network.PackDDMObject(network.CodePointACCRDBRM, nil)

				hdr1 := network.BuildDSSHeader(len(secchkRmObj), network.DSSTypeReply, true, false, false, 1)
				hdr2 := network.BuildDSSHeader(len(accrdbRmObj), network.DSSTypeReply, false, false, false, 1)

				var respBuf bytes.Buffer
				respBuf.Write(hdr1)
				respBuf.Write(secchkRmObj)
				respBuf.Write(hdr2)
				respBuf.Write(accrdbRmObj)
				if _, err := c.Write(respBuf.Bytes()); err != nil {
					return
				}

				// 5. Read post-connect setup packets
				for {
					hdr, cp, _, _, err := network.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == network.CodePointRDBCMM || !hdr.Chained {
						break
					}
				}

				// Respond with SQLCARD (SQLCODE=0)
				sqlcardPayload := make([]byte, 20)
				sqlcardPayload[0] = 0x00
				binary.LittleEndian.PutUint32(sqlcardPayload[1:5], 0)
				copy(sqlcardPayload[5:10], "00000")
				sqlcardObj := network.PackDDMObject(network.CodePointSQLCARD, sqlcardPayload)
				sqlcardHdr := network.BuildDSSHeader(len(sqlcardObj), network.DSSTypeReply, false, false, false, 1)
				if _, err := c.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
					return
				}

				// Loop to handle EXCSQLIMM for DDL (CREATE TABLE)
				for {
					_, cp, _, _, err := network.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == network.CodePointEXCSQLIMM {
						// Read chained SQLSTT and RDBCMM
						for {
							hdr, _, _, _, err := network.ReadDSS(c)
							if err != nil || !hdr.Chained {
								break
							}
						}

						// Respond with SQLCARD (SQLCODE=0)
						if _, err := c.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
							return
						}
					}
				}
			}(conn)
		}
	}()

	connStr := fmt.Sprintf("db2://db2inst1:password@127.0.0.1:%d/SAMPLE?ssl=false", port)
	db, err := sql.Open("db2", connStr)
	if err != nil {
		t.Fatalf("sql.Open('db2') failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Calling QueryContext with CREATE TABLE (DDL) should succeed and return empty rows without -517 error
	rows, err := db.QueryContext(ctx, "CREATE TABLE USERS (ID INT NOT NULL PRIMARY KEY, NAME VARCHAR(100))")
	if err != nil {
		t.Fatalf("db.QueryContext with CREATE TABLE failed: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("rows.Columns() failed: %v", err)
	}
	if len(cols) != 0 {
		t.Fatalf("expected 0 columns from CREATE TABLE, got %d", len(cols))
	}

	if rows.Next() {
		t.Fatalf("expected 0 rows from CREATE TABLE, got at least 1")
	}
}
