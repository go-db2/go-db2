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
