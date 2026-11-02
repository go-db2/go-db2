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
- **Secure by Default Transport**: Connections require TLS/SSL encryption (`ssl=true`) by default. Plaintext authentication over unencrypted TCP is rejected unless explicitly enabled via `allow_plaintext_auth=true`.
- **SQL Injection Prevention**: Administrative procedures (`SYSPROC.ADMIN_CMD`) and dynamic DDL statements enforce strict identifier sanitization and whitelist validation (`internal/validate`).
- **Encrypted Authentication**: Supports DRDA SECMEC 9 (Diffie-Hellman Key Exchange + DES password encryption) and SECMEC 7/11 (Kerberos / GSSAPI token authentication) to prevent credential sniffing on the wire.

