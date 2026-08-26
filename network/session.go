package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// SessionConfig holds configuration parameters for establishing a Db2 session.
type SessionConfig struct {
	Host              string
	Port              int
	Database          string
	User              string
	Password          string
	UseSSL            bool
	SSLClientCertPath string
	TLSConfig         *tls.Config
	Timeout           time.Duration
}

// ReplyPacket represents a single decoded DSS reply with its outer DDM codepoint and raw payload.
type ReplyPacket struct {
	Header    *DSSHeader
	CodePoint CodePoint
	Data      []byte
	MoreData  bool
}

// Session manages the underlying network socket and DRDA protocol state with the Db2 server.
type Session struct {
	cfg           SessionConfig
	conn          net.Conn
	mu            sync.Mutex
	correlationID uint16
	secmec        uint16
	encoding      StringEncoding
	endian        binary.ByteOrder
	prdid         string
	pkgid         string
	pkgcnstkn     string
	pkgsn         uint16
	qryblksz      uint32
	closed        bool
}

// NewSession instantiates a new Db2 network session configuration.
func NewSession(cfg SessionConfig) *Session {
	if cfg.Port == 0 {
		if cfg.UseSSL {
			cfg.Port = 50001
		} else {
			cfg.Port = 50000
		}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	dbPadded := fmt.Sprintf("%-18s", cfg.Database)
	if len(dbPadded) > 18 {
		dbPadded = dbPadded[:18]
	}
	cfg.Database = dbPadded

	return &Session{
		cfg:           cfg,
		correlationID: 1,
		secmec:        SecMecUSRIDPWD, // Default to plain text or negotiate
		encoding:      EncodingCP500,
		endian:        binary.LittleEndian,
		prdid:         "SQL12010",
		pkgid:         "SYSSH200",
		pkgcnstkn:     "SYSLVL01",
		pkgsn:         65,
		qryblksz:      65535,
	}
}

// Connect dials the Db2 server via TCP/TLS and completes the DRDA handshake.
func (s *Session) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		return errors.New("db2: session already connected")
	}

	address := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	dialer := &net.Dialer{
		Timeout: s.cfg.Timeout,
	}

	var rawConn net.Conn
	var err error

	if s.cfg.UseSSL {
		tlsConfig := s.cfg.TLSConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{
				ServerName: s.cfg.Host,
			}
			if s.cfg.SSLClientCertPath != "" {
				certData, certErr := os.ReadFile(s.cfg.SSLClientCertPath)
				if certErr != nil {
					return fmt.Errorf("failed to read SSL certificate file: %w", certErr)
				}
				rootCAs := x509.NewCertPool()
				if !rootCAs.AppendCertsFromPEM(certData) {
					return errors.New("failed to parse SSL root certificate from PEM")
				}
				tlsConfig.RootCAs = rootCAs
			}
		}
		rawConn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		rawConn, err = dialer.DialContext(ctx, "tcp", address)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to Db2 at %s: %w", address, err)
	}

	if tcpConn, ok := rawConn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	s.conn = rawConn
	s.closed = false

	// Perform full DRDA Handshake
	if err := s.handshake(ctx); err != nil {
		_ = s.conn.Close()
		s.conn = nil
		s.closed = true
		return err
	}

	return nil
}

