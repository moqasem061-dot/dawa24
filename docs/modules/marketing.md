# Marketing digest (social posts)

`GET /api/v1/integrations/marketing/digest?hours=24` feeds the n8n workflow
**Dawa24 Social Posts (Facebook, Instagram, X)**, which publishes one post to all
three platforms at 13:00 and 21:00 Cairo time.

## Endpoint

- Mounted only when `MARKETING_BRIDGE_TOKEN` is set (32+ chars, `openssl rand -hex 32`).
  It is a separate secret from `TELEGRAM_BRIDGE_TOKEN`.
- `Authorization: Bearer <token>`, compared in constant time; no session, no CSRF.
- Read-only. `hours` is 1–72 (default 24).
- Response: `new_offers` (at most 10), `site_url`, `offers_url`, `image_url`
  (`/static/img/doctor-capsule-social.jpg`, a 1080×1080 JPEG, because Instagram accepts only JPEG).

## Which offers are "new"

`promo/postgres.ListPublishedOffers` applies the buyer offer rule's `live`,
`supplier` and `branch` predicates, and requires at least one product. It then
keeps offers whose publish moment falls inside the window. The publish moment is
the latest of `created_at`, `approved_at` and `starts_at`, so an old draft
approved today counts as new today.

Cities come from the offer's own approved location rules when it has any.
Otherwise they come from the supplier's active weekly coverage for the offer's
branch. At most 12 names are returned; `cities_total` holds the full count.

## Workflow behaviour

1. **Fetch.** Fetch the digest, then load the last 40 rows of the n8n data table
   `dawa24_social_posts`.
2. **Plan** (Code node):
   - If this date and slot already has a `published` or `partial` row, stop.
   - Drop offers that were already announced.
   - Pick a fallback content pillar that hasn't been used recently:
     health tip, medication safety, pharmacy business, platform spotlight,
     supplier growth, or night wellness.
3. **Write.** A GPT copywriter (Basic LLM Chain with a structured output) writes
   Egyptian Arabic copy. It skips offers that look like test data, falls back to
   the pillar, and never invents numbers.
4. **Check.** A deterministic check covers:
   - length, links and `#` inside the text;
   - offer ids must come from the digest;
   - every `%` or `جنيه` figure must appear in the digest;
   - similarity to recent posts;
   - X weighted length.

   A failed post gets one rewrite. If the rewrite also fails, the run fails.
5. **Publish, in parallel:**
   - Facebook: Page `/photos`.
   - Instagram: `/media`, wait 20 seconds, then `/media_publish`.
   - X: `/2/media/upload`, then `/2/tweets`.

   Every node continues on error.
6. **Record.** Save one history row with status `published`, `partial` or
   `failed`. Offers in a `failed` row can be announced again next slot. If every
   platform failed, the run is marked as failed.
