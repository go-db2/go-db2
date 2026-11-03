package db2

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseDSN_URL(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected *Config
	}{
		{
			name: "Basic URL",
			dsn:  "db2://db2inst1:password@localhost:50000/SAMPLE?allow_plaintext_auth=true",
			expected: &Config{
				Host:               "localhost",
				Port:               50000,
				Database:           "SAMPLE",
				User:               "db2inst1",
				Password:           "password",
				UseSSL:             false,
				AllowPlaintextAuth: true,
				Timeout:            30 * time.Second,
			},
		},
		{
			name: "SSL URL with params",
			dsn:  "db2s://myuser:secret@db2server:50001/PRODDB?timeout=15s&block_size=131072",
			expected: &Config{
				Host:      "db2server",
				Port:      50001,
				Database:  "PRODDB",
				User:      "myuser",
				Password:  "secret",
				UseSSL:    true,
				Timeout:   15 * time.Second,
				BlockSize: 131072,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseDSN(tt.dsn)
			if err != nil {
				t.Fatalf("ParseDSN(%q) error: %v", tt.dsn, err)
			}
			if cfg.Host != tt.expected.Host {
				t.Errorf("Host = %q, want %q", cfg.Host, tt.expected.Host)
			}
			if cfg.Port != tt.expected.Port {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.expected.Port)
			}
			if cfg.Database != tt.expected.Database {
				t.Errorf("Database = %q, want %q", cfg.Database, tt.expected.Database)
			}
			if cfg.User != tt.expected.User {
				t.Errorf("User = %q, want %q", cfg.User, tt.expected.User)
			}
			if cfg.Password != tt.expected.Password {
				t.Errorf("Password = %q, want %q", cfg.Password, tt.expected.Password)
			}
			if cfg.UseSSL != tt.expected.UseSSL {
				t.Errorf("UseSSL = %v, want %v", cfg.UseSSL, tt.expected.UseSSL)
			}
			if cfg.Timeout != tt.expected.Timeout {
				t.Errorf("Timeout = %v, want %v", cfg.Timeout, tt.expected.Timeout)
			}
			if tt.expected.BlockSize > 0 && cfg.BlockSize != tt.expected.BlockSize {
				t.Errorf("BlockSize = %d, want %d", cfg.BlockSize, tt.expected.BlockSize)
			}
		})
	}
}

func TestParseDSN_KeyValue(t *testing.T) {
	dsn := "host=mydb2.local;port=50000;database=TESTDB;user=alice;password=secret123;ssl=false;timeout=45s;allow_plaintext_auth=true;"
	cfg, err := ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q) error: %v", dsn, err)
	}

	if cfg.Host != "mydb2.local" {
		t.Errorf("Host = %q, want mydb2.local", cfg.Host)
	}
	if cfg.Port != 50000 {
		t.Errorf("Port = %d, want 50000", cfg.Port)
	}
	if cfg.Database != "TESTDB" {
		t.Errorf("Database = %q, want TESTDB", cfg.Database)
	}
	if cfg.User != "alice" {
		t.Errorf("User = %q, want alice", cfg.User)
	}
	if cfg.Password != "secret123" {
		t.Errorf("Password = %q, want secret123", cfg.Password)
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want 45s", cfg.Timeout)
	}
}

func TestParseDSN_Invalid(t *testing.T) {
	if _, err := ParseDSN(""); err == nil {
		t.Errorf("expected error for empty DSN, got nil")
	}
}

