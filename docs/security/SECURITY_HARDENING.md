# Security hardening — deploy and infrastructure actions

Code-side fixes are in the tree. The items below cannot be fixed in code and must be done by an operator.

## P0 — do first
1. **Rotate the PostgreSQL `postgres` password.** It appears in git history, and in production `platform_admin.system_settings` (`gateway_configuration`, `ai_configuration` → `api_key`). Replace those keys with real Gateway keys after rotating.
2. **Firewall 5432** on the DB VM so only the app host's IP can reach it. Better still, use a private network. Do not expose it publicly.
3. **Firewall 8080** (and 8070). The server now binds `${SERVER_BIND_ADDR:-172.17.0.1}:8070` in docker-compose, so only the reverse proxy on the host reaches it.
4. Create a non-superuser app role later. RLS is inert while the app connects as `postgres`.

## DNS / mail
- SPF: `v=spf1 include:<mail provider> -all`
- DKIM: publish the provider's selector key.
- DMARC: `_dmarc TXT "v=DMARC1; p=quarantine; rua=mailto:dmarc@<domain>"`, then move to `p=reject`.
- MX: a real MX, or a null MX (`0 .`) if the domain sends but does not receive mail.
- CAA: `0 issue "letsencrypt.org"`.
- DNSSEC: enable at the registrar/DNS host.

## TLS (reverse proxy, e.g. Caddy/nginx on Elestio)
- TLS 1.2+ only, ECDHE suites only (forward secrecy), TLS 1.3 preferred.
- PQC key exchange: enable `X25519MLKEM768` (Caddy ≥2.9 / OpenSSL 3.5 do this by default).
- SNI: set a default certificate so clients that don't send SNI still get a valid certificate.

## Application settings
- `MODULE_API_ENABLED` defaults to false in prod. The legacy JSON module APIs stay unmounted; only enable them if a client needs them.
- `APP_BASE_URL` must be set. The sitemap and robots output use it, never the Host header.
- Migration 217 creates `dawa24_sql_console` (SELECT-only, with secrets columns excluded). The admin SQL console and Capsule datasets run under it.

## Already fixed in code
- CSP now uses `default-src 'none'` plus explicit sources.
- Per-request cryptographic CSP nonces (`script-src 'self' 'nonce-<base64>' 'unsafe-eval'`) are applied to all inline `<script>` tags across all 70+ templates. Injected scripts without a valid per-request nonce are blocked by modern browsers.
- Isolated element event attributes under `script-src-attr 'unsafe-inline'` so `on*=` handlers remain functional while preventing `<script>` tag injection.
- The visitor and MFA cookies are `Secure` + `HttpOnly` + `SameSite`.
- robots.txt no longer lists private areas; they get `X-Robots-Tag: noindex` instead.
- The sitemap excludes admin and API paths.
- IDOR fixes: org, billing invoice, quotes, inventory, workflow coverage, ingest sessions.
- The notification leak in the assistant is fixed.
- The delivery code no longer has a fixed fallback, and there is no fallback price at checkout.

## CSP remaining work
- **Replacing `on*=` attributes**: 437 legacy inline event handlers remain. Converting them to `addEventListener` or Alpine `@click` directives will allow removing `script-src-attr 'unsafe-inline'`.
- **Eliminating `'unsafe-eval'`**: Alpine 3 uses `eval` / `new Function()` for reactive expression evaluation. Eliminating `'unsafe-eval'` requires migrating components to Alpine's CSP build (`@alpinejs/csp`) with registered component functions.

## Notes
- `FetchGatewayModels` takes an operator-supplied URL. It is admin-only; keep it that way (SSRF).
- Uploaded files are served publicly under unguessable names; never upload private documents there.
