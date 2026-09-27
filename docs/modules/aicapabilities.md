# Module: AI Capabilities

## Overview

The `aicapabilities` bounded context exposes intelligent domain augmentation (catalog product matching, column detection, search query expansion) strictly through the platform gateway (`internal/platform/gateway`).

## Invariants & Guarantees

1. **AI Provider Isolation:** Zero direct AI vendor SDKs or vendor names exist outside the platform gateway boundary.
2. **Deterministic Fallbacks:** When the AI gateway is disabled (`GATEWAY_ENABLED=false`) or encounters network timeouts/failures, the module instantly falls back to deterministic rule-based Arabic string normalization (`arabic.Similarity`) and keyword expansion.
3. **Black-Hole Resiliency:** Tested against unroutable destination IPs (RFC 5737 TEST-NET-1), ensuring zero panics or user-facing errors during upstream AI service interruptions.

## Every row goes to the AI (2026-09-27)

All five matching tools — smart order, vendor import, savings import, the
master-catalogue import and the compare tool — now put every row to the model
whenever retrieval found any catalogue candidate:

- **غير مطابق / يحتاج المراجعة** are resolved. The plausibility floor
  (`matchflow.Ceilings.MinPlausible`, 0.15–0.40) that dropped weak rows is 0 in
  every profile.
- **مطابق** rows matched on a name are checked. The smart order's
  `Matcher.Verify` existed but was never called; it now feeds the AI stage.
  Identifier matches (barcode, SKU, the buyer's own confirmed mapping) are not
  sent — a model cannot improve on them.
- A row with **no** candidate is not sent: the model may only answer with a
  product it was shown.

Safety is unchanged: an answer is applied only above the apply floor, after the
identity screen and the pair check; a check the model will not confirm flags the
row for manual review instead of replacing it. When a file outruns the budget,
the risk order spends it on ambiguous, then unresolved, then settled rows — the
catalogue import's pre-cap now follows the same order.

Also fixed: the catalogue import counted unresolved rows as unmatched twice, and
the admin saving import mis-counted disputed/confirmed rows as new matches.