func TestParseDSN_RedactPassword_SpecialChars(t *testing.T) {
	secretPwd := "SecretP@ssw0rd!Special@123"

	// 1. URL DSN with @ in password and invalid port error
	urlDSN := fmt.Sprintf("db2://admin:%s@localhost:invalidport/sample", secretPwd)
	_, err := ParseDSN(urlDSN)
	if err == nil {
		t.Fatal("expected error for invalid port in URL DSN, got nil")
	}
	if !errors.Is(err, ErrInvalidConnectionStr) {
		t.Fatalf("expected errors.Is(err, ErrInvalidConnectionStr) to be true, got err=%v", err)
	}
	if strings.Contains(err.Error(), secretPwd) {
		t.Fatalf("ParseDSN leaked password in error message: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "******") {
		t.Fatalf("ParseDSN error message missing '******' redaction: %s", err.Error())
	}

	// 2. Key-Value DSN with password and malformed URL scheme error
	kvDSN := fmt.Sprintf("db2://admin:%s@:invalidport/sample", secretPwd)
	_, err = ParseDSN(kvDSN)
	if err == nil {
		t.Fatal("expected error for malformed host/port, got nil")
	}
	if !errors.Is(err, ErrInvalidConnectionStr) {
		t.Fatalf("expected errors.Is(err, ErrInvalidConnectionStr) to be true, got err=%v", err)
	}
	if strings.Contains(err.Error(), secretPwd) {
		t.Fatalf("ParseDSN KV error leaked password: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "******") {
		t.Fatalf("ParseDSN error message missing '******' redaction: %s", err.Error())
	}

	// 3. Verify redactDSNPassword function directly on strings with special characters
	redactedURL := redactDSNPassword(urlDSN)
	if strings.Contains(redactedURL, secretPwd) {
		t.Fatalf("redactDSNPassword failed to redact URL password containing '@': %s", redactedURL)
	}
	if !strings.Contains(redactedURL, "******") {
		t.Fatalf("redactDSNPassword did not insert '******' into URL string: %s", redactedURL)
	}

	redactedKV := redactDSNPassword(fmt.Sprintf("host=localhost;port=50000;database=db;user=admin;password=%s;", secretPwd))
	if strings.Contains(redactedKV, secretPwd) {
		t.Fatalf("redactDSNPassword failed to redact KV password: %s", redactedKV)
	}
	if !strings.Contains(redactedKV, "******") {
		t.Fatalf("redactDSNPassword did not insert '******' into KV string: %s", redactedKV)
	}
}

func TestParseDSN_KerberosSPN_RedactionPreservation(t *testing.T) {
	secretPwd := "SecretP@ss123"
	spn := "db2/server.corp.local@CORP.LOCAL"
	dsn := fmt.Sprintf("db2://user:%s@localhost:50000/TESTDB?spn=%s", secretPwd, spn)

	redacted := redactDSNPassword(dsn)
	if strings.Contains(redacted, secretPwd) {
		t.Fatalf("redactDSNPassword failed to redact password: %s", redacted)
	}
	if !strings.Contains(redacted, spn) {
		t.Fatalf("redactDSNPassword mutilated Kerberos SPN parameter in query string: %s", redacted)
	}
	expected := fmt.Sprintf("db2://user:******@localhost:50000/TESTDB?spn=%s", spn)
	if redacted != expected {
		t.Fatalf("redacted string mismatch: got %q, want %q", redacted, expected)
	}
}

func TestParseDSN_QueryParamsPasswordRedaction(t *testing.T) {
	secretPwd := "MySecretPassword123"

	queryDSNs := []string{
		fmt.Sprintf("db2://user@localhost:50000/TESTDB?password=%s&ssl=true", secretPwd),
		fmt.Sprintf("db2://localhost:50000/TESTDB?user=admin&pwd=%s&timeout=30s", secretPwd),
		fmt.Sprintf("db2s://localhost:50001/PRODDB?pwd=%s", secretPwd),
	}

	for i, dsn := range queryDSNs {
		t.Run(fmt.Sprintf("DSN_%d", i+1), func(t *testing.T) {
			redacted := redactDSNPassword(dsn)
			if strings.Contains(redacted, secretPwd) {
				t.Fatalf("redactDSNPassword failed to redact query param password: %s", redacted)
			}
			if !strings.Contains(redacted, "******") {
				t.Fatalf("redactDSNPassword did not contain '******' in redacted query string: %s", redacted)
			}
		})
	}
}

func TestParseDSN_Kerberos(t *testing.T) {
	dsn := "db2://localhost:50000/TESTDB?security_mechanism=kerberos&spn=db2/server.corp.local@CORP.LOCAL&krb5_config=/etc/krb5.conf&krb5_keytab=/etc/db2.keytab&krb5_ccache=/tmp/krb5cc_1000"
	cfg, err := ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q) error: %v", dsn, err)
	}

	if cfg.SecurityMechanism != 7 {
		t.Errorf("SecurityMechanism = %d, want 7", cfg.SecurityMechanism)
	}
	if cfg.KerberosSPN != "db2/server.corp.local@CORP.LOCAL" {
		t.Errorf("KerberosSPN = %q, want db2/server.corp.local@CORP.LOCAL", cfg.KerberosSPN)
	}
	if cfg.Krb5ConfigFile != "/etc/krb5.conf" {
		t.Errorf("Krb5ConfigFile = %q, want /etc/krb5.conf", cfg.Krb5ConfigFile)
	}
	if cfg.Krb5KeytabFile != "/etc/db2.keytab" {
		t.Errorf("Krb5KeytabFile = %q, want /etc/db2.keytab", cfg.Krb5KeytabFile)
	}
	if cfg.Krb5CCacheFile != "/tmp/krb5cc_1000" {
		t.Errorf("Krb5CCacheFile = %q, want /tmp/krb5cc_1000", cfg.Krb5CCacheFile)
	}

	sessCfg := cfg.toSessionConfig()
	if sessCfg.SecurityMechanism != 7 {
		t.Errorf("toSessionConfig().SecurityMechanism = %d, want 7", sessCfg.SecurityMechanism)
	}
	if sessCfg.KerberosSPN != "db2/server.corp.local@CORP.LOCAL" {
		t.Errorf("toSessionConfig().KerberosSPN = %q, want db2/server.corp.local@CORP.LOCAL", sessCfg.KerberosSPN)
	}
}

