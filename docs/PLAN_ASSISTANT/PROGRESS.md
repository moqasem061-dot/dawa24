# AI Assistant — Progress

| Phase | Task | Status | Commit | Tests | Notes |
|---|---|---|---|---|---|
| 1 | Credentials reach the Gateway | done | — | T1.1-T1.4 | `config_source.go` dynamic settings resolution with 30s cache TTL |
| 1 | Remove the provider-name leak | done | — | T1.5 | `check-provider-isolation` passes 100%, migration 102 applied |
| 2 | Streaming chat | done | — | T2.1-T2.5 | `chat_stream.go` with incremental SSE decoding, token streaming, cancellation |
| 2 | Multimodal content parts | done | — | T2.6 | OpenAI dialect parts (`image_url`, `input_audio`, `video_url`, `file`) |
| 2 | Transcription endpoint | done | — | T2.2 | `transcribe.go` with 10MB client guard and Whisper parameters |
| 2 | Capabilities fetch + cache | done | — | T2.7 | `capabilities.go` with runtime `/v1/models` fetching and 5m cache |
| 3 | Assistant module + routing | done | — | T3.1-T3.9 | `routing.go` multimodal pre-pass pipeline and digest generation |
| 3 | System prompt | done | — | T3.10 | Versioned prompt `2026-08-20.1` with medical safety + no system access bounds |
| 4 | SSE endpoint + cancellation | done | — | T4.1-T4.5 | `POST /api/v1/assistant/messages` SSE streaming with flusher |
| 4 | Rate limiting | done | — | T4.3 | Sliding window 20 req/min per user with 429 response |
| 5 | Attachment validation + lifecycle | done | — | T5.1-T5.5 | MIME sniffing, size caps, opaque user-bound handles |
| 6 | Native STT + Whisper fallback | done | — | T6.1-T6.4 | `POST /api/v1/assistant/transcribe` with Whisper-1 |
| 7 | Persistence + RLS | done | — | T7.1-T7.4 | Migration 103 with FORCE RLS + PostgreSQL repository |
| 8 | Frontend rebuild (B1-B6) | done | — | T8.1-T8.5 | Replaced simulated setTimeout with real SSE, uploads, speech recognition, and abort controllers |
| 9 | Performance + security verification | done | — | T1-T8 | Complete test suite passes with zero provider leaks and zero key exposure |

## 2026-09-02 — read-only business agent

See `03_READ_ONLY_AGENT.md`. The assistant now answers from the caller's own
authorized data through a permission-gated tool registry, and writes nothing.

| Item | Status | Notes |
|---|---|---|
| Cross-user conversation leak closed | done | `GetOwnedConversation` scopes by org, user and agent role in SQL |
| Owner-controlled permission | done | `pharmacy/vendor/platform.assistant.use` in the RBAC catalogue |
| Least privilege | done | every tool also requires the permission its dashboard screen requires |
| Three agents, one model | done | persona and tool set differ; `assistant.primary` throughout |
| Role routing | done | `assistant.AgentFor` on `DashboardScope()`; no client input |
| Signed, actor-bound handles | done | forgery, expiry, kind and binding all verified |
| Read-only data access layer | done | 17 read methods, no write path, RLS on every query |
| Tool registry + 10-step dispatch | done | audited in `assistant.tool_audit` |
| Server-owned resumable turns | done | Redis in production, memory in development |
| EventSource client | done | fixes the resize/minimise token loss |
| Attachments in object storage | done | rows + digests, no base64 in memory or Postgres |
| Tenant-keyed transcription, dynamic model | done | admin catalogue discovery, 15-minute cache |
| Markdown renderer with tables | done | escaped first, http(s) links only |
| Context/usage meter | done | real `context_window` from `/v1/models` |
| Six-month retention | done | indexed deadline, daily worker sweep, stated in the drawer |
| Security suite | done | 17 hostile-model cases plus handle, stream, agent and fence tests |
| Order creation / write tools | **dropped** | out of scope by decision; the agent is read-only |

## 2026-09-13 — Capsule v2 (datasets + confirmed actions)

See `05_CAPSULE_V2.md`.

| Item | Status | Notes |
|---|---|---|
| Dataset engine, full-table aggregates | done | 3 dashboards; projections it covers retired |
| Read-only DB role | done | migration 214; EXPLAIN of every plan passes as the role |
| Cross-tenant integration test | done | every dataset, group-by, record and export, two tenants |
| Exports (CSV/XLSX) | done | migration 215; owner-only download; Telegram `sendDocument` |
| Runtime: parallel tools, 10 rounds, 150 s | done | |
| Prompt v2 (short, no invention) | done | `2026-09-13.v2.1` |
| Confirmed actions | done | migration 216; 16 commands; `*.assistant.act` |
| Handlers refactored onto shared cores | done | fixed on the way: stock-adjust IDOR, wallet charged before offer check, approval side effects after failed approval, buyer cancel ownership |
| Telegram buttons and files | done | n8n Capsule workflow version `8aef00c2` published |
| 18-month fixture + ~600 graded cases, model bake-off | open | |
| Admin observability screen, load test | open | |
