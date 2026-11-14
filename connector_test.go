package db2

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-db2/go-db2/internal/drda"
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

	if connector.cfg == cfg {
		t.Fatal("expected NewConnector to clone Config defensively, got same pointer")
	}
	if connector.cfg.Host != cfg.Host || connector.cfg.Port != cfg.Port || connector.cfg.Database != cfg.Database || connector.cfg.User != cfg.User {
		t.Errorf("expected cloned cfg fields to match %+v, got %+v", cfg, connector.cfg)
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

	cfg, err := ParseDSN(fmt.Sprintf("db2://db2inst1:password@127.0.0.1:%d/SAMPLE?ssl=false&allow_plaintext_auth=true", port))
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

func TestNewConnector_NilAndDefensiveClone(t *testing.T) {
	t.Run("NilConfigUsesDefaults", func(t *testing.T) {
		c := NewConnector(nil)
		if c == nil || c.cfg == nil {
			t.Fatal("expected NewConnector(nil) to initialize default Config")
		}
		if c.cfg.Host != "localhost" || c.cfg.Port != 50000 || c.cfg.Params == nil {
			t.Fatalf("unexpected default config from NewConnector(nil): %+v", c.cfg)
		}
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := c.Connect(canceledCtx); err == nil {
			t.Fatal("expected error calling Connect with canceled context, not nil-pointer panic")
		}
	})

	t.Run("MutatingCallerConfigDoesNotAffectConnector", func(t *testing.T) {
		cfg := NewConfig()
		cfg.Host = "127.0.0.1"
		cfg.User = "orig_user"
		cfg.Params = map[string]string{"app": "v1"}
		cfg.TLSConfig = &tls.Config{ServerName: "orig.db2.local", MinVersion: tls.VersionTLS12}

		c := NewConnector(cfg)

		// Mutate caller-owned structs after NewConnector returns
		cfg.User = "mutated_user"
		cfg.Params["app"] = "mutated"
		cfg.Params["injected"] = "evil"
		cfg.TLSConfig.ServerName = "evil.db2.local"

		if c.cfg.User != "orig_user" {
			t.Fatalf("expected cloned User 'orig_user', got %q", c.cfg.User)
		}
		if c.cfg.Params["app"] != "v1" {
			t.Fatalf("expected cloned Params['app'] 'v1', got %q", c.cfg.Params["app"])
		}
		if _, exists := c.cfg.Params["injected"]; exists {
			t.Fatal("mutating caller Params map leaked into connector")
		}
		if c.cfg.TLSConfig == nil || c.cfg.TLSConfig == cfg.TLSConfig {
			t.Fatal("expected cloned TLSConfig pointer")
		}
		if c.cfg.TLSConfig.ServerName != "orig.db2.local" {
			t.Fatalf("expected cloned TLSConfig.ServerName 'orig.db2.local', got %q", c.cfg.TLSConfig.ServerName)
		}
	})

	t.Run("NilConnectorOrNilCfgConnectReturnsError", func(t *testing.T) {
		var nilConn *Connector
		if _, err := nilConn.Connect(context.Background()); err == nil {
			t.Fatal("expected error calling Connect on nil *Connector")
		}

		zeroConn := &Connector{}
		if _, err := zeroConn.Connect(context.Background()); err == nil {
			t.Fatal("expected error calling Connect on zero Connector with nil cfg")
		}
	})

	t.Run("ConcurrentMutationAndConnect", func(t *testing.T) {
		srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
			return writeSQLCARDOK(c)
		})
		host, portStr, err := net.SplitHostPort(srv.addr)
		if err != nil {
			t.Fatalf("SplitHostPort failed: %v", err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			t.Fatalf("Atoi failed: %v", err)
		}

		cfg := NewConfig()
		cfg.Host = host
		cfg.Port = port
		cfg.Database = "TESTDB"
		cfg.User = "orig_user"
		cfg.Password = "secret"
		cfg.AllowPlaintextAuth = true
		cfg.Params = map[string]string{"app": "v1"}
		cfg.TLSConfig = &tls.Config{ServerName: "orig.db2.local", MinVersion: tls.VersionTLS12}

		connector := NewConnector(cfg)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				cfg.User = fmt.Sprintf("mutated_%d", i)
				cfg.Params["app"] = fmt.Sprintf("v_%d", i)
				cfg.Params["injected"] = "evil"
				cfg.TLSConfig.ServerName = fmt.Sprintf("evil_%d.db2.local", i)
			}
		}()

		for g := 0; g < 4; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 5; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					dConn, err := connector.Connect(ctx)
					cancel()
					if err != nil {
						t.Errorf("concurrent Connect failed: %v", err)
						return
					}
					c := dConn.(*Conn)
					if c.cfg.User != "orig_user" || c.cfg.Params["app"] != "v1" || c.cfg.TLSConfig.ServerName != "orig.db2.local" {
						t.Errorf("concurrent mutation leaked into opened Conn: %+v", c.cfg)
					}
					_ = dConn.Close()
				}
			}()
		}

		wg.Wait()
	})
}
