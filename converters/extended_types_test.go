package converters

import (
	"testing"
	"time"
)

func TestDecFloat16EncodeDecode(t *testing.T) {
	testCases := []string{
		"0",
		"123.456",
		"-987.654",
		"1000",
		"0.000123",
		"1.23E+2",
		"-1.23E+2",
		"Infinity",
		"-Infinity",
		"NaN",
	}

	for _, tc := range testCases {
		b, err := EncodeDFP(tc, 8)
		if err != nil {
			t.Fatalf("EncodeDFP(%s, 8) error: %v", tc, err)
		}
		if len(b) != 8 {
			t.Fatalf("EncodeDFP(%s, 8) expected 8 bytes, got %d", tc, len(b))
		}

		decoded, err := DecodeDFP(b)
		if err != nil {
			t.Fatalf("DecodeDFP error for %s: %v", tc, err)
		}

		t.Logf("DECFLOAT(16) %s -> %s", tc, decoded)
		if tc == "123.456" && decoded != "123.456" {
			t.Fatalf("Expected 123.456, got %s", decoded)
		}
		if tc == "Infinity" && decoded != "Infinity" {
			t.Fatalf("Expected Infinity, got %s", decoded)
		}
		if tc == "-Infinity" && decoded != "-Infinity" {
			t.Fatalf("Expected -Infinity, got %s", decoded)
		}
		if tc == "NaN" && decoded != "NaN" {
			t.Fatalf("Expected NaN, got %s", decoded)
		}
	}
}

func TestDecFloat34EncodeDecode(t *testing.T) {
	testCases := []string{
		"0",
		"12345678901234567890.12345678901234",
		"-98765432109876543210.98765432109876",
		"9.87654321E+20",
		"Infinity",
		"NaN",
	}

	for _, tc := range testCases {
		b, err := EncodeDFP(tc, 16)
		if err != nil {
			t.Fatalf("EncodeDFP(%s, 16) error: %v", tc, err)
		}
		if len(b) != 16 {
			t.Fatalf("EncodeDFP(%s, 16) expected 16 bytes, got %d", tc, len(b))
		}

		decoded, err := DecodeDFP(b)
		if err != nil {
			t.Fatalf("DecodeDFP error for %s: %v", tc, err)
		}

		t.Logf("DECFLOAT(34) %s -> %s", tc, decoded)
		if tc == "Infinity" && decoded != "Infinity" {
			t.Fatalf("Expected Infinity, got %s", decoded)
		}
		if tc == "NaN" && decoded != "NaN" {
			t.Fatalf("Expected NaN, got %s", decoded)
		}
	}
}

func TestTimestampWithTimeZoneParsing(t *testing.T) {
	tzTests := []struct {
		input string
		hour  int
		min   int
	}{
		{"2026-08-27-23.45.00.000000-03:00", 23, 45},
		{"2026-08-27-20.15.30.123456+02:00", 20, 15},
		{"2026-08-27 18:30:00.000000-05:00", 18, 30},
	}

	for _, tt := range tzTests {
		layouts := []string{
			"2006-01-02-15.04.05.000000-07:00",
			"2006-01-02-15.04.05.000000+07:00",
			"2006-01-02 15:04:05.000000-07:00",
		}
		var parsed time.Time
		var err error
		for _, l := range layouts {
			if parsed, err = time.Parse(l, tt.input); err == nil {
				break
			}
		}
		if err != nil {
			t.Fatalf("Failed to parse %s: %v", tt.input, err)
		}
		if parsed.Hour() != tt.hour || parsed.Minute() != tt.min {
			t.Fatalf("Parsed time mismatch for %s: got %v", tt.input, parsed)
		}
	}
}
