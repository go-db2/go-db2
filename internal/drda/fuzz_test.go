package drda

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func FuzzReadDSS(f *testing.F) {
	// Valid minimal EXCSAT DSS packet
	f.Add([]byte{0x00, 0x0E, 0xD0, 0x01, 0x00, 0x01, 0x00, 0x08, 0x10, 0x41, 0xDE, 0xAD, 0xBE, 0xEF})
	// Frame length < 6 and zero length
	f.Add([]byte{0x00, 0x04, 0xD0, 0x01, 0x00, 0x01})
	f.Add([]byte{0x00, 0x00, 0xD0, 0x01, 0x00, 0x01})
	// DSS continuation flag (0x8000) + valid second segment
	f.Add([]byte{
		0x80, 0x0A, 0xD0, 0x03, 0x00, 0x01, 0x00, 0x08, 0x14, 0x6C,
		0x00, 0x06, 0xAA, 0xBB, 0xCC, 0xDD,
	})
	// DDM extended length 0x8004 (runs to end of DSS)
	f.Add([]byte{0x00, 0x0E, 0xD0, 0x03, 0x00, 0x01, 0x80, 0x04, 0x14, 0x6C, 0x01, 0x02, 0x03, 0x04})
	// DDM extended length 0x8008 (4-byte explicit length)
	f.Add([]byte{0x00, 0x12, 0xD0, 0x03, 0x00, 0x01, 0x80, 0x08, 0x14, 0x6C, 0x00, 0x00, 0x00, 0x04, 0x01, 0x02, 0x03, 0x04})
	// Invalid magic
	f.Add([]byte{0x00, 0x0A, 0xAA, 0x01, 0x00, 0x01, 0x00, 0x04, 0x10, 0x41})

	f.Fuzz(func(t *testing.T, data []byte) {
		hdr, _, _, _, err := ReadDSS(bytes.NewReader(data))
		if err == nil {
			if hdr == nil {
				t.Fatal("ReadDSS returned nil header with nil error")
			}
			if hdr.Length&0x7FFF < 6 {
				t.Fatalf("ReadDSS returned invalid header frame length: 0x%04X", hdr.Length)
			}
		}
	})
}

func FuzzParseDDMReply(f *testing.F) {
	// Normal parameters: SECMEC=3, RDBNAM="DB"
	f.Add([]byte{0x00, 0x06, 0x11, 0xA2, 0x00, 0x03, 0x00, 0x06, 0x21, 0x10, 'D', 'B'})
	// Extended 0x8004
	f.Add([]byte{0x80, 0x04, 0x11, 0x53, 'D', 'G', 'N'})
	// Extended 0x8008
	f.Add([]byte{0x80, 0x08, 0x11, 0x53, 0x00, 0x00, 0x00, 0x04, 'T', 'E', 'S', 'T'})
	// Truncated headers and empty input
	f.Add([]byte{0x80, 0x08, 0x11, 0x53, 0x00})
	f.Add([]byte{0x00, 0x02})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseDDMReply(data)
	})
}

