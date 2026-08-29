package db2

import (
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
			dsn:  "db2://db2inst1:password@localhost:50000/SAMPLE",
			expected: &Config{
				Host:     "localhost",
				Port:     50000,
				Database: "SAMPLE",
				User:     "db2inst1",
				Password: "password",
				UseSSL:   false,
				Timeout:  30 * time.Second,
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
	dsn := "host=mydb2.local;port=50000;database=TESTDB;user=alice;password=secret123;ssl=false;timeout=45s;"
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

	sessCfg := cfg.ToSessionConfig()
	if sessCfg.SecurityMechanism != 7 {
		t.Errorf("ToSessionConfig().SecurityMechanism = %d, want 7", sessCfg.SecurityMechanism)
	}
	if sessCfg.KerberosSPN != "db2/server.corp.local@CORP.LOCAL" {
		t.Errorf("ToSessionConfig().KerberosSPN = %q, want db2/server.corp.local@CORP.LOCAL", sessCfg.KerberosSPN)
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

	sessCfg := cfg.ToSessionConfig()
	if sessCfg.ClientApplName != "orders-service" {
		t.Errorf("ToSessionConfig().ClientApplName = %q, want orders-service", sessCfg.ClientApplName)
	}
	if sessCfg.ClientWrkstnName != "k8s-pod-99" {
		t.Errorf("ToSessionConfig().ClientWrkstnName = %q, want k8s-pod-99", sessCfg.ClientWrkstnName)
	}
}
