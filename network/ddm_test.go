package network

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/go-db2/go-db2/converters"
)

func TestPackEXCSAT(t *testing.T) {
	mgrLevels := DefaultManagerLevels()
	excsatBytes, err := PackEXCSAT("go-db2", "myhost", "go-db2-v0.1", "go-db2", mgrLevels)
	if err != nil {
		t.Fatalf("PackEXCSAT failed: %v", err)
	}

	if len(excsatBytes) < 4 {
		t.Fatalf("PackEXCSAT output too short: %d", len(excsatBytes))
	}

	totalLen := binary.BigEndian.Uint16(excsatBytes[0:2])
	if int(totalLen) != len(excsatBytes) {
		t.Errorf("EXCSAT length mismatch: header says %d, actual is %d", totalLen, len(excsatBytes))
	}

	cp := CodePoint(binary.BigEndian.Uint16(excsatBytes[2:4]))
	if cp != CodePointEXCSAT {
		t.Errorf("expected Codepoint EXCSAT (0x1041), got %v", cp)
	}

	// Parse sub-objects
	subObjects, err := ParseDDMReply(excsatBytes[4:])
	if err != nil {
		t.Fatalf("ParseDDMReply failed on EXCSAT payload: %v", err)
	}

	// Verify EXTNAM
	if extNameBytes, ok := subObjects[CodePointEXTNAM]; !ok {
		t.Errorf("missing EXTNAM in EXCSAT")
	} else if decoded := converters.DecodeCP500(extNameBytes); decoded != "go-db2" {
		t.Errorf("EXTNAM = %q, want 'go-db2'", decoded)
	}

	// Verify SRVNAM
	if srvNameBytes, ok := subObjects[CodePointSRVNAM]; !ok {
		t.Errorf("missing SRVNAM in EXCSAT")
	} else if decoded := converters.DecodeCP500(srvNameBytes); decoded != "myhost" {
		t.Errorf("SRVNAM = %q, want 'myhost'", decoded)
	}

	// Verify MGRLVLLS
	if mgrBytes, ok := subObjects[CodePointMGRLVLLS]; !ok {
		t.Errorf("missing MGRLVLLS in EXCSAT")
	} else {
		if len(mgrBytes) != len(mgrLevels)*4 {
			t.Errorf("MGRLVLLS length = %d, want %d", len(mgrBytes), len(mgrLevels)*4)
		}
	}
}

func TestPackACCSEC(t *testing.T) {
	secToken := []byte{0x01, 0x02, 0x03, 0x04}
	accsecBytes, err := PackACCSEC("TESTDB", SecMecUSRIDPWD, secToken)
	if err != nil {
		t.Fatalf("PackACCSEC failed: %v", err)
	}

	cp := CodePoint(binary.BigEndian.Uint16(accsecBytes[2:4]))
	if cp != CodePointACCSEC {
		t.Errorf("expected Codepoint ACCSEC (0x106D), got %v", cp)
	}

	subObjects, err := ParseDDMReply(accsecBytes[4:])
	if err != nil {
		t.Fatalf("ParseDDMReply failed on ACCSEC: %v", err)
	}

	// Verify SECMEC
	if secmecBytes, ok := subObjects[CodePointSECMEC]; !ok {
		t.Errorf("missing SECMEC in ACCSEC")
	} else {
		secmecVal := binary.BigEndian.Uint16(secmecBytes)
		if secmecVal != SecMecUSRIDPWD {
			t.Errorf("SECMEC = %d, want %d", secmecVal, SecMecUSRIDPWD)
		}
	}

	// Verify RDBNAM
	if rdbBytes, ok := subObjects[CodePointRDBNAM]; !ok {
		t.Errorf("missing RDBNAM in ACCSEC")
	} else if decoded := converters.DecodeCP500(rdbBytes); decoded != "TESTDB" {
		t.Errorf("RDBNAM = %q, want 'TESTDB'", decoded)
	}

	// Verify SECTKN
	if sectknBytes, ok := subObjects[CodePointSECTKN]; !ok {
		t.Errorf("missing SECTKN in ACCSEC")
	} else if !bytes.Equal(sectknBytes, secToken) {
		t.Errorf("SECTKN mismatch: got %X, want %X", sectknBytes, secToken)
	}
}

func makeQRYDSCPayload(numFields int) []byte {
	var buf []byte
	for i := 0; i < numFields; {
		chunkSize := numFields - i
		if chunkSize > 84 {
			chunkSize = 84
		}
		groupLen := (1 + chunkSize) * 3
		buf = append(buf, byte(groupLen), 0x76, 0xD0)
		for j := 0; j < chunkSize; j++ {
			fieldIdx := i + j
			buf = append(buf, byte(fieldIdx%256), 0x01, 0x02)
		}
		i += chunkSize
	}
	return buf
}

