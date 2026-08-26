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
