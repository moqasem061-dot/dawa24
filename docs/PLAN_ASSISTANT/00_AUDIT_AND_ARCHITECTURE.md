# AI Assistant — Audit & Architecture

**Date:** 2026-08-20
**Audience:** Gemini 3.7 Flash (executing agent)
**Scope:** the floating "كبسولة" assistant, its Gateway integration, and the
admin settings that configure it.

Read this file completely before opening `01_IMPLEMENTATION.md`. Every decision
in the implementation plan follows from something measured here.

---

## PART 1 — What exists today

### 1.1 The assistant is a `setTimeout` with canned strings

`internal/ui/components/capsule_assistant.templ`, 483 lines, mounted globally
from `internal/ui/layouts/base.templ:39`. The entire "AI" is this:

```js
sendMessage() {
    ...
    this.isProcessing = true;
    setTimeout(() => {                          // ← 700 ms of theatre
        this.isProcessing = false;
        const responseText = this.generateResponse(q);
        this.typewriterMessage(responseText);
    }, 700);
},

generateResponse(q) {
    const lower = (q || '').toLowerCase();
    if (lower.includes('مضاد')) return 'تم فحص أحدث عروض المضادات الحيوية...';
    if (lower.includes('مقارنة')) return 'يمكنك النقر على أي صنف...';
    if (lower.includes('تبريد')) return 'تشترط لائحة التوريد حفظ الأدوية...';
    if (lower.includes('صوتي')) return 'تم استلام التسجيل الصوتي بنجاح وجاري ربطه...';
    return 'تم استلام طلبك ومطابقته مع قواعد بيانات الأدوية...';
},
```

**There is no network call anywhere in the component.** `grep -n "fetch(" ` over
the file returns nothing. Five hardcoded Arabic answers matched by keyword.

Note the fourth one: it tells the user their voice recording is "being connected
to the speech-to-text pipeline". Nothing is recorded, uploaded or transcribed.

### 1.2 What the UI shell already does well (keep it)

The chrome is genuinely built and worth preserving:

| Feature | State |
|---|---|
| Floating trigger + slide-out drawer, Alpine transitions | works |
| Drag-and-drop overlay (`onDragEnter`/`onDrop`) | works, files land in `attachedFiles` |
| Attachment chips with remove | works |
| Voice-record UI (start / cancel / finish) | **UI only** — no `MediaRecorder`, no upload |
| Suggested-question chips | works, calls `askChip()` → `sendMessage()` |
| Typewriter rendering | works, but over a fake string |
| Auto-growing textarea | works |

### 1.3 The UI bugs visible in your screenshot

| # | Bug | Cause |
|---|---|---|
| B1 | Drawer overflows the viewport; input row is cut off at the bottom | fixed `height:640px` with `max-height:calc(100vh - 48px)`, but the internal flex column has no `min-height:0` on the scroll region, so the message list refuses to shrink and pushes the composer off-screen |
| B2 | Placeholder text visibly clipped ("اكتب استفسارك الصيدلي..." then a cut second line) | textarea starts at `height:38px` while its line-height needs more; the auto-grow script only runs on `input`, never on mount |
| B3 | Drawer is `position:fixed; left:24px` unconditionally | on mobile this leaves a 24px gutter and a 430px box wider than the screen; it needs a full-screen sheet under ~640px |
| B4 | Emoji used as UI controls (`✕` for remove) | `capsule_assistant.templ:194` — the rest of the platform uses the icon set in `components/icons.templ` |
| B5 | The header claims "متصل" (online) with a green pulse dot at all times | hardcoded; no health signal |
| B6 | Drawer sits at `z-index:9995` over the admin sidebar and cannot be moved | no minimise state |

### 1.4 The Gateway client exists and is well built — but is not used by the assistant

`internal/platform/gateway/gateway.go`, 399 lines. It has:

* a capability abstraction (`product.match`, `catalog.chat`, …) that maps to
  Gateway *aliases*, never provider model names;
* per-capability timeout + retry budgets;
* a circuit breaker (`newBreaker(5, 30s, 60s)`);
* typed errors so callers branch once (`ErrUnavailable`, `ErrCircuitOpen`, …);
* correct wire format: `POST {BaseURL}/v1/chat/completions`,
  `Authorization: Bearer <key>`, `X-Client-App: <app>`.

**What it cannot do, and the assistant needs:**