func TestParseQRYDSC(t *testing.T) {
	t.Run("SingleGroup_5Fields", func(t *testing.T) {
		payload := makeQRYDSCPayload(5)
		fields, err := ParseQRYDSC(payload)
		if err != nil {
			t.Fatalf("ParseQRYDSC failed: %v", err)
		}
		if len(fields) != 5 {
			t.Fatalf("expected 5 fields, got %d", len(fields))
		}
		for i, f := range fields {
			if f.Type != byte(i%256) {
				t.Errorf("field %d: expected type %d, got %d", i, i%256, f.Type)
			}
			if len(f.PS) != 2 || f.PS[0] != 0x01 || f.PS[1] != 0x02 {
				t.Errorf("field %d: unexpected PS %v", i, f.PS)
			}
		}
	})

	t.Run("MultiGroup_100Fields", func(t *testing.T) {
		payload := makeQRYDSCPayload(100)
		fields, err := ParseQRYDSC(payload)
		if err != nil {
			t.Fatalf("ParseQRYDSC failed for 100 fields: %v", err)
		}
		if len(fields) != 100 {
			t.Fatalf("expected 100 fields, got %d", len(fields))
		}
		for i, f := range fields {
			if f.Type != byte(i%256) {
				t.Errorf("field %d: expected type %d, got %d", i, i%256, f.Type)
			}
		}
	})
}

func TestPackPKGNAMCSN(t *testing.T) {
	pkgBytes := PackPKGNAMCSN("SAMPLE", "SYSH200", "TOKEN12", 1)
	if len(pkgBytes) < 4 {
		t.Fatalf("PackPKGNAMCSN output too short: %d", len(pkgBytes))
	}

	totalLen := binary.BigEndian.Uint16(pkgBytes[0:2])
	if int(totalLen) != len(pkgBytes) {
		t.Errorf("PKGNAMCSN length mismatch: header says %d, actual is %d", totalLen, len(pkgBytes))
	}

	cp := CodePoint(binary.BigEndian.Uint16(pkgBytes[2:4]))
	if cp != CodePointPKGNAMCSN {
		t.Errorf("expected Codepoint PKGNAMCSN (0x2113), got %v", cp)
	}

	payload := pkgBytes[4:]
	expectedPayload := []byte("SAMPLE            " + "NULLID            " + "SYSH200           " + " TOKEN12")
	expectedPayload = append(expectedPayload, 0x00, 0x01) // uint16(1)

	if !bytes.Equal(payload, expectedPayload) {
		t.Errorf("payload mismatch:\ngot:  %q\nwant: %q", payload, expectedPayload)
	}

	// Test with empty consistency token
	pkgBytesEmptyToken := PackPKGNAMCSN("SAMPLE", "SYSH200", "", 1)
	payloadEmptyToken := pkgBytesEmptyToken[4:]
	expectedEmptyTokenPayload := []byte("SAMPLE            " + "NULLID            " + "SYSH200           ")
	expectedEmptyTokenPayload = append(expectedEmptyTokenPayload, []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01}...)
	expectedEmptyTokenPayload = append(expectedEmptyTokenPayload, 0x00, 0x01)

	if !bytes.Equal(payloadEmptyToken, expectedEmptyTokenPayload) {
		t.Errorf("payload mismatch for empty token:\ngot:  %v\nwant: %v", payloadEmptyToken, expectedEmptyTokenPayload)
	}
}

