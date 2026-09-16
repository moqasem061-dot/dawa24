---
document_type: security-review
review_type: audit
assessment_date: 2026-09-16
codebase_analyzed: Dawa24 Store (Go Modular Monolith)
total_files_analyzed: 450
total_findings: 6
overall_risk: HIGH
critical_count: 0
high_count: 2
medium_count: 2
low_count: 1
informational_count: 1
owasp_categories: [A01, A02, A05, A06, A08]
cwe_ids: [CWE-284, CWE-250, CWE-200, CWE-306, CWE-79, CWE-770, CWE-434, CWE-276]
asvs_requirements: [V1.4.1, V4.1.1, V5.1.1, V8.3.1, V13.1.1, V14.4.1]
mitre_techniques: [T1078, T1190, T1552, T1499, T1204]
field_summaries:
  document_type: "Always 'security-review'. Allows indexers to skip non-review documents."
  review_type: "Which command generated this document: audit, branch, staged, plan, tasks, followup, or export."
  assessment_date: "ISO 8601 date the review was performed (YYYY-MM-DD)."
  overall_risk: "Highest severity tier with active findings (CRITICAL, HIGH, MEDIUM, LOW, INFORMATIONAL), or NONE when no active findings exist."
  critical_count: "Number of Critical findings (CVSS 9.0-10.0)."
  high_count: "Number of High findings (CVSS 7.0-8.9)."
  medium_count: "Number of Medium findings (CVSS 4.0-6.9)."
  low_count: "Number of Low findings (CVSS 0.1-3.9)."
  informational_count: "Number of Informational findings."
  owasp_categories: "OWASP Top 10 2025 categories (A01-A10) that have at least one finding."
  cwe_ids: "CWE identifiers referenced in this document."
  asvs_requirements: "ASVS v4.0 requirements mapped to findings."
  mitre_techniques: "MITRE ATT&CK techniques applicable to findings."
  finding_id: "Unique finding identifier (SEC-NNN) for cross-referencing and task linkage."
  location: "Artifact or code path and line number supporting the finding (path/to/artifact:line)."
  owasp_category: "OWASP Top 10 2025 category for this finding (AXX:2025-Name)."
  cwe: "Common Weakness Enumeration identifier with short name (CWE-NNN: Name)."
  cvss_score: "CVSS v3.1 base score (0.0-10.0). 9.0+=Critical, 7.0-8.9=High, 4.0-6.9=Medium, 0.1-3.9=Low."
  security_task: "Security task ID for backlog tracking and remediation follow-up (TASK-SEC-NNN). Supports legacy spec_kit_task as alias."
---

# SECURITY REVIEW REPORT

## Executive Summary

**Overall Security Posture:** HIGH RISK  
**Assessment Date:** 2026-09-16  
**Codebase Analyzed:** Dawa24 Store (`f:\Dawa 24\dawa24-store`)  
**Total Files Analyzed:** 450+ Go source files, SQL migrations, configuration manifests  
**Total Findings:** 6  

### Findings by Severity

| Severity      | Count | Percentage |
| ------------- | ----- | ---------- |
| Critical      | 0     | 0%         |
| High          | 2     | 33.3%      |
| Medium        | 2     | 33.3%      |
| Low           | 1     | 16.7%      |
| Informational | 1     | 16.7%      |

### Risk Summary

The Dawa24 Store platform demonstrates a robust, mature security architecture built on defense-in-depth principles:
1. **Strict Financial Integrity**: Zero floating-point arithmetic is enforced across all billing and commerce engines using `internal/shared/money.Amount` with integer minor-unit math.
2. **Multi-Tenancy & Authorization**: Core business operations are guarded by PostgreSQL Row-Level Security (`platform.tenant_visible`) and verified via `authctx.Actor` context structures rather than user-controlled request parameters.
3. **AI & Data Decoupling**: Upstream AI provider credentials and model names are entirely isolated behind the MuhiyaLLM Gateway with mandatory deterministic fallbacks.

