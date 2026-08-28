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