func TestPackCommands(t *testing.T) {
	// 1. PackPRPSQLSTT
	prpBytes := PackPRPSQLSTT("SYSH200", "TOKEN12", 1, "SAMPLE")
	if int(binary.BigEndian.Uint16(prpBytes[0:2])) != len(prpBytes) {
		t.Errorf("PackPRPSQLSTT length mismatch: %d vs %d", binary.BigEndian.Uint16(prpBytes[0:2]), len(prpBytes))
	}
	if CodePoint(binary.BigEndian.Uint16(prpBytes[2:4])) != CodePointPRPSQLSTT {
		t.Errorf("PackPRPSQLSTT codepoint mismatch: %v", binary.BigEndian.Uint16(prpBytes[2:4]))
	}
	subObjs, err := ParseDDMReply(prpBytes[4:])
	if err != nil {
		t.Fatalf("ParseDDMReply failed on PRPSQLSTT: %v", err)
	}
	if _, ok := subObjs[CodePointPKGNAMCSN]; !ok {
		t.Errorf("missing PKGNAMCSN in PRPSQLSTT")
	}
	if rtn, ok := subObjs[CodePointRTNSQLDA]; !ok || !bytes.Equal(rtn, []byte{241}) {
		t.Errorf("unexpected RTNSQLDA in PRPSQLSTT: %v", rtn)
	}

	// 2. PackEXCSQLIMM
	immBytes := PackEXCSQLIMM("SYSH200", "TOKEN12", 1, "SAMPLE")
	if int(binary.BigEndian.Uint16(immBytes[0:2])) != len(immBytes) {
		t.Errorf("PackEXCSQLIMM length mismatch")
	}
	if CodePoint(binary.BigEndian.Uint16(immBytes[2:4])) != CodePointEXCSQLIMM {
		t.Errorf("PackEXCSQLIMM codepoint mismatch")
	}
	subObjs, err = ParseDDMReply(immBytes[4:])
	if err != nil {
		t.Fatalf("ParseDDMReply failed on EXCSQLIMM: %v", err)
	}
	if cmt, ok := subObjs[CodePointRDBCMTOK]; !ok || !bytes.Equal(cmt, []byte{241}) {
		t.Errorf("unexpected RDBCMTOK in EXCSQLIMM: %v", cmt)
	}

	// 3. PackDSCSQLSTT
	dscBytes := PackDSCSQLSTT("SYSH200", "TOKEN12", 1, "SAMPLE")
	if CodePoint(binary.BigEndian.Uint16(dscBytes[2:4])) != CodePointDSCSQLSTT {
		t.Errorf("PackDSCSQLSTT codepoint mismatch")
	}
	subObjs, err = ParseDDMReply(dscBytes[4:])
	if err != nil {
		t.Fatalf("ParseDDMReply failed on DSCSQLSTT: %v", err)
	}
	if typ, ok := subObjs[CodePointTYPSQLDA]; !ok || !bytes.Equal(typ, []byte{1}) {
		t.Errorf("unexpected TYPSQLDA in DSCSQLSTT: %v", typ)
	}

	// 4. PackEXCSQLSTT
	excBytes := PackEXCSQLSTT("SYSH200", "TOKEN12", 1, "SAMPLE")
	if CodePoint(binary.BigEndian.Uint16(excBytes[2:4])) != CodePointEXCSQLSTT {
		t.Errorf("PackEXCSQLSTT codepoint mismatch")
	}
	subObjs, err = ParseDDMReply(excBytes[4:])
	if err != nil {
		t.Fatalf("ParseDDMReply failed on EXCSQLSTT: %v", err)
	}
	if cmt, ok := subObjs[CodePointRDBCMTOK]; !ok || !bytes.Equal(cmt, []byte{241}) {
		t.Errorf("unexpected RDBCMTOK in EXCSQLSTT: %v", cmt)
	}

	// 5. PackOPNQRY
	opnBytes := PackOPNQRY("SYSH200", "TOKEN12", 1, "SAMPLE", 32767)
	if int(binary.BigEndian.Uint16(opnBytes[0:2])) != len(opnBytes) {
		t.Errorf("PackOPNQRY length mismatch")
	}
	if CodePoint(binary.BigEndian.Uint16(opnBytes[2:4])) != CodePointOPNQRY {
		t.Errorf("PackOPNQRY codepoint mismatch")
	}
	subObjs, err = ParseDDMReply(opnBytes[4:])
	if err != nil {
		t.Fatalf("ParseDDMReply failed on OPNQRY: %v", err)
	}
	if _, ok := subObjs[CodePointPKGNAMCSN]; !ok {
		t.Errorf("missing PKGNAMCSN in OPNQRY")
	}
	if blk, ok := subObjs[CodePointQRYBLKSZ]; !ok || binary.BigEndian.Uint32(blk) != 32767 {
		t.Errorf("unexpected QRYBLKSZ in OPNQRY: %v", blk)
	}
	if maxBlk, ok := subObjs[CodePointMAXBLKEXT]; !ok || binary.BigEndian.Uint16(maxBlk) != 32767 {
		t.Errorf("unexpected MAXBLKEXT in OPNQRY: %v", maxBlk)
	}
	if cls, ok := subObjs[CodePointQRYCLSIMP]; !ok || !bytes.Equal(cls, []byte{0x01}) {
		t.Errorf("unexpected QRYCLSIMP in OPNQRY: %v", cls)
	}

	// 6. PackEXCSQLSET and PackSQLINTR
	setBytes := PackEXCSQLSET("SYSH200", "TOKEN12", 1, "SAMPLE")
	if CodePoint(binary.BigEndian.Uint16(setBytes[2:4])) != CodePointEXCSQLSET {
		t.Errorf("PackEXCSQLSET codepoint mismatch")
	}
	intrBytes := PackSQLINTR("SYSH200", "TOKEN12", 1, "SAMPLE")
	if CodePoint(binary.BigEndian.Uint16(intrBytes[2:4])) != CodePointSQLINTR {
		t.Errorf("PackSQLINTR codepoint mismatch")
	}
}

