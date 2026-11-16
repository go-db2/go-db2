package codec

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func FuzzDecodeField(f *testing.F) {
	f.Add(DRDATypeInteger, byte(0x00), byte(0x04), []byte{0x00, 0x00, 0x04, 0xD2}, false)
	f.Add(DRDATypeNInteger, byte(0x00), byte(0x04), []byte{0xFF}, false)
	f.Add(DRDATypeNInteger, byte(0x00), byte(0x04), []byte{0x00, 0xD2, 0x04, 0x00, 0x00}, true)
	f.Add(DRDATypeSmall, byte(0x00), byte(0x02), []byte{0x00, 0x2A}, false)
	f.Add(DRDATypeInteger8, byte(0x00), byte(0x08), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0xD2}, false)
	f.Add(DRDAType1ByteInt, byte(0x00), byte(0x01), []byte{0xFE}, false)
	f.Add(DRDATypeFloat4, byte(0x00), byte(0x04), []byte{0x40, 0x49, 0x0F, 0xDB}, false)
	f.Add(DRDATypeFloat8, byte(0x00), byte(0x08), []byte{0x40, 0x09, 0x21, 0xFB, 0x54, 0x44, 0x2D, 0x18}, false)
	f.Add(DRDATypeChar, byte(0x00), byte(0x05), []byte("HI   "), false)
	f.Add(DRDATypeVarChar, byte(0x00), byte(0x0A), []byte{0x00, 0x05, 'H', 'e', 'l', 'l', 'o'}, false)
	f.Add(DRDATypeGraphic, byte(0x00), byte(0x02), []byte{0x00, 'A', 0x00, 'B'}, false)
	f.Add(DRDATypeVarGraph, byte(0x00), byte(0x04), []byte{0x00, 0x02, 0x00, 'A', 0x00, 'B'}, false)
	f.Add(DRDATypeBoolean, byte(0x00), byte(0x00), []byte{}, false)
	f.Add(DRDATypeBoolean, byte(0x00), byte(0x01), []byte{0x01}, false)
	f.Add(DRDATypeDate, byte(0x00), byte(0x0A), []byte("2026-11-16"), false)
	f.Add(DRDATypeTime, byte(0x00), byte(0x08), []byte("12.34.56"), false)
	f.Add(DRDATypeTimestamp, byte(0x00), byte(0x1A), []byte("2026-11-16-12.34.56.123456"), false)
	f.Add(DRDATypeDecimal, byte(0x05), byte(0x02), []byte{0x01, 0x23, 0x4C}, false)
	f.Add(DRDATypeDecimal, byte(0x03), byte(0xFE), []byte{0x12, 0x3C}, false)
	f.Add(DRDATypeDecFloat, byte(0x00), byte(0x08), []byte{0x22, 0x38, 0x00, 0x00, 0x00, 0x00, 0x01, 0x23}, false)
	f.Add(DRDATypeDecFloat, byte(0x00), byte(0x10), make([]byte, 16), false)
	f.Add(DRDATypeVarBinary, byte(0x00), byte(0x0A), []byte{0x00, 0x03, 0xDE, 0xAD, 0xBE}, false)
	f.Add(DRDATypeLOBLOC, byte(0x00), byte(0x04), []byte{0x01, 0x02, 0x03, 0x04}, false)
	f.Add(byte(0xFE), byte(0x00), byte(0x04), []byte{0x01, 0x02, 0x03, 0x04}, false)

	f.Fuzz(func(t *testing.T, drdaType, ps0, ps1 uint8, data []byte, littleEndian bool) {
		var endian binary.ByteOrder = binary.BigEndian
		if littleEndian {
			endian = binary.LittleEndian
		}
		_, _ = DecodeField(drdaType, []byte{ps0, ps1}, bytes.NewReader(data), endian)
	})
}

func FuzzDecodePackedDecimal(f *testing.F) {
	f.Add([]byte{0x12, 0x3C}, int8(2))
	f.Add([]byte{0x12, 0x3C}, int8(0))
	f.Add([]byte{0x12, 0x3C}, int8(-2))
	f.Add([]byte{0x00, 0x0D}, int8(-2))
	f.Add([]byte{}, int8(0))
	f.Add(bytes.Repeat([]byte{0x11}, 35), int8(5))
	f.Add([]byte{0xFF, 0xFF}, int8(1))
	f.Add([]byte{0x12, 0x3C}, int8(-128))
	f.Add([]byte{0x12, 0x3C}, int8(127))

	f.Fuzz(func(t *testing.T, b []byte, scale int8) {
		_ = DecodePackedDecimal(b, int(scale))
	})
}

