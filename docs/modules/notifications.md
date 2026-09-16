# Module: Notifications & Messaging

## Overview

The `notifications` bounded context handles multi-channel message dispatching (SMS, WhatsApp, Email, In-App), template rendering, and user notification inboxes.

## Schema Mapping

- **PostgreSQL Schemas:** `notifications`
- **Migrations:** `013_notifications.up.sql`
- **Tables Owned:**
  - `notifications.templates` — Localized message templates.
  - `notifications.logs` — Delivery audit logs and in-app feed records.

## Invariants & Rules

1. **Multi-Channel Dispatch:** Dispatches across `sms`, `whatsapp`, `email`, and `in_app` with tracked delivery states (`pending`, `sent`, `delivered`, `failed`).
2. **Tenant Isolation:** In-app notification feeds are isolated with `FORCE ROW LEVEL SECURITY`.