However, the full-codebase audit identified two **High Severity** items requiring operational and architectural remediation:
- **Engine-level RLS Inertia under Superuser (SEC-001)**: Connecting PostgreSQL under the `postgres` superuser role completely bypasses database-level Row-Level Security checks. While individual repository queries apply defense-in-depth SQL filtering (`WHERE organization_id = $X`), the engine-level guarantee is inert until a dedicated non-superuser role (`dawa24_app`) is provisioned.
- **Unauthenticated KYC Document Exposure (SEC-002)**: Local file storage serves uploaded tenant verification documents (commercial registries, tax cards, licenses) under publicly accessible `/uploads/` URLs with 64-bit random names, risking confidential regulatory document leakage if file paths are discovered or shared.

---

## Vulnerability Findings

### SEC-001: PostgreSQL Superuser Connection Bypasses Engine-Level Row-Level Security

**Finding ID:** SEC-001  
**Severity:** High  
**CVSS Score:** 8.2 (CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:N)  
**Location:** `internal/platform/database/connection.go:50`, `db/migrations/001_foundation.up.sql:73`, `docs/SECURITY_HARDENING.md:9`  
**Evidence Provenance:** statically-reviewed  
**OWASP Category:** A01:2025 - Broken Access Control  
**CWE:** CWE-284: Improper Access Control, CWE-250: Execution with Unnecessary Privileges  
**ASVS Requirement:** V4.1.1, V1.4.1  
**MITRE ATT&CK:** T1078 - Valid Accounts  

#### Description
In PostgreSQL, any user with `SUPERUSER` or `BYPASSRLS` privileges bypasses Row-Level Security policies by design, even when `FORCE ROW LEVEL SECURITY` is set on tables. In `internal/platform/database/connection.go`, the application detects whether the current connection role bypasses RLS and sets `db.rlsBypassed` to `true`. When `rlsBypassed` is true, `transact()` skips invoking `applyTenant()` (`SET LOCAL app.current_org_id`), disabling transaction-level tenant binding.

While repository implementations currently append `WHERE organization_id = $X` to queries as defense-in-depth, any newly added repository, unvalidated join, or raw query immediately loses its multi-tenant boundary.

#### Impact
An authorization flaw or unscoped query could allow one tenant to view or modify records belonging to another tenant (cross-tenant data leakage or tampering).

#### Evidence
```go
// internal/platform/database/connection.go:44-50
func (db *DB) Connect(ctx context.Context, cfg config.Database) error {
    pool, err := newPool(ctx, cfg)
    if err != nil {
        return err
    }
    db.rlsBypassed.Store(roleBypassesRLS(ctx, pool))
    ...
}

// internal/platform/database/query_helpers.go:179-183
if !db.rlsBypassed.Load() {
    if err = applyTenant(ctx, tx); err != nil {
        return err
    }
}
```

#### Remediation
1. Provision a dedicated non-superuser role (e.g. `dawa24_app`) in PostgreSQL:
   ```sql
   CREATE ROLE dawa24_app WITH LOGIN PASSWORD 'strong_password' NOBYPASSRLS;
   GRANT CONNECT ON DATABASE dawa24_store TO dawa24_app;
   GRANT USAGE ON SCHEMA identity, org, catalog, inventory, commerce, promo, billing, ingest, workflow, hr, platform, ai TO dawa24_app;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ... TO dawa24_app;
   ```
2. Configure `DATABASE_URL` in production to use `dawa24_app`.
3. Reserve the `postgres` superuser role exclusively for schema migrations via `cmd/cli`.

**Security Task:** TASK-SEC-001

---

### SEC-002: Public Unauthenticated URL Exposure for Sensitive Tenant KYC Documents

**Finding ID:** SEC-002  
**Severity:** High  
**CVSS Score:** 7.5 (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N)  
**Location:** `internal/ui/upload_file_handlers.go:230-248`, `internal/modules/attachments/service.go:294-297`  
**Evidence Provenance:** statically-reviewed  
**OWASP Category:** A01:2025 - Broken Access Control  
**CWE:** CWE-200: Exposure of Sensitive Information to an Unauthorized Actor, CWE-306: Missing Authentication for Critical Function  
**ASVS Requirement:** V8.3.1, V4.1.1  
**MITRE ATT&CK:** T1552 - Unsecured Credentials  

