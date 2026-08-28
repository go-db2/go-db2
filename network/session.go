package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/go-db2/go-db2/converters"
	"github.com/go-db2/go-db2/network/security"
	"github.com/go-db2/go-db2/types"
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
	BlockSize         int
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
	rdbinttkn     []byte
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

	blksz := uint32(cfg.BlockSize)
	if blksz < 1024 {
		blksz = 65535
	}

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
		qryblksz:      blksz,
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

	// 3. Pack SECCHK (supporting SECMEC 9 with DH key exchange & DES encryption)
	var secchk []byte
	if (s.secmec == SecMecEUSRIDPWD || s.secmec == 9) && len(secTkn) >= 20 {
		clientPriv, privErr := security.GenerateDHPrivateKey()
		if privErr != nil {
			return fmt.Errorf("failed to generate DH private key: %w", privErr)
		}
		clientPub := security.CalculateDHPublicKey(clientPriv)
		encPassword, encErr := security.EncryptPasswordSECMEC9(s.cfg.Password, secTkn, clientPriv)
		if encErr != nil {
			return fmt.Errorf("failed to encrypt password with SECMEC 9: %w", encErr)
		}
		secchk, err = PackSECCHKWithBytes(s.secmec, clientPub, s.cfg.Database, s.cfg.User, encPassword, true, s.encoding)
	} else {
		secchk, err = PackSECCHK(s.secmec, secTkn, s.cfg.Database, s.cfg.User, s.cfg.Password, s.encoding)
	}
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
		if r.CodePoint == CodePointACCRDBRM {
			sub, _ := ParseDDMReply(r.Data)
			if inttkn, ok := sub[CodePointRDBINTTKN]; ok {
				s.rdbinttkn = inttkn
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

// ExecDirect executes a DDL or DML statement immediately without parameter binding.
func (s *Session) ExecDirect(ctx context.Context, sql string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return 0, errors.New("db2: connection is closed")
	}

	s.correlationID = 1
	var err error

	// 1. EXCSQLIMM (Execute Immediate)
	imm := PackEXCSQLIMM(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database)
	s.correlationID, err = WriteRequestDSS(s.conn, imm, s.correlationID, true, false)
	if err != nil {
		return 0, fmt.Errorf("failed to write EXCSQLIMM: %w", err)
	}

	// 2. SQLSTT (SQL Statement Text)
	stt := PackSQLSTT(sql)
	s.correlationID, err = WriteRequestDSS(s.conn, stt, s.correlationID, false, false)
	if err != nil {
		return 0, fmt.Errorf("failed to write SQLSTT: %w", err)
	}

	// 3. RDBCMM (Commit)
	cmm := PackRDBCMM()
	s.correlationID, err = WriteRequestDSS(s.conn, cmm, s.correlationID, false, true)
	if err != nil {
		return 0, fmt.Errorf("failed to write RDBCMM: %w", err)
	}

	replies, err := s.readReplyChain()
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var affectedRows int64
	for _, r := range replies {
		if r.CodePoint == CodePointSQLCARD {
			code, state, msg, rows, parseErr := ParseSQLCARD(r.Data, s.endian)
			if parseErr != nil {
				return 0, parseErr
			}
			if code < 0 {
				return 0, fmt.Errorf("db2: SQLCODE=%d SQLSTATE=%s %s", code, state, msg)
			}
			affectedRows += rows
		}
		if r.CodePoint == CodePointSQLERRRM {
			sub, _ := ParseDDMReply(r.Data)
			return 0, fmt.Errorf("db2: %s", string(sub[CodePointSRVDGN]))
		}
	}

	return affectedRows, nil
}

// QueryDirect prepares, describes and executes a query statement returning raw column headers and decoded row data.
func (s *Session) QueryDirect(ctx context.Context, sql string) ([]ColumnDescription, [][]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return nil, nil, errors.New("db2: connection is closed")
	}

	s.correlationID = 1
	var err error

	// 1. PRPSQLSTT
	prp := PackPRPSQLSTT(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database)
	s.correlationID, err = WriteRequestDSS(s.conn, prp, s.correlationID, true, false)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write PRPSQLSTT: %w", err)
	}

	// 2. SQLSTT
	stt := PackSQLSTT(sql)
	s.correlationID, err = WriteRequestDSS(s.conn, stt, s.correlationID, false, false)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write SQLSTT: %w", err)
	}

	// 3. OPNQRY
	opn := PackOPNQRY(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database, s.qryblksz)
	s.correlationID, err = WriteRequestDSS(s.conn, opn, s.correlationID, false, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write OPNQRY: %w", err)
	}

	replies, err := s.readReplyChain()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read query response: %w", err)
	}

	var columns []ColumnDescription
	var fields []FieldDescriptor
	var rows [][]any
	var extdtaList [][]byte
	needCNTQRY := false
	var qryinsid uint64
	var cntqryID uint16 = 1

	for _, r := range replies {
		if r.CodePoint == CodePointSQLDARD {
			cols, err := ParseSQLDARD(r.Data, s.endian)
			if err != nil {
				return nil, nil, err
			}
			if len(cols) > 0 {
				columns = cols
			}
		} else if r.CodePoint == CodePointQRYDSC {
			flds, err := ParseQRYDSC(r.Data)
			if err != nil {
				return nil, nil, err
			}
			fields = flds
		} else if r.CodePoint == CodePointEXTDTA {
			extdtaList = append(extdtaList, r.Data)
		} else if r.CodePoint == CodePointQRYDTA {
			reader := bytes.NewReader(r.Data)
			for reader.Len() >= 2 {
				var rowHdr [2]byte
				if _, err := io.ReadFull(reader, rowHdr[:]); err != nil {
					break
				}
				if rowHdr[0] != 0xFF {
					break
				}

				row := make([]any, len(fields))
				for i, f := range fields {
					val, err := converters.DecodeField(f.Type, f.PS, reader, s.endian)
					if err != nil {
						return nil, nil, fmt.Errorf("failed to decode column %d (Type=0x%02X, PS=%x, RemainingBytes=%d): %w", i+1, f.Type, f.PS, reader.Len(), err)
					}
					row[i] = val
				}
				rows = append(rows, row)
			}
		} else if r.CodePoint == CodePointOPNQRYRM {
			sub, _ := ParseDDMReply(r.Data)
			if insidBytes, ok := sub[CodePointQRYINSID]; ok && len(insidBytes) >= 8 {
				qryinsid = binary.BigEndian.Uint64(insidBytes)
			}
			needCNTQRY = true
			cntqryID = r.Header.CorrelationID
		} else if r.CodePoint == CodePointSQLERRRM {
			sub, _ := ParseDDMReply(r.Data)
			return nil, nil, fmt.Errorf("db2: %s", string(sub[CodePointSRVDGN]))
		} else if r.CodePoint == CodePointSQLCARD {
			code, state, msg, _, parseErr := ParseSQLCARD(r.Data, s.endian)
			if parseErr != nil {
				return nil, nil, parseErr
			}
			if code < 0 {
				return nil, nil, fmt.Errorf("db2: SQLCODE=%d SQLSTATE=%s %s", code, state, msg)
			}
		}
	}

	if needCNTQRY {
		cntqry := PackCNTQRY(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database, s.qryblksz, qryinsid)
		_, err = WriteRequestDSS(s.conn, cntqry, cntqryID, false, true)
		if err != nil {
			return nil, nil, err
		}

		cntReplies, err := s.readReplyChain()
		if err != nil {
			return nil, nil, err
		}

		for _, r := range cntReplies {
			if r.CodePoint == CodePointEXTDTA {
				extdtaList = append(extdtaList, r.Data)
			} else if r.CodePoint == CodePointQRYDTA {
				reader := bytes.NewReader(r.Data)
				for reader.Len() >= 2 {
					var rowHdr [2]byte
					if _, err := io.ReadFull(reader, rowHdr[:]); err != nil {
						break
					}
					if rowHdr[0] != 0xFF {
						break
					}

					row := make([]any, len(fields))
					for i, f := range fields {
						val, err := converters.DecodeField(f.Type, f.PS, reader, s.endian)
						if err != nil {
							return nil, nil, fmt.Errorf("failed to decode column %d in QueryDirect CNTQRY (Type=0x%02X, PS=%x, RemainingBytes=%d): %w", i+1, f.Type, f.PS, reader.Len(), err)
						}
						row[i] = val
					}
					rows = append(rows, row)
				}
			}
		}
	}

	if len(extdtaList) > 0 {
		stitchEXTDTA(fields, rows, extdtaList)
	}

	return columns, rows, nil
}

