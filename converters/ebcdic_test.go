package converters

import (
	"bytes"
	"errors"
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

func TestEncodeCP500_ASCIIFastPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{
			name: "PureASCII_AlphaNumeric",
			in:   "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		},
		{
			name: "ASCII_Boundaries_NUL_AND_DEL",
			in:   string([]byte{0x00, 0x01, 0x20, 0x7F}),
		},
		{
			name: "UTF8_MultiByte_Transition",
			in:   "Hello World \u00E9",
		},
		{
			name: "EmptyString",
			in:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeCP500(tt.in)
			if err != nil {
				t.Fatalf("EncodeCP500(%q) unexpected error: %v", tt.in, err)
			}
			if len(encoded) != len([]rune(tt.in)) {
				t.Errorf("len(encoded) = %d; want %d", len(encoded), len([]rune(tt.in)))
			}
			decoded := DecodeCP500(encoded)
			if decoded != tt.in {
				t.Errorf("DecodeCP500 = %q; want %q", decoded, tt.in)
			}
		})
	}
}

func TestEncodeCP500_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{
			name: "ASCII_Bounds",
			in:   "\x00\x01\x1f !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{}~\x7f",
		},
		{
			name: "Latin1_Accents",
			in:   "áéíóúÁÉÍÓÚàèìòùÀÈÌÒÙâêîôûÂÊÎÔÛäëïöüÄËÏÖÜçÇñÑß",
		},
		{
			name: "Mixed_ASCII_Latin1",
			in:   "IBM Db2 São Paulo / München - 100% pure Go!",
		},
		{
			name: "SQL_Patterns",
			in:   "SELECT col1, col2 FROM schema.table WHERE col3 = 'test' AND col4 <= 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := EncodeCP500(tt.in)
			if err != nil {
				t.Fatalf("EncodeCP500 failed: %v", err)
			}
			got := DecodeCP500(enc)
			if got != tt.in {
				t.Errorf("RoundTrip got %q; want %q", got, tt.in)
			}
		})
	}
}

func TestEncodeCP500_InvalidCharacters(t *testing.T) {
	unmappedInputs := []struct {
		name string
		in   string
	}{
		{name: "UnmappedVerticalBar", in: "|"},
		{name: "Greek", in: "Ελληνικά"},
		{name: "Cyrillic", in: "Русский"},
		{name: "Arabic", in: "العربية"},
		{name: "CJK_Japanese", in: "日本語"},
		{name: "CJK_Chinese", in: "中文"},
		{name: "Emoji", in: "⚡🚀Db2"},
	}

	for _, tt := range unmappedInputs {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncodeCP500(tt.in)
			if err == nil {
				t.Fatalf("EncodeCP500(%q) expected error, got nil", tt.in)
			}
			if !errors.Is(err, ErrInvalidEBCDIC) {
				t.Errorf("EncodeCP500(%q) err = %v; want %v", tt.in, err, ErrInvalidEBCDIC)
			}
		})
	}
}

func TestEncodeCP500_ASCIIDetection(t *testing.T) {
	fastStr := "Boundary\x7FTest"
	fastEnc, err := EncodeCP500(fastStr)
	if err != nil {
		t.Fatalf("EncodeCP500(%q) error: %v", fastStr, err)
	}

	slowStr := "Boundary\u00A0Test"
	slowEnc, err := EncodeCP500(slowStr)
	if err != nil {
		t.Fatalf("EncodeCP500(%q) error: %v", slowStr, err)
	}

	if DecodeCP500(fastEnc) != fastStr {
		t.Errorf("fast-path decode mismatch: got %q, want %q", DecodeCP500(fastEnc), fastStr)
	}
	if DecodeCP500(slowEnc) != slowStr {
		t.Errorf("fallback decode mismatch: got %q, want %q", DecodeCP500(slowEnc), slowStr)
	}

	nulDelStr := "\x00\x7F"
	nulDelEnc, err := EncodeCP500(nulDelStr)
	if err != nil {
		t.Fatalf("EncodeCP500 NUL/DEL error: %v", err)
	}
	if !bytes.Equal(nulDelEnc, []byte{unicodeToCP500Direct[0x00], unicodeToCP500Direct[0x7F]}) {
		t.Errorf("EncodeCP500 NUL/DEL = %v; want [%v %v]", nulDelEnc, unicodeToCP500Direct[0x00], unicodeToCP500Direct[0x7F])
	}
}

func TestAppendCP500(t *testing.T) {
	tests := []struct {
		name      string
		prefix    []byte
		in        string
		wantError bool
	}{
		{
			name:      "ASCII_WithPrefix",
			prefix:    []byte{0x01, 0x02, 0x03, 0x04},
			in:        "go-db2",
			wantError: false,
		},
		{
			name:      "ASCII_EmptyPrefix",
			prefix:    nil,
			in:        "TESTDB",
			wantError: false,
		},
		{
			name:      "NonASCII_WithPrefix",
			prefix:    []byte{0xAA, 0xBB},
			in:        "São Paulo",
			wantError: false,
		},
		{
			name:      "UnmappedCharacter_Error",
			prefix:    []byte{0x10},
			in:        "日本語",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefixCopy := append([]byte(nil), tt.prefix...)
			got, err := AppendCP500(prefixCopy, tt.in)
			if tt.wantError {
				if err == nil {
					t.Fatalf("AppendCP500(%q) expected error, got nil", tt.in)
				}
				if !errors.Is(err, ErrInvalidEBCDIC) {
					t.Errorf("AppendCP500(%q) err = %v; want %v", tt.in, err, ErrInvalidEBCDIC)
				}
				return
			}
			if err != nil {
				t.Fatalf("AppendCP500(%q) unexpected error: %v", tt.in, err)
			}

			// Verify prefix was preserved
			if !bytes.HasPrefix(got, tt.prefix) {
				t.Errorf("AppendCP500 output lost prefix: got %v, want prefix %v", got[:len(tt.prefix)], tt.prefix)
			}

			// Verify payload matches EncodeCP500
			payload := got[len(tt.prefix):]
			wantPayload, err := EncodeCP500(tt.in)
			if err != nil {
				t.Fatalf("EncodeCP500(%q) unexpected error: %v", tt.in, err)
			}
			if !bytes.Equal(payload, wantPayload) {
				t.Errorf("AppendCP500 payload = %v; want %v", payload, wantPayload)
			}
		})
	}
}