func FuzzParseSQLDTARD(f *testing.F) {
	makeObj := func(cp uint16, body []byte) []byte {
		buf := make([]byte, 4+len(body))
		binary.BigEndian.PutUint16(buf[0:2], uint16(len(buf)))
		binary.BigEndian.PutUint16(buf[2:4], cp)
		copy(buf[4:], body)
		return buf
	}

	// Simple FDODSC + FDODTA
	simple := append(
		makeObj(0x0010, []byte{0x09, 0x76, 0xD0, DRDATypeSmall, 0x00, 0x02, DRDATypeInteger, 0x00, 0x04}),
		makeObj(0x147A, []byte{0xFF, 0x00, 0x00, 0x2A, 0x00, 0x00, 0x00, 0x64})...,
	)
	f.Add(simple, false)

	// Multi-group FDODSC + FDODTA
	multi := append(
		makeObj(0x0010, []byte{0x06, 0x76, 0xD0, DRDATypeSmall, 0x00, 0x02, 0x06, 0x76, 0xD0, DRDATypeSmall, 0x00, 0x02}),
		makeObj(0x147A, []byte{0xFF, 0x00, 0x00, 0x0A, 0x00, 0x14})...,
	)
	f.Add(multi, false)

	// Extended length 0x8004 and 0x8008
	ext8004 := append(
		makeObj(0x0010, []byte{0x06, 0x76, 0xD0, DRDATypeSmall, 0x00, 0x02}),
		[]byte{0x80, 0x04, 0x14, 0x7A, 0xFF, 0x00, 0x00, 0x2A}...,
	)
	f.Add(ext8004, false)

	ext8008 := append(
		makeObj(0x0010, []byte{0x06, 0x76, 0xD0, DRDATypeSmall, 0x00, 0x02}),
		[]byte{0x80, 0x08, 0x14, 0x7A, 0x00, 0x00, 0x00, 0x04, 0xFF, 0x00, 0x00, 0x2A}...,
	)
	f.Add(ext8008, true)

	// Truncated
	f.Add([]byte{0x00, 0x08, 0x00, 0x10}, false)
	f.Add([]byte{}, false)

	f.Fuzz(func(t *testing.T, data []byte, littleEndian bool) {
		var endian binary.ByteOrder = binary.BigEndian
		if littleEndian {
			endian = binary.LittleEndian
		}
		_, _ = ParseSQLDTARD(data, endian)
	})
}

func FuzzDecodeDFP(f *testing.F) {
	dfpSeeds := []struct {
		val  string
		size int
	}{
		{"123.456", 8},
		{"0", 8},
		{"-0", 8},
		{"Infinity", 8},
		{"-Infinity", 8},
		{"NaN", 8},
		{"12345678901234567890.12345678901234", 16},
		{"Infinity", 16},
		{"NaN", 16},
	}
	for _, s := range dfpSeeds {
		b, err := EncodeDFP(s.val, s.size)
		if err != nil {
			f.Fatalf("EncodeDFP(%q, %d) seed failed: %v", s.val, s.size, err)
		}
		f.Add(b)
	}
	f.Add(bytes.Repeat([]byte{0xFF}, 8))
	f.Add([]byte{})
	f.Add([]byte{0x01})
	f.Add(make([]byte, 7))
	f.Add(make([]byte, 9))
	f.Add(make([]byte, 15))
	f.Add(make([]byte, 17))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := DecodeDFP(data)
		if len(data) != 8 && len(data) != 16 && err == nil {
			t.Fatalf("DecodeDFP(%d bytes) returned nil error, want non-nil error", len(data))
		}
	})
}

func FuzzDecodeCP500(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xC1, 0xC2, 0xC3})
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	f.Add(allBytes)

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = DecodeCP500(data)
	})
}

func FuzzDecodeUTF16BE(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x00, 'A', 0x00, 'B'})
	f.Add([]byte{0xD8, 0x3D, 0xDE, 0x80})
	f.Add([]byte{0xD8, 0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = DecodeUTF16BE(data)
	})
}
