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

## Conversation context type (audit batch 7, migration 253)

Conversations started by the inquiry flow carry `context_type = 'general'`
(`chat.ContextGeneral`). Migration 049 had rebuilt `chat.conversations` with a
CHECK that did not list `general`, so starting a conversation failed with
SQLSTATE 23514. Migration 253 adds `general` to the CHECK. No rows change.

Open: the conversation records no counterparty organization and the
counterparty org is never added as a participant, so the supplier side of a
new conversation cannot list it. `TestConversationVisibleToBothOrgsAndUnread`
stays red until that is decided. See `remaining-batches-report.md`, batch 7.