// PrepareAndDescribe prepares a query/statement and retrieves its output columns and input parameter descriptors.
func (s *Session) PrepareAndDescribe(ctx context.Context, sql string) ([]ColumnDescription, []ColumnDescription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return nil, nil, errors.New("db2: connection is closed")
	}

	s.correlationID = 1
	var err error

	// 1. PRPSQLSTT
	prp := PackPRPSQLSTT(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database)
	s.correlationID, err = WriteRequestDSS(s.conn, prp, s.correlationID, true, false)
	if err != nil {
		return nil, nil, err
	}

	// 2. SQLSTT
	stt := PackSQLSTT(sql)
	s.correlationID, err = WriteRequestDSS(s.conn, stt, s.correlationID, false, false)
	if err != nil {
		return nil, nil, err
	}

	// 3. DSCSQLSTT
	dsc := PackDSCSQLSTT(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database)
	s.correlationID, err = WriteRequestDSS(s.conn, dsc, s.correlationID, false, true)
	if err != nil {
		return nil, nil, err
	}

	replies, err := s.readReplyChain()
	if err != nil {
		return nil, nil, err
	}

	var outputCols, paramCols []ColumnDescription
	for _, r := range replies {
		if r.CodePoint == CodePointSQLDARD {
			cols, err := ParseSQLDARD(r.Data, s.endian)
			if err != nil {
				return nil, nil, err
			}
			if len(r.Data) > 0 && r.Data[0] == 0xFF {
				paramCols = cols
			} else {
				outputCols = cols
			}
		} else if r.CodePoint == CodePointSQLCARD {
			code, state, msg, _, parseErr := ParseSQLCARD(r.Data, s.endian)
			if parseErr != nil {
				return nil, nil, parseErr
			}
			if code < 0 {
				return nil, nil, fmt.Errorf("db2: SQLCODE=%d SQLSTATE=%s %s", code, state, msg)
			}
		} else if r.CodePoint == CodePointSQLERRRM {
			sub, _ := ParseDDMReply(r.Data)
			return nil, nil, fmt.Errorf("db2: %s", string(sub[CodePointSRVDGN]))
		}
	}

	return outputCols, paramCols, nil
}