func FuzzParseSQLDARD(f *testing.F) {
	// 0xFF (no SQLCARD), hasName=false, 2 columns
	noSQLCARD := []byte{0xFF, 0x01, 0x00, 0x02}
	noSQLCARD = append(noSQLCARD, 0x00, 0x0A, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x01, 0xF4, 0x00, 0x00)
	noSQLCARD = append(noSQLCARD, make([]byte, 29)...)
	noSQLCARD = append(noSQLCARD, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x01, 0xF0, 0x00, 0x00)
	noSQLCARD = append(noSQLCARD, make([]byte, 29)...)
	f.Add(noSQLCARD, false)

	// 0x00 + valid SQLCARD (>54 bytes) + 0xFF + named columns (normal & 0xFF,0xFF,0xFF delimiter)
	withCard := make([]byte, 54)
	withCard[0] = 0x00
	copy(withCard[5:10], "00000")
	withCard = append(withCard, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00) // 3 skipped uint16 chunks
	withCard = append(withCard, 0xFF)                               // delimiter before header
	withCard = append(withCard, 0x01)                               // rest[0] != 0x00
	withCard = append(withCard, 0x00, 0x02)                         // numCols = 2
	// Col 1 using standard hasName=true name/label blocks
	withCard = append(withCard, 0x00, 0x0A, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x01, 0xF4, 0x00, 0x00)
	withCard = append(withCard, make([]byte, 9)...)
	withCard = append(withCard, 0x00, 0x02, 'I', 'D', 0x00, 0x00) // nameBytes
	withCard = append(withCard, 0x00, 0x02, 'I', 'D', 0x00, 0x00) // labelBytes
	withCard = append(withCard, 0x00, 0x00, 0x00, 0x00)           // skipName
	withCard = append(withCard, make([]byte, 7)...)               // trailing 7 bytes
	// Col 2 using 0xFF,0xFF,0xFF delimiter
	withCard = append(withCard, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x01, 0xC0, 0x00, 0x00)
	withCard = append(withCard, 0x04, 'C', 'O', 'L', '2', 0xFF, 0xFF, 0xFF)
	f.Add(withCard, false)

	// Truncated numCols=0xFFFF
	f.Add([]byte{0xFF, 0x01, 0xFF, 0xFF, 0x00, 0x0A, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x01, 0xF4}, true)
	f.Add([]byte{}, false)

	f.Fuzz(func(t *testing.T, data []byte, littleEndian bool) {
		var endian binary.ByteOrder = binary.BigEndian
		if littleEndian {
			endian = binary.LittleEndian
		}
		cols, err := ParseSQLDARD(data, endian)
		if err == nil && len(cols) > len(data)/16 {
			t.Fatalf("ParseSQLDARD returned %d cols for %d bytes", len(cols), len(data))
		}
	})
}

func FuzzParseQRYDSC(f *testing.F) {
	// Single group
	f.Add([]byte{0x09, 0x76, 0xD0, 0x02, 0x00, 0x04, 0x33, 0x00, 0x10})
	// Multi-group (>84 fields)
	multiGroup := make([]byte, 0, 3+84*3+9)
	multiGroup = append(multiGroup, 0xFF, 0x76, 0xD0)
	multiGroup = append(multiGroup, bytes.Repeat([]byte{0x02, 0x00, 0x04}, 84)...)
	multiGroup = append(multiGroup, 0x09, 0x76, 0xD0, 0x04, 0x00, 0x02, 0x32, 0x00, 0x10)
	f.Add(multiGroup)
	// groupLen < 3 and truncated
	f.Add([]byte{0x02, 0x76, 0xD0})
	f.Add([]byte{0x0C, 0x76, 0xD0, 0x02, 0x00})
	f.Add([]byte{0x01})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseQRYDSC(data)
	})
}

func FuzzParseSQLCARD(f *testing.F) {
	// Null SQLCARD
	f.Add([]byte{0xFF}, false)
	// Minimal 00000 (18 bytes)
	minOK := make([]byte, 18)
	minOK[0] = 0x00
	copy(minOK[5:10], "00000")
	f.Add(minOK, true)

	// Negative SQLCODE with RDBNAME and \xff-separated message
	negCard := make([]byte, 54)
	negCard[0] = 0x00
	negSQLCode := int32(-204)
	binary.LittleEndian.PutUint32(negCard[1:5], uint32(negSQLCode))
	copy(negCard[5:10], "42704")
	negCard = append(negCard, 0x00, 0x06)
	negCard = append(negCard, []byte("TESTDB")...)
	negCard = append(negCard, 0x00, 0x09)
	negCard = append(negCard, []byte("ERR\xffTOKEN")...)
	f.Add(negCard, true)

	// SQLERRRM envelope wrapping SQLCARD + SRVDGN
	sqlerrrm := make([]byte, 0, 4+len(negCard)+10)
	sqlerrrm = append(sqlerrrm, byte((4+len(negCard))>>8), byte(4+len(negCard)), 0x24, 0x08)
	sqlerrrm = append(sqlerrrm, negCard...)
	sqlerrrm = append(sqlerrrm, 0x00, 0x07, 0x11, 0x53, 0xC4, 0xC2, 0xF2)
	f.Add(sqlerrrm, true)

	// Truncated
	f.Add([]byte{0x00, 0x01}, false)
	f.Add([]byte{}, false)

	f.Fuzz(func(t *testing.T, data []byte, littleEndian bool) {
		var endian binary.ByteOrder = binary.BigEndian
		if littleEndian {
			endian = binary.LittleEndian
		}
		_, _, _, _, _ = ParseSQLCARD(data, endian)
		_ = parseSQLERRRM(data, endian)
	})
}

