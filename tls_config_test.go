package db2

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// startTLSMockServer runs the mock DRDA handshake behind TLS, serving a
// self-signed certificate for 127.0.0.1, and returns the certificate so a
// test can trust it.
func startTLSMockServer(t *testing.T) (int, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "db2 mock"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
	if err != nil {
		t.Fatalf("failed to start TLS mock listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				mockHandshake(c)
			}(conn)
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, cert
}

func TestConfigTLSConfig_TrustsInMemoryCA(t *testing.T) {
	port, cert := startTLSMockServer(t)

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	tlsConfig := &tls.Config{RootCAs: pool}

	cfg := NewConfig()
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.Database = "TESTDB"
	cfg.User = "db2user"
	cfg.Password = "secret"
	cfg.UseSSL = true
	cfg.TLSConfig = tlsConfig

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := NewConnector(cfg).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect with an in-memory CA failed: %v", err)
	}
	_ = conn.Close()

	if tlsConfig.MinVersion != 0 {
		t.Errorf("the caller's tls.Config was modified: MinVersion = %#x", tlsConfig.MinVersion)
	}
}

func TestConfigTLSConfig_UntrustedWithoutIt(t *testing.T) {
	port, _ := startTLSMockServer(t)

	cfg := NewConfig()
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.Database = "TESTDB"
	cfg.User = "db2user"
	cfg.Password = "secret"
	cfg.UseSSL = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := NewConnector(cfg).Connect(ctx)
	if err == nil {
		_ = conn.Close()
		t.Fatal("Connect trusted a self-signed certificate that was never configured")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("expected a certificate verification error, got: %v", err)
	}
}
