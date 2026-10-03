package converters

import (
	"encoding/binary"
	"testing"
)

func BenchmarkDecodeCP500(b *testing.B) {
	data := []byte{0xC9, 0xC2, 0xD4, 0x40, 0xC4, 0xA2, 0xF2} // "IBM Db2"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodeCP500(data)
	}
}

func BenchmarkEncodeCP500(b *testing.B) {
	str := "IBM Db2 Pure Go Driver High Performance"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = EncodeCP500(str)
	}
}

func BenchmarkDecodePackedDecimal(b *testing.B) {
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x8C} // 123456.78
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodePackedDecimal(data, 2)
	}
}

func BenchmarkDecodeUTF16BE(b *testing.B) {
	data := []byte{0x00, 0x49, 0x00, 0x42, 0x00, 0x4D, 0x00, 0x20, 0x00, 0x44, 0x00, 0x62, 0x00, 0x32} // "IBM Db2"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodeUTF16BE(data)
	}
}

func BenchmarkEncodePackedDecimalParam(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encodePackedDecimalParam(199.99, 10, 2)
	}
}

func BenchmarkParseSQLDTARD(b *testing.B) {
	// 5 output parameters: Smallint, Integer, VarChar, Timestamp, Decimal
	dscBytes := []byte{
		0x12, 0x76, 0xD0,
		DRDATypeSmall, 0x00, 0x02,
		DRDATypeInteger, 0x00, 0x04,
		DRDATypeVarChar, 0x00, 0x0A,
		DRDATypeTimestamp, 0x00, 0x1A,
		DRDATypeDecimal, 0x08, 0x02,
	}

	var dtaBuf []byte
	dtaBuf = append(dtaBuf, 0xFF, 0x00) // header
	// Smallint 42
	dtaBuf = append(dtaBuf, 0x00, 0x2A)
	// Integer 1000
	dtaBuf = append(dtaBuf, 0x00, 0x00, 0x03, 0xE8)
	// VarChar "Hello"
	dtaBuf = append(dtaBuf, 0x00, 0x05)
	dtaBuf = append(dtaBuf, "Hello"...)
	// Timestamp "2026-03-30-12.34.56.789012"
	dtaBuf = append(dtaBuf, "2026-03-30-12.34.56.789012"...)
	// Decimal 123456.78
	dtaBuf = append(dtaBuf, 0x01, 0x23, 0x45, 0x67, 0x8C)

	makeObj := func(cp uint16, body []byte) []byte {
		buf := make([]byte, 4+len(body))
		binary.BigEndian.PutUint16(buf[0:2], uint16(len(buf)))
		binary.BigEndian.PutUint16(buf[2:4], cp)
		copy(buf[4:], body)
		return buf
	}

	var payload []byte
	payload = append(payload, makeObj(0x0010, dscBytes)...)
	payload = append(payload, makeObj(0x147A, dtaBuf)...)

	endian := binary.BigEndian

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSQLDTARD(payload, endian)
		if err != nil {
			b.Fatal(err)
		}
	}
}