func FuzzDecodeRowsPartial(f *testing.F) {
	// Seed 1: 0xFF, 0x00 + SmallInt + VarChar + Decimal
	dsc1 := []byte{
		0x0C, 0x76, 0xD0,
		0x04, 0x00, 0x02,
		0x32, 0x00, 0x0A,
		0x0E, 0x05, 0x02,
	}
	dta1 := []byte{
		0xFF, 0x00,
		0x00, 0x2A,
		0x00, 0x02, 'H', 'I',
		0x12, 0x34, 0x5C,
	}
	f.Add(dsc1, dta1, false)

	// Seed 2: SQLCA 0x00 + SQLCODE=0 + SQLCAXGRP + SmallInt
	dsc2 := []byte{0x06, 0x76, 0xD0, 0x04, 0x00, 0x02}
	dta2 := []byte{
		0x00,
		0x00, 0x00, 0x00, 0x00,
		'0', '0', '0', '0', '0',
		'S', 'Q', 'L', ' ', ' ', ' ', ' ', ' ',
		0x00,
	}
	dta2 = append(dta2, make([]byte, 35)...)                // SQLERRD1-6 (24B) + SQLWARN0-A (11B)
	dta2 = append(dta2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00) // 3 empty strings
	dta2 = append(dta2, 0xFF, 0x00, 0x00, 0x2A)             // SQLDIAGGRP=null, row data ind=0x00, SmallInt=42
	f.Add(dsc2, dta2, false)

	// Seed 3: Truncated row spanning mid-column
	dsc3 := []byte{0x09, 0x76, 0xD0, 0x04, 0x00, 0x02, 0x32, 0x00, 0x10}
	dta3 := []byte{0xFF, 0x00, 0x00, 0x2A, 0x00, 0x05, 'A', 'B'}
	f.Add(dsc3, dta3, false)

	// Seed 4: Empty QRYDTA
	f.Add([]byte{0x06, 0x76, 0xD0, 0x04, 0x00, 0x02}, []byte{}, true)

	f.Fuzz(func(t *testing.T, qrydsc []byte, qrydta []byte, littleEndian bool) {
		fields, err := ParseQRYDSC(qrydsc)
		if err != nil || len(fields) == 0 {
			if len(qrydsc) >= 3 {
				fields = []FieldDescriptor{{
					Type: qrydsc[0],
					PS:   [2]byte{qrydsc[1], qrydsc[2]},
				}}
			} else {
				fields = nil
			}
		}
		if len(fields) > 64 {
			fields = fields[:64]
		}

		var endian binary.ByteOrder = binary.BigEndian
		if littleEndian {
			endian = binary.LittleEndian
		}

		rows, consumed, err := decodeRowsPartial(fields, qrydta, endian)
		if consumed < 0 || consumed > len(qrydta) {
			t.Fatalf("decodeRowsPartial returned out-of-range consumed=%d (len=%d)", consumed, len(qrydta))
		}
		if err == nil {
			for i, row := range rows {
				if len(row) != len(fields) {
					t.Fatalf("row %d length = %d, want %d", i, len(row), len(fields))
				}
			}
		}
		_, _ = decodeRows(fields, qrydta, endian)
	})
}
