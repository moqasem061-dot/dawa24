# AI Assistant — Decisions

Record every answer to an open question and every assumption made.

```markdown
## Q<N> — <question>
**Evidence:** <file:line in the Gateway or Store source>
**Answer:**
**If wrong:** <what breaks, and the test that would catch it>
```

---

## Q1 — Does the Gateway SSE carry a separate `reasoning` delta?
**Evidence:** `proxy/translator.go:232` defines `ReasoningContent string `json:"reasoning_content,omitempty"`` in the OpenAI delta chunk; `proxy/translator.go:910` emits `thinking` and `reasoning_content` delta when upstream reasoning is present.
**Answer:** Yes, the Gateway preserves and emits `reasoning_content` (or `thinking`) in SSE delta frames for reasoning models like `qwen3.7-flash` (when thinking is active). The client decoder can read `delta.ReasoningContent` / `delta.Thinking` and map it to `StreamEvent.Reasoning`.
**If wrong:** If no reasoning is emitted, `StreamEvent.Reasoning` remains empty and normal `StreamEvent.Delta` is used without impact.

---

## Q2 — Exact multipart field names for `/v1/audio/transcriptions`
**Evidence:** `proxy/handler.go:2877` reads `r.FormFile("file")`, `r.FormValue("model")` (defaults to `whisper-1`), `r.FormValue("language")` (defaults to `"ar"`), `r.FormValue("response_format")` (pinned to `verbose_json`).
**Answer:** Form file field name is `"file"`, model field is `"model"`, language field is `"language"`.
**If wrong:** Gateway returns 400 `invalid_request_error` ("Failed to get file field"). Tested in transcription unit test.

---

## Q3 — `maxTranscriptionRequestBytes` and `maxChatRequestBytes` values
**Evidence:** `proxy/handler.go:97, 102`: `const maxChatRequestBytes = 24 << 20` (24 MiB), `const maxTranscriptionRequestBytes = 100 << 20` (100 MiB).
**Answer:** Chat payload cap is 24 MiB; audio transcription upload cap is 100 MiB.
**If wrong:** Request returns 413 / `http.MaxBytesReader` error. Client enforces conservative 10 MiB limit per turn locally.

---

## Q4 — Virtual Key model access
**Evidence:** Virtual keys with standard catalog access can access models exposing their capability flags via `GET /v1/capabilities`.
**Answer:** Runtime capability lookup queries `GET /v1/capabilities` and maps `qwen3.7-flash` to `RolePrimary`, `voxtral-small-24b-2507` to `RoleAttachment`, and `whisper-1` to `RoleTranscribe`.
**If wrong:** Fallback to conservative default (text only, 0 MB) on failure without crashing.

---

## Q5 — Is `X-Client-App` gated on an allowlist?
**Evidence:** `proxy/handler.go:336` reads `X-Client-App` string header directly for logging and metrics; `proxy/agent.go:57` activates the agent loop only if `getClientAppName(r) == "MuhiyaChat"`.
**Answer:** `X-Client-App` is not restricted to a closed list. Using `X-Client-App: Dawa24Assistant` correctly identifies Dawa24 Store in request logs while intentionally keeping the Gateway's native `MuhiyaChat` agent loop disabled.
**If wrong:** None.

---

## Q6 — How are `video_url` and multimodal parts expected to carry data?
**Evidence:** `proxy/media.go:19-22, 70-95` supports standard OpenAI parts: `image_url` with `{"url": "..."}`, `video_url` with `{"url": "..."}`, `input_audio` with `{"data": "...", "format": "..."}`, and `file` with `{"url": "...", "name": "...", "mime_type": "..."}` or data URIs.
**Answer:** `image_url` and `video_url` parts accept Data URIs (`data:<mime>;base64,<bytes>`) or HTTP URLs. Document attachments going to Voxtral use standard multipart/content parts.
**If wrong:** Gateway returns 400 `invalid_request_error`. Tested in multimodal serialization test.

---

## Q7 — Should Capsule act, not only read? (reverses `03_READ_ONLY_AGENT.md`)
**Evidence:** Client request, 2026-09-13: the assistant should do what the user can do, on the web and Telegram. The read-only agent could not answer aggregate questions (25 rows per call) or act.
**Answer:** Yes, through confirmed commands only (`05_CAPSULE_V2.md`). The client accepted these recommendations:
1. Model chosen by eval score; per-org spend capped by the gateway virtual-key quota.
2. Actions allowed on Telegram with the same confirmation step.
3. Separate `*.assistant.act` permission, held by default only by organisation owners (not the platform `admin` role).
4. Never through the assistant: passwords, payments and withdrawals, roles, deletion. Wallet checkout is refused.
**If wrong:** Revoke `*.assistant.act` from the owner roles and Capsule is read-only again; `propose_action` is not offered without it.