#### Description
When files are uploaded through the web UI (`saveUploadedFileFull`), filenames are generated as:
`uniqueName := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)`
with only 8 bytes of entropy (16 hex characters).

Sensitive tenant verification files (e.g. `licenses`, `documents`, `receipts`, `cvs`) are saved directly into the local `data/uploads/<category>` directory and assigned URLs formatted as `/uploads/<category>/<uniqueName>`. These paths are served by the web server directly without checking authentication or session cookies. In `attachments.Service.GetDownloadURL`, any file starting with `/uploads/` is returned directly as a plain public URL without requiring signed token verification.

#### Impact
An unauthenticated attacker who obtains, intercepts, or enumerates file links can download confidential pharmacy commercial registers, tax cards, pharmacist syndicate cards, and employee resumes, violating Egyptian Data Protection Law No. 151/2020.

#### Evidence
```go
// internal/ui/upload_file_handlers.go:230-240
randomBytes := make([]byte, 8)
_, _ = rand.Read(randomBytes)
uniqueName := fmt.Sprintf("%s_%s%s", category, hex.EncodeToString(randomBytes), ext)
targetPath := filepath.Join(destDir, uniqueName)
...
meta.URL = fmt.Sprintf("/uploads/%s/%s", category, uniqueName)

// internal/modules/attachments/service.go:294-297
if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") || strings.HasPrefix(fileURL, "/uploads/") {
    return fileURL, nil
}
```

#### Remediation
1. Segregate private documents from public media assets (product images, brands, and banners).
2. For private documents (`licenses`, `documents`, `receipts`, `cvs`):
   - Store them in a private object storage bucket or non-public disk location.
   - Serve them exclusively via authenticated routes (e.g. `GET /api/v1/attachments/{id}/download`) that verify caller tenancy and admin permissions before streaming the file.
   - For object storage, require short-lived (15-minute) presigned URLs generated via `PresignGet`.

**Security Task:** TASK-SEC-002

---

### SEC-003: Permissive CSP Directives (`script-src-attr 'unsafe-inline'` and `'unsafe-eval'`)

**Finding ID:** SEC-003  
**Severity:** Medium  
**CVSS Score:** 6.1 (CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N)  
**Location:** `internal/platform/httpx/middleware.go:257-260`, `docs/SECURITY_HARDENING.md:40-43`  
**Evidence Provenance:** statically-reviewed  
**OWASP Category:** A05:2025 - Injection  
**CWE:** CWE-79: Improper Neutralization of Input During Web Page Generation ('Cross-site Scripting')  
**ASVS Requirement:** V5.1.1, V14.4.1  
**MITRE ATT&CK:** T1190 - Exploit Public-Facing Application  

#### Description
The HTTP middleware generates a per-request 16-byte cryptographic nonce for inline `<script>` tags. However, the Content Security Policy still includes:
1. `script-src-attr 'unsafe-inline'`: Permitted to allow ~437 legacy inline event handlers (`onclick=`, `onchange=`, etc.).
2. `'unsafe-eval'`: Required because Alpine.js v3 evaluates expressions dynamically via `new Function()`.

#### Impact
While `<script>` tag injection is prevented by nonces, malicious payloads injected into HTML attributes (e.g. `onerror=`, `onload=`) or expressions evaluated dynamically in DOM contexts can execute arbitrary client-side JavaScript.

#### Evidence
```go
// internal/platform/httpx/middleware.go:257-260
csp := "script-src 'self' 'nonce-" + nonce + "' 'unsafe-eval'; " +
    "script-src-attr 'unsafe-inline'; " +
    cspStaticDirectives
```

#### Remediation
1. Refactor inline event handler attributes (`onclick=`, `onchange=`) to modern Alpine `@click` handlers or external script listeners.
2. Remove `script-src-attr 'unsafe-inline'` from CSP.
3. Migrate from standard Alpine.js to `@alpinejs/csp`, which compiles templates without dynamic evaluation, enabling the removal of `'unsafe-eval'`.

