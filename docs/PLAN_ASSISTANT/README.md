# AI Assistant — plan index

1. **`00_AUDIT_AND_ARCHITECTURE.md`** — what exists, what the Gateway offers,
   the target architecture, decisions already made, open questions. Read fully.
2. **`01_IMPLEMENTATION.md`** — nine phases, in order.
3. `DECISIONS.md` — answers to Q1–Q6 and any assumption you had to make.
4. `PROGRESS.md` — one row per phase task.
5. `05_CAPSULE_V2.md` — the current assistant: datasets, confirmed actions,
   deploy notes. `03_READ_ONLY_AGENT.md` is superseded.

## The three findings that shape everything

**The assistant is a `setTimeout`.** `capsule_assistant.templ:417` waits 700 ms
and returns one of five hardcoded Arabic strings matched by keyword. There is no
`fetch(` anywhere in the file.

**The admin credentials are never used.** `gateway.HTTPClient` reads
`config.Gateway` from environment variables. The admin screen writes to
`platform_admin.system_settings`. Nothing joins them, so saving a valid
`api.muhiya.com` key changes nothing.

**The Gateway silently strips media a named model cannot accept**
(`proxy/media.go:214`). `qwen3.7-flash` has no DOCS or AUDIO capability, so
attaching a PDF to it produces a confident answer about nothing. That is why
Voxtral runs first — it is not an optimisation, it is the reason the pipeline
has two stages.

## Model roles

| Role | Model | Handles |
|---|---|---|
| `assistant.primary` | `qwen3.7-flash` | conversation, images, video |
| `assistant.attachment` | `voxtral-small-24b-2507` | documents, audio → text digest |
| `assistant.transcribe` | `whisper-1` | STT when the browser has none |

Model names appear **only** inside `internal/platform/gateway/`
(`AGENTS.md` R2, enforced by `make check-provider-isolation`, which **fails
today** because provider names leaked into `platform_admin`).

## Non-negotiable

- The API key never reaches the browser.
- No tools, no agent loop, no system/database access. The `Tools` field exists
  and is asserted empty, so adding tools later is not a rewrite.
- `X-Client-App` must not be `MuhiyaChat` — that enables the Gateway's own agent
  loop (`proxy/agent.go:57`).
- Read `/v1/capabilities` at runtime. Never hardcode which model takes what.
- Nothing ships as a stub. The current assistant is what a stub looks like.
