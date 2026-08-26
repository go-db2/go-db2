package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/go-db2/go-db2/converters"
)

// StringEncoding defines the character encoding used for packing DDM strings.
type StringEncoding string

const (
	EncodingCP500 StringEncoding = "cp500" // IBM EBCDIC 500
	EncodingUTF8  StringEncoding = "utf-8" // UTF-8
)

// ManagerLevel represents a DRDA Manager and its negotiated support level.
type ManagerLevel struct {
	Manager CodePoint // Manager Codepoint (e.g., AGENT, SQLAM, CMNTCPIP, etc.)
	Level   uint16    // Manager Version/Level
}

// PackDDMObject wraps raw body bytes into a DDM object header (2 bytes length + 2 bytes codepoint + body).
func PackDDMObject(cp CodePoint, body []byte) []byte {
	totalLen := uint16(len(body) + 4)
	buf := make([]byte, 4+len(body))
	binary.BigEndian.PutUint16(buf[0:2], totalLen)
	binary.BigEndian.PutUint16(buf[2:4], uint16(cp))
	copy(buf[4:], body)
	return buf
}

// PackBytes wraps a binary slice into a DDM parameter object.
func PackBytes(cp CodePoint, val []byte) []byte {
	return PackDDMObject(cp, val)
}

// PackUint16 encodes a 16-bit integer into a DDM parameter object.
func PackUint16(cp CodePoint, val uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, val)
	return PackDDMObject(cp, buf)
}

// PackUint32 encodes a 32-bit integer into a DDM parameter object.
func PackUint32(cp CodePoint, val uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, val)
	return PackDDMObject(cp, buf)
}

// PackString encodes a string into a DDM parameter object using the specified encoding.
func PackString(cp CodePoint, val string, enc StringEncoding) ([]byte, error) {
	var encoded []byte
	var err error

	switch enc {
	case EncodingCP500:
		encoded, err = converters.EncodeCP500(val)
		if err != nil {
			return nil, fmt.Errorf("failed to encode string to CP500: %w", err)
		}
	default:
		encoded = []byte(val)
	}

	return PackDDMObject(cp, encoded), nil
}

// PackNullString encodes a nullable string for SQL statements / parameters.
func PackNullString(val *string, enc StringEncoding) []byte {
	if val == nil {
		return []byte{0xFF}
	}
	var b []byte
	switch enc {
	case EncodingCP500:
		var err error
		b, err = converters.EncodeCP500(*val)
		if err != nil {
			b = []byte(*val)
		}
	default:
		b = []byte(*val)
	}

	buf := make([]byte, 1+4+len(b))
	buf[0] = 0x00
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(b)))
	copy(buf[5:], b)
	return buf
}

// DefaultManagerLevels returns standard client manager levels for Db2 DRDA negotiation.
func DefaultManagerLevels() []ManagerLevel {
	return []ManagerLevel{
		{Manager: CodePointAGENT, Level: 10},
		{Manager: CodePointSQLAM, Level: 11},
		{Manager: CodePointCMNTCPIP, Level: 5},
		{Manager: CodePointRDB, Level: 12},
		{Manager: CodePointSECMGR, Level: 9},
		{Manager: CodePointUNICODEMGR, Level: 1208},
	}
}

// PackEXCSAT builds the EXCSAT (Exchange Server Attributes) DDM command.
func PackEXCSAT(extName, srvName, srvRlsLv, srvClsNm string, mgrLevels []ManagerLevel) ([]byte, error) {
	var body []byte

	// 1. External Name (EXTNAM)
	extNameObj, err := PackString(CodePointEXTNAM, extName, EncodingCP500)
	if err != nil {
		return nil, err
	}
	body = append(body, extNameObj...)

	// 2. Server Name (SRVNAM)
	srvNameObj, err := PackString(CodePointSRVNAM, srvName, EncodingCP500)
	if err != nil {
		return nil, err
	}
	body = append(body, srvNameObj...)

	// 3. Server Release Level (SRVRLSLV)
	srvRlsLvObj, err := PackString(CodePointSRVRLSLV, srvRlsLv, EncodingCP500)
	if err != nil {
		return nil, err
	}
	body = append(body, srvRlsLvObj...)

	// 4. Manager-Level List (MGRLVLLS)
	mgrListBytes := make([]byte, len(mgrLevels)*4)
	for i, ml := range mgrLevels {
		binary.BigEndian.PutUint16(mgrListBytes[i*4:i*4+2], uint16(ml.Manager))
		binary.BigEndian.PutUint16(mgrListBytes[i*4+2:i*4+4], ml.Level)
	}
	body = append(body, PackBytes(CodePointMGRLVLLS, mgrListBytes)...)

	// 5. Server Class Name (SRVCLSNM)
	srvClsNmObj, err := PackString(CodePointSRVCLSNM, srvClsNm, EncodingCP500)
	if err != nil {
		return nil, err
	}
	body = append(body, srvClsNmObj...)

	return PackDDMObject(CodePointEXCSAT, body), nil
}