func TestParseDSN_ClientInfo(t *testing.T) {
	dsn := "db2://localhost:50000/TESTDB?client_applname=orders-service&client_wrkstnname=k8s-pod-99&client_userid=cust_123&client_acctng=dept_finance&client_corr_token=trace-xyz-789"
	cfg, err := ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q) error: %v", dsn, err)
	}

	if cfg.ClientApplName != "orders-service" {
		t.Errorf("ClientApplName = %q, want orders-service", cfg.ClientApplName)
	}
	if cfg.ClientWrkstnName != "k8s-pod-99" {
		t.Errorf("ClientWrkstnName = %q, want k8s-pod-99", cfg.ClientWrkstnName)
	}
	if cfg.ClientUserid != "cust_123" {
		t.Errorf("ClientUserid = %q, want cust_123", cfg.ClientUserid)
	}
	if cfg.ClientAcctng != "dept_finance" {
		t.Errorf("ClientAcctng = %q, want dept_finance", cfg.ClientAcctng)
	}
	if cfg.ClientCorrToken != "trace-xyz-789" {
		t.Errorf("ClientCorrToken = %q, want trace-xyz-789", cfg.ClientCorrToken)
	}

	sessCfg := cfg.toSessionConfig()
	if sessCfg.ClientApplName != "orders-service" {
		t.Errorf("toSessionConfig().ClientApplName = %q, want orders-service", sessCfg.ClientApplName)
	}
	if sessCfg.ClientWrkstnName != "k8s-pod-99" {
		t.Errorf("toSessionConfig().ClientWrkstnName = %q, want k8s-pod-99", sessCfg.ClientWrkstnName)
	}
}

func TestConfig_StringAndGoString(t *testing.T) {
	cfg := Config{
		Host:     "db2.example.com",
		Port:     50000,
		Database: "TESTDB",
		User:     "db2admin",
		Password: "super_secret_password",
		UseSSL:   true,
		Timeout:  10 * time.Second,
	}

	expected := `Config{Host:db2.example.com, Port:50000, Database:TESTDB, User:db2admin, Password:"******", UseSSL:true, Timeout:10s}`

	// Test String() method
	if str := cfg.String(); str != expected {
		t.Errorf("Config.String() = %q, want %q", str, expected)
	}

	// Test fmt.Sprintf("%v", cfg)
	if str := fmt.Sprintf("%v", cfg); str != expected {
		t.Errorf("fmt.Sprintf(%%v, cfg) = %q, want %q", str, expected)
	}

	// Test GoString() method
	if goStr := cfg.GoString(); goStr != expected {
		t.Errorf("Config.GoString() = %q, want %q", goStr, expected)
	}

	// Test fmt.Sprintf("%#v", cfg)
	if goStr := fmt.Sprintf("%#v", cfg); goStr != expected {
		t.Errorf("fmt.Sprintf(%%#v, cfg) = %q, want %q", goStr, expected)
	}

	// Ensure sensitive password is not leaked in output
	if strings.Contains(cfg.String(), "super_secret_password") {
		t.Errorf("Config.String() leaked sensitive password!")
	}
	if strings.Contains(cfg.GoString(), "super_secret_password") {
		t.Errorf("Config.GoString() leaked sensitive password!")
	}
}