| Missing | Why it matters |
|---|---|
| Streaming | `chatRequest.Stream` is set to `false`; the response decoder reads one JSON body. An assistant that waits 8 s then dumps a wall of text feels broken. |
| Multimodal content parts | `chatMessage.Content` is a plain `string`. The Gateway accepts `image_url`, `input_audio`, `video_url`, `file` parts — this client cannot express them. |
| Multi-turn history | `Request` carries one `System` and one `Input` string. No conversation. |
| Transcription | No `POST /v1/audio/transcriptions` support. |
| Naming a specific model | Capability→alias only. The assistant must reach `qwen3.7-flash` and `voxtral-small-24b-2507` by name. |
| Cancellation surfaced to the caller | `Invoke` is blocking; there is no way to abort a stream. |

### 1.5 🔴 The admin-configured credentials are never used

This is the central defect behind "not connected to the AI Gateway credentials
configured in the Admin Settings".

There are **two disconnected configuration stores**:

| | Written by | Read by |
|---|---|---|
| `config.Gateway` (env vars: `BaseURL`, `VirtualKey`, `ClientApp`, `Timeout`, `Enabled`) | process environment at boot | `gateway.HTTPClient` — **the only thing that actually calls the Gateway** |
| `platform_admin.system_settings` key `gateway_configuration` + the AI settings blob | the admin screen in your screenshot | `AdminSettingsPage` (to redisplay the form) and nothing else |

Proof:

```bash
grep -rn "GetGatewaySettings\|GetAISettings" internal/ cmd/ --include=*.go | grep -v platform_admin/
# internal/ui/admin_handlers.go       — renders the form
# internal/ui/admin_reference_handlers.go — renders the (fabricated) integrations list
```

`gateway.HTTPClient` never sees them. An administrator can type a valid
`api.muhiya.com` key, press Save, and change nothing about the platform's
behaviour.

### 1.6 🔴 The provider-isolation invariant is already broken

`AGENTS.md` rule R2 and `make check-provider-isolation`: provider names must not
appear outside `internal/platform/gateway/`. They do:

```
internal/modules/platform_admin/domain.go:165   // gemini, openai, anthropic, deepseek, custom
internal/modules/platform_admin/domain.go:166   // e.g. gemini-1.5-pro, gpt-4o
internal/modules/platform_admin/service.go:182  Provider: "gemini",
internal/modules/platform_admin/service.go:183  Model:    "gemini-1.5-pro",
```

Your screenshot shows the consequence: the Store's own admin panel asks an
operator to type a provider model name (`gemini-1.5-flash`) into the Store's
database. That is the Gateway's job. The plan resolves this in Phase 1.

### 1.7 `aicapabilities` module

`internal/modules/aicapabilities/` — 147 lines of production code. It wraps
`gateway.Client` for `product.match`. It is **not** the right home for the
assistant: the assistant is a UI-facing conversational surface, not a domain
capability. Leave it alone.

---

## PART 2 — What the Muhiya Gateway actually offers

Read from `F:\MuhiyaWorkspace\MuhiyaWorkspace`. This is the contract you build
against; do not guess it.

### 2.1 Endpoints (`main.go:520-548`)

| Endpoint | Purpose |
|---|---|
| `POST /v1/chat/completions` | OpenAI dialect. Streaming via `"stream": true` (SSE). |
| `POST /v1/audio/transcriptions` | Multipart, OpenAI dialect. Larger body cap than chat (`maxTranscriptionRequestBytes`). |
| `GET /v1/capabilities` | Per-model modality flags. **This is how you learn what a model accepts — do not hardcode it.** |
| `GET /v1/models` | Model list. |
| `GET /health` | Liveness. |

### 2.2 Authentication (`proxy/handler.go:608-620`)

```
Authorization: Bearer sk-virt-…      (preferred)
x-api-key: sk-virt-…                 (accepted)
X-Client-App: <name>                 (identifies the caller in request_logs)
```

A revoked or expired key returns **401**. A database blip during auth returns
**503** — retryable. Distinguish these.

### 2.3 The capability payload (`proxy/catalog.go:63-72`)

```go
type muhiyaCapabilities struct {
    Vision            bool     `json:"vision"`
    Thinking          bool     `json:"thinking"`
    Audio             bool     `json:"audio"`
    Video             bool     `json:"video"`
    Documents         bool     `json:"documents"`
    MaxAttachmentMB   int      `json:"max_attachment_mb"`
    AcceptedMIMETypes []string `json:"accepted_mime_types"`
    InputModalities   []string `json:"input_modalities"`
}
```

