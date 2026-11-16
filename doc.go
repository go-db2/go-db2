// Package db2 provides a pure Go driver for IBM Db2 databases implementing the database/sql/driver interface.
//
// The driver communicates directly with IBM Db2 database servers over TCP/IP using the Distributed
// Relational Database Architecture (DRDA) and Distributed Data Management (DDM) wire protocol.
// It requires zero C dependencies (CGO_ENABLED=0) and does not depend on IBM CLI/ODBC drivers (clidriver).
//
// # Connection String Formats (DSN)
//
// Connections can be configured using URL-style strings (db2:// or db2s://) or Key-Value strings:
//
//	db2://username:password@host:50000/database?ssl=false&allow_plaintext_auth=true
//	db2s://username:password@host:50001/database?ssl_root_ca=/path/to/ca.pem
//	host=localhost;port=50000;database=MYDB;user=usr;password=pwd;ssl=false;allow_plaintext_auth=true;
//
// # Key Parameters
//
//   - ssl (aliases: use_ssl, usessl, tls): Enable TLS/SSL transport encryption (default: false for db2:// and
//     Key-Value DSNs; automatically true for db2s:// or when ssl_root_ca / ssl_client_cert_path / ssl_client_key_path
//     is provided).
//   - TLS verification: Server certificate verification is strict by default (InsecureSkipVerify is false). There is no
//     DSN parameter to disable TLS verification; to supply custom in-memory CAs, client certificates, or custom TLS
//     settings, configure Config.TLSConfig programmatically with NewConnector.
//   - ssl_root_ca, ssl_client_cert_path, ssl_client_key_path: Paths to PEM root CA, client certificate, and private key.
//   - port: Server TCP port (default: 50000 when SSL is disabled, 50001 when SSL is enabled).
//   - timeout (aliases: connect_timeout, conn_timeout): Dial/connect timeout (default: 30s). Query execution and
//     I/O cancellations/timeouts are controlled via context.Context.
//   - block_size (aliases: blocksize, qryblksz): DRDA query block size in bytes (1024..1048576, default: 65535).
//   - allow_plaintext_auth: Allow unencrypted plaintext password authentication (SECMEC 3) over non-TLS TCP (default: false).
//   - security_mechanism (aliases: secmec, auth_mechanism, auth): DRDA authentication mechanism:
//     9 or dh_encrypted_password (Diffie-Hellman encrypted user/password),
//     3 or plain (plaintext user/password, requires TLS or allow_plaintext_auth=true),
//     4 or user_only (user ID only, rejects non-empty password),
//     7 or kerberos, 11 or encrypted_kerberos.
//     Note: Native RFC 4120 keytab/ccache ticket exchange for Kerberos (SECMEC 7/11) is disabled at runtime until
//     full RFC 4120 AP-REQ integration is available (pre-generated tokens are validated when provided).
//   - client_applname, client_wrkstnname, client_userid, client_acctng, client_corr_token: Client accounting and workload attribution registers.
//
// # Basic Usage
//
//	import (
//	    "database/sql"
//	    _ "github.com/go-db2/go-db2"
//	)
//
//	func main() {
//	    db, err := sql.Open("db2", "db2://usr:pwd@localhost:50000/MYDB?ssl=false&allow_plaintext_auth=true")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    defer db.Close()
//
//	    var val int
//	    if err := db.QueryRow("SELECT 1 FROM SYSIBM.SYSDUMMY1").Scan(&val); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// # Error Inspection
//
// Structured database errors can be inspected via errors.As and *db2.Error:
//
//	var dbErr *db2.Error
//	if errors.As(err, &dbErr) {
//	    fmt.Printf("SQLCODE: %d, SQLSTATE: %s\n", dbErr.SQLCode, dbErr.SQLState)
//	}
package db2
