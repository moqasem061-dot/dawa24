# Matching Evaluation Test Data

This directory contains synthetic fixtures and adversarial test cases used by offline evaluation harnesses and CI quality gates.

## Contents

- `corpus.lock`: Pinned SHA-256 hashes of the baseline commit, reference catalogue snapshot, tuning cases, and held-out manifest reference.
- `adversarial/`: Synthetic edge cases and adversarial examples verifying strict hard-conflict prevention (SC-002) and prompt injection safety. Contains only synthetic brand names and fictionalized data.

## Note on Real Data

In compliance with repository privacy and security rules:
- **No real pharmacy data, customer data, private pricelists, or actual held-out corpora are committed to this repository.**
- Production exports, de-identified evaluation sets, and held-out test corpora remain outside version control.
