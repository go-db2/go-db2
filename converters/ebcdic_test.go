package converters

import (
	"testing"
)

func TestCP500EncodingDecoding(t *testing.T) {
	testStrings := []string{
		"pydrda",
		"go-db2",
		"TESTDB",
		"USER123",
		"localhost",
		"SELECT 1 FROM SYSIBM.SYSDUMMY1",
	}

	for _, s := range testStrings {
		t.Run(s, func(t *testing.T) {
			encoded, err := EncodeCP500(s)
			if err != nil {
				t.Fatalf("EncodeCP500(%q) error: %v", s, err)
			}
			decoded := DecodeCP500(encoded)
			if decoded != s {
				t.Errorf("DecodeCP500(EncodeCP500(%q)) = %q; want %q", s, decoded, s)
			}
		})
	}
}
