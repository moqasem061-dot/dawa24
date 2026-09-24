# Chat & Messaging System Oversight (Audit & Security)

## Overview
The chat system manages peer-to-peer discussions between pharmacies, vendors, and platform support.

## Security & Privacy Gates
- **Peer Conversations**: Visible only to participating organizations and their assigned staff.
- **Admin Chat History Oversight (`/admin/chat/history`)**:
  - Gated behind `platform.activity_log.view` and `chat.history.view`.
  - Every administrative view of a private thread is logged to the system audit trail with administrator ID, timestamp, and target conversation ID.
  - No customer/vendor secrets or credentials should be logged.

## Decision Trees (`/admin/chat/tree`)
- Guided diagnostic and product recommendation trees (`catalog.finder_questions`, `catalog.finder_options`, `catalog.finder_results`).
- Aliased to `/admin/finder` for parity with Laravel's `ChatTree`.

## Posting is limited to the two organizations (security fix 2026-09-24)

`Service.SendMessage` loads the conversation and refuses a sender whose organization is neither `organization_id` nor `counterparty_org_id`. The UI posted by the id in the URL and relied on row-level security alone, which a connection that bypasses RLS does not apply.
