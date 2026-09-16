# PLAN V6 — Deleted

Every route, page, handler, table and test removed.

| What | Kind | Reason | Restored by |
|---|---|---|---|
| `GET/POST /auth/2fa-challenge`, `GET/POST /settings/security/2fa*` | Handlers & Routes | Fake 2FA security theatre that bypassed credentials | Phase C Task C.9 |
| `GET /invoices/{id}/pdf`, `GET /orders/{id}/pdf` | Handlers & Routes | Corrupted 5-line static text stubs pretending to be PDFs | Phase C Task C.10 |
| `defaultModelRegistry` in `admin_trash_handlers.go` | Mock constant | Fabricated row counts (1240, 14200, etc.) | Phase C Task C.7 |
| Hardcoded reference slices in `admin_reference_handlers.go` | Mock data | Fabricated items ("Twilio", "facebook.com", Egypt) | Phase C Task C.6 |