// ExecWithParams executes a prepared statement with parameter arguments and returns affected rows and any output parameters.
func (s *Session) ExecWithParams(ctx context.Context, paramCols []ColumnDescription, args []any) (int64, []any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return 0, nil, errors.New("db2: connection is closed")
	}

	colTypes := make([]types.SQLType, len(paramCols))
	colLens := make([]int64, len(paramCols))
	precs := make([]int, len(paramCols))
	scales := make([]int, len(paramCols))

	for i, c := range paramCols {
		colTypes[i] = types.SQLType(c.SQLType)
		colLens[i] = c.Length
		precs[i] = c.Precision
		scales[i] = c.Scale
	}

	sqldta, err := converters.BuildSQLDTA(colTypes, colLens, precs, scales, args, s.endian)
	if err != nil {
		return 0, nil, err
	}

	// 1. EXCSQLSTT
	exc := PackEXCSQLSTT(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database)
	curID, err := WriteRequestDSS(s.conn, exc, 1, true, false)
	if err != nil {
		return 0, nil, err
	}

	// 2. SQLDTA
	_, err = WriteRequestDSS(s.conn, sqldta, curID, false, false)
	if err != nil {
		return 0, nil, err
	}

	// 3. RDBCMM
	cmm := PackRDBCMM()
	_, err = WriteRequestDSS(s.conn, cmm, 1, false, true)
	if err != nil {
		return 0, nil, err
	}

	replies, err := s.readReplyChain()
	if err != nil {
		return 0, nil, err
	}

	// If the server sent SQLDARD in the first chain, a second chain follows with SQLDTARD / SQLCARD
	hasOnlyDARD := false
	for _, r := range replies {
		if r.CodePoint == CodePointSQLDARD {
			hasOnlyDARD = true
			break
		}
	}
	if hasOnlyDARD {
		replies2, err := s.readReplyChain()
		if err == nil {
			replies = append(replies, replies2...)
		}
	}

	var affectedRows int64
	var outValues []any
	for _, r := range replies {
		if r.CodePoint == CodePointSQLDTARD {
			vals, parseErr := converters.ParseSQLDTARD(r.Data, s.endian)
			if parseErr == nil && len(vals) > 0 {
				outValues = vals
			}
		} else if r.CodePoint == CodePointSQLCARD {
			code, state, msg, rows, parseErr := ParseSQLCARD(r.Data, s.endian)
			if parseErr != nil {
				return 0, nil, parseErr
			}
			if code < 0 {
				return 0, nil, fmt.Errorf("db2: SQLCODE=%d SQLSTATE=%s %s", code, state, msg)
			}
			affectedRows += rows
		} else if r.CodePoint == CodePointSQLERRRM {
			sub, _ := ParseDDMReply(r.Data)
			return 0, nil, fmt.Errorf("db2: %s", string(sub[CodePointSRVDGN]))
		}
	}

	return affectedRows, outValues, nil
}

