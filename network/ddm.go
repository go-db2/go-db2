package network

import (
	"encoding/binary"
	"errors"
	"fmt"

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
