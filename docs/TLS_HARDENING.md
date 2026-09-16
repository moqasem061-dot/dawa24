# TLS Hardening Guide — Grade A+ Forward Secrecy & Post-Quantum Cryptography

This document details the TLS remediation implemented for the Dawa24 Store platform, resolving the SSL Labs / testssl.sh audit findings and achieving **Grade A+** with 100% Perfect Forward Secrecy (PFS) and Post-Quantum Cryptography (PQC).

---

## 1. Audit Findings & Root Cause Analysis

| Finding from Scan | Root Cause | Impact | Resolution |
|:---|:---|:---:|:---|
| **Forward Secrecy not supported (Grade capped to B)** | Static RSA key exchange suites (`TLS_RSA_WITH_*`) were permitted in TLS 1.2. | **Grade Capped to B**; past session data can be decrypted if the private key is ever compromised. | **Eliminated all `TLS_RSA_*` cipher suites.** Only ECDHE key exchange suites are permitted. |
| **Weak CBC & Legacy Ciphers Offered** | CBC-mode ciphers (`AES_128_CBC`, `AES_256_CBC`, `CAMELLIA`, `ARIA`, `CCM`) were accepted. | Vulnerable to padding oracle attacks (e.g. Lucky13, POODLE/BEAST). | **Eliminated all CBC, Camellia, ARIA, and CCM ciphers.** Enforced AEAD-only ciphers (GCM & ChaCha20-Poly1305). |
| **Server has no preference** | The server/proxy did not enforce its own cipher suite preference order over the client's. | Clients could negotiate weaker cipher suites. | Enforced `PreferServerCipherSuites = true` (Go) / `ssl_prefer_server_ciphers on;` (Nginx). |
| **No PQC (Post-Quantum Cryptography) Key Exchange** | Elliptic curves were limited to classical curves (P-256, P-384, X25519) without quantum-resistant algorithms. | Vulnerable to "Harvest Now, Decrypt Later" quantum attacks. | Enabled **`X25519MLKEM768`** and **`SecP256r1MLKEM768`** hybrid post-quantum key exchange in Go and reverse proxy. |
| **SNI Only Warning** | The proxy required SNI without serving a default fallback certificate for legacy clients. | Informational warning for non-SNI clients. | Configured default server fallback block with valid certificate. |

---

## 2. Hardened Cipher Suites & Curves

### Approved TLS 1.2 Cipher Suites (PFS & AEAD Only)
1. `TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384` (0xc02c) — ECDH secp256r1/X25519, FS, 256-bit AEAD
2. `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384` (0xc030) — ECDH secp256r1/X25519, FS, 256-bit AEAD
3. `TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256` (0xcca9) — ECDH secp256r1/X25519, FS, 256-bit AEAD
4. `TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256` (0xcca8) — ECDH secp256r1/X25519, FS, 256-bit AEAD
5. `TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256` (0xc02b) — ECDH secp256r1/X25519, FS, 128-bit AEAD
6. `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256` (0xc02f) — ECDH secp256r1/X25519, FS, 128-bit AEAD

*(TLS 1.3 cipher suites `TLS_AES_128_GCM_SHA256`, `TLS_AES_256_GCM_SHA384`, and `TLS_CHACHA20_POLY1305_SHA256` are automatically active and cannot be downgraded).*

### Approved Key Exchange Curves (Including PQC)
1. **`X25519MLKEM768`** (Post-Quantum hybrid Kyber/ML-KEM 768 + X25519)
2. **`SecP256r1MLKEM768`** (Post-Quantum hybrid ML-KEM 768 + NIST P-256)
3. **`X25519`** (Classical Curve25519)
4. **`CurveP256`** (NIST P-256)

---

## 3. Implementation Options

### Option A: Native Go Platform TLS (Direct HTTPS)
The Go application binary includes native TLS termination:
- Configured in [`internal/platform/httpx/tls.go`](file:///f:/Dawa%2024/dawa24-store/internal/platform/httpx/tls.go) via `httpx.DefaultTLSConfig()`.
- Set the following environment variables:
  ```env
  TLS_ENABLED=true
  TLS_CERT_FILE=/etc/ssl/certs/fullchain.pem
  TLS_KEY_FILE=/etc/ssl/private/privkey.pem
  PORT=443
  ```
- Starts automatically in [`cmd/server/main.go`](file:///f:/Dawa%2024/dawa24-store/cmd/server/main.go) with `srv.ListenAndServeTLS()`.

### Option B: Reverse Proxy (Elest.io / Nginx / Caddy)
When running behind an edge reverse proxy (standard in production Docker compose):
1. **Nginx**: Drop in [`deploy/tls/nginx-hardened.conf`](file:///f:/Dawa%2024/dawa24-store/deploy/tls/nginx-hardened.conf).
   - In your Elest.io service dashboard or host Nginx config:
     ```nginx
     ssl_protocols TLSv1.2 TLSv1.3;
     ssl_prefer_server_ciphers on;
     ssl_ciphers 'ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256';
     ssl_ecdh_curve X25519MLKEM768:X25519Kyber768Draft00:X25519:prime256v1:secp384r1;
     add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
     ```
2. **Caddy**: Use [`deploy/tls/Caddyfile`](file:///f:/Dawa%2024/dawa24-store/deploy/tls/Caddyfile).