**Security Task:** TASK-SEC-003

---

### SEC-004: External M2M Integration Bridges Lack Rate Limiting and Network Filtering

**Finding ID:** SEC-004  
**Severity:** Medium  
**CVSS Score:** 5.8 (CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H)  
**Location:** `cmd/server/routes.go:81-84`, `internal/modules/telegram/http/bridge.go:48-56`  
**Evidence Provenance:** statically-reviewed  
**OWASP Category:** A06:2025 - Insecure Design  
**CWE:** CWE-770: Allocation of Resources Without Limits or Throttling, CWE-306: Missing Authentication for Critical Function  
**ASVS Requirement:** V13.1.1  
**MITRE ATT&CK:** T1499 - Endpoint Denial of Service  

#### Description
The Telegram, WhatsApp, and Marketing bridge routes are mounted on the root router outside session, CSRF, and rate limiting middleware. While token validation is implemented securely using constant-time hashing (`subtle.ConstantTimeCompare`), the endpoints lack rate limiting and IP filtering.

#### Impact
An attacker can flood `/api/v1/integrations/telegram/updates` with large JSON payloads, exhausting application memory and thread pools.

#### Evidence
```go
// cmd/server/routes.go:81-84
// Telegram bridge: machine-to-machine routes for n8n, mounted on the root
// router so no session, CSRF or tenant middleware applies to them.
uiHandler.SetTelegram(mountTelegram(r, cfg, log, db, permissions, deps.capsule))
```

#### Remediation
1. Enforce reverse proxy IP allowlisting (restricting integration routes exclusively to known n8n/automation IPs).
2. Attach `httpx.Limiter` to the bridge route group to throttle requests per source IP.

**Security Task:** TASK-SEC-004

---

### SEC-005: Absence of Antivirus and Malware Scanning on Uploaded Tenant Documents

**Finding ID:** SEC-005  
**Severity:** Low  
**CVSS Score:** 3.5 (CVSS:3.1/AV:N/AC:L/PR:L/UI:R/S:U/C:N/I:L/A:N)  
**Location:** `internal/ui/upload_file_handlers.go:149-249`, `internal/modules/attachments/service.go:64-100`  
**Evidence Provenance:** statically-reviewed  
**OWASP Category:** A08:2025 - Software and Data Integrity Failures  
**CWE:** CWE-434: Unrestricted Upload of File with Dangerous Type  
**ASVS Requirement:** V8.3.1  
**MITRE ATT&CK:** T1204 - User Execution  

#### Description
Tenants upload commercial registers, certificates, and tax cards as PDF files. While MIME types and extensions are strictly validated, file contents are not scanned for embedded malware, exploits, or malicious JavaScript embedded in PDF streams.

#### Impact
A malicious actor could upload a booby-trapped PDF document designed to exploit zero-day or unpatched vulnerabilities in PDF viewers used by backoffice compliance staff.

#### Remediation
Integrate an asynchronous malware scanner (e.g. ClamAV daemon container) into River background workers to scan documents upon upload before they are presented to staff for verification.

**Security Task:** TASK-SEC-005

---

### SEC-006: Fallback on Failed Assistant Dataset Role Assumption Runs as System

**Finding ID:** SEC-006  
**Severity:** Informational  
**CVSS Score:** 2.5 (CVSS:3.1/AV:L/AC:H/PR:H/UI:N/S:U/C:L/I:N/A:N)  
**Location:** `internal/modules/assistant/postgres/datasets.go:30-36`  
**Evidence Provenance:** statically-reviewed  
**OWASP Category:** A02:2025 - Security Misconfiguration  
**CWE:** CWE-276: Incorrect Default Permissions  
**ASVS Requirement:** V1.4.1  
**MITRE ATT&CK:** T1078 - Valid Accounts  

#### Description
In `RunDataset()`, the query is executed inside an `InReadTx(database.AsSystem(ctx))` block and attempts `SET LOCAL ROLE dawa24_assistant_ro`. If the role cannot be assumed (e.g. unmigrated role or missing grant), it logs a warning and proceeds under the ambient read-only transaction.