// ExecBatchWithParams executes a batch of parameter sets against the currently prepared statement.
func (s *Session) ExecBatchWithParams(ctx context.Context, paramCols []ColumnDescription, batchRows [][]any) (int64, error) {
	if len(batchRows) == 0 {
		return 0, nil
	}

	var totalAffected int64
	for _, rowArgs := range batchRows {
		affected, _, err := s.ExecWithParams(ctx, paramCols, rowArgs)
		if err != nil {
			return totalAffected, err
		}
		totalAffected += affected
	}
	return totalAffected, nil
}

// QueryWithParams opens a query on a prepared statement with parameter arguments.
func (s *Session) QueryWithParams(ctx context.Context, outputCols, paramCols []ColumnDescription, args []any) ([]ColumnDescription, [][]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return nil, nil, errors.New("db2: connection is closed")
	}

	colTypes := make([]types.SQLType, len(paramCols))
	colLens := make([]int64, len(paramCols))
	precs := make([]int, len(paramCols))
	scales := make([]int, len(paramCols))

	for i, c := range paramCols {
		colTypes[i] = types.SQLType(c.SQLType)
		colLens[i] = c.Length
		precs[i] = c.Precision
		scales[i] = c.Scale
	}

	sqldta, err := converters.BuildSQLDTA(colTypes, colLens, precs, scales, args, s.endian)
	if err != nil {
		return nil, nil, err
	}

	// 1. OPNQRY with QRYBLKSZ
	opnqry := PackOPNQRY(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database, s.qryblksz)
	curID, err := WriteRequestDSS(s.conn, opnqry, 1, true, false)
	if err != nil {
		return nil, nil, err
	}

	// 2. SQLDTA with parameters
	_, err = WriteRequestDSS(s.conn, sqldta, curID, false, true)
	if err != nil {
		return nil, nil, err
	}

	replies, err := s.readReplyChain()
	if err != nil {
		return nil, nil, err
	}

	var columns []ColumnDescription = outputCols
	var fields []FieldDescriptor
	var rows [][]any
	var extdtaList [][]byte
	needCNTQRY := false
	var qryinsid uint64
	var cntqryID uint16 = 1

	for _, r := range replies {
		if r.CodePoint == CodePointSQLDARD {
			cols, err := ParseSQLDARD(r.Data, s.endian)
			if err != nil {
				return nil, nil, err
			}
			if len(cols) > 0 {
				columns = cols
			}
		} else if r.CodePoint == CodePointQRYDSC {
			flds, err := ParseQRYDSC(r.Data)
			if err != nil {
				return nil, nil, err
			}
			fields = flds
		} else if r.CodePoint == CodePointEXTDTA {
			extdtaList = append(extdtaList, r.Data)
		} else if r.CodePoint == CodePointOPNQRYRM {
			sub, _ := ParseDDMReply(r.Data)
			if insidBytes, ok := sub[CodePointQRYINSID]; ok && len(insidBytes) >= 8 {
				qryinsid = binary.BigEndian.Uint64(insidBytes)
			}
			needCNTQRY = true
			cntqryID = r.Header.CorrelationID
		} else if r.CodePoint == CodePointQRYDTA {
			reader := bytes.NewReader(r.Data)
			for reader.Len() >= 2 {
				var rowHdr [2]byte
				if _, err := io.ReadFull(reader, rowHdr[:]); err != nil {
					break
				}
				if rowHdr[0] != 0xFF {
					break
				}

				row := make([]any, len(fields))
				for i, f := range fields {
					val, err := converters.DecodeField(f.Type, f.PS, reader, s.endian)
					if err != nil {
						return nil, nil, fmt.Errorf("failed to decode column %d: %w", i+1, err)
					}
					row[i] = val
				}
				rows = append(rows, row)
			}
		} else if r.CodePoint == CodePointSQLCARD {
			code, state, msg, _, parseErr := ParseSQLCARD(r.Data, s.endian)
			if parseErr != nil {
				return nil, nil, parseErr
			}
			if code < 0 {
				return nil, nil, fmt.Errorf("db2: SQLCODE=%d SQLSTATE=%s %s", code, state, msg)
			}
		} else if r.CodePoint == CodePointSQLERRRM {
			sub, _ := ParseDDMReply(r.Data)
			return nil, nil, fmt.Errorf("db2: %s", string(sub[CodePointSRVDGN]))
		}
	}

	if needCNTQRY {
		cntqry := PackCNTQRY(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database, s.qryblksz, qryinsid)
		_, err = WriteRequestDSS(s.conn, cntqry, cntqryID, false, true)
		if err != nil {
			return nil, nil, err
		}

		cntReplies, err := s.readReplyChain()
		if err != nil {
			return nil, nil, err
		}

		for _, r := range cntReplies {
			if r.CodePoint == CodePointEXTDTA {
				extdtaList = append(extdtaList, r.Data)
			} else if r.CodePoint == CodePointQRYDTA {
				reader := bytes.NewReader(r.Data)
				for reader.Len() >= 2 {
					var rowHdr [2]byte
					if _, err := io.ReadFull(reader, rowHdr[:]); err != nil {
						break
					}
					if rowHdr[0] != 0xFF {
						break
					}

					row := make([]any, len(fields))
					for i, f := range fields {
						val, err := converters.DecodeField(f.Type, f.PS, reader, s.endian)
						if err != nil {
							return nil, nil, fmt.Errorf("failed to decode column %d in CNTQRY (Type=0x%02X, PS=%x, RemainingBytes=%d): %w", i+1, f.Type, f.PS, reader.Len(), err)
						}
						row[i] = val
					}
					rows = append(rows, row)
				}
			}
		}
	}

	if len(extdtaList) > 0 {
		stitchEXTDTA(fields, rows, extdtaList)
	}

	// Send commit to close query lock
	s.correlationID = 1
	s.correlationID, err = WriteRequestDSS(s.conn, PackRDBCMM(), s.correlationID, false, true)
	if err == nil {
		_, _ = s.readReplyChain()
	}

	return columns, rows, nil
}

