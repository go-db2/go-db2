package security

import (
	"bytes"
	"os"
	"testing"
)

func TestFormatServicePrincipal(t *testing.T) {
	tests := []struct {
		name     string
		spn      string
		host     string
		realm    string
		expected string
	}{
		{
			name:     "explicit SPN",
			spn:      "db2/custom.domain.com@EXAMPLE.COM",
			host:     "localhost",
			realm:    "OTHER",
			expected: "db2/custom.domain.com@EXAMPLE.COM",
		},
		{
			name:     "host only",
			spn:      "",
			host:     "db2server.internal",
			realm:    "",
			expected: "db2/db2server.internal",
		},
		{
			name:     "host and realm",
			spn:      "",
			host:     "db2server.internal",
			realm:    "corp.local",
			expected: "db2/db2server.internal@CORP.LOCAL",
		},
		{
			name:     "empty all defaults to localhost",
			spn:      "",
			host:     "",
			realm:    "",
			expected: "db2/localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatServicePrincipal(tt.spn, tt.host, tt.realm)
			if got != tt.expected {
				t.Fatalf("FormatServicePrincipal() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGSSAPIToken(t *testing.T) {
	apReq := []byte("SIMULATED_KERBEROS_AP_REQ_TICKET")
	token, err := BuildGSSAPIToken(apReq)
	if err != nil {
		t.Fatalf("BuildGSSAPIToken() error: %v", err)
	}

	if len(token) == 0 {
		t.Fatal("BuildGSSAPIToken() returned empty token")
	}

	// First byte must be Application 0 ASN.1 tag 0x60
	if token[0] != 0x60 {
		t.Fatalf("Expected token[0] == 0x60, got 0x%02X", token[0])
	}

	// Payload must contain the AP-REQ bytes
	if !bytes.Contains(token, apReq) {
		t.Fatal("GSSAPI token does not contain original AP-REQ bytes")
	}
}

func TestAcquireKerberosToken_DirectRawToken(t *testing.T) {
	raw := []byte("PRE_GENERATED_GSSAPI_TOKEN")
	cfg := KerberosConfig{
		RawToken: raw,
	}

	token, err := AcquireKerberosToken(cfg)
	if err != nil {
		t.Fatalf("AcquireKerberosToken() error: %v", err)
	}
	if !bytes.Equal(token, raw) {
		t.Fatalf("Expected %v, got %v", raw, token)
	}
}

func TestAcquireKerberosToken_MissingCredentialsFails(t *testing.T) {
	cfg := KerberosConfig{
		Username: "db2user@EXAMPLE.COM",
		Host:     "db2host",
	}

	_, err := AcquireKerberosToken(cfg)
	if err == nil {
		t.Fatal("Expected error when no authentic Kerberos credentials (keytab/ccache) are found, got nil")
	}
}

func TestParseKeytab_And_BuildKeytab(t *testing.T) {
	entries := []KeytabEntry{
		{
			Principal: "db2/db2server.corp.local@CORP.LOCAL",
			KeyType:   18, // AES256-CTS-HMAC-SHA1-96
			KVNO:      1,
			Key:       []byte("0123456789abcdef0123456789abcdef"), // 32 bytes AES256 key
		},
		{
			Principal: "db2admin@CORP.LOCAL",
			KeyType:   23, // RC4-HMAC
			KVNO:      2,
			Key:       []byte("secret_rc4_key_16b!"),
		},
	}

	keytabBytes, err := BuildKeytab(entries...)
	if err != nil {
		t.Fatalf("BuildKeytab() error: %v", err)
	}

	if len(keytabBytes) < 2 || keytabBytes[0] != 0x05 || keytabBytes[1] != 0x02 {
		t.Fatalf("BuildKeytab() generated invalid header: %02X %02X", keytabBytes[0], keytabBytes[1])
	}

	parsed, err := ParseKeytab(keytabBytes)
	if err != nil {
		t.Fatalf("ParseKeytab() error: %v", err)
	}

	if len(parsed) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(parsed))
	}

	if parsed[0].Principal != entries[0].Principal {
		t.Errorf("Entry[0] Principal = %q, want %q", parsed[0].Principal, entries[0].Principal)
	}
	if parsed[0].KeyType != entries[0].KeyType {
		t.Errorf("Entry[0] KeyType = %d, want %d", parsed[0].KeyType, entries[0].KeyType)
	}
	if parsed[0].KVNO != entries[0].KVNO {
		t.Errorf("Entry[0] KVNO = %d, want %d", parsed[0].KVNO, entries[0].KVNO)
	}
	if !bytes.Equal(parsed[0].Key, entries[0].Key) {
		t.Errorf("Entry[0] Key mismatch")
	}

	if parsed[1].Principal != entries[1].Principal {
		t.Errorf("Entry[1] Principal = %q, want %q", parsed[1].Principal, entries[1].Principal)
	}
	if parsed[1].KeyType != entries[1].KeyType {
		t.Errorf("Entry[1] KeyType = %d, want %d", parsed[1].KeyType, entries[1].KeyType)
	}
}

func TestAcquireKerberosToken_Keytab(t *testing.T) {
	entries := []KeytabEntry{
		{
			Principal: "db2/db2server.corp.local@CORP.LOCAL",
			KeyType:   18,
			KVNO:      3,
			Key:       []byte("aes256_service_key_32_bytes_len!"),
		},
	}

	keytabBytes, err := BuildKeytab(entries...)
	if err != nil {
		t.Fatalf("BuildKeytab error: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "test_keytab_*.keytab")
	if err != nil {
		t.Fatalf("Failed to create temp keytab: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(keytabBytes); err != nil {
		t.Fatalf("Failed to write keytab: %v", err)
	}
	_ = tmpFile.Close()

	cfg := KerberosConfig{
		KeytabFile:       tmpFile.Name(),
		ServicePrincipal: "db2/db2server.corp.local@CORP.LOCAL",
		Host:             "db2server.corp.local",
	}

	token, err := AcquireKerberosToken(cfg)
	if err != nil {
		t.Fatalf("AcquireKerberosToken() error: %v", err)
	}

	if token[0] != 0x60 {
		t.Fatalf("Expected GSSAPI tag 0x60, got 0x%02X", token[0])
	}

	if !bytes.Contains(token, []byte("KRB5_KEYTAB_TOKEN:db2/db2server.corp.local@CORP.LOCAL:KVNO=3:TYPE=18")) {
		t.Fatal("Token does not contain expected Keytab metadata")
	}
}

func TestParseKeytab_HighBitNumComponents(t *testing.T) {
	// Construct keytab entry body with numComponents = 1 (0x0001)
	var entryBody []byte
	entryBody = append(entryBody, 0x00, 0x01) // numComponents = 1
	entryBody = append(entryBody, 0x00, 0x04) // realmLen = 4
	entryBody = append(entryBody, []byte("TEST")...)
	// Add 1 component ("comp1")
	entryBody = append(entryBody, 0x00, 0x05)
	entryBody = append(entryBody, []byte("comp1")...)
	// Trailing metadata padding (name_type 4, timestamp 4, kvno8 1, keytype 2, keylen 2, key 16, kvno32 4)
	entryBody = append(entryBody, make([]byte, 33)...)

	// Wrap in keytab v2 format
	var data []byte
	data = append(data, 0x05, 0x02) // Keytab v2 header
	eLen := uint32(len(entryBody))
	data = append(data, byte(eLen>>24), byte(eLen>>16), byte(eLen>>8), byte(eLen&0xFF))
	data = append(data, entryBody...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseKeytab panicked: %v", r)
		}
	}()

	entries, err := ParseKeytab(data)
	if err != nil {
		t.Fatalf("unexpected error parsing keytab: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Principal != "comp1@TEST" {
		t.Fatalf("expected principal 'comp1@TEST', got %q", entries[0].Principal)
	}
}

func TestParseKeytab_MalformedNegativeLength(t *testing.T) {
	// Construct keytab header (v2) followed by a 4-byte int32 MinInt32 (-2147483648 / 0x80000000)
	minInt32Data := []byte{0x05, 0x02, 0x80, 0x00, 0x00, 0x00}
	_, err := ParseKeytab(minInt32Data)
	if err == nil {
		t.Fatal("expected error when parsing keytab entry with MinInt32 length, got nil")
	}

	// Construct keytab header (v2) followed by a negative entry length that exceeds data bounds (-10)
	outOfBoundsData := []byte{0x05, 0x02, 0xFF, 0xFF, 0xFF, 0xF6} // 0xFFFFFFF6 = -10
	_, err = ParseKeytab(outOfBoundsData)
	if err == nil {
		t.Fatal("expected error when parsing keytab entry with out-of-bounds negative length, got nil")
	}
}

func TestBuildGSSAPIToken_LargePayload(t *testing.T) {
	// Create payload larger than 65,535 bytes (e.g. 70,000 bytes)
	largeAPReq := make([]byte, 70000)
	for i := range largeAPReq {
		largeAPReq[i] = byte(i % 256)
	}

	token, err := BuildGSSAPIToken(largeAPReq)
	if err != nil {
		t.Fatalf("BuildGSSAPIToken failed for large payload: %v", err)
	}

	if token[0] != 0x60 {
		t.Fatalf("expected GSSAPI tag 0x60, got 0x%02X", token[0])
	}

	// For length 70015 (payload = 13 bytes OID/ID + 70000 bytes apReq = 70013),
	// ASN.1 DER length encoding requires 0x83 (3-byte length: 0x01, 0x11, 0x5D)
	if token[1] != 0x83 {
		t.Fatalf("expected ASN.1 3-byte length tag 0x83, got 0x%02X", token[1])
	}

	decodedLen := int(token[2])<<16 | int(token[3])<<8 | int(token[4])
	if decodedLen != len(token)-5 {
		t.Fatalf("expected decoded length %d, got %d", len(token)-5, decodedLen)
	}
}

func TestBuildGSSAPIToken_LengthBoundaryTransitions(t *testing.T) {
	tests := []struct {
		name            string
		apReqSize       int
		expectedTag     byte
		headerHeaderLen int
	}{
		{
			name:            "Boundary 65,535 bytes (0x82 tag)",
			apReqSize:       65535 - 13, // total payload = 65,535
			expectedTag:     0x82,
			headerHeaderLen: 4, // 0x60 + 0x82 + 2 bytes len
		},
		{
			name:            "Boundary 65,536 bytes (0x83 tag transition)",
			apReqSize:       65536 - 13, // total payload = 65,536
			expectedTag:     0x83,
			headerHeaderLen: 5, // 0x60 + 0x83 + 3 bytes len
		},
		{
			name:            "Boundary 16,777,215 bytes (0x83 tag)",
			apReqSize:       16777215 - 13, // total payload = 16,777,215
			expectedTag:     0x83,
			headerHeaderLen: 5, // 0x60 + 0x83 + 3 bytes len
		},
		{
			name:            "Boundary 16,777,216 bytes (0x84 tag transition)",
			apReqSize:       16777216 - 13, // total payload = 16,777,216
			expectedTag:     0x84,
			headerHeaderLen: 6, // 0x60 + 0x84 + 4 bytes len
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apReq := make([]byte, tt.apReqSize)
			token, err := BuildGSSAPIToken(apReq)
			if err != nil {
				t.Fatalf("BuildGSSAPIToken() error: %v", err)
			}

			if token[0] != 0x60 {
				t.Fatalf("expected ASN.1 tag 0x60, got 0x%02X", token[0])
			}

			if token[1] != tt.expectedTag {
				t.Fatalf("expected length tag 0x%02X, got 0x%02X", tt.expectedTag, token[1])
			}

			expectedTotalLen := tt.headerHeaderLen + tt.apReqSize + 13
			if len(token) != expectedTotalLen {
				t.Fatalf("expected token total length %d, got %d", expectedTotalLen, len(token))
			}
		})
	}
}

func TestParseKeytab_MalformedNumComponents(t *testing.T) {
	// Construct keytab entry claiming numComponents = 10000 (0x2710) in a 20-byte payload
	var entryBody []byte
	entryBody = append(entryBody, 0x27, 0x10) // numComponents = 10000
	entryBody = append(entryBody, 0x00, 0x04) // realmLen = 4
	entryBody = append(entryBody, []byte("TEST")...)
	entryBody = append(entryBody, make([]byte, 10)...)

	var data []byte
	data = append(data, 0x05, 0x02) // Keytab v2 header
	eLen := uint32(len(entryBody))
	data = append(data, byte(eLen>>24), byte(eLen>>16), byte(eLen>>8), byte(eLen&0xFF))
	data = append(data, entryBody...)

	_, err := ParseKeytab(data)
	if err == nil {
		t.Fatal("expected error for keytab entry with malformed numComponents exceeding remaining bytes, got nil")
	}
}

func TestParseKeytab_TruncatedComponent(t *testing.T) {
	// Construct keytab entry with numComponents = 2, but provide only 1 component and truncate entryData
	var entryBody []byte
	entryBody = append(entryBody, 0x00, 0x02) // numComponents = 2
	entryBody = append(entryBody, 0x00, 0x04) // realmLen = 4
	entryBody = append(entryBody, []byte("TEST")...)
	// Component 1 ("comp1")
	entryBody = append(entryBody, 0x00, 0x05)
	entryBody = append(entryBody, []byte("comp1")...)
	// Omit Component 2 - truncated!

	var data []byte
	data = append(data, 0x05, 0x02) // Keytab v2 header
	eLen := uint32(len(entryBody))
	data = append(data, byte(eLen>>24), byte(eLen>>16), byte(eLen>>8), byte(eLen&0xFF))
	data = append(data, entryBody...)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseKeytab panicked on truncated component: %v", r)
		}
	}()

	entries, err := ParseKeytab(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for truncated component keytab, got %d", len(entries))
	}
}
