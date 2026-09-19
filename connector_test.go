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

func TestNewConnectorAndDriver(t *testing.T) {
	cfg := NewConfig()
	cfg.Host = "127.0.0.1"
	cfg.Port = 50000
	cfg.Database = "TESTDB"
	cfg.User = "db2user"

	connector := NewConnector(cfg)
	if connector == nil {
		t.Fatal("NewConnector returned nil")
	}

	if connector.cfg != cfg {
		t.Errorf("expected cfg to be %v, got %v", cfg, connector.cfg)
	}

	if connector.driver == nil {
		t.Fatal("expected driver to be non-nil")
	}

	if connector.Driver() != connector.driver {
		t.Errorf("Connector.Driver() returned %v, expected %v", connector.Driver(), connector.driver)
	}
}

func TestConnector_Driver(t *testing.T) {
	t.Run("DefaultDriver", func(t *testing.T) {
		cfg := NewConfig()
		connector := NewConnector(cfg)
		if connector == nil {
			t.Fatal("NewConnector returned nil")
		}

		drv := connector.Driver()
		if drv == nil {
			t.Fatal("expected Connector.Driver() to be non-nil")
		}

		if drv != connector.driver {
			t.Errorf("expected Connector.Driver() to return internal driver %v, got %v", connector.driver, drv)
		}
	})

	t.Run("CustomDriver", func(t *testing.T) {
		cfg := NewConfig()
		mockDriver := &Driver{}
		connector := &Connector{
			cfg:    cfg,
			driver: mockDriver,
		}

		drv := connector.Driver()
		if drv != mockDriver {
			t.Errorf("expected Connector.Driver() to return custom driver %v, got %v", mockDriver, drv)
		}
	})
}

func TestConnectorConnect_Error(t *testing.T) {
	// Connector with invalid / unreachable address or canceled context
	cfg := NewConfig()
	cfg.Host = "127.0.0.1"
	cfg.Port = 1 // unlikely to have a db2 server listening on port 1
	cfg.Timeout = 100 * time.Millisecond

	connector := NewConnector(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	cancel() // canceled context

	conn, err := connector.Connect(ctx)
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Fatal("expected error connecting with canceled context, got nil")
	}
}

func TestConnectorOpenDB_Success(t *testing.T) {
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

				// 6. Loop to handle Ping (RDBCMM) or Begin/Commit
				for {
					_, cp, _, _, err := network.ReadDSS(c)
					if err != nil {
						return
					}
					if cp == network.CodePointRDBCMM {
						if _, err := c.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
							return
						}
					}
				}
			}(conn)
		}
	}()

	cfg, err := ParseDSN(fmt.Sprintf("db2://db2inst1:password@127.0.0.1:%d/SAMPLE?ssl=false", port))
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}

	connector := NewConnector(cfg)
	db := sql.OpenDB(connector)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("db.PingContext() using sql.OpenDB(connector) failed: %v", err)
	}
}