// PackEXCSAT_MGRLVLLS builds an EXCSAT packet containing only MGRLVLLS (e.g. for setting CCSIDMGR).
func PackEXCSAT_MGRLVLLS(mgrLevels []ManagerLevel) []byte {
	mgrListBytes := make([]byte, len(mgrLevels)*4)
	for i, ml := range mgrLevels {
		binary.BigEndian.PutUint16(mgrListBytes[i*4:i*4+2], uint16(ml.Manager))
		binary.BigEndian.PutUint16(mgrListBytes[i*4+2:i*4+4], ml.Level)
	}
	body := PackBytes(CodePointMGRLVLLS, mgrListBytes)
	return PackDDMObject(CodePointEXCSAT, body)
}

// PackACCSEC builds the ACCSEC (Access Security) DDM command.
func PackACCSEC(database string, secmec uint16, sectkn []byte) ([]byte, error) {
	var body []byte

	// 1. Security Mechanism (SECMEC)
	body = append(body, PackUint16(CodePointSECMEC, secmec)...)

	// 2. Database Name (RDBNAM) in CP500
	rdbNamObj, err := PackString(CodePointRDBNAM, database, EncodingCP500)
	if err != nil {
		return nil, err
	}
	body = append(body, rdbNamObj...)

	// 3. Optional Security Token (SECTKN)
	if len(sectkn) > 0 {
		body = append(body, PackBytes(CodePointSECTKN, sectkn)...)
	}

	return PackDDMObject(CodePointACCSEC, body), nil
}

// PackSECCHK builds the SECCHK (Security Check) DDM command for user authentication.
func PackSECCHK(secmec uint16, sectkn []byte, database, user, password string, enc StringEncoding) ([]byte, error) {
	var body []byte

	// 1. Security Mechanism (SECMEC)
	body = append(body, PackUint16(CodePointSECMEC, secmec)...)

	// 2. Database Name (RDBNAM)
	rdbNamObj, err := PackString(CodePointRDBNAM, database, enc)
	if err != nil {
		return nil, err
	}
	body = append(body, rdbNamObj...)

	// 3. User ID (USRID)
	usrIdObj, err := PackString(CodePointUSRID, user, enc)
	if err != nil {
		return nil, err
	}
	body = append(body, usrIdObj...)

	// 4. Password (PASSWORD)
	pwdObj, err := PackString(CodePointPASSWORD, password, enc)
	if err != nil {
		return nil, err
	}
	body = append(body, pwdObj...)

	return PackDDMObject(CodePointSECCHK, body), nil
}

// Default constants for ACCRDB
var (
	defaultCRRTKN = []byte{
		0xD5, 0xC6, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF1,
		0x2E, 0xC3, 0xF0, 0xC1, 0xF5, 0x01, 0x55, 0x63,
		0x0D, 0x5A, 0x11,
	}
	defaultTYPDEFOVR = []byte{
		0x00, 0x06, 0x11, 0x9C, 0x04, 0xB8,
		0x00, 0x06, 0x11, 0x9D, 0x04, 0xB0,
		0x00, 0x06, 0x11, 0x9E, 0x04, 0xB8,
	}
)

// PackACCRDB builds the ACCRDB (Access Relational Database) DDM command.
func PackACCRDB(prdid, rdbnam string, enc StringEncoding) ([]byte, error) {
	var body []byte

	// 1. RDBNAM
	rdbNamObj, err := PackString(CodePointRDBNAM, rdbnam, enc)
	if err != nil {
		return nil, err
	}
	body = append(body, rdbNamObj...)

	// 2. RDBACCCL (SQLAM = 0x2407)
	body = append(body, PackUint16(CodePointRDBACCCL, uint16(CodePointSQLAM))...)

	// 3. PRDID
	prdidObj, err := PackString(CodePointPRDID, prdid, enc)
	if err != nil {
		return nil, err
	}
	body = append(body, prdidObj...)

	// 4. TYPDEFNAM ("QTDSQLX86")
	typDefNamObj, err := PackString(CodePointTYPDEFNAM, "QTDSQLX86", enc)
	if err != nil {
		return nil, err
	}
	body = append(body, typDefNamObj...)

	// 5. CRRTKN
	body = append(body, PackBytes(CodePointCRRTKN, defaultCRRTKN)...)

	// 6. TYPDEFOVR
	body = append(body, PackBytes(CodePointTYPDEFOVR, defaultTYPDEFOVR)...)

	return PackDDMObject(CodePointACCRDB, body), nil
}