// handshake runs the DRDA initial protocol exchange (EXCSAT -> ACCSEC -> SECCHK -> ACCRDB).
func (s *Session) handshake(ctx context.Context) error {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "go-db2-client"
	}

	// 1. Pack EXCSAT
	excsat, err := PackEXCSAT("go-db2", hostname, "SQL12010", "go-db2", DefaultManagerLevels())
	if err != nil {
		return fmt.Errorf("failed to pack EXCSAT: %w", err)
	}

	// 2. Pack ACCSEC
	accsec, err := PackACCSEC(s.cfg.Database, s.secmec, nil)
	if err != nil {
		return fmt.Errorf("failed to pack ACCSEC: %w", err)
	}

	// Send EXCSAT (chained) and ACCSEC (last in chain)
	s.correlationID = 1
	s.correlationID, err = WriteRequestDSS(s.conn, excsat, s.correlationID, false, false)
	if err != nil {
		return fmt.Errorf("failed to send EXCSAT: %w", err)
	}
	s.correlationID, err = WriteRequestDSS(s.conn, accsec, s.correlationID, false, true)
	if err != nil {
		return fmt.Errorf("failed to send ACCSEC: %w", err)
	}

	// Read reply for ACCSEC
	negotiatedSecMec, secTkn, err := s.parseACCSECRD()
	if err != nil {
		return fmt.Errorf("ACCSEC negotiation failed: %w", err)
	}

	// If server wants another SECMEC, update and resend ACCSEC
	s.correlationID = 1
	if negotiatedSecMec != 0 && negotiatedSecMec != s.secmec {
		s.secmec = negotiatedSecMec
		accsecRetry, err := PackACCSEC(s.cfg.Database, s.secmec, nil)
		if err != nil {
			return err
		}
		s.correlationID, err = WriteRequestDSS(s.conn, accsecRetry, s.correlationID, false, false)
		if err != nil {
			return err
		}
	}

	// 3. Pack SECCHK
	secchk, err := PackSECCHK(s.secmec, secTkn, s.cfg.Database, s.cfg.User, s.cfg.Password, s.encoding)
	if err != nil {
		return fmt.Errorf("failed to pack SECCHK: %w", err)
	}

	// 4. Pack ACCRDB
	accrdb, err := PackACCRDB(s.prdid, s.cfg.Database, s.encoding)
	if err != nil {
		return fmt.Errorf("failed to pack ACCRDB: %w", err)
	}

	// Send SECCHK (chained) and ACCRDB (last in chain)
	s.correlationID, err = WriteRequestDSS(s.conn, secchk, s.correlationID, false, false)
	if err != nil {
		return fmt.Errorf("failed to send SECCHK: %w", err)
	}
	s.correlationID, err = WriteRequestDSS(s.conn, accrdb, s.correlationID, false, true)
	if err != nil {
		return fmt.Errorf("failed to send ACCRDB: %w", err)
	}

	// Parse reply chain for SECCHK and ACCRDB
	replies, err := s.readReplyChain()
	if err != nil {
		return fmt.Errorf("handshake response failed: %w", err)
	}

	// Validate reply status
	for _, r := range replies {
		if r.CodePoint == CodePointSECCHKRM {
			sub, _ := ParseDDMReply(r.Data)
			if secChkCd, ok := sub[CodePointSECCHKCD]; ok && len(secChkCd) > 0 && secChkCd[0] != 0 {
				return fmt.Errorf("authentication failed: SECCHKCD=%d", secChkCd[0])
			}
		}
		if r.CodePoint == CodePointRDBAFLRM || r.CodePoint == CodePointRDBATHRM {
			return errors.New("authorization failed connecting to relational database")
		}
		if r.CodePoint == CodePointSQLERRRM {
			sub, _ := ParseDDMReply(r.Data)
			srvDgn := string(sub[CodePointSRVDGN])
			return fmt.Errorf("server error during handshake: %s", srvDgn)
		}
	}

	// Post-connect setup: Configure UTF-8 CCSID and client environment
	if err := s.setPostConnectVariables(hostname); err != nil {
		return fmt.Errorf("failed to set post-connect variables: %w", err)
	}

	return nil
}