func isLOBType(t uint8) bool {
	switch t {
	case converters.DRDATypeLOBLOC, converters.DRDATypeNLOBLOC,
		converters.DRDATypeCLOBLOC, converters.DRDATypeNCLOBLOC,
		converters.DRDATypeDBCSCLOBLOC, converters.DRDATypeNDBCSCLOBLOC,
		converters.DRDATypeLOBBytes, converters.DRDATypeNLOBBytes,
		converters.DRDATypeLOBCSBCS, converters.DRDATypeNLOBCSBCS,
		0x10, 0x11, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9:
		return true
	default:
		return false
	}
}

func isCLOBType(t uint8) bool {
	switch t {
	case converters.DRDATypeCLOBLOC, converters.DRDATypeNCLOBLOC,
		converters.DRDATypeDBCSCLOBLOC, converters.DRDATypeNDBCSCLOBLOC,
		converters.DRDATypeLOBCSBCS, converters.DRDATypeNLOBCSBCS,
		0xF6, 0xF7, 0xF8, 0xF9:
		return true
	default:
		return false
	}
}

func stitchEXTDTA(fields []FieldDescriptor, rows [][]any, extdtaList [][]byte) {
	var lobIndices []int
	for i, f := range fields {
		if isLOBType(f.Type) {
			lobIndices = append(lobIndices, i)
		}
	}
	if len(lobIndices) == 0 {
		return
	}

	extIdx := 0
	for rowIdx := range rows {
		for _, colIdx := range lobIndices {
			if rows[rowIdx][colIdx] != nil && extIdx < len(extdtaList) {
				data := extdtaList[extIdx]
				// EXTDTA data starts with a 1-byte status flag (0x00 = valid data)
				if len(data) > 0 && data[0] == 0x00 {
					data = data[1:]
				}
				fType := fields[colIdx].Type
				if isCLOBType(fType) {
					rows[rowIdx][colIdx] = string(data)
				} else {
					rows[rowIdx][colIdx] = data
				}
				extIdx++
			}
		}
	}
}

// Interrupt requests cancellation of any active statement on the session via SQLINTR.
func (s *Session) Interrupt() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.conn == nil {
		return errors.New("db2: connection is closed")
	}

	intr := PackSQLINTR(s.pkgid, s.pkgcnstkn, s.pkgsn, s.cfg.Database)
	_, err := WriteRequestDSS(s.conn, intr, 1, false, true)
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
