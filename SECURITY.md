# Security Policy

## Supported Versions

Only the latest release on the `main` branch receives security updates and bug fixes.

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| < 1.0.0 | :white_check_mark: |

## Reporting a Vulnerability

The `go-db2` maintainers take security seriously. If you discover a security vulnerability, please do **NOT** open a public issue on GitHub.

Instead, please report the vulnerability privately via:
- GitHub Security Advisory: Submit a private advisory at `https://github.com/go-db2/go-db2/security/advisories/new`
- Email: Contact project maintainers directly with full reproduction details.

Please provide:
1. Detailed description of the vulnerability and attack vector.
2. Minimal code snippet or DSN triggering the issue.
3. Affected versions, Go runtime versions, and IBM Db2 server edition/version.

We will acknowledge receipt within 48 hours and work with you to remediate the issue prior to public disclosure.

## Security Architecture & Invariants

- **Pure Go / Zero-CGO**: `go-db2` contains zero C dependencies (`CGO_ENABLED=0`) and does not use `unsafe`, preventing memory corruption, buffer overflows, and binary injection vulnerabilities typical of C wrapper drivers.
- **Transport Security & Plaintext Guard**: Standard `db2://` and Key-Value DSNs default to `ssl=false`, whereas `db2s://` URLs and DSNs specifying `ssl_root_ca`, `ssl_client_cert_path`, or `ssl_client_key_path` automatically enable TLS 1.2+ (`ssl=true`) with strict server certificate verification (`InsecureSkipVerify: false`). Plaintext password authentication (`SECMEC 3`) over unencrypted TCP is rejected by default unless explicitly enabled via `allow_plaintext_auth=true`.
- **SQL Injection Prevention**: Administrative procedures (`SYSPROC.ADMIN_CMD`) and dynamic DDL statements enforce strict identifier sanitization and whitelist validation (`internal/validate`).
- **Encrypted Authentication**: Supports DRDA SECMEC 9 (Diffie-Hellman key exchange + DES password encryption) to protect credentials on the wire, with downgrade protection enforced during `ACCSEC`/`SECCHK` negotiation. Native Kerberos (`SECMEC 7/11`) ticket exchange via keytab/ccache is currently disabled at runtime until a compliant RFC 4120 `AP-REQ` subpackage is integrated.