// setPostConnectVariables sends CCSID and locale initialization queries to Db2.
func (s *Session) setPostConnectVariables(hostname string) error {
	s.correlationID = 1

	// EXCSAT with CCSIDMGR = 1208 (UTF-8)
	excsatMgr := PackEXCSAT_MGRLVLLS([]ManagerLevel{{Manager: CodePointCCSIDMGR, Level: 1208}})
	var err error
	s.correlationID, err = WriteRequestDSS(s.conn, excsatMgr, s.correlationID, false, false)
	if err != nil {
		return err
	}

	// EXCSQLSET
	excSqlSet := PackEXCSQLSET(s.pkgid, "", 1, s.cfg.Database)
	s.correlationID, err = WriteRequestDSS(s.conn, excSqlSet, s.correlationID, true, false)
	if err != nil {
		return err
	}

	// SET CLIENT WRKSTNNAME
	sqlWrkStn := PackSQLSTT(fmt.Sprintf("SET CLIENT WRKSTNNAME '%s'", hostname))
	s.correlationID, err = WriteRequestDSS(s.conn, sqlWrkStn, s.correlationID, false, false)
	if err != nil {
		return err
	}

	// RDBCMM (Commit setup)
	rdbcmm := PackRDBCMM()
	s.correlationID, err = WriteRequestDSS(s.conn, rdbcmm, s.correlationID, false, true)
	if err != nil {
		return err
	}

	// Drain responses
	_, err = s.readReplyChain()
	return err
}

// parseACCSECRD reads and processes the ACCSECRD reply during security negotiation.
func (s *Session) parseACCSECRD() (uint16, []byte, error) {
	replies, err := s.readReplyChain()
	if err != nil {
		return 0, nil, err
	}

	var secmec uint16
	var sectkn []byte

	for _, r := range replies {
		if r.CodePoint == CodePointRDBNFNRM {
			return 0, nil, errors.New("relational database not found (RDBNFNRM)")
		}
		if r.CodePoint == CodePointACCSECRD {
			sub, err := ParseDDMReply(r.Data)
			if err != nil {
				return 0, nil, err
			}
			if smBytes, ok := sub[CodePointSECMEC]; ok && len(smBytes) >= 2 {
				secmec = binary.BigEndian.Uint16(smBytes[:2])
			}
			if stBytes, ok := sub[CodePointSECTKN]; ok {
				sectkn = stBytes
			}
		}
	}

	return secmec, sectkn, nil
}

// readReplyChain reads all chained DSS reply packets until the end of the chain.
func (s *Session) readReplyChain() ([]ReplyPacket, error) {
	var packets []ReplyPacket
	chained := true

	for chained {
		hdr, cp, data, moreData, err := ReadDSS(s.conn)
		if err != nil {
			return nil, err
		}
		packets = append(packets, ReplyPacket{
			Header:    hdr,
			CodePoint: cp,
			Data:      data,
			MoreData:  moreData,
		})
		chained = hdr.Chained
	}

	return packets, nil
}

// Ping checks if the database session is responsive.
func (s *Session) Ping(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return errors.New("db2: connection is closed")
	}

	// RDBCMM as a lightweight ping
	s.correlationID = 1
	rdbcmm := PackRDBCMM()
	var err error
	s.correlationID, err = WriteRequestDSS(s.conn, rdbcmm, s.correlationID, false, true)
	_, err = s.readReplyChain()
	return err
}

// Commit commits the active transaction.
func (s *Session) Commit(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return errors.New("db2: connection is closed")
	}

	s.correlationID = 1
	rdbcmm := PackRDBCMM()
	var err error
	s.correlationID, err = WriteRequestDSS(s.conn, rdbcmm, s.correlationID, false, true)
	if err != nil {
		return err
	}

	_, err = s.readReplyChain()
	return err
}

// Rollback rolls back the active transaction.
func (s *Session) Rollback(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return errors.New("db2: connection is closed")
	}

	s.correlationID = 1
	rdbrllbck := PackRDBRLLBCK()
	var err error
	s.correlationID, err = WriteRequestDSS(s.conn, rdbrllbck, s.correlationID, false, true)
	if err != nil {
		return err
	}

	_, err = s.readReplyChain()
	return err
}

// Close gracefully closes the session and underlying network connection.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}