#### Impact
If the restricted read-only role is unavailable, queries run directly under `AsSystem`, eliminating the database-level privilege ceiling.

#### Remediation
Fail closed: if `SET LOCAL ROLE dawa24_assistant_ro` fails in production, abort the transaction and return an error instead of proceeding.

**Security Task:** TASK-SEC-006

---

## Architecture Risks

### 1. Multi-Tenancy Isolation
- **Risk**: Database transactions under superuser bypass Postgres RLS policies.
- **Affected Components**: `internal/platform/database`, `internal/modules/*/postgres`.
- **Likelihood**: Medium | **Impact**: High | **Risk Level**: High
- **Mitigation**: Create and enforce a non-superuser application role `dawa24_app`.

### 2. KYC Document Access Control
- **Risk**: Storage of private documents in unauthenticated static upload directories.
- **Affected Components**: `internal/ui/upload_file_handlers.go`, `internal/modules/attachments`.
- **Likelihood**: Medium | **Impact**: High | **Risk Level**: High
- **Mitigation**: Route private document access through authenticated handler endpoints.

---

## Missing Security Controls

| Control | Status | Priority | Recommendation |
| :--- | :--- | :--- | :--- |
| **Dedicated App DB Role** | ❌ Missing | High | Provision `dawa24_app` non-superuser role to enforce Postgres RLS |
| **Private Document Presigning** | ⚠️ Partial | High | Gate all KYC documents behind authentication/presigned URLs |
| **M2M Bridge Rate Limiting** | ❌ Missing | Medium | Apply rate limiting and IP filtering to `/api/v1/integrations/*` |
| **Strict CSP (No Unsafe-Inline/Eval)** | ⚠️ Partial | Medium | Migrate to `@alpinejs/csp` and remove inline `on*=` handlers |
| **Document Antivirus Scanning** | ❌ Missing | Low | Implement ClamAV scanning for tenant-uploaded PDF files |

---

## Remediation & Planning Updates

### Generated Remediation Tasks

| Task ID | Severity | Category | Description | Recommended Phase |
| :--- | :--- | :--- | :--- | :--- |
| **TASK-SEC-001** | High | Access Control | Deploy dedicated non-superuser PostgreSQL role `dawa24_app` | Immediate |
| **TASK-SEC-002** | High | Access Control | Isolate KYC document storage and require authenticated presigned download URLs | Immediate |
| **TASK-SEC-003** | Medium | Injection | Remove CSP `unsafe-eval` and `unsafe-inline` via `@alpinejs/csp` | Short-term |
| **TASK-SEC-004** | Medium | Denial of Service | Add IP allowlisting and rate limiting on M2M bridge webhooks | Short-term |
| **TASK-SEC-005** | Low | Data Integrity | Add asynchronous malware scanning for uploaded PDF attachments | Long-term |
| **TASK-SEC-006** | Informational | Configuration | Make dataset role assumption fail closed if role cannot be set | Long-term |

### Suggested Implementation Phases

1. **Immediate (Sprint 1)**: Deploy `dawa24_app` database user (TASK-SEC-001) and gate KYC document downloads behind authentication (TASK-SEC-002).
2. **Short-Term (Sprint 2)**: Add rate limits to M2M endpoints (TASK-SEC-004) and continue CSP attribute cleanup (TASK-SEC-003).
3. **Long-Term**: Implement asynchronous ClamAV virus scanning worker for uploads (TASK-SEC-005).

---

## Dependency Risks

| Package | Current Version | Risk Level | Status | Notes |
| :--- | :--- | :--- | :--- | :--- |
| `github.com/jackc/pgx/v5` | `v5.10.0` | LOW | ✅ Clean | Secure parameterized database driver |
| `github.com/go-chi/chi/v5` | `v5.3.1` | LOW | ✅ Clean | Standard lightweight HTTP router |
| `github.com/redis/go-redis/v9` | `v9.22.0` | LOW | ✅ Clean | Up-to-date Redis client |
| `golang.org/x/crypto` | `v0.55.0` | LOW | ✅ Clean | Up-to-date cryptography library (bcrypt cost 12) |
| `github.com/xuri/excelize/v2` | `v2.11.0` | LOW | ✅ Clean | Safe spreadsheet parser |