func BenchmarkPackPKGNAMCSN(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PackPKGNAMCSN("SAMPLE", "SYSH200", "TOKEN12", 1)
	}
}

func BenchmarkPackPRPSQLSTT(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PackPRPSQLSTT("SYSH200", "TOKEN12", 1, "SAMPLE")
	}
}

func BenchmarkPackEXCSQLIMM(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PackEXCSQLIMM("SYSH200", "TOKEN12", 1, "SAMPLE")
	}
}

func BenchmarkPackOPNQRY(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PackOPNQRY("SYSH200", "TOKEN12", 1, "SAMPLE", 32767)
	}
}

func BenchmarkPackSQLSTT(b *testing.B) {
	sql := "SELECT ID, NAME, SALARY FROM EMPLOYEE WHERE DEPT = ?"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PackSQLSTT(sql)
	}
}

func BenchmarkPackCNTQRY(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PackCNTQRY("SYSH200", "TOKEN12", 1, "SAMPLE", 32767, 12345678)
	}
}

func BenchmarkPackOPNQRYWithParams(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PackOPNQRYWithParams("SYSH200", "TOKEN12", 1, "SAMPLE", 32767)
	}
}

func BenchmarkParseQRYDSC(b *testing.B) {
	benchmarks := []struct {
		name      string
		numFields int
	}{
		{"Small_5Cols", 5},
		{"Medium_20Cols", 20},
		{"Large_60Cols", 60},
	}

	for _, bm := range benchmarks {
		payload := makeQRYDSCPayload(bm.numFields)
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = ParseQRYDSC(payload)
			}
		})
	}
}

func TestParseSQLDARD_TruncatedPayloadBounds(t *testing.T) {
	// Truncated SQLDARD payload claiming 2 columns, but only enough bytes for 1 column + partial trailing metadata
	var buf []byte
	buf = append(buf, 0x01)       // Indicator (no SQLCARD, no name)
	buf = append(buf, 0x00, 0x02) // numCols = 2

	// Column 1 (16 bytes)
	buf = append(buf, 0x00, 0x0A)                                     // prec = 10
	buf = append(buf, 0x00, 0x02)                                     // scale = 2
	buf = append(buf, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04) // length = 4
	buf = append(buf, 0x01, 0xF4)                                     // sqltype = 500

	// Trailing metadata for Column 1 truncated to 20 bytes (< 29 bytes required)
	buf = append(buf, make([]byte, 20)...)

	cols, err := ParseSQLDARD(buf, binary.BigEndian)
	if err != nil {
		t.Fatalf("unexpected error parsing truncated SQLDARD: %v", err)
	}

	// Should safely parse 1 column and not attempt to parse column 2 from truncated trailing bytes
	if len(cols) != 1 {
		t.Fatalf("expected 1 column parsed, got %d", len(cols))
	}
}

func TestParseSQLDARD_ExcessiveNumColsCap(t *testing.T) {
	// Construct payload claiming 65535 columns, but providing payload for only 1 column
	var buf []byte
	buf = append(buf, 0xFF)       // Indicator 0xFF: no leading SQLCARD
	buf = append(buf, 0x01)       // Indicator (hasName = false)
	buf = append(buf, 0xFF, 0xFF) // numCols = 65535

	// Column 1 (16 bytes)
	buf = append(buf, 0x00, 0x0A)                                     // prec = 10
	buf = append(buf, 0x00, 0x02)                                     // scale = 2
	buf = append(buf, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04) // length = 4
	buf = append(buf, 0x01, 0xF4)                                     // sqltype = 500
	buf = append(buf, make([]byte, 29)...)                            // Trailing metadata

	cols, err := ParseSQLDARD(buf, binary.BigEndian)
	if err != nil {
		t.Fatalf("unexpected error parsing excessive numCols SQLDARD: %v", err)
	}

	if len(cols) != 1 {
		t.Fatalf("expected 1 column parsed, got %d", len(cols))
	}
}

func TestParseSQLCARD_TruncatedPayloadBounds(t *testing.T) {
	// Construct SQLCARD payload with truncated RDB name length
	var buf []byte
	buf = append(buf, 0x00)                   // Non-null indicator
	buf = append(buf, 0x00, 0x00, 0x00, 0x00) // SQLCODE = 0
	buf = append(buf, []byte("00000")...)     // SQLSTATE
	buf = append(buf, make([]byte, 45)...)    // Header padding to exceed 36+18 threshold

	// RDB name length says 100 bytes, but payload ends immediately
	buf = append(buf, 0x00, 100)

	code, state, msg, _, err := ParseSQLCARD(buf, binary.BigEndian)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 || state != "00000" {
		t.Fatalf("unexpected code/state: %d / %s", code, state)
	}
	if msg != "" {
		t.Fatalf("expected empty message for truncated RDB name, got %q", msg)
	}
}