func TestConfig_SanitizeLogInjection(t *testing.T) {
	cfg := Config{
		Host:     "db2.example.com\r\n[SECURITY] Fake log entry",
		Port:     50000,
		Database: "TESTDB\nINJECTED",
		User:     "db2admin\x00user",
		Password: "super_secret_password",
		UseSSL:   true,
		Timeout:  10 * time.Second,
	}

	expected := `Config{Host:db2.example.com[SECURITY] Fake log entry, Port:50000, Database:TESTDBINJECTED, User:db2adminuser, Password:"******", UseSSL:true, Timeout:10s}`

	if str := cfg.String(); str != expected {
		t.Errorf("Config.String() = %q, want %q", str, expected)
	}
	if goStr := cfg.GoString(); goStr != expected {
		t.Errorf("Config.GoString() = %q, want %q", goStr, expected)
	}

	if strings.Contains(cfg.String(), "\r") || strings.Contains(cfg.String(), "\n") || strings.Contains(cfg.String(), "\x00") {
		t.Errorf("Config.String() contains unsanitized control characters!")
	}
}

func BenchmarkSanitizeLogString_Clean(b *testing.B) {
	s := "db2server.corp.internal"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sanitizeLogString(s)
	}
}

func BenchmarkSanitizeLogString_WithControlChars(b *testing.B) {
	s := "db2server.corp.internal\r\n[SECURITY] Log injection"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sanitizeLogString(s)
	}
}

func TestParseDSN_StrictValidation(t *testing.T) {
	// Unknown parameter error
	_, err := ParseDSN("db2://user:pwd@localhost:50000/DB?sll=true")
	if err == nil || !strings.Contains(err.Error(), `unknown DSN parameter: "sll"`) {
		t.Fatalf("expected unknown parameter error for 'sll', got: %v", err)
	}

	// Invalid boolean value error
	_, err = ParseDSN("db2://user:pwd@localhost:50000/DB?ssl=ture")
	if err == nil || !strings.Contains(err.Error(), `invalid boolean value for ssl: "ture"`) {
		t.Fatalf("expected invalid boolean error for 'ssl=ture', got: %v", err)
	}

	// Malformed key-value parameter error
	_, err = ParseDSN("host=localhost;invalid_token;port=50000")
	if err == nil || !strings.Contains(err.Error(), `malformed DSN parameter: "invalid_token"`) {
		t.Fatalf("expected malformed DSN parameter error, got: %v", err)
	}

	// Invalid port
	_, err = ParseDSN("host=localhost;port=abc;database=SAMPLE")
	if err == nil || !strings.Contains(err.Error(), `invalid port`) {
		t.Fatalf("expected invalid port error, got: %v", err)
	}

	// Invalid timeout
	_, err = ParseDSN("host=localhost;timeout=badtimeout;database=SAMPLE")
	if err == nil || !strings.Contains(err.Error(), `invalid timeout value`) {
		t.Fatalf("expected invalid timeout error, got: %v", err)
	}

	// Valid aliases parse successfully
	aliasesDSN := "host=localhost;port=50000;database=SAMPLE;user=db2inst1;password=secret;" +
		"tls=true;ssl_ca=/path/to/ca.pem;sslcert=/path/to/cert.pem;sslkey=/path/to/key.pem;" +
		"blocksize=131072;auth=encrypted-password;client_applname=my_app"
	cfg, err := ParseDSN(aliasesDSN)
	if err != nil {
		t.Fatalf("failed to parse valid aliases: %v", err)
	}
	if !cfg.UseSSL {
		t.Errorf("expected UseSSL=true from tls=true")
	}
	if cfg.SSLRootCAPath != "/path/to/ca.pem" {
		t.Errorf("expected SSLRootCAPath to be set, got %q", cfg.SSLRootCAPath)
	}
	if cfg.BlockSize != 131072 {
		t.Errorf("expected BlockSize=131072, got %d", cfg.BlockSize)
	}
	if cfg.SecurityMechanism != 9 {
		t.Errorf("expected SecurityMechanism=9 from auth=encrypted-password, got %d", cfg.SecurityMechanism)
	}
	if cfg.ClientApplName != "my_app" {
		t.Errorf("expected ClientApplName=my_app, got %q", cfg.ClientApplName)
	}
}

