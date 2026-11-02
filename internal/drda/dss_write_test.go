package drda

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

// segmentLengths walks a stream of DSS frames and returns the length of every
// segment in it, the first 6-byte header and each 2-byte continuation alike.
func segmentLengths(stream []byte) []int {
	var lengths []int
	for len(stream) >= 2 {
		l := binary.BigEndian.Uint16(stream[0:2])
		n := int(l &^ dssContinuationFlag)
		if n < 2 || n > len(stream) {
			return append(lengths, n)
		}
		lengths = append(lengths, n)
		stream = stream[n:]
	}
	return lengths
}

// A statement longer than a DDM length can hold has to go out with an
// extended length, over a DSS continued in segments of at most 32767 bytes.
func TestWriteRequestDSS_LargeStatement(t *testing.T) {
	for _, size := range []int{1000, 32000, 32753, 32754, 40000, 65535, 70000, 1 << 20} {
		sql := strings.Repeat("x", size)
		t.Run(fmt.Sprintf("%dB", size), func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := WriteRequestDSS(&buf, PackSQLSTT(sql), 1, false, false); err != nil {
				t.Fatalf("WriteRequestDSS failed: %v", err)
			}
			if _, err := WriteRequestDSS(&buf, PackRDBCMM(), 2, false, true); err != nil {
				t.Fatalf("WriteRequestDSS failed: %v", err)
			}
			for i, l := range segmentLengths(buf.Bytes()) {
				if l > 0x7FFF {
					t.Fatalf("segment %d is %d bytes, over 32767", i, l)
				}
			}

			hdr, cp, data, _, err := ReadDSS(&buf)
			if err != nil {
				t.Fatalf("ReadDSS failed: %v", err)
			}
			if cp != CodePointSQLSTT || hdr.Type != DSSTypeObject || !hdr.Chained || hdr.CorrelationID != 1 {
				t.Fatalf("got cp=%04X type=%d chained=%v id=%d, want a chained SQLSTT object with id 1",
					uint16(cp), hdr.Type, hdr.Chained, hdr.CorrelationID)
			}
			want := append(PackNullString(&sql, EncodingUTF8), PackNullString(nil, EncodingUTF8)...)
			if !bytes.Equal(data, want) {
				t.Fatalf("SQLSTT read back as %d bytes, want %d", len(data), len(want))
			}

			hdr, cp, _, _, err = ReadDSS(&buf)
			if err != nil {
				t.Fatalf("reading the next request failed: %v", err)
			}
			if cp != CodePointRDBCMM || hdr.Chained || hdr.CorrelationID != 2 {
				t.Fatalf("next request misread: cp=%04X chained=%v id=%d", uint16(cp), hdr.Chained, hdr.CorrelationID)
			}
			if buf.Len() != 0 {
				t.Fatalf("%d bytes left over after the last request", buf.Len())
			}
		})
	}
}
