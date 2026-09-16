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