This maps exactly onto the badges in your models screenshot
(VISION / THINKING / AUDIO / VIDEO / DOCS).

### 2.4 🔴 The behaviour that makes the Voxtral pre-pass necessary

`proxy/media.go:14-15`:

> "Every input modality a request can carry is detected here, matched against the
> operator-set per-model capability flags, and either **routed to a capable model
> (router requests)** or **stripped with an honest note (explicit-model
> requests)**."

`stripUnsupportedMediaParts` (`media.go:214`) removes parts the named model
cannot take and substitutes `mediaOmittedNote`.

**Consequence:** when the assistant names `qwen3.7-flash` explicitly and attaches
a PDF, the Gateway silently drops the PDF and Qwen answers about nothing. This is
not a bug to work around — it is why the two-stage pipeline exists.

### 2.5 Content-part dialect (`proxy/media.go:19-22`)

```
image_url    → vision
input_audio  → audio
video_url    → video
file         → documents (e.g. PDF)
```

### 2.6 The three models (from your models screen)

| Virtual name | Capabilities | Role in this plan |
|---|---|---|
| `qwen3.7-flash` | VISION · THINKING · VIDEO · MUHIYACHAT · ctx 1,000,000 · out 66,000 | primary conversational model |
| `voxtral-small-24b-2507` | AUDIO · DOCS · MUHIYACHAT · ctx 32,000 | attachment-understanding pre-pass |
| `whisper-1` | TRANSCRIBE · $0.0015/min | STT fallback when the browser has none |

Note `qwen3.7-flash` has **no DOCS and no AUDIO** badge. That is the routing rule
in one line: **images and video go straight to Qwen; documents and audio go
through Voxtral first.**

Note also Voxtral's context is **32,000**, not a million. Its output must be
summarised, not pasted whole, or a large PDF will blow the window.

⚠️ **Do not hardcode this table.** Read `/v1/capabilities` at runtime and cache
it (Phase 3). The badges change when the operator changes them.

---

## PART 3 — Target architecture

### 3.1 The shape

```
Browser (Alpine component)
   │  POST /api/v1/assistant/messages     (SSE down)
   │  POST /api/v1/assistant/attachments  (multipart, returns handles)
   │  POST /api/v1/assistant/transcribe   (multipart, whisper fallback)
   ▼
internal/modules/assistant/          ← NEW module
   service.go        conversation orchestration
   routing.go        which model gets which attachment
   prompt.go         system prompt, versioned
   attachments.go    validation, size caps, MIME allowlist
   http/handlers.go  SSE endpoint, cancellation
   postgres/         conversations + messages (Phase 7)
   ▼
internal/platform/gateway/           ← EXTENDED, not replaced
   gateway.go        existing capability API (untouched)
   chat.go           NEW: streaming, multimodal, named models
   transcribe.go     NEW: /v1/audio/transcriptions
   capabilities.go   NEW: /v1/capabilities + cache
   config_source.go  NEW: admin-settings-backed credentials
   ▼
api.muhiya.com
```

### 3.2 Why a new module rather than extending `aicapabilities`

`aicapabilities` serves domain capabilities (match a product name). The
assistant is a conversational surface with its own persistence, streaming and
attachment lifecycle. Mixing them would put SSE plumbing inside a matching
service. `AGENTS.md` rule R5 keeps modules from importing each other, so the
assistant gets its own bounded context.

### 3.3 Why the Gateway package is extended, not bypassed

Rule R2 is not negotiable: `qwen3.7-flash`, `voxtral-small-24b-2507` and
`whisper-1` are model identifiers. They may appear **only** inside
`internal/platform/gateway/`. The assistant module asks for a *role*
(`RolePrimary`, `RoleAttachment`, `RoleTranscribe`); the gateway package resolves
the role to a model name from configuration.

This is the same discipline the capability system already uses, extended to
named models because the assistant genuinely needs three specific ones.

### 3.4 Request flow, end to end

```
1. User types / attaches / speaks
2. Browser: native SpeechRecognition if available          ─┐
   else record → POST /api/v1/assistant/transcribe → text   ┘  (Phase 6)
3. Browser: POST attachments → handles                       (Phase 5)
4. Browser: POST /api/v1/assistant/messages {text, handles}
5. Server: classify each attachment against live capabilities
      image / video  → carry to primary as a content part
      doc / audio    → Voxtral pre-pass → structured text digest
      unsupported    → reject with a clear reason, before any model call
6. Server: assemble [system, …history, user(text + parts + digests)]
7. Server: stream primary model → SSE → browser
8. Server: persist the turn                                  (Phase 7)
```

