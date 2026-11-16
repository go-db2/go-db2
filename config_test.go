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

func TestParseDSN_SecurityMechanismWhitelistAndUSRIDONL(t *testing.T) {
	const secretPass = "SuperSecret123!"

	t.Run("Rejects invalid, overflow, and non-whitelisted values in URL and KV forms", func(t *testing.T) {
		invalidCases := []struct {
			name string
			val  string
		}{
			{name: "uint16 overflow to 3 (65539)", val: "65539"},
			{name: "uint16 overflow to 4 (65540)", val: "65540"},
			{name: "zero numeric", val: "0"},
			{name: "legacy secmec 6 numeric", val: "6"},
			{name: "legacy secmec6 alias", val: "secmec6"},
			{name: "unsupported numeric 99", val: "99"},
			{name: "negative -1", val: "-1"},
			{name: "unknown alias", val: "unknown"},
		}

		for _, tc := range invalidCases {
			urlDSN := "db2://user:" + secretPass + "@localhost:50000/SAMPLE?security_mechanism=" + tc.val
			if _, err := ParseDSN(urlDSN); err == nil {
				t.Errorf("%s (URL): expected error, got nil", tc.name)
			}

			kvDSN := "HOST=localhost;PORT=50000;DATABASE=SAMPLE;UID=user;PWD=" + secretPass + ";SecurityMechanism=" + tc.val
			if _, err := ParseDSN(kvDSN); err == nil {
				t.Errorf("%s (KV): expected error, got nil", tc.name)
			}
		}
	})

	t.Run("Rejects USRIDONL (4) with non-empty password without leaking password", func(t *testing.T) {
		cases := []struct {
			name string
			dsn  string
		}{
			{
				name: "URL numeric 4 with password",
				dsn:  "db2://user:" + secretPass + "@localhost:50000/SAMPLE?security_mechanism=4",
			},
			{
				name: "URL user_only with password",
				dsn:  "db2://user:" + secretPass + "@localhost:50000/SAMPLE?security_mechanism=user_only",
			},
			{
				name: "KV numeric 4 with password",
				dsn:  "HOST=localhost;PORT=50000;DATABASE=SAMPLE;UID=user;PWD=" + secretPass + ";security_mechanism=4",
			},
			{
				name: "KV user_only with password",
				dsn:  "HOST=localhost;PORT=50000;DATABASE=SAMPLE;UID=user;PWD=" + secretPass + ";security_mechanism=user_only",
			},
		}

		for _, tc := range cases {
			_, err := ParseDSN(tc.dsn)
			if err == nil {
				t.Errorf("%s: expected error, got nil", tc.name)
				continue
			}
			if !strings.Contains(err.Error(), "USRIDONL (4) does not allow a password") {
				t.Errorf("%s: unexpected error message: %v", tc.name, err)
			}
			if strings.Contains(err.Error(), secretPass) {
				t.Errorf("%s: error leaked password: %v", tc.name, err)
			}
		}
	})

	t.Run("Allows USRIDONL (4) without password and supported mechanisms", func(t *testing.T) {
		validNoPass := []string{
			"db2://user@localhost:50000/SAMPLE?security_mechanism=4",
			"db2://user@localhost:50000/SAMPLE?security_mechanism=user_only",
			"HOST=localhost;PORT=50000;DATABASE=SAMPLE;UID=user;security_mechanism=4",
			"HOST=localhost;PORT=50000;DATABASE=SAMPLE;UID=user;security_mechanism=user_only",
		}
		for _, dsn := range validNoPass {
			cfg, err := ParseDSN(dsn)
			if err != nil {
				t.Errorf("ParseDSN(%q) unexpected error: %v", dsn, err)
				continue
			}
			if cfg.SecurityMechanism != 4 {
				t.Errorf("ParseDSN(%q) SecurityMechanism = %d, want 4", dsn, cfg.SecurityMechanism)
			}
		}

		// Supported numeric mechanisms with password
		for _, code := range []string{"3", "7", "9"} {
			dsn := "db2://user:" + secretPass + "@localhost:50000/SAMPLE?allow_plaintext_auth=true&security_mechanism=" + code
			if _, err := ParseDSN(dsn); err != nil {
				t.Errorf("ParseDSN(secmec=%s) unexpected error: %v", code, err)
			}
		}
		// Supported Kerberos (11) without password
		if _, err := ParseDSN("db2://@localhost:50000/SAMPLE?security_mechanism=11"); err != nil {
			t.Errorf("ParseDSN(secmec=11) unexpected error: %v", err)
		}
	})
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

func TestRedactAndParseDSN_EdgeCases(t *testing.T) {
	t.Run("redactDSNPassword masks special characters and preserves SPN", func(t *testing.T) {
		cases := []struct {
			name     string
			input    string
			expected string
			leaks    []string
		}{
			{
				name:     "Password with @ and hash",
				input:    "db2://admin:p@ss#word@localhost:50000/db",
				expected: "db2://admin:******@localhost:50000/db",
				leaks:    []string{"p@ss", "word", "p@ss#word"},
			},
			{
				name:     "Password with @ and slash",
				input:    "db2://admin:p@ss/123@localhost:50000/db?allow_plaintext_auth=true",
				expected: "db2://admin:******@localhost:50000/db?allow_plaintext_auth=true",
				leaks:    []string{"p@ss", "123", "p@ss/123"},
			},
			{
				name:     "Password with question mark and no port/db",
				input:    "db2://admin:sec?ret@localhost",
				expected: "db2://admin:******@localhost",
				leaks:    []string{"sec", "ret", "sec?ret"},
			},
			{
				name:     "Password with slash and no port/db",
				input:    "db2://admin:sec/ret@localhost",
				expected: "db2://admin:******@localhost",
				leaks:    []string{"sec", "ret", "sec/ret"},
			},
			{
				name:     "Password with hash and space in bad host",
				input:    "db2://admin:sec#ret@bad host:50000/db",
				expected: "db2://admin:******@bad host:50000/db",
				leaks:    []string{"sec", "ret", "sec#ret"},
			},
			{
				name:     "Password with ?, /, # and ! in bad host",
				input:    "db2://admin:p?a/s#w@bad!host:50000/db",
				expected: "db2://admin:******@bad!host:50000/db",
				leaks:    []string{"p?a/s#w", "a/s#w"},
			},
			{
				name:     "Query password param with spn containing @",
				input:    "db2://user@localhost:50000/TESTDB?password=secret&spn=db2/a@B",
				expected: "db2://user@localhost:50000/TESTDB?password=******&spn=db2/a@B",
				leaks:    []string{"secret"},
			},
			{
				name:     "Passwordless URL with spn containing @ is preserved",
				input:    "db2://localhost:50000/TESTDB?spn=db2/server.corp.local@CORP.LOCAL",
				expected: "db2://localhost:50000/TESTDB?spn=db2/server.corp.local@CORP.LOCAL",
			},
			{
				name:     "Password with @ plus query spn containing @",
				input:    "db2://user:SecretP@ss123@localhost:50000/TESTDB?spn=db2/server.corp.local@CORP.LOCAL",
				expected: "db2://user:******@localhost:50000/TESTDB?spn=db2/server.corp.local@CORP.LOCAL",
				leaks:    []string{"SecretP@ss123", "ss123"},
			},
			{
				name:     "Quoted error string from url.Parse",
				input:    `parse "db2://admin:p@ss#word@bad host:50000/db": invalid character " " in host name`,
				expected: `parse "db2://admin:******@bad host:50000/db": invalid character " " in host name`,
				leaks:    []string{"p@ss#word", "ss#word"},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := redactDSNPassword(tc.input)
				if got != tc.expected {
					t.Fatalf("redactDSNPassword(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.expected)
				}
				for _, leak := range tc.leaks {
					if strings.Contains(got, leak) {
						t.Fatalf("redactDSNPassword leaked secret fragment %q in %q", leak, got)
					}
				}
			})
		}
	})

	t.Run("ParseDSN parses valid edge URLs and never leaks passwords on failure", func(t *testing.T) {
		// Valid URL with @ and / in password
		cfg, err := ParseDSN("db2://admin:p@ss/123@localhost:50000/db?allow_plaintext_auth=true")
		if err != nil {
			t.Fatalf("unexpected error parsing p@ss/123: %v", err)
		}
		if cfg.Host != "localhost" || cfg.Port != 50000 || cfg.Database != "db" || cfg.Password != "p@ss/123" || cfg.User != "admin" {
			t.Fatalf("unexpected parsed config: %+v, Password=%q", cfg, cfg.Password)
		}

		// Valid URL with @ and # in password
		cfg2, err := ParseDSN("db2://admin:p@ss#word@localhost:50000/db?allow_plaintext_auth=true")
		if err != nil {
			t.Fatalf("unexpected error parsing p@ss#word: %v", err)
		}
		if cfg2.Host != "localhost" || cfg2.Port != 50000 || cfg2.Database != "db" || cfg2.Password != "p@ss#word" {
			t.Fatalf("unexpected parsed config2: %+v", cfg2)
		}

		// Valid explicitly empty password
		cfgEmpty, err := ParseDSN("db2://admin:@localhost:50000/testdb")
		if err != nil {
			t.Fatalf("unexpected error parsing empty password DSN: %v", err)
		}
		if cfgEmpty.User != "admin" || cfgEmpty.Password != "" {
			t.Fatalf("expected user=admin password=\"\", got user=%q password=%q", cfgEmpty.User, cfgEmpty.Password)
		}

		// Invalid URLs that fail at authority/host/port/percent-encoding MUST contain "******" and never leak secrets
		invalidEchoCases := []struct {
			dsn   string
			leaks []string
		}{
			{
				dsn:   "db2://admin:sec#ret@bad host:50000/db",
				leaks: []string{"sec", "ret", "sec#ret"},
			},
			{
				dsn:   "db2://admin:p?a/s#w@bad!host:50000/db",
				leaks: []string{"p?a/s#w", "a/s#w"},
			},
			{
				dsn:   "db2://admin:sec?ret@localhost:badport/db",
				leaks: []string{"sec", "ret", "sec?ret"},
			},
			{
				dsn:   "db2://admin:p@ss:word@localhost:badport/testdb",
				leaks: []string{"p@ss", "word", "p@ss:word"},
			},
			{
				dsn:   "db2://admin:my%zzpass@localhost:50000/testdb",
				leaks: []string{"my%zzpass", "my", "pass"},
			},
		}

		for _, tc := range invalidEchoCases {
			_, err := ParseDSN(tc.dsn)
			if err == nil {
				t.Fatalf("expected ParseDSN(%q) to fail, got nil", tc.dsn)
			}
			if !errors.Is(err, ErrInvalidConnectionStr) {
				t.Fatalf("expected ErrInvalidConnectionStr for %q, got %v", tc.dsn, err)
			}
			errMsg := err.Error()
			if !strings.Contains(errMsg, "******") {
				t.Fatalf("expected redacted placeholder ****** in error for %q, got: %s", tc.dsn, errMsg)
			}
			for _, leak := range tc.leaks {
				if strings.Contains(errMsg, leak) {
					t.Fatalf("ParseDSN(%q) error leaked fragment %q: %s", tc.dsn, leak, errMsg)
				}
			}
		}

		// Invalid URL without database name must fail cleanly without leaking password fragments as port/host
		for _, noDB := range []struct {
			dsn   string
			leaks []string
		}{
			{dsn: "db2://admin:sec?ret@localhost", leaks: []string{"sec", "ret", "sec?ret"}},
			{dsn: "db2://admin:sec/ret@localhost", leaks: []string{"sec", "ret", "sec/ret"}},
		} {
			_, err := ParseDSN(noDB.dsn)
			if err == nil {
				t.Fatalf("expected ParseDSN(%q) to fail, got nil", noDB.dsn)
			}
			if !errors.Is(err, ErrInvalidConnectionStr) {
				t.Fatalf("expected ErrInvalidConnectionStr for %q, got %v", noDB.dsn, err)
			}
			errMsg := err.Error()
			for _, leak := range noDB.leaks {
				if strings.Contains(errMsg, leak) {
					t.Fatalf("ParseDSN(%q) error leaked fragment %q: %s", noDB.dsn, leak, errMsg)
				}
			}
		}
	})
}

func TestParseDSN_HostValidation(t *testing.T) {
	if !isValidDSNHost("fe80::1%eth0") {
		t.Fatal("expected valid IPv6 zone ID to pass isValidDSNHost")
	}
	if !isValidDSNHost("2001:db8::1") {
		t.Fatal("expected IPv6 literal to pass isValidDSNHost")
	}
	for _, invalid := range []string{"1.2.3.4%eth0", "fe80::1%bad!host", "bad!host", "bad host", "notip%eth0"} {
		if isValidDSNHost(invalid) {
			t.Fatalf("expected isValidDSNHost(%q) to return false", invalid)
		}
	}

	// End-to-end URL parse with valid IPv6 + zone ID
	cfg, err := ParseDSN("db2://user:pass@[fe80::1%25eth0]:50000/TESTDB?allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("expected IPv6 zone ID URL to parse cleanly, got: %v", err)
	}
	if cfg.Host != "fe80::1%eth0" {
		t.Fatalf("expected Host=%q, got %q", "fe80::1%eth0", cfg.Host)
	}

	// End-to-end rejection of invalid host with zone-like suffix
	if _, err := ParseDSN("db2://user:pass@1.2.3.4%25eth0:50000/TESTDB?allow_plaintext_auth=true"); err == nil {
		t.Fatal("expected ParseDSN to reject IPv4 with zone ID")
	}
}

func TestParseDSN_DocAlignment_SEC07(t *testing.T) {
	// Parameters erroneously documented previously MUST fail
	if _, err := ParseDSN("db2://u:p@localhost:50000/DB?ssl_insecure=true"); err == nil || !strings.Contains(err.Error(), `unknown DSN parameter: "ssl_insecure"`) {
		t.Fatalf("expected unknown DSN parameter error for ssl_insecure, got: %v", err)
	}
	if _, err := ParseDSN("db2://u:p@localhost:50000/DB?security=9"); err == nil || !strings.Contains(err.Error(), `unknown DSN parameter: "security"`) {
		t.Fatalf("expected unknown DSN parameter error for security, got: %v", err)
	}

	// Defaults & documented parameters match actual runtime behavior
	cfgDefault, err := ParseDSN("db2://u@localhost/DB")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfgDefault.UseSSL {
		t.Fatal("expected db2:// default UseSSL=false")
	}
	if cfgDefault.Port != 50000 {
		t.Fatalf("expected db2:// default Port=50000, got %d", cfgDefault.Port)
	}
	if cfgDefault.Timeout != 30*time.Second {
		t.Fatalf("expected default Timeout=30s, got %v", cfgDefault.Timeout)
	}
	if cfgDefault.BlockSize != 65535 {
		t.Fatalf("expected default BlockSize=65535, got %d", cfgDefault.BlockSize)
	}

	cfgSSL, err := ParseDSN("db2s://u:p@localhost/DB?security_mechanism=9&client_userid=appusr&client_corr_token=corr-1")
	if err != nil {
		t.Fatalf("ParseDSN(db2s://...) failed: %v", err)
	}
	if !cfgSSL.UseSSL || cfgSSL.Port != 50001 || cfgSSL.SecurityMechanism != 9 || cfgSSL.ClientUserid != "appusr" || cfgSSL.ClientCorrToken != "corr-1" {
		t.Fatalf("unexpected db2s config: %+v", cfgSSL)
	}
}

func TestParseDSN_ConnTimeout(t *testing.T) {
	t.Run("ValidURLAndKeyValue", func(t *testing.T) {
		cases := []struct {
			name        string
			dsn         string
			wantTimeout time.Duration
		}{
			{
				name:        "URL duration",
				dsn:         "db2://u@localhost/DB?conn_timeout=10s",
				wantTimeout: 10 * time.Second,
			},
			{
				name:        "URL integer seconds",
				dsn:         "db2://u@localhost/DB?conn_timeout=15",
				wantTimeout: 15 * time.Second,
			},
			{
				name:        "Key-Value duration",
				dsn:         "host=localhost;database=DB;user=u;conn_timeout=12s",
				wantTimeout: 12 * time.Second,
			},
			{
				name:        "Key-Value integer seconds",
				dsn:         "host=localhost;database=DB;user=u;conn_timeout=20",
				wantTimeout: 20 * time.Second,
			},
			{
				name:        "Key-Value last parameter wins (timeout after conn_timeout)",
				dsn:         "host=localhost;database=DB;user=u;conn_timeout=10s;timeout=25s",
				wantTimeout: 25 * time.Second,
			},
			{
				name:        "Key-Value last parameter wins (conn_timeout after timeout)",
				dsn:         "host=localhost;database=DB;user=u;timeout=25s;conn_timeout=10s",
				wantTimeout: 10 * time.Second,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg, err := ParseDSN(tc.dsn)
				if err != nil {
					t.Fatalf("ParseDSN(%q) unexpected error: %v", tc.dsn, err)
				}
				if cfg.Timeout != tc.wantTimeout {
					t.Errorf("Timeout = %v, want %v", cfg.Timeout, tc.wantTimeout)
				}
			})
		}
	})

	t.Run("InvalidValuesAndPasswordRedaction", func(t *testing.T) {
		const secretPass = "SecretPass123!"
		cases := []struct {
			name string
			dsn  string
		}{
			{
				name: "URL negative duration",
				dsn:  "db2://u:" + secretPass + "@localhost/DB?ssl=true&conn_timeout=-1s",
			},
			{
				name: "URL non-numeric duration",
				dsn:  "db2://u:" + secretPass + "@localhost/DB?ssl=true&conn_timeout=abc",
			},
			{
				name: "KV negative duration",
				dsn:  "host=localhost;database=DB;user=u;password=" + secretPass + ";ssl=true;conn_timeout=-1s",
			},
			{
				name: "KV non-numeric duration",
				dsn:  "host=localhost;database=DB;user=u;password=" + secretPass + ";ssl=true;conn_timeout=abc",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := ParseDSN(tc.dsn)
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tc.name)
				}
				if !strings.Contains(err.Error(), "invalid timeout value") {
					t.Errorf("expected invalid timeout value error, got: %v", err)
				}
				if strings.Contains(err.Error(), secretPass) {
					t.Errorf("error leaked password: %v", err)
				}
			})
		}
	})
}

