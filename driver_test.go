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

	"github.com/go-db2/go-db2/internal/drda"
)

func TestDatabaseSqlOpenAndPing(t *testing.T) {
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
					hdr, cp, _, _, err := drda.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == drda.CodePointACCSEC || !hdr.Chained {
						break
					}
				}

				// 2. Respond with ACCSECRD
				accsecRdBody := drda.PackUint16(drda.CodePointSECMEC, drda.SecMecUSRIDPWD)
				accsecRdObj := drda.PackDDMObject(drda.CodePointACCSECRD, accsecRdBody)
				dssHdr := drda.BuildDSSHeader(len(accsecRdObj), drda.DSSTypeReply, false, false, false, 1)
				if _, err := c.Write(append(dssHdr, accsecRdObj...)); err != nil {
					return
				}

				// 3. Read SECCHK + ACCRDB
				for {
					hdr, cp, _, _, err := drda.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == drda.CodePointACCRDB || !hdr.Chained {
						break
					}
				}

				// 4. Respond with SECCHKRM + ACCRDBRM
				secchkRmBody := drda.PackBytes(drda.CodePointSECCHKCD, []byte{0x00})
				secchkRmObj := drda.PackDDMObject(drda.CodePointSECCHKRM, secchkRmBody)
				accrdbRmObj := drda.PackDDMObject(drda.CodePointACCRDBRM, nil)

				hdr1 := drda.BuildDSSHeader(len(secchkRmObj), drda.DSSTypeReply, true, false, false, 1)
				hdr2 := drda.BuildDSSHeader(len(accrdbRmObj), drda.DSSTypeReply, false, false, false, 1)

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
					hdr, cp, _, _, err := drda.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == drda.CodePointRDBCMM || !hdr.Chained {
						break
					}
				}

				// Respond with SQLCARD (SQLCODE=0)
				sqlcardPayload := make([]byte, 20)
				sqlcardPayload[0] = 0x00
				binary.LittleEndian.PutUint32(sqlcardPayload[1:5], 0)
				copy(sqlcardPayload[5:10], "00000")
				sqlcardObj := drda.PackDDMObject(drda.CodePointSQLCARD, sqlcardPayload)
				sqlcardHdr := drda.BuildDSSHeader(len(sqlcardObj), drda.DSSTypeReply, false, false, false, 1)
				if _, err := c.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
					return
				}

				// 6. Loop to handle Ping (RDBCMM) or Begin/Commit
				for {
					_, cp, _, _, err := drda.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == drda.CodePointEXCSAT {
						excsatRdObj := drda.PackDDMObject(drda.CodePointEXCSATRD, nil)
						excsatRdHdr := drda.BuildDSSHeader(len(excsatRdObj), drda.DSSTypeReply, false, false, false, 1)
						if _, err := c.Write(append(excsatRdHdr, excsatRdObj...)); err != nil {
							return
						}
					} else if cp == drda.CodePointRDBCMM {
						if _, err := c.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
							return
						}
					}
				}
			}(conn)
		}
	}()

	// Open standard database/sql connection
	connStr := fmt.Sprintf("db2://db2inst1:password@127.0.0.1:%d/SAMPLE?ssl=false&allow_plaintext_auth=true", port)
	db, err := sql.Open("db2", connStr)
	if err != nil {
		t.Fatalf("sql.Open('db2') failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Ping database
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("db.PingContext() failed: %v", err)
	}

	// Begin and Commit transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("db.BeginTx() failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("tx.Commit() failed: %v", err)
	}
}