// PackRDBCMM builds the RDBCMM (Relational Database Commit) DDM command.
func PackRDBCMM() []byte {
	return PackDDMObject(CodePointRDBCMM, nil)
}

// PackRDBRLLBCK builds the RDBRLLBCK (Relational Database Rollback) DDM command.
func PackRDBRLLBCK() []byte {
	return PackDDMObject(CodePointRDBRLLBCK, nil)
}

// PackPKGNAMCSN formats the package name, consistency token, and section number.
func PackPKGNAMCSN(database, pkgid, pkgcnstkn string, pkgsn uint16) []byte {
	dbPadded := fmt.Sprintf("%-18s", database)
	nullidPadded := fmt.Sprintf("%-18s", "NULLID")
	pkgidPadded := fmt.Sprintf("%-18s", pkgid)

	var payload []byte
	payload = append(payload, []byte(dbPadded)...)
	payload = append(payload, []byte(nullidPadded)...)
	payload = append(payload, []byte(pkgidPadded)...)

	if pkgcnstkn == "" {
		payload = append(payload, []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}...)
	} else {
		tokenPadded := fmt.Sprintf("%8s", pkgcnstkn)
		payload = append(payload, []byte(tokenPadded)...)
	}

	snBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(snBytes, pkgsn)
	payload = append(payload, snBytes...)

	return PackBytes(CodePointPKGNAMCSN, payload)
}

// PackEXCSQLSET builds an EXCSQLSET command (e.g. for setting client workstation name).
func PackEXCSQLSET(pkgid, pkgcnstkn string, pkgsn uint16, database string) []byte {
	body := PackPKGNAMCSN(database, pkgid, pkgcnstkn, pkgsn)
	return PackDDMObject(CodePointEXCSQLSET, body)
}

// PackSQLSTT builds an SQLSTT object containing a raw SQL query.
func PackSQLSTT(sql string) []byte {
	body := append(PackNullString(&sql, EncodingUTF8), PackNullString(nil, EncodingUTF8)...)
	return PackDDMObject(CodePointSQLSTT, body)
}

// ParseDDMReply parses a response buffer into a map of CodePoint to parameter data bytes.
func ParseDDMReply(data []byte) (map[CodePoint][]byte, error) {
	results := make(map[CodePoint][]byte)
	offset := 0

	for offset < len(data) {
		if offset+4 > len(data) {
			return nil, errors.New("incomplete DDM parameter header in reply")
		}

		paramLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		codePoint := CodePoint(binary.BigEndian.Uint16(data[offset+2 : offset+4]))

		if paramLen < 4 || offset+paramLen > len(data) {
			return nil, fmt.Errorf("invalid DDM parameter length %d at offset %d", paramLen, offset)
		}

		paramData := data[offset+4 : offset+paramLen]
		results[codePoint] = paramData
		offset += paramLen
	}

	return results, nil
}

// ParseSQLCARD parses the SQL Communications Area (SQLCARD) structure into SQLCODE, SQLSTATE, and Message.
func ParseSQLCARD(obj []byte, endian binary.ByteOrder) (int32, string, string, error) {
	if len(obj) == 0 {
		return 0, "", "", errors.New("empty SQLCARD payload")
	}
	if obj[0] == 0xFF {
		return 0, "", "", nil // Valid empty / no error
	}
	if len(obj) < 18 {
		return 0, "", "", fmt.Errorf("SQLCARD payload too short: %d bytes", len(obj))
	}

	sqlcode := int32(endian.Uint32(obj[1:5]))
	sqlstate := string(obj[5:10])

	// Try extracting human-readable message if present
	var message string
	if len(obj) > 36+18 {
		rest := obj[36+18:]
		// Extract RDB name
		if len(rest) >= 2 {
			rdbLen := int(binary.BigEndian.Uint16(rest[:2]))
			if len(rest) >= 2+rdbLen {
				rest = rest[2+rdbLen:]
			}
		}
		// Extract error message
		if len(rest) >= 2 {
			msgLen := int(binary.BigEndian.Uint16(rest[:2]))
			if len(rest) >= 2+msgLen {
				rawMsg := rest[2 : 2+msgLen]
				message = string(rawMsg)
				message = strings.ReplaceAll(message, "\xff", ", ")
			}
		}
	}

	return sqlcode, sqlstate, message, nil
}