func TestParseDSN_PortPreservation(t *testing.T) {
	// AC-6: Explicit port in host=host:50000 with ssl=true preserves 50000
	cfg1, err := ParseDSN("host=mydb.local:50000;ssl=true;database=SAMPLE;user=u;pwd=p")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg1.Port != 50000 {
		t.Errorf("expected Port=50000, got %d", cfg1.Port)
	}

	// Explicit port in port=50000 with ssl=true preserves 50000
	cfg2, err := ParseDSN("host=mydb.local;port=50000;ssl=true;database=SAMPLE;user=u;pwd=p")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg2.Port != 50000 {
		t.Errorf("expected Port=50000, got %d", cfg2.Port)
	}

	// URL with explicit port and ssl=true preserves 50000
	cfg3, err := ParseDSN("db2://u:p@mydb.local:50000/SAMPLE?ssl=true")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg3.Port != 50000 {
		t.Errorf("expected Port=50000, got %d", cfg3.Port)
	}

	// DSN without port specified and ssl=true defaults to 50001
	cfg4, err := ParseDSN("host=mydb.local;ssl=true;database=SAMPLE;user=u;pwd=p")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg4.Port != 50001 {
		t.Errorf("expected Port=50001, got %d", cfg4.Port)
	}

	// db2s URL without port defaults to 50001
	cfg5, err := ParseDSN("db2s://u:p@mydb.local/SAMPLE")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg5.Port != 50001 {
		t.Errorf("expected Port=50001, got %d", cfg5.Port)
	}

	// DSN without port specified and ssl=false defaults to 50000
	cfg6, err := ParseDSN("host=mydb.local;ssl=false;database=SAMPLE;user=u;pwd=p;allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg6.Port != 50000 {
		t.Errorf("expected Port=50000, got %d", cfg6.Port)
	}
}

func TestParseDSN_PlaintextAuthPolicy(t *testing.T) {
	// SECMEC 3 without TLS and without allow_plaintext_auth fails
	_, err := ParseDSN("host=localhost;port=50000;database=SAMPLE;user=u;password=p;ssl=false;security_mechanism=plain")
	if err == nil || !strings.Contains(err.Error(), "plaintext authentication over unencrypted connection is disabled by default") {
		t.Fatalf("expected plaintext auth error, got: %v", err)
	}

	// SECMEC 3 without TLS but with allow_plaintext_auth=true succeeds
	cfg, err := ParseDSN("host=localhost;port=50000;database=SAMPLE;user=u;password=p;ssl=false;security_mechanism=plain;allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("expected success with allow_plaintext_auth=true, got: %v", err)
	}
	if !cfg.AllowPlaintextAuth {
		t.Errorf("expected AllowPlaintextAuth=true")
	}
}

func TestParseDSN_SecurityMechanismZeroPlaintextPolicy(t *testing.T) {
	// DSN with password, without SSL, without allow_plaintext_auth, and without explicit security_mechanism (defaults to 0) must fail
	_, err := ParseDSN("db2://db2inst1:password@localhost:50000/SAMPLE")
	if err == nil || !strings.Contains(err.Error(), "plaintext authentication over unencrypted connection is disabled by default") {
		t.Fatalf("expected plaintext auth error for default security mechanism, got: %v", err)
	}

	// DSN without password is permitted even without SSL
	cfg, err := ParseDSN("db2://db2inst1@localhost:50000/SAMPLE")
	if err != nil {
		t.Fatalf("expected success for passwordless DSN, got: %v", err)
	}
	if cfg.User != "db2inst1" {
		t.Errorf("User = %q, want db2inst1", cfg.User)
	}

	// DSN with password and SSL enabled is permitted
	cfg, err = ParseDSN("db2s://db2inst1:password@localhost:50001/SAMPLE")
	if err != nil {
		t.Fatalf("expected success for SSL DSN, got: %v", err)
	}
	if !cfg.UseSSL {
		t.Errorf("expected UseSSL=true")
	}

	// DSN with password and dh_encrypted_password (9) is permitted without SSL
	cfg, err = ParseDSN("db2://db2inst1:password@localhost:50000/SAMPLE?security_mechanism=dh_encrypted_password")
	if err != nil {
		t.Fatalf("expected success for SECMEC 9 DSN without SSL, got: %v", err)
	}
	if cfg.SecurityMechanism != 9 {
		t.Errorf("expected SecurityMechanism=9, got: %d", cfg.SecurityMechanism)
	}
}

func BenchmarkParseDSN(b *testing.B) {
	dsn := "db2://myuser:secretpassword@db2host.corp.internal:50000/PRODUCTION?timeout=30s&block_size=65535&allow_plaintext_auth=true"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseDSN(dsn)
		if err != nil {
			b.Fatal(err)
		}
	}
}
