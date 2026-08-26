package network

import (
	"bytes"
	"testing"
)

func TestWriteAndReadRequestDSS(t *testing.T) {
	// Create a dummy DDM payload: 2 bytes length (8) + 2 bytes EXCSAT (0x1041) + 4 bytes data
	payload := []byte{0x00, 0x08, 0x10, 0x41, 0xDE, 0xAD, 0xBE, 0xEF}

	var buf bytes.Buffer
	curID := uint16(1)

	nextID, err := WriteRequestDSS(&buf, payload, curID, false, true)
	if err != nil {
		t.Fatalf("WriteRequestDSS failed: %v", err)
	}

	if nextID != 2 {
		t.Errorf("expected nextID = 2, got %d", nextID)
	}

	rawBytes := buf.Bytes()
	if len(rawBytes) != 6+len(payload) {
		t.Fatalf("expected packet length %d, got %d", 6+len(payload), len(rawBytes))
	}

	if rawBytes[2] != DSSMagic {
		t.Errorf("expected magic 0xD0, got 0x%02X", rawBytes[2])
	}

	// Test Reading back the DSS packet
	hdr, cp, data, moreData, err := ReadDSS(&buf)
	if err != nil {
		t.Fatalf("ReadDSS failed: %v", err)
	}

	if hdr.Length != uint16(len(rawBytes)) {
		t.Errorf("expected header length %d, got %d", len(rawBytes), hdr.Length)
	}
	if hdr.Type != DSSTypeRequest {
		t.Errorf("expected DSSTypeRequest (1), got %d", hdr.Type)
	}
	if hdr.Chained {
		t.Errorf("expected Chained = false, got true")
	}
	if hdr.CorrelationID != 1 {
		t.Errorf("expected CorrelationID = 1, got %d", hdr.CorrelationID)
	}
	if cp != CodePointEXCSAT {
		t.Errorf("expected Codepoint EXCSAT (0x1041), got %v", cp)
	}
	if !bytes.Equal(data, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("payload mismatch: got %X, want DEADBEEF", data)
	}
	if moreData {
		t.Errorf("expected moreData = false, got true")
	}
}

func TestObjectDSSTypeDetection(t *testing.T) {
	// SQLSTT should be detected as DSSTypeObject (3)
	payload := []byte{0x00, 0x06, 0x24, 0x14, 0x01, 0x02} // SQLSTT = 0x2414

	var buf bytes.Buffer
	_, err := WriteRequestDSS(&buf, payload, 1, true, false)
	if err != nil {
		t.Fatalf("WriteRequestDSS failed: %v", err)
	}

	hdr, cp, _, _, err := ReadDSS(&buf)
	if err != nil {
		t.Fatalf("ReadDSS failed: %v", err)
	}

	if hdr.Type != DSSTypeObject {
		t.Errorf("expected DSSTypeObject (3), got %d", hdr.Type)
	}
	if !hdr.Chained {
		t.Errorf("expected Chained = true for non-last packet")
	}
	if !hdr.SameID {
		t.Errorf("expected SameID = true")
	}
	if cp != CodePointSQLSTT {
		t.Errorf("expected CodePointSQLSTT, got %v", cp)
	}
}

func TestInvalidDSSMagic(t *testing.T) {
	invalidBuf := bytes.NewBuffer([]byte{0x00, 0x0A, 0xAA, 0x01, 0x00, 0x01})
	_, _, _, _, err := ReadDSS(invalidBuf)
	if err == nil {
		t.Errorf("expected error for invalid magic byte, got nil")
	}
}
