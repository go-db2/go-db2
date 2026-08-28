package converters

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestGraphicUTF16BE_EncodeDecode(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ASCII", "Hello World"},
		{"Portuguese", "Ação, Coração, Informática, São Paulo"},
		{"Japanese Kanji/Kana", "日本語のテスト (Tokyo/Osaka)"},
		{"Chinese Simplified/Traditional", "数据库 / 資料庫"},
		{"Emoji with Surrogate Pairs", "🚀 Go-Db2 Pure Go 📦 ✨"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeUTF16BE(tt.input)
			if len(encoded)%2 != 0 {
				t.Fatalf("encoded length must be even, got %d", len(encoded))
			}
			decoded := DecodeUTF16BE(encoded)
			if decoded != tt.input {
				t.Fatalf("mismatch: expected %q, got %q", tt.input, decoded)
			}
		})
	}
}

func TestGraphicPadding(t *testing.T) {
	str := "DB2"
	encoded := EncodeUTF16BE(str) // 3 runes = 6 bytes
	padded := PadGraphicUTF16BE(encoded, 10) // 10 chars = 20 bytes

	if len(padded) != 20 {
		t.Fatalf("expected 20 bytes for 10 DBCS chars, got %d", len(padded))
	}

	decoded := DecodeUTF16BE(padded)
	trimmed := TrimRightGraphicSpaces(decoded)
	if trimmed != "DB2" {
		t.Fatalf("expected trimmed 'DB2', got %q", trimmed)
	}
}

func TestDecodeField_GraphicAndVarGraph(t *testing.T) {
	// 1. DRDATypeGraphic with padding
	rawGraphic := PadGraphicUTF16BE(EncodeUTF16BE("IBM"), 5) // 5 chars = 10 bytes
	psGraphic := make([]byte, 2)
	binary.BigEndian.PutUint16(psGraphic, 5) // 5 characters

	reader := bytes.NewReader(rawGraphic)
	val, err := DecodeField(DRDATypeGraphic, psGraphic, reader, binary.BigEndian)
	if err != nil {
		t.Fatalf("DecodeField Graphic failed: %v", err)
	}
	if val != "IBM" {
		t.Fatalf("expected 'IBM', got %q", val)
	}

	// 2. DRDATypeNVarGraph with length prefix (3 chars)
	text := "漢字" // 2 chars
	u16 := EncodeUTF16BE(text)
	var varGraphBuf bytes.Buffer
	varGraphBuf.WriteByte(0x00) // Not null
	var lenHeader [2]byte
	binary.BigEndian.PutUint16(lenHeader[:], uint16(len(u16)/2))
	varGraphBuf.Write(lenHeader[:])
	varGraphBuf.Write(u16)

	valVG, err := DecodeField(DRDATypeNVarGraph, []byte{0x00, 0x50}, &varGraphBuf, binary.BigEndian)
	if err != nil {
		t.Fatalf("DecodeField VarGraph failed: %v", err)
	}
	if valVG != text {
		t.Fatalf("expected %q, got %q", text, valVG)
	}
}
