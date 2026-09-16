# Module: AI Capabilities

## Overview

The `aicapabilities` bounded context exposes intelligent domain augmentation (catalog product matching, column detection, search query expansion) strictly through the platform gateway (`internal/platform/gateway`).

## Invariants & Guarantees

1. **AI Provider Isolation:** Zero direct AI vendor SDKs or vendor names exist outside the platform gateway boundary.
2. **Deterministic Fallbacks:** When the AI gateway is disabled (`GATEWAY_ENABLED=false`) or encounters network timeouts/failures, the module instantly falls back to deterministic rule-based Arabic string normalization (`arabic.Similarity`) and keyword expansion.
3. **Black-Hole Resiliency:** Tested against unroutable destination IPs (RFC 5737 TEST-NET-1), ensuring zero panics or user-facing errors during upstream AI service interruptions.