func TestParseDSN_NoPasswordLeakWhenNotStructural(t *testing.T) {
	const secretPass = "SecretPass123"
	cases := []struct {
		name string
		dsn  string
	}{
		{
			name: "missing host after @",
			dsn:  "db2://user:" + secretPass + "@",
		},
		{
			name: "missing host with query after @",
			dsn:  "db2://user:" + secretPass + "@?ssl=true",
		},
		{
			name: "equals sign and invalid port in authority",
			dsn:  "db2://user:" + secretPass + "@a=b:badport",
		},
		{
			name: "double @ and invalid port",
			dsn:  "db2://user:" + secretPass + "@a@b:badport",
		},
		{
			name: "equals sign in host with valid port",
			dsn:  "db2://user:" + secretPass + "@a=b:50000/DB",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDSN(tc.dsn)
			if err == nil {
				t.Fatalf("expected ParseDSN(%q) to fail, got nil", tc.dsn)
			}
			if !errors.Is(err, ErrInvalidConnectionStr) {
				t.Fatalf("expected ErrInvalidConnectionStr, got: %v", err)
			}
			if strings.Contains(err.Error(), secretPass) {
				t.Fatalf("ParseDSN(%q) leaked password in error: %s", tc.dsn, err.Error())
			}
		})
	}
}
