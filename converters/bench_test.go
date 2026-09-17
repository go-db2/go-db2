package converters

import (
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