### 3.5 Boundaries this design holds

| Boundary | How |
|---|---|
| No tools, no agent loop | the assistant module never imports another module and has no function-calling path. `X-Client-App` is set to a Dawa24-specific value, **not** `MuhiyaChat`, so the Gateway's native agent loop (`proxy/agent.go:57`) cannot engage. |
| No database or admin access | the module's only repository is its own conversations/messages tables. |
| No files outside user uploads | attachments are read from the request; nothing touches the filesystem or object storage beyond the assistant's own bucket prefix. |
| Future tools stay cheap to add | §3.6 |

### 3.6 The seam that makes tools addable later without a rewrite

Define, from the start:

```go
// Turn is one exchange. A future tool-calling loop appends ToolCall/ToolResult
// messages to Messages and re-enters the same Stream call; nothing else in the
// pipeline needs to know.
type Turn struct {
    Messages []Message   // typed roles: system | user | assistant | tool
    Tools    []ToolSpec  // ALWAYS EMPTY IN THIS PHASE. The field exists so the
                         // request shape does not change when tools arrive.
}
```

Ship `Tools` as an always-empty slice and assert it is empty in a test. When
tools are wanted, the wire shape, the persistence and the streaming decoder
already accommodate them.

---

## PART 4 — Decisions already made (do not re-litigate)

| # | Decision | Reason |
|---|---|---|
| D1 | Credentials come from admin settings, with env as fallback | the operator screen must actually work; env stays so a fresh deploy boots |
| D2 | The API key is never sent to the browser | the browser calls the Store; the Store calls the Gateway |
| D3 | Model names live only in `internal/platform/gateway/` | rule R2, enforced by `make check-provider-isolation` |
| D4 | Roles, not capabilities, for the assistant's three models | a capability alias cannot express "this exact model for this exact job" |
| D5 | `/v1/capabilities` is read at runtime and cached | the badge set is operator-controlled; hardcoding it guarantees drift |
| D6 | Voxtral output is a **summary**, capped | its context is 32k; a 200-page PDF cannot be pasted whole |
| D7 | SSE, not WebSocket | one-directional, survives proxies, no new infrastructure |
| D8 | `X-Client-App: Dawa24Assistant` | keeps the Gateway's MuhiyaChat agent loop off |
| D9 | Attachments are validated **before** any model call | a 400 costs nothing; a rejected model call costs money and latency |
| D10 | No tools in this phase, but the `Tools` field exists | §3.6 |

---

## PART 5 — Open questions to settle by inspection, not assumption

Answer each in `docs/PLAN_ASSISTANT/DECISIONS.md` before the phase that needs it.

| # | Question | How to answer |
|---|---|---|
| Q1 | Does the Gateway's SSE frame carry a `reasoning` delta separate from `content`? | `grep -n "reasoning" F:/MuhiyaWorkspace/MuhiyaWorkspace/proxy/agent.go` — line 538 mentions one. Decide whether to render or discard it. |
| Q2 | Exact multipart field names for `/v1/audio/transcriptions` | read `serveTranscriptionClient` in `proxy/handler.go` |
| Q3 | `maxTranscriptionRequestBytes` and `maxChatRequestBytes` values | `grep -rn "maxTranscriptionRequestBytes\s*=" F:/MuhiyaWorkspace/MuhiyaWorkspace/proxy/` |
| Q4 | Does the Store's virtual key have access to all three models? | `GET /v1/capabilities` with the configured key; if a model is absent, the operator must enable it |
| Q5 | Is `X-Client-App` gated to an allowlist on the Gateway? | `grep -n "getClientAppName" F:/MuhiyaWorkspace/MuhiyaWorkspace/proxy/agent.go` |
| Q6 | How are `video_url` parts expected to carry data — URL or base64? | `grep -n "video_url" F:/MuhiyaWorkspace/MuhiyaWorkspace/proxy/media.go` |

**If a question cannot be answered from the Gateway source, implement the
OpenAI-standard behaviour, write the assumption into `DECISIONS.md`, and add a
test that fails loudly if the Gateway disagrees.** Never guess silently.

---

Continue to `01_IMPLEMENTATION.md`.