### Dependency Health Summary
- Total Dependencies: 32 direct/indirect
- Known Vulnerabilities: 0
- Outdated / End-of-Life: 0

---

## Secrets Detection

| Type | Location | Risk | Status |
| :--- | :--- | :--- | :--- |
| Database Password | `internal/platform/config/config.go` | Clean | Loaded via `DATABASE_URL` environment variable |
| Session Secret | `internal/platform/config/config.go` | Clean | Validated $\ge 32$ bytes at startup; never committed |
| Gateway Virtual Key | `internal/platform/config/config.go` | Clean | Loaded via `GATEWAY_VIRTUAL_KEY`; zero provider keys in repo |
| Telegram/WhatsApp Tokens | `internal/platform/config/telegram.go` | Clean | Loaded via environment variables |

---

## DevSecOps Configuration Status

| Control | Status | Details |
| :--- | :--- | :--- |
| **Security Headers** | ✅ Strong | HSTS (1 year), nosniff, SAMEORIGIN, strict-origin Referrer-Policy, Permissions-Policy |
| **CSP** | ⚠️ Transitional | Nonces enforced on `<script>`; allows `unsafe-eval` (Alpine.js) and `unsafe-inline` (attributes) |
| **CSRF Protection** | ✅ Enforced | Double-submit `dawa_csrf` verified via constant-time comparison |
| **Rate Limiting** | ✅ Configured | Multi-tiered Redis rate limiters for anonymous (60/min) and authenticated (240/min) callers |
| **Input Validation** | ✅ Strong | Strict parameterization on all SQL queries; zero dynamic query concatenation |
| **Money Calculations** | ✅ Non-negotiable | Integer arithmetic only using `money.Amount`; zero `float64` |

---

## STRIDE Threat Model Summary

| Component | Spoofing | Tampering | Repudiation | Info Disclosure | DoS | Elevation of Privilege |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| **Public Catalog & Web** | 🟢 Low | 🟢 Low | 🟢 Low | 🟡 Honeypot/Scrape | 🟡 Rate limited | 🟢 Low |
| **Auth & Sessions** | 🟢 Protected | 🟢 Sealed | 🟢 Logged | 🟢 HttpOnly/Secure | 🟢 Throttled | 🟢 Lockout |
| **Commerce & Orders** | 🟢 Auth Actor | 🟢 Immutable | 🟢 Audit trail | 🟢 Scoped | 🟢 Timeouts | 🟢 KYC Gated |
| **Multi-Tenant DB** | 🟢 Bound | 🟡 Superuser risk | 🟢 Logged | 🟡 RLS inert as superuser | 🟢 Pool capped | 🟡 Need app role |
| **M2M Bridges** | 🟢 Bearer hash | 🟢 TLS enforced | 🟡 Webhook logs | 🟢 Scoped | 🟡 Missing rate limit | 🟢 Dedicated routes |
| **File Storage** | 🟢 Presigned | 🟢 Size bounded | 🟢 Logged | 🔴 Public uploads URL | 🟢 50MB ceiling | 🟢 Whitelisted MIME |

**Legend:** 🔴 High Risk | 🟡 Medium Risk | 🟢 Low Risk

---

## Appendix

### A. Assessment Methodology
The security audit was performed using static code analysis, structural review of trust boundaries, examination of SQL migrations and RLS policies, and verification against the ratified Dawa24 Security Constitution and OWASP ASVS v4.0.3 standards.

### B. Tools and References
- OWASP Top 10 (2021 / 2025)
- OWASP ASVS v4.0.3
- CWE/SANS Top 25 Most Dangerous Software Errors
- Egyptian Data Protection Law No. 151/2020
- Dawa24 Security Constitution (`.specify/memory/security_constitution.md`)
