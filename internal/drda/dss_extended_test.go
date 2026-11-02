package drda

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// replyDSS frames body (a complete DDM object) as one reply DSS, splitting it
// into continuation segments the way a server does once it exceeds 32767 bytes.
func replyDSS(body []byte, chained bool, correlationID uint16) []byte {
	const maxSegment = 0x7FFF
	var out bytes.Buffer

	first := body
	rest := []byte(nil)
	if 6+len(body) > maxSegment {
		first, rest = body[:maxSegment-6], body[maxSegment-6:]
	}
	length := uint16(6 + len(first))
	if rest != nil {
		length |= 0x8000
	}
	flags := DSSTypeReply
	if chained {
		flags |= DSSFlagChained
	}
	hdr := make([]byte, 6)
	binary.BigEndian.PutUint16(hdr[0:2], length)
	hdr[2] = DSSMagic
	hdr[3] = flags
	binary.BigEndian.PutUint16(hdr[4:6], correlationID)
	out.Write(hdr)
	out.Write(first)

	for len(rest) > 0 {
		seg := rest
		if 2+len(seg) > maxSegment {
			seg = rest[:maxSegment-2]
		}
		rest = rest[len(seg):]
		segLen := uint16(2 + len(seg))
		if len(rest) > 0 {
			segLen |= 0x8000
		}
		out.Write(binary.BigEndian.AppendUint16(nil, segLen))
		out.Write(seg)
	}
	return out.Bytes()
}

// ddmObject builds a DDM object with a plain 2-byte length.
func ddmObject(cp CodePoint, data []byte) []byte {
	obj := binary.BigEndian.AppendUint16(nil, uint16(4+len(data)))
	obj = binary.BigEndian.AppendUint16(obj, uint16(cp))
	return append(obj, data...)
}

// ddmObjectExtended builds a DDM object whose length does not fit in 15 bits:
// the high bit is set and extBytes bytes of length follow the codepoint. With
// extBytes == 0 the object runs to the end of its DSS.
func ddmObjectExtended(cp CodePoint, data []byte, extBytes int) []byte {
	obj := binary.BigEndian.AppendUint16(nil, 0x8000|uint16(4+extBytes))
	obj = binary.BigEndian.AppendUint16(obj, uint16(cp))
	switch extBytes {
	case 4:
		obj = binary.BigEndian.AppendUint32(obj, uint32(len(data)))
	case 8:
		obj = binary.BigEndian.AppendUint64(obj, uint64(len(data)))
	}
	return append(obj, data...)
}

func TestReadDSS_ExtendedLengthObjects(t *testing.T) {
	// Db2 sends an XML value as EXTDTA with length 0x8004: no extended
	// length bytes, the object ends where the DSS ends.
	xml := []byte("\x00<root><item id=\"1\">x</item></root>")
	big := bytes.Repeat([]byte("abcdefghij"), 20000) // 200000 bytes, spans several segments

	tests := []struct {
		name string
		obj  []byte
		want []byte
	}{
		{"to end of DSS (0x8004)", ddmObjectExtended(CodePointEXTDTA, xml, 0), xml},
		{"4-byte extended length (0x8008)", ddmObjectExtended(CodePointEXTDTA, big, 4), big},
		{"8-byte extended length (0x800C)", ddmObjectExtended(CodePointEXTDTA, big, 8), big},
		{"to end of a segmented DSS", ddmObjectExtended(CodePointEXTDTA, big, 0), big},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The object is followed by another reply, which must still frame.
			var stream bytes.Buffer
			stream.Write(replyDSS(tt.obj, true, 1))
			stream.Write(replyDSS(ddmObject(CodePointSQLCARD, []byte{0xFF}), false, 1))

			_, cp, data, _, err := ReadDSS(&stream)
			if err != nil {
				t.Fatalf("ReadDSS failed: %v", err)
			}
			if cp != CodePointEXTDTA {
				t.Fatalf("codepoint = %04X, want EXTDTA", uint16(cp))
			}
			if !bytes.Equal(data, tt.want) {
				t.Fatalf("payload = %d bytes, want %d", len(data), len(tt.want))
			}

			hdr, cp, data, _, err := ReadDSS(&stream)
			if err != nil {
				t.Fatalf("reading the next reply failed: %v", err)
			}
			if cp != CodePointSQLCARD || !bytes.Equal(data, []byte{0xFF}) || hdr.Chained {
				t.Fatalf("next reply misread: cp=%04X data=%x chained=%v", uint16(cp), data, hdr.Chained)
			}
		})
	}
}

// A result set bigger than one segment arrives as a DSS continued over
// several segments; every segment has to be read before the next DSS.
func TestReadDSS_ContinuedOverSeveralSegments(t *testing.T) {
	rows := bytes.Repeat([]byte{0xFF, 0x00, 'r', 'o', 'w'}, 30000) // 150000 bytes
	var stream bytes.Buffer
	stream.Write(replyDSS(ddmObjectExtended(CodePointQRYDTA, rows, 0), true, 2))
	stream.Write(replyDSS(ddmObject(CodePointENDQRYRM, nil), false, 2))

	hdr, cp, data, _, err := ReadDSS(&stream)
	if err != nil {
		t.Fatalf("ReadDSS failed: %v", err)
	}
	if cp != CodePointQRYDTA || !hdr.Chained {
		t.Fatalf("got cp=%04X chained=%v, want chained QRYDTA", uint16(cp), hdr.Chained)
	}
	if !bytes.Equal(data, rows) {
		t.Fatalf("payload = %d bytes, want %d", len(data), len(rows))
	}

	_, cp, _, _, err = ReadDSS(&stream)
	if err != nil {
		t.Fatalf("reading the next reply failed: %v", err)
	}
	if cp != CodePointENDQRYRM {
		t.Fatalf("next reply codepoint = %04X, want ENDQRYRM", uint16(cp))
	}
}
