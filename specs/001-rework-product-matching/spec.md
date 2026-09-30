# Feature Specification: Product Matching Rework

**Feature Branch**: Existing `main` checkout; no feature branch created by this specification

**Feature Directory**: `specs/001-rework-product-matching`

**Created**: 2026-09-30

**Status**: Draft — specification validated; ready for technical planning

**Input**: Comprehensively rework product matching across vendor ingest, savings products imports, Smart Order, and all other identity consumers. Substantially improve the deterministic tool first. Rework AI integration and prompt engineering so qualified models deliver measurable additional accuracy and review reduction within short, enforced time and spending limits.

The intended outcome is a dependable product-identity capability throughout Dawa24. Pharmacies and suppliers should obtain useful results quickly, understand unresolved rows, correct them once within the appropriate scope, and continue working when AI is disabled or unavailable. AI earns its place by improving measured outcomes beyond that deterministic foundation.

“Comprehensive” means complete coverage of identity rules, consumers, failure modes, review, operations, and evaluation; it does not require complexity for its own sake. “Any model” means a provider-independent contract with qualification and graceful degradation, not a promise that every model can perform pharmaceutical matching.

This document specifies required behavior. Its numerical targets are proposed acceptance criteria, not measured achievements. Repository findings and their limits are in [repository-context.md](repository-context.md).

## User Scenarios & Testing *(mandatory)*

P1 establishes deterministic correctness, usable review, and operational safety. P2 adds reusable knowledge and qualified AI improvements after that foundation. Both priorities are required for the complete rework; each story has an independently observable outcome even where it uses shared capabilities.

### User Story 1 - Import supplier lists reliably with AI disabled (Priority: P1)

As a supplier, I want known names, Arabic spelling variations, identifiers, and packaging information to resolve accurately so that importing stock and prices does not require re-identifying my catalogue every time.

**Why this priority**: Vendor ingest is frequent and high-volume; a wrong association updates the wrong item's commercial data.

**Independent Test**: Import a labelled supplier file with AI disabled, confirm columns, review results, correct exceptions, and commit. Compare identities, quantities, and prices with approved expectations.

**Acceptance Scenarios**:

1. **Given** Arabic, English, mixed-script, abbreviated, and misspelled known products, **When** matching runs, **Then** sufficiently identified rows resolve consistently and uncertain rows expose useful candidates and reasons.
2. **Given** an internal supplier number coincides with another item's catalogue code, **When** the column has not been declared a catalogue identifier, **Then** the collision cannot independently authorize a match.
3. **Given** rows share a name but state different strengths or packs, **When** repeated work is grouped, **Then** the identity questions remain distinct and each row retains its original stock and price.
4. **Given** a 50,000-row file contains malformed lines, **When** processing completes, **Then** every row has an accounted outcome and malformed lines cannot terminate unrelated processing.
5. **Given** AI is off, unconfigured, or unreachable, **When** import preparation runs, **Then** deterministic matching and manual completion remain available without an AI waiting stage.

### User Story 2 - Keep different medicines and packages distinct (Priority: P1)

As a pharmacy or supplier, I want the exact intended catalogue item identified so that similar names do not silently change what is ordered, stocked, or compared.

**Why this priority**: False acceptance is more damaging than an unresolved row. Identity must survive every acceptance shortcut.

**Independent Test**: Run labelled positive pairs and confusable negatives through each workflow using identifiers, saved mappings, exact names, fuzzy names, and AI proposals separately.

**Acceptance Scenarios**:

1. **Given** similar candidates at different strengths, **When** a row states one strength, **Then** a conflicting strength is rejected regardless of similarity or claimed confidence.
2. **Given** explicit tablet/capsule, ordinary/modified-release, or route differences, **When** the source identifies one, **Then** the other is not accepted as the same sellable item.
3. **Given** a product has packs of 10 and 30, **When** the row names 30, **Then** a product-family match cannot authorize ordering or pricing the pack of 10.
4. **Given** several strengths or extensions remain possible because the source omits the distinction, **When** matching runs, **Then** it reports ambiguity and the missing information.
5. **Given** different identifiable brands share an ingredient or manufacturer, **When** the requested brand is known, **Then** the other brand is never silently substituted.
6. **Given** a unique identifier contradicts an explicit name or strength, **When** acceptance is considered, **Then** an identifier-versus-description conflict is shown for correction.

### User Story 3 - Import savings lists consistently (Priority: P1)

As a customer, vendor, or authorized administrator, I want savings imports to preserve identity and commercial inputs across every entry point so that my saved list represents what I uploaded.

**Why this priority**: Savings has multiple upload and staging paths that must not drift.

**Independent Test**: Import equivalent labelled lists through customer, vendor, and administrator-assisted savings flows; compare decisions and effective scope, then verify committed entries.

**Acceptance Scenarios**:

1. **Given** equal evidence, visibility, identifier declarations, and policy, **When** any savings path runs, **Then** identity outcomes and review reasons agree.
2. **Given** only an internal code column is mapped, **When** preparation runs, **Then** that value is not also silently treated as a barcode.
3. **Given** a matched row updates an existing savings entry, **When** review is displayed, **Then** product, pack/unit, quantity, and monetary changes are visible before commit.
4. **Given** unresolved or excluded rows, **When** the workflow permits committing other rows, **Then** those exceptions remain accounted for and create no invented association.
5. **Given** an administrator imports for a target organization, **When** a correction is remembered, **Then** its scope is the explicit target context, not the administrator's unrelated organization.

### User Story 4 - Build Smart Orders from the intended products (Priority: P1)

As a pharmacy buyer, I want identity resolved independently of suppliers so that availability or lower prices cannot make the system choose a different product.

**Why this priority**: Matching errors cannot be repaired by supplier ranking.

**Independent Test**: Prepare a labelled order containing available, unavailable, ambiguous, and differently packaged items with AI disabled; inspect review and finalize eligible lines through the normal order flow.

**Acceptance Scenarios**:

1. **Given** an identified product has no eligible offer, **When** supplier selection runs, **Then** it remains identified but unavailable rather than changing to an in-stock sibling.
2. **Given** requested pack and offered selling unit differ, **When** the offer is considered, **Then** only an explicit validated unit relationship permits conversion; otherwise review is required.
3. **Given** ambiguous identity, **When** order review opens, **Then** no suggestion becomes a confirmed order item until ambiguity is resolved.
4. **Given** a buyer edits a line while AI runs, **When** a late response arrives, **Then** it cannot overwrite the edit or create an order effect.
5. **Given** stock, eligibility, permissions, or identity changes after review, **When** finalization runs, **Then** affected lines are revalidated under established order rules.

### User Story 5 - Protect the master catalogue during imports (Priority: P1)

As a catalogue administrator, I want strict separation between updating an existing product and proposing a new one so that similarity mistakes cannot overwrite canonical data.

**Why this priority**: Master-data errors propagate to all customers and vendors.

**Independent Test**: Stage known items, new items, duplicate identities, conflicting identifiers, and identifier-only rows; inspect every proposed mutation before explicit commit.

**Acceptance Scenarios**:

1. **Given** a near-identical sibling and an omitted distinguishing attribute, **When** an update is proposed, **Then** the system requests review instead of choosing a record to overwrite.
2. **Given** no eligible existing target is found, **When** staging completes, **Then** the row is a possible new item, not proof that no equivalent exists anywhere.
3. **Given** an identifier-only row the current import policy preserves, **When** a placeholder name is generated, **Then** it contributes no fabricated identity evidence.
4. **Given** mapping or policy changes and prepare is rerun, **When** new results are saved, **Then** staging reflects the new revision and the catalogue remains unchanged until commit.
5. **Given** canonical identity fields would change, **When** the administrator reviews the proposal, **Then** old/new values and affected associations are visible; AI cannot silently approve the change.

### User Story 6 - Compare equivalent products and selling units (Priority: P1)

As a customer or supplier, I want comparisons to distinguish exact item equality from broad resemblance so that reported savings are meaningful.

**Why this priority**: Grouping can introduce false associations even when import matching is correct.

**Independent Test**: Compare two and then multiple labelled files containing exact equivalents, similar brands, different packs, and products absent from the master catalogue.

**Acceptance Scenarios**:

1. **Given** rows identify the same product and sellable pack, **When** comparison runs, **Then** grouping has a visible identity basis.
2. **Given** rows share only a prefix or manufacturer, **When** grouping runs, **Then** distinct products remain separate even if reported savings decrease.
3. **Given** one file prices a strip and another a box, **When** results appear, **Then** an explicit valid conversion is used or prices are marked not directly comparable.
4. **Given** two source files identify the same item absent from the master catalogue, **When** grouped, **Then** source-to-source equivalence is labelled and creates no canonical link.
5. **Given** a previously linked row becomes unresolved on rerun, **When** results are saved, **Then** the old link is cleared or superseded and cannot survive as a stale match.

### User Story 7 - Review uncertainty efficiently in both languages (Priority: P1)

As a reviewer, I want to see why a row needs attention, what differentiates candidates, and what my action affects so that exceptions can be resolved quickly and correctly.

**Why this priority**: Abstention is useful only when a human can complete the task.

**Independent Test**: Resolve a mixed review queue in Arabic RTL and English, using keyboard navigation, narrow-screen display, candidate search, and bulk actions.

**Acceptance Scenarios**:

1. **Given** candidates differ in strength, form, or pack, **When** compared, **Then** differences appear beside original source values.
2. **Given** the correct item is outside the shortlist, **When** the reviewer searches the authorized catalogue, **Then** it can be selected without re-uploading.
3. **Given** a bulk correction, **When** previewed, **Then** affected rows are named and rows with different identity evidence are excluded.
4. **Given** a hard contradiction, **When** confirmation is attempted, **Then** ordinary confirmation cannot bypass it; an explicit source/catalogue fact correction must first be recorded.
5. **Given** background results change totals, **When** review refreshes, **Then** filters, focus, row position, selections, and counts remain consistent with saved state.

### User Story 8 - Reuse corrections safely (Priority: P2)

As an organization operator, I want remembered confirmations and rejections to improve repeat files without leaking private conventions or applying obsolete answers.

**Why this priority**: Repeat-file learning can reduce review work without new AI requests.

**Independent Test**: Correct a row, repeat it in another applicable workflow and another organization, change the target identity, revoke the memory, and repeat again.

**Acceptance Scenarios**:

1. **Given** applicable human-confirmed memory, **When** the question recurs in scope, **Then** it is reused without remote AI and labelled as human-confirmed knowledge.
2. **Given** another organization uses the same code for another item, **When** its file runs, **Then** the first organization's memory has no effect.
3. **Given** authorized curation publishes catalogue knowledge, **When** an organization permits inheritance, **Then** it may reuse the fact without seeing the originating tenant's private data.
4. **Given** source facts, target identity/pack, policy, or visibility changed, **When** a past answer is considered, **Then** it is revalidated or marked stale.
5. **Given** an association is rejected or revoked, **When** the question recurs, **Then** old automated answers cannot resurrect it.

### User Story 9 - Get useful AI assistance without blocking work (Priority: P2)

As an importer or buyer, I want optional AI to address worthwhile unresolved questions while deterministic results are already usable.

**Why this priority**: The rework must improve both time spent and actual benefit.

**Independent Test**: Enable AI on labelled settled, ambiguous, resolvable, and unresolvable rows; simulate fast, slow, unavailable, and budget-exhausted service while reviewing results.

**Acceptance Scenarios**:

1. **Given** deterministic preparation is complete but AI is pending, **When** review opens, **Then** saved results are usable and the user can review, stop enhancement, or proceed with eligible rows.
2. **Given** AI provides a valid selection supported by row-specific evidence, **When** acceptance checks pass, **Then** the result records both its deterministic baseline and AI contribution.
3. **Given** an adequate deterministic answer, **When** ordinary enhancement runs, **Then** it does not routinely spend calls reconfirming that answer.
4. **Given** abstention, malformed output, timeout, or service failure, **When** enhancement ends, **Then** the deterministic outcome remains usable and the actual reason is reported.
5. **Given** the user edits or commits a revision, **When** a late AI answer arrives, **Then** it cannot change saved human decisions or committed business records.
6. **Given** no accepted new resolutions, **When** the summary appears, **Then** it reports zero improvement rather than counting confirmations, changed scores, or requests.

### User Story 10 - Use different qualified models consistently (Priority: P2)

As an operator, I want underlying AI configurations to be replaceable without rewriting matching rules or weakening acceptance gates.

**Why this priority**: Model independence must preserve usefulness, safety, and observability.

**Independent Test**: Evaluate at least two configurations, including one without native constrained-output support, plus an incompatible configuration against the same blind cases.

**Acceptance Scenarios**:

1. **Given** different response-format capabilities, **When** configurations are qualified, **Then** they must meet the same applicable identity, output-validation, accuracy, abstention, and deadline rules.
2. **Given** plausible wrong answers or consistently slow responses, **When** qualification runs, **Then** that configuration remains disabled for automatic application.
3. **Given** a model change, **When** cached answers are considered, **Then** reuse follows a recorded compatibility decision rather than inherited confidence.
4. **Given** an unsupported language or request size, **When** a configuration is selected, **Then** assistance is narrowed or disabled with a reason while deterministic work continues.

### User Story 11 - Treat prompts as an evaluated product component (Priority: P2)

As the matching owner, I want concise, versioned instructions, explicit abstention, and representative difficult examples so that prompt changes are evaluated rather than judged by impressive wording.

**Why this priority**: Prompt instructions alone cannot prove correctness; conflicting or oversized instructions waste time.

**Independent Test**: Compare prompt candidates on an isolated tuning set and evaluate the selected version on locked Arabic shorthand, incomplete identity, confusable variants, hostile text, and absent-candidate cases.

**Acceptance Scenarios**:

1. **Given** the true product is absent, **When** queried, **Then** the contract permits abstention instead of requiring the best available guess.
2. **Given** source text contains instructions to ignore matching rules, **When** submitted, **Then** it remains untrusted data and cannot authorize actions.
3. **Given** a prompt increases answers but decreases precision, **When** evaluated, **Then** it fails promotion.
4. **Given** a model confirms its earlier suggestion, **When** quality is assessed, **Then** self-agreement is not independent ground truth.
5. **Given** the prompt omits evidence needed to distinguish packs, **When** evaluated, **Then** it cannot qualify for automatic pack-level decisions.

### User Story 12 - Resume work and understand failures (Priority: P1)

As an operator, I want progress, retries, cancellations, and diagnostics to describe saved work accurately so that interruption does not create duplicate writes or indefinite waiting.

**Why this priority**: Recovery is normal for large imports and external-service outages.

**Independent Test**: Interrupt preparation/enhancement, reopen from another session, exhaust AI budgets, and rerun failed work; reconcile rows, decisions, and committed effects.

**Acceptance Scenarios**:

1. **Given** processing stops unexpectedly, **When** reopened, **Then** the run shows saved progress and a bounded recovery action, not an indefinitely live progress bar.
2. **Given** an exhausted AI budget, **When** enhancement ends, **Then** unattempted rows, abstentions, rejected answers, and accepted results are counted separately.
3. **Given** partial success followed by retry, **When** work resumes, **Then** valid answers remain and repeated responses do not duplicate effects.
4. **Given** an authorized support investigation, **When** a decision is inspected, **Then** evidence, versions, reasons, timings, and scope are available without exposing unrelated tenant data or credentials.

### User Story 13 - Prove improvement before broad rollout (Priority: P1)

As the product owner, I want reproducible comparison with the current system so that release depends on accuracy, speed, usability, and isolation results.

**Why this priority**: More matches or more complexity is not proof of improvement.

**Independent Test**: Run frozen evaluation and load profiles against the current system, new deterministic matching, and each proposed AI configuration; determine release eligibility from measured gates.

**Acceptance Scenarios**:

1. **Given** held-out data never used for tuning, **When** scored, **Then** precision, coverage, retrieval failures, review burden, and wrong associations are reported separately by workflow and language.
2. **Given** a hard-identity violation despite high aggregate accuracy, **When** release is assessed, **Then** it fails the applicable gate.
3. **Given** deterministic matching qualifies but AI does not, **When** rollout proceeds, **Then** the deterministic improvement may ship with failing AI disabled and its remaining work explicitly incomplete.
4. **Given** a limited rollout reveals regression, **When** rolled back, **Then** source rows, review history, and committed records are preserved.
5. **Given** one workflow lacks sufficient evidence, **When** overall completion is assessed, **Then** success elsewhere cannot mark the whole rework complete.

### Edge Cases

These rules apply to reruns, saved knowledge, manual choices, and automated proposals as well as first-time matching. Product/brand examples are synthetic identity fixtures, not clinical advice.

| Case | Required behavior |
|---|---|
| Arabic alef/hamza, ya/alef maqsura, tatweel, diacritics, alternate spaces | Improve retrieval without modifying retained source text or collapsing distinct identities. |
| Arabic-Indic, Eastern Arabic-Indic, and Latin digits | Interpret numeric evidence consistently; preserve identifier leading zeroes. |
| Conflicting Arabic and English names | Flag contradictory evidence rather than taking the higher-scoring language. |
| Invisible/bidirectional characters, joined words, repeated bilingual names | Safe readable display and repeatable interpretation without hidden numeric changes. |
| Transliteration, abbreviation, OCR-like spelling noise | Retrieve plausible products without treating a shared prefix as identity proof; OCR extraction is outside scope. |
| `500 mg` and `0.5 g` | Equivalent mass can corroborate identity; other attributes still apply. |
| `250 mg/5 mL` and `50 mg/mL` | Recognize equivalent concentration; check total volume and pack separately. |
| `5/10 mg` versus `10/5 mg` combinations | Never sort numbers into equality; ingredient-to-strength association must be known. |
| Only one combination strength is supplied | Missing data is not agreement; require independent evidence that uniquely identifies the full item. |
| Percent, activity units, unclear decimal separator | No invented conversion or rounding; uncertainty requires review. |
| Price annotation beside dose/pack | Keep price, strength, total content, pack count, and requested quantity distinct. |
| `2 × 10` and 20 units | Equal counts do not prove identical selling packs; require an approved packaging relationship. |
| Chained/multiple multiplication or huge numeric values | Bound interpretation, isolate invalid rows, and avoid overflow or process termination. |
| Explicit tablet/capsule, cream/gel, ordinary/dissolving tablet | Distinguish forms; only explicit scoped corrections may repair known mislabelling. |
| Night/day, plus/extra, release markers, pediatric/adult, device size | Preserve identity distinctions; do not discard as general noise. |
| “Product A, alternative to Product B” | Do not interpret Product B's mention as requested substitution. |
| Identifier names A while description names B | Surface the contradiction; identifiers and AI cannot bypass it. |
| Duplicate/damaged/truncated/scientific-notation identifier | Do not infer lost digits or choose the first hit. |
| Same supplier code in multiple namespaces | Resolve only in the declared applicable source scope. |
| Duplicate catalogue records with identical normalized names | Preserve ambiguity unless trustworthy evidence selects one. |
| Several supplier offers for one canonical item | Keep identity separate from offer, price, stock, and package eligibility. |
| Correct target outside shortlist/loading cap | Broader authorized recovery and visible retrieval incompleteness; no silent catalogue truncation. |
| No match versus unavailable catalogue lookup | Distinct outcomes and recovery actions. |
| Empty catalogue | Preserve allowed manual/new-item paths; no invented match or repeated empty AI request. |
| Row changes while AI/another reviewer works | Older results cannot overwrite newer revisions. |
| Missing/unknown/duplicate/conflicting AI references | Preserve independently valid answers; reject ambiguous row correspondence. |
| AI chooses a neighboring row's candidate | Check this row's eligibility and identity; batch membership alone is insufficient. |
| Authorization/quota/configuration failure | Stop futile remote attempts with an accurate reason. |
| Deleted/merged/inactive/inaccessible target | Revalidate memory and staged choices; no silent unrelated redirect. |
| Identity facts change under the same product ID | Invalidate affected answers despite unchanged identifiers. |
| Prompt-like product or catalogue content | Inert evidence, no actions or permission changes. |
| Manual rejection then renewed AI suggestion | Suppress automatic resurrection within the rejection's scope. |
| Header, totals, repeated page banners, blanks | Account for exclusions; do not count them as successfully matched products. |
| Repeated commit/cancel/refresh/retry | At most one intended business effect per row/revision. |

## Requirements *(mandatory)*

### Scope and product boundaries

The five primary workflows and every entry point assigning or grouping product identity within them are in scope. Using a shared text utility alone does not make an unrelated feature a matching consumer.

| Workflow | Entry points to cover | Matching responsibility | Owning workflow retains |
|---|---|---|---|
| Vendor ingest | Supplier wizard, administrator-assisted organization import, staging/rerun, supported background execution | Canonical product, required sellable variant, or explicit unresolved state | Stock/price mutations, upload authorization, commit policy |
| Savings | Customer, vendor, administrator-assisted, direct upload, staged wizard, supported delegates | Product/pack association and review state | Ownership, duplicate policy, quantity/money updates |
| Smart Order | New run, resume, reprocess, manual rematch, foreground/background execution | Requested identity and required unit/pack | Supplier eligibility, stock, coverage, ranking, checkout |
| Master catalogue | Preview/prepare, rerun, review, commit validation | Existing-item proposal or possible-new-item state | Authorized canonical edits, creation and update commit |
| Comparison | Upload/remap/rerun, row matching, multi-supplier and head-to-head grouping, corrections | Canonical identity or qualified source-to-source equivalence | Entitlements, financial calculation, results presentation |

Out of scope: replacing the whole catalogue/search product, unrelated screen redesigns, clinical substitution, prescribing, rebuilding upload/OCR extraction, changing procurement optimization, adding provider dependencies to business modules, training a general-purpose model, or automatically merging canonical records. Targeted repairs to identity evidence supplied by mapping/parsing are in scope. Existing image/attachment association remains intact; fuzzy image-to-product matching is not added.

### Decision vocabulary and identity policy

Each row has one primary decision state. Provenance, AI progress, catalogue warnings, and later business actions are separate attributes.

| State | Meaning | Consequence |
|---|---|---|
| Accepted | Meets the qualified automatic policy at required identity granularity. | Eligible for owning workflow review/commit; no immediate business mutation. |
| Confirmed by reviewer | Authorized selection after resolving hard contradictions. | Same downstream revalidation; actor and corrections recorded. |
| Needs review | Plausible candidates without sufficient acceptance evidence. | Suggestions only; no automatic business association. |
| Ambiguous | Multiple eligible identities remain indistinguishable. | Ask for distinguishing evidence or explicit selection. |
| No eligible candidate found | No candidate passed retrieval and identity eligibility. | Broader search, source correction, exclusion, or permitted new-item proposal. |
| Invalid input | Insufficiently interpretable source row. | Repair or exclude with reason. |
| Excluded | Explicit rule/user action removes the row from matching. | Preserve reason and source location. |
| Temporarily unavailable | Required deterministic data/processing unavailable. | Retry/recovery; not evidence of product nonexistence. |

A decision may identify a canonical product while still requiring a sellable pack before downstream action. “Accepted” is evaluated at the granularity required by that action. AI state is separate: off, ineligible, queued, running, completed, stopped, failed, or obsolete. AI completion does not mean identity improvement.

Identity facts are **agreeing**, **conflicting**, **unstated**, or **uncertain**. Unstated is not automatically agreeing. A **hard conflict** is an explicit identity contradiction blocking acceptance until corrected. Retrieval hints help find candidates but do not necessarily authorize a match.

### Functional Requirements

#### A. Consistent behavior across consumers

- **FR-001**: The system MUST inventory every identity-assignment and comparison-grouping entry point in scope, including alternate uploads, remapping, reruns, saved decisions, and background execution. Acceptance evidence MUST map every entry point to a tested workflow.
- **FR-002**: Equal source facts, effective organization/source scope, catalogue visibility and revision, identifier declarations, and policy version MUST produce the same deterministic identity decision and reasons across consumers. Different business eligibility remains separate and explainable.
- **FR-003**: Profiles MAY differ in required granularity, review strictness, and budgets but MUST NOT weaken hard identity constraints. Additional restrictions MUST be recorded in the run policy.
- **FR-004**: Every workflow MUST be completable through deterministic matching and manual review with AI off, issuing no remote AI requests and applying no cached AI-only judgements disguised as deterministic decisions.
- **FR-005**: Parsing, candidate retrieval, identity acceptance, AI assistance, human review, and business commit MUST have distinguishable outcomes. Failure in one stage MUST NOT be relabelled as success or product nonexistence in another.
- **FR-006**: Record identifiers, imported-row provenance, order-number conventions, ownership, entitlements, and permissions MUST remain compatible. Implementation MUST document changed matching behavior in affected module documentation.

#### B. Source evidence and Arabic-first interpretation

- **FR-007**: Rows MUST retain original source values/location, confirmed column bindings, declared identifier namespace, and explicit corrections. Normalized/inferred facts MUST remain distinguishable from original and corrected facts.
- **FR-008**: Arabic, English, mixed scripts, equivalent digits, common Egyptian pharmacy abbreviations, and curated transliterations MUST participate in retrieval. Normalization MUST be repeatable and preserve distinctions necessary for identity.
- **FR-009**: When present, the system MUST distinguish product/brand name, ingredients, strength and measurement basis, form, route, release/extension, total content, pack structure, selling unit, manufacturer, source code, barcode, and requested quantity. Uncertain extraction MUST be exposed instead of fabricated.
- **FR-010**: Money, commercial notes, requested quantity, strength, volume, and pack count MUST remain separate evidence classes. A price change alone MUST NOT change identity; approximate monetary comparison MUST NOT justify acceptance.
- **FR-011**: Comparisons MUST preserve exact numeric meaning, combination component associations, concentration denominators, and total volume. Only supported dimensional conversions may establish equality; rounding, sorting component numbers, or assuming missing denominators MUST NOT establish identity.
- **FR-012**: Potential identity qualifiers, including pediatric/adult, release, combination, form, device size, and pack details, MUST NOT be discarded as generic noise. Curated exceptions require bounded scope and labelled positive/negative examples.
- **FR-013**: Conflicting explicit source fields or languages MUST produce visible conflict. An inferred hint MUST NOT silently override explicit evidence, and selecting whichever field scores highest MUST NOT resolve a contradiction.
- **FR-014**: Blank, header, total, malformed, over-limit, and excluded rows MUST receive accounted outcomes. One defective row MUST NOT terminate a worker or discard unrelated results.

#### C. Identifier trust and identity checks

- **FR-015**: Identifier matching MUST require the relevant column to be mapped and its use explicitly selected or inherited from a previously confirmed applicable mapping profile. Barcode, supplier code, and canonical code MUST remain distinct namespaces.
- **FR-016**: Identifier validation MUST preserve original digits/leading zeroes, enforce the declared format, and detect incomplete or ambiguous values. Barcodes MUST satisfy the declared scheme's validity rules; an unknown scheme MUST NOT be guessed into an authoritative identifier.
- **FR-017**: Supplier codes MUST resolve within their declared source namespace. A collision with a canonical code MUST NOT independently authorize acceptance without the correct explicit namespace declaration and its continuing applicability.
- **FR-018**: Duplicate identifiers, conflicting identifiers, and identifier-versus-description contradictions MUST enter review. Identifier shortcuts MUST NOT bypass hard conflicts, visibility, target existence, or required selling-pack checks.
- **FR-019**: Every acceptance path, including exact names, identifiers, saved mappings, manual choices, and AI, MUST enforce applicable identity constraints before committing any downstream association.
- **FR-020**: Different identifiable brands, ingredient combinations, strengths, routes, explicit forms, release formulations, or distinguishing extensions MUST NOT become the same item because of similarity, manufacturer, therapeutic category, price, or availability.
- **FR-021**: Tablet/capsule wording and explicit packaging differences MUST NOT have universal equivalence. A documented source correction or catalogue-approved relationship may resolve known shorthand only within its evidence and scope, without erasing a real distinction.
- **FR-022**: Canonical-product, sellable-pack, vendor-offer, and source-to-source comparison identities MUST be distinguishable. Required pack count, volume, unit, or dimensions MUST be resolved before dependent business actions.
- **FR-023**: Missing attributes MUST remain missing. Acceptance with omitted attributes is allowed only when independent evidence uniquely identifies the required item and the qualified policy permits it; unresolved siblings MUST remain ambiguous.
- **FR-024**: Manufacturer, shared ingredient, common prefix, price proximity, popularity, and stock MAY support retrieval or later business selection but none alone may establish identity or justify substitution.

#### D. Deterministic retrieval, ranking, and abstention

- **FR-025**: Retrieval MUST consider authorized identifiers, applicable human mappings, exact normalized names, curated aliases, transliteration, token/spelling similarity, and structured identity evidence where available. Missing optional metadata MUST NOT prevent otherwise valid retrieval.
- **FR-026**: Finding candidates and accepting them MUST be evaluated separately. Diagnostics MUST distinguish unavailable catalogue data, no retrieved candidate, conflicting candidates, insufficient evidence, and ambiguity.
- **FR-027**: Retrieval MUST cover the supported searchable catalogue without a silent first-N-products limit. Incomplete retrieval caused by an operational limit MUST be visible and cannot prove absence of a match.
- **FR-028**: Matching MUST use a declared catalogue revision or equivalent consistent evidence snapshot. Freshness failures MUST be detected and use a usable authoritative fallback when available; otherwise report temporary unavailability.
- **FR-029**: Ranking MUST reward distinguishing evidence while retaining hard conflicts as vetoes. Price or manufacturer agreement MUST NOT overpower conflicting strength, form, combination, or pack facts.
- **FR-030**: Equal inputs under one policy MUST produce stable candidate ordering and decisions across reruns, source row order, and execution concurrency. A tie-break may order suggestions but MUST NOT make indistinguishable products an automatic winner.
- **FR-031**: Acceptance thresholds MUST be qualified against labelled outcomes, separately by profile when needed. Ranking scores and model confidence MUST NOT be presented as calibrated correctness probabilities without supporting evidence.
- **FR-032**: Review MUST show up to five distinct useful candidates with identifying differences and access to wider authorized search. Count, rank, and score alone MUST NOT imply acceptance.
- **FR-033**: Empty candidate sets, absent true targets, duplicate catalogue identities, and under-specified source rows MUST be valid outcomes with manual recovery. Coverage targets MUST NOT force guesses.
- **FR-034**: Repeated computation MAY be shared only for equivalent identity questions, including decisive structured facts, scope, and policy. Each source row MUST retain its own commercial values, provenance, decision, and commit outcome.

#### E. Consumer completion rules

- **FR-035**: Vendor ingest MUST stage canonical identity and required sellable details beside original stock/money values. Identity changes MUST invalidate stale downstream previews before commit.
- **FR-036**: Every savings entry point MUST honor the same identifier declarations, identity outcomes, AI eligibility, and memory scope. Existing-entry updates MUST show product, pack/unit, quantity, and monetary consequences before commit.
- **FR-037**: Smart Order MUST preserve requested identity independently of suppliers. “Identified but no eligible offer” MUST differ from “unmatched”; availability or price MUST NOT silently choose another item.
- **FR-038**: Pack/unit conversions MUST use explicit validated relationships and exact quantity/money calculations. Missing conversion evidence MUST block the affected business action for review.
- **FR-039**: Master-catalogue imports MUST distinguish updates, possible new products, duplicate identities, and unresolved rows. No proposed match may change canonical identity or create/merge records without the owning workflow's explicit authorized commit.
- **FR-040**: Head-to-head and multi-supplier grouping MUST enforce identity/pack checks even without canonical IDs. Comparison-only equivalence MUST NOT create a master catalogue association.
- **FR-041**: Rerunning a row MUST replace or explicitly supersede the old decision, including clearing a formerly accepted link. Consumers MUST NOT retain a stale product ID when the latest outcome is unresolved or invalid.
- **FR-042**: Administrator-assisted actions MUST resolve the effective target organization and source context explicitly. The actor's unrelated organization MUST NOT determine attribution, memory, or visibility.

#### F. Review and bilingual presentation

- **FR-043**: Review MUST show original/corrected facts, selected or suggested identity, critical differences, state, evidence source, and concise reasons. Suggestions MUST be distinct from committed associations.
- **FR-044**: Review MUST support filtering by state, conflict/missing attribute, decision source, AI outcome, and unresolved pack/unit; searching source text and candidates; and returning to source rows without re-uploading.
- **FR-045**: Authorized reviewers MUST be able to select/search eligible candidates, correct source facts, reject suggestions, exclude rows, and request permitted catalogue maintenance. Ordinary confirmation MUST NOT override an unresolved hard conflict.
- **FR-046**: Bulk actions MUST preview their scope and affect only rows equivalent for the action. A choice MUST NOT propagate across different strengths, packs, namespaces, or explicit rejections.
- **FR-047**: Corrections/bulk actions MUST be reversible before commit and record actor, time, before/after decision, reason, and scope. After commit, changes use the owning workflow's authorized correction process.
- **FR-048**: Human edits and explicit rejections MUST take precedence over pending automation for that revision. Concurrent reviewers MUST receive a conflict notice rather than silently overwrite each other.
- **FR-049**: All added labels, reasons, progress, errors, and actions MUST be bilingual Arabic/English. RTL, mixed-direction identifiers, keyboard navigation, screen-reader announcements, and narrow-screen review MUST be usable.
- **FR-050**: Refresh/resume MUST preserve saved choices. Background changes MUST preserve usable focus, filters, and row context; counts MUST reflect saved rows rather than unsaved optimistic updates.

#### G. Memory and curated knowledge

- **FR-051**: Memory MUST distinguish human confirmation, human rejection, curated alias/relationship, inherited platform knowledge, and automated AI answer. AI output MUST NOT be labelled human or independently verified knowledge.
- **FR-052**: Decisions MUST retain effective organization/source scope, decisive row facts, target identity, relevant versions, provenance, and validity. A normalized-name key alone is insufficient when other facts differ.
- **FR-053**: In compatible scopes, explicit rejection/newer human correction MUST outrank older automation; organization decisions MUST outrank platform suggestions. Precedence MUST NOT bypass hard conflicts or authorization.
- **FR-054**: Private memory MUST NOT influence another organization. Platform publication requires authorized curation excluding private files, supplier codes, negotiated prices, buyer quantities, and tenant-specific context; organizations retain their inheritance preference.
- **FR-055**: Reuse MUST be invalidated or revalidated after changes to decisive source facts, target identity/pack, eligibility, visibility, normalization, retrieval, acceptance policy, prompt contract, or qualified AI configuration. Unchanged candidate IDs alone MUST NOT prove compatibility.
- **FR-056**: Negative answers MUST be scoped to their evidence/catalogue state. Added or corrected products MUST allow reconsideration; cached “none” MUST NOT permanently prevent retrieval.
- **FR-057**: AI-only cached answers require enabled applicable AI policy and current validation. Applicable human/curated knowledge remains usable with AI off. Cross-workflow reuse requires compatible scope, evidence, granularity, and policy.
- **FR-058**: Authorized users MUST inspect, revoke, correct, and where permitted promote memory. Revocation MUST affect the next matching action without rewriting historical committed records or audit evidence.

#### H. AI usefulness and acceptance

- **FR-059**: AI MUST be optional, entitlement-aware, attributed to the effective organization, and visibly available only under a qualified configuration. New runs default off until an applicable policy is deliberately enabled; saved choices remain visible and revalidated.
- **FR-060**: Ordinary AI work MUST prioritize unresolved questions plausibly resolvable from available evidence. Invalid rows, empty eligible candidate sets, unresolved hard conflicts, authoritative settled identities, and protected human decisions MUST NOT consume ordinary enhancement calls.
- **FR-061**: Accepted name matches MAY be sampled in separately labelled quality-audit mode with its own budget. Routine enhancement MUST NOT send every accepted name row for reconfirmation. Audit concerns create explicit review items rather than silently replacing accepted identities.
- **FR-062**: AI MUST receive sufficient row-specific evidence, including required form, route, strength basis, and pack distinctions. It MUST choose an existing candidate eligible for that row or abstain; shared batch membership alone is not authorization.
- **FR-063**: Every proposal MUST pass current scope, existence, candidate eligibility, identity, pack, and qualified acceptance checks. Confidence, eloquent reasons, or agreement between model calls MUST NOT override a failed check.
- **FR-064**: The deterministic baseline MUST be retained separately from AI proposals, new accepted resolutions, confirmations, review flags, rejected proposals, abstentions, and failures. An applied change is not independently verified correctness.
- **FR-065**: AI disagreement MUST NOT silently switch an accepted deterministic or human target. Human choices remain protected; substantive new contradictory evidence may create an explicit review concern.
- **FR-066**: Unavailability, deadline exhaustion, malformed output, and abstention MUST preserve usable deterministic results. Technical failure alone MUST NOT demote a valid match or block its normal review/commit path.
- **FR-067**: AI value MUST be evaluated against the new deterministic engine as well as the old baseline. Failing quality, latency, or cost gates MUST prevent automatic application for the affected configuration/workflow.
- **FR-068**: Qualification MAY restrict assistance to supported language, row-type, or workflow cohorts. Unsupported cohorts remain deterministic/manual and are reported; limited qualification MUST NOT be presented as full completion.

#### I. Prompt and model contract

- **FR-069**: The maintained system prompt MUST define matching as identifying the same requested item at required granularity; state hard exclusions and missing-evidence behavior; forbid therapeutic substitution/invention; permit abstention; and agree with deterministic policy.
- **FR-070**: Prompt rules MUST be separated from untrusted source/catalogue content. Uploaded instructions, role labels, links, and product descriptions MUST NOT change the task, request secrets, trigger actions, or widen visibility.
- **FR-071**: Prompts MUST distinguish original facts, explicit corrections, and uncertain parser hints. They MUST NOT teach that pack size is always irrelevant, that tablet/capsule are universally equal, or that a partial combination proves complete equality.
- **FR-072**: The prompt package MUST include concise maintained rules, a documented example-selection policy, and positive, negative, ambiguous, and no-candidate examples. Coverage includes Arabic shorthand, cross-script names, strength/form/pack distinctions, manufacturer/prefix traps, and uncertainty, without using held-out answers.
- **FR-073**: The answer contract MUST identify each row, express selection/abstention, and supply compact structured evidence/status for relevant attributes and a bounded reason category. Hidden reasoning or long explanations MUST NOT be required for acceptance or diagnosis.
- **FR-074**: Returned references, IDs, states, evidence values, and supplied confidence ranges MUST be validated. Unknown references/invented targets are rejected; contradictory duplicate answers invalidate that reference instead of accepting the first arrival.
- **FR-075**: Partial output MUST preserve independently valid row answers. Missing/invalid answers retain deterministic results and accurate counts; bounded retry may address only unresolved output within remaining budgets.
- **FR-076**: Compatibility MUST depend on observable language/input limits, response contract, output allowance, deadline behavior, and measured qualification. Native constrained-output support is optional; application-side validation is mandatory for every model.
- **FR-077**: Model-specific accommodations MUST stay behind the existing AI capability boundary. Business matching MUST NOT depend on provider names, provider confidence scales, or independently drifting provider-specific policy/prompt forks.
- **FR-078**: Request sizing MUST honor qualified input/output limits, multilingual text, and any non-answer generation allowance. A universal bytes-to-token estimate or universally supported instruction to disable reasoning MUST NOT be assumed.
- **FR-079**: An additional verification call is allowed only when evaluation demonstrates error reduction worth its cost/latency. Correlated answers, self-confirmation, or two model agreements MUST NOT count as independent correctness evidence.
- **FR-080**: Prompt, examples, model configuration, answer contract, and policy revisions MUST be versioned, evaluated, and reversible. Prompt changes require measured comparison; length and stylistic sophistication are not acceptance criteria.

#### J. Latency, spending, and cancellation

- **FR-081**: Deterministic results MUST be saved and usable before optional AI finishes. Revision-safe additions may arrive progressively; users MUST be able to stop enhancement or continue the owning workflow without waiting for AI's deadline.
- **FR-082**: Default AI deadlines MUST be at most **30 seconds** for up to 100 eligible distinct questions, **90 seconds** for 101–1,000, and **180 seconds** above 1,000, from scheduling including queue time. Class uses the fixed question count for that revision; exhaustion preserves completed work and identifies remaining rows.
- **FR-083**: Each remote attempt MUST have a deadline of at most **15 seconds**, further bounded by remaining run time. A distinct question may appear in at most **three remote attempts per run**, counting proposal, retry, split, and verification. Qualified policies may be stricter.
- **FR-084**: Every run MUST enforce request, input/output allowance, concurrency, and monetary ceilings across all attempts, including retry/split/verification. Before dispatch, the remaining spending limit must cover the bounded maximum authorized attempt cost or an equivalent enforced upstream reservation. If that bound cannot be established, no further paid attempt starts. No retry mechanism bypasses ceilings; unavailable actual cost is “unknown,” not zero.
- **FR-085**: Retries MUST distinguish transient faults from authorization failure, exhausted quota, unsupported capability, and invalid configuration. Non-recoverable conditions stop futile work; all waits/retries count inside the original deadline.
- **FR-086**: Equivalent concurrent questions MAY share work within authorized scope. Repetition MUST NOT cause per-row remote calls, discard valid answers, or repurchase answers already obtained in the same revision.
- **FR-087**: Scheduling MUST prevent large files/organizations from starving others. Saturation MUST leave deterministic review usable and visibly defer/skip optional AI within deadlines.
- **FR-088**: Cancellation, remapping, policy change, or human editing MUST stop or obsolete dependent work. Answers after the relevant deadline, revision, cancellation, or commit MUST have no effect on newer decisions/business records.
- **FR-089**: Deterministic matching MUST meet the time targets below without AI-derived answers, remote models, an embedding service, or per-row external calls.

#### K. Durable progress and transitions

- **FR-090**: Runs MUST retain source/mapping revision, effective scope, catalogue/policy versions, counts, stages, and decisions sufficiently for reopening/resuming; transient progress displays are not authority.
- **FR-091**: Progress MUST distinguish deterministic preparation, review readiness, enhancement, and commit. It MUST NOT announce readiness/100% before corresponding results are saved.
- **FR-092**: Restart/retry MUST preserve saved work without duplicate business effects. New revisions supersede earlier automation while retaining audit history and still-applicable human decisions.
- **FR-093**: Matching and AI MUST NOT themselves commit inventory, savings, financial comparisons, catalogue edits, or orders. Commit uses owning-workflow authorization and revalidates identity, eligibility, required unit/pack, and revision.
- **FR-094**: Ready, failed, cancelled, and resumed runs MUST account for each source row once in primary-state totals. Secondary AI statistics may overlap primary states only with explicit denominators.
- **FR-095**: Row errors MUST retain actionable reasons/location; run failure MUST expose bounded retry/correction. Interrupted work MUST NOT remain indefinitely marked live.

#### L. Isolation and evidence handling

- **FR-096**: Files, staged rows, private aliases, decisions, caches, progress, exports, and review actions MUST enforce organization/source ownership and roles. Marketplace catalogue discovery MUST NOT imply access to other tenants' private matching data.
- **FR-097**: Cross-organization administration and platform curation MUST be explicitly authorized, with actor/target scope recorded. Identical text, candidate IDs, or cache keys MUST NOT cross privacy boundaries.
- **FR-098**: AI receives only permitted identity evidence needed for the question. Credentials, customer identities, entire files, negotiated prices, purchasing quantities, and unrelated rows MUST NOT be included merely because they are available.
- **FR-099**: Product text/AI output MUST be bounded, safely displayed data. Matching MUST NOT execute content, follow embedded instructions/links, or permit it to alter authorization/catalogue records.
- **FR-100**: Diagnostics and retention MUST follow existing project policy, retaining necessary provenance without introducing indefinite raw-prompt/private-file retention. Missing durations for new production diagnostic payloads require a bounded planning decision before collection.

#### M. Observability and measured value

- **FR-101**: Run summaries MUST reconcile raw/excluded/invalid rows, distinct questions, deterministic outcomes, memory reuse, AI eligibility/attempts, new resolutions, review flags, abstentions, invalid/rejected answers, technical failures, and unattempted work.
- **FR-102**: Operators MUST separate time obtaining/parsing source data, preparing catalogue evidence, matching, saving, waiting for AI, generating answers, validating, and finalizing. Measurements include workflow and applicable versions.
- **FR-103**: AI reporting MUST show new accepted resolutions, independent correctness where known, extra waiting, attempt counts, known total cost, cost per verified additional correct resolution, and why zero gain occurred. Confirmations and changed scores add no resolution gain.
- **FR-104**: Authorized diagnostic inspection MUST connect outcomes to facts, candidates, conflicts, source, memory, versions, and revision history; it distinguishes retrieved-but-rejected from never-retrieved targets.
- **FR-105**: Quality reporting MUST separate precision, coverage, candidate recall, review workload, unavailable data, absent targets, ambiguous inputs, and stale-memory corrections, segmented by workflow, script, identifiers, difficulty, and pack sensitivity.
- **FR-106**: Operators MUST disable failing AI configurations/policies and restore qualified versions. Release procedures MUST identify owners and alert thresholds for identity violations, correction growth, latency, stale knowledge, and spending overruns.

#### N. Qualification and rollout

- **FR-107**: Before tuning, record the current revision and measure it on approved evaluation/load profiles. Anecdotes and synthetic timings MUST NOT be presented as production accuracy or verified AI benefit.
- **FR-108**: Evaluation MUST combine authorized de-identified real patterns and constructed difficult cases. Labels describe valid target sets, required granularity, explicit facts, unique resolvability, and expected abstention/conflict.
- **FR-109**: Tuning, prompt examples, learned aliases, and held-out evaluation MUST be separated by source/template families and repeated expression families. Human adjudication resolves disputed labels; model output cannot be its own ground truth.
- **FR-110**: Compare current deterministic, current AI-enabled where configured, new deterministic, and new AI-enabled configurations. Every workflow MUST exercise disabled AI and upstream failure.
- **FR-111**: Regression coverage MUST include all edge-case categories, acceptance paths, comparison grouping, identifier conflicts, stale memory, concurrent edits, late answers, malformed output, isolation, recovery, and commit revalidation.
- **FR-112**: Performance qualification MUST cover cold/warm operation, realistic scripts, duplicate-heavy/distinct files, ambiguous siblings, multiple organizations, and concurrent deterministic/AI work. Repeat-cache performance MUST be separate from first-use.
- **FR-113**: Rollout MUST progress through offline qualification, observation without modifying user decisions, a limited reversible cohort, and broader enablement after gates pass. Failing AI may stay off while qualified deterministic work ships.
- **FR-114**: Documentation, comments, defaults, and screens MUST agree on changed identity behavior and AI limits. Obsolete promises about thresholds, memory scope, or latency MUST be corrected during implementation.
- **FR-115**: Completion MUST cover every inventoried entry point, deterministic gates, at least two qualified AI configurations, review usability, and recovery. Unqualified workflows/models or unmet targets MUST be named as incomplete.

### Normative identity examples

These synthetic fixtures constrain behavior, not scoring algorithms or literal production-prompt wording. Assume authorized catalogue visibility and no unstated independent evidence.

| Source | Candidate situation | Required outcome |
|---|---|---|
| `Brand A 500 mg tablets 20` | Same identity, `0.5 g`, same form/approved pack | May accept when uniquely identified. |
| `Brand A 500 mg tablets 20` | Same name, `250 mg` | Reject strength conflict. |
| `Brand A 500 mg tablets 20` | Same strength, explicit capsule | Reject form conflict unless an explicit scoped source correction resolves it. |
| `Brand A XR 10 mg` | Ordinary-release `Brand A 10 mg` | Reject release conflict. |
| `Brand A Plus 5/10 mg` | `Brand A Plus 5/20 mg` | Reject component-strength conflict. |
| `Brand A Plus 5 mg` | Two variants differ in the omitted component | Ambiguous; no inferred missing component. |
| `Brand A` | Several strengths/packs | Request distinguishing evidence; popularity cannot decide. |
| Arabic alias versus English canonical name | Curated transliteration, matching facts, unique target | May accept; script alone is not a conflict. |
| `Brand A 100 mg/2 mL injection` | Same mass, 5 mL container, different concentration | Equal mass alone is insufficient; explicit conflicting concentration is rejected. |
| `Brand A 250 mg/5 mL, 100 mL` | `Brand A 50 mg/mL, 100 mL`, same form/pack | Equivalent concentration may corroborate identity. |
| `Brand A 20 tablets` | Canonical item has 10- and 20-tablet packs | Resolve 20-tablet pack before pack-dependent action. |
| `Brand A 2 strips` | Ten tablets/strip, three strips/box | No invented box identity or quantity conversion without approved relationship. |
| `Product A / Manufacturer X` | Product B shares only Manufacturer X | Reject manufacturer-only evidence. |
| Supplier code `00123` | Unrelated canonical item uses the digits | No authoritative hit without valid namespace/declaration and applicable checks. |
| True target absent from supplied candidates | Several similar items | Abstain. |
| Same raw name, separate structured `10 mg` and `20 mg` rows | Otherwise identical context | Distinct questions and separately preserved results. |

### Acceptance coverage and requirement traceability

This is an evidence map, not an implementation checklist. Planning must attach executable evidence without reducing verification to shared-engine unit tests alone.

| Requirements | Stories | Required evidence | Outcome gates |
|---|---|---|---|
| FR-001–006 | 1, 3–6, 13 | Entry-point inventory, equal-context replay, disabled-AI workflows | SC-001, 009, 013 |
| FR-007–014 | 1–6 | Labelled normalization/extraction, original-value retention, invalid-row accounting | SC-001–004, 011 |
| FR-015–024 | 1–6 | Every acceptance path against contradictions, omissions, identifiers, packs | SC-001, 002, 010 |
| FR-025–034 | 1–6 | Recall, catalogue completeness, stable replay, ambiguity and deduplication | SC-003–005, 013 |
| FR-035–042 | 1, 3–6 | Workflow review/commit, stale-link clearing, offer/unit separation | SC-009–011, 013 |
| FR-043–050 | 7, 12 | Arabic/English task observation, accessible review, bulk and concurrent edits | SC-006, 010, 011 |
| FR-051–058 | 8 | Scoped reuse/revocation, target changes, negative memory and tenant isolation | SC-010, 012 |
| FR-059–068 | 9, 13 | Paired AI evaluation, faults, protected choices, audit mode | SC-007–009, 014 |
| FR-069–080 | 10, 11 | Prompt evaluation, hostile output/input cases, two configurations | SC-002, 007, 014 |
| FR-081–089 | 9, 12 | Deadlines, full attempt accounting, saturation, cancellation, cost | SC-005, 008, 009, 014 |
| FR-090–095 | 4, 9, 12 | Resume, durable readiness, idempotency, obsolete results, commit | SC-009–011 |
| FR-096–100 | 8, 10, 12 | Role/scope isolation, minimized payloads, hostile input, diagnostics access | SC-010 |
| FR-101–106 | 9, 12, 13 | Reconciled counts, quality/cost reports, rollback rehearsal | SC-011, 014, 015 |
| FR-107–115 | 13 | Frozen corpus, baselines, qualification, rollout, consistent docs | SC-001–015 |

### Key Entities *(include if feature involves data)*

- **Source Row**: Original values/location, organization/source context, mappings, declared identifiers, quantities, monetary values, and corrections.
- **Identity Evidence**: Original/corrected/normalized/inferred facts, provenance, certainty, and interpretations used to find or reject candidates.
- **Catalogue Identity**: Authorized canonical product, attributes, aliases, revision, eligibility, and sellable pack/variant needed for an action.
- **Candidate Assessment**: Row-specific candidate, retrieval basis, agreeing/missing/conflicting facts, relative rank, and acceptance/rejection reasons.
- **Match Decision**: Primary state, target at required granularity, reasons, source, versions, deterministic baseline, and decision revision.
- **Matching Policy**: Versioned identity rules, granularity, permitted source declarations, review restrictions, qualified thresholds, and consumer limits.
- **Matching Run**: Resumable preparation/review/enhancement context with frozen source/mapping scope, versions, counts, budgets, timestamps, and state.
- **Review Action**: Authorized correction, choice, rejection, exclusion, undo, or memory action and exact affected rows/revisions.
- **Scoped Knowledge**: Human mapping/rejection, curated alias/relationship, or explicitly published fact, with ownership, applicability, and revocation.
- **AI Attempt and Proposal**: Bounded remote question/answer linked to rows/versions, distinct from acceptance, with validation, timing, usage, and obsolescence.
- **Qualified AI Configuration**: Compatible model/prompt/answer-contract combination, supported cohorts, measured quality/latency/cost, limits, and enablement.
- **Evaluation Case and Report**: Labelled evidence, expected target/abstention, leakage-safe split, difficulty, frozen inputs, outcomes, and release decision.

## Success Criteria *(mandatory)*

### Measurement rules

All numbers are release targets, not measured achievements. Revisions require an explicit recorded product decision supported by baseline evidence. A missed target must not be hidden by changing its denominator.

Before tuning, freeze an authorized corpus of at least **6,000 distinct identity questions**: at least 2,000 for development/tuning and **4,000 held out**. The held-out set covers all five workflows with at least 500 relevant cases each; a case may exercise multiple workflows, but overall counts deduplicate it. Include at least 300 pack/unit-sensitive cases, 300 close-strength/form/extension confusions, 200 absent-catalogue cases, and 200 inherently ambiguous cases. Categories may overlap. At least 50% of questions are Arabic-led, 15% English-led, and 15% mixed-script. A separate adversarial collection supplements this representative corpus.

Two knowledgeable reviewers label high-risk/disputed cases. Unresolved label disagreements stay visible as uncertainty but are excluded from accuracy estimates. Keep source-file/template families and near-duplicate expressions within one split. Synthetic fixtures can test rules but cannot alone establish real-file coverage, usability, or AI value. Existing remembered answers are not test labels; any memory applicable during evaluation must predate the frozen split and be included in the baseline disclosure.

Use these definitions consistently:

- **Accepted precision**: correct automatically accepted identities divided by all automatically accepted identities, at required granularity. Wrong pack/variant counts as wrong. Report point estimates and a one-sided 95% lower confidence bound using a declared method held constant between comparisons.
- **Resolvable row**: reviewers agree that the supplied evidence uniquely identifies an eligible target in the frozen catalogue. Missing targets and inherent ambiguity are not resolvable; they remain in total-row and abstention reporting.
- **Automatic coverage**: correct automatically accepted resolvable rows divided by all resolvable rows. Also report all-source-row coverage, exclusions, and errors to expose denominator changes.
- **Candidate recall at 20**: resolvable rows whose target appears among at most 20 retrieved candidates divided by resolvable rows. This measures finding independently of acceptance; review may show fewer.
- **Net AI resolution gain**: newly correct accepted resolutions minus newly wrong acceptances minus previously correct accepted rows lost because of AI, divided by the new deterministic engine's remaining resolvable rows. Confirmations/score changes add zero. Report each term and the additional AI-accepted subset's precision separately.
- **Reviewable time**: from source availability plus confirmed mapping to saved, usable deterministic results. Include source reading/preparation, catalogue evidence, matching, and saving; exclude upload transfer, human thinking, and optional AI completion.
- **AI elapsed time**: from enhancement scheduling through queueing, all attempts, validation, and saving additions. Stopped/failed runs remain in timing and outcome reporting.
- **Verified AI unit cost**: known total cost, including unsuccessful attempts, divided by independently verified additional correct resolutions. Zero improvements means “no verified improvement” with total cost; unavailable accounting means “unknown,” never zero unit cost.

Accuracy qualification needs at least **1,000 automatically accepted held-out decisions overall** and enough accepted samples in every enabled workflow to evaluate its confidence bound. If sample size is insufficient, extend independent evaluation or keep automatic application disabled for that cohort. AI cohorts need at least **100 remaining resolvable cases each**; extend the held-out collection with independently labelled authorized cases if needed, retaining separate representative-distribution reporting. An insufficient sample is not a pass.

Performance uses an unchanged, recorded production-sized reference environment and a catalogue of **100,000 eligible canonical items plus their sellable variants**. Record capacity, dataset fingerprint, total variant count, file format, cold/warm state, competing load, and policy before comparisons. Measure at least **30 runs per file-size/load condition**, covering 100, 1,000, 10,000, and 50,000 rows and concurrent work from five organizations. Report cold first-use separately from warm memory and AI cache reuse. The plan defines reproducible fixtures and deployment details without weakening these outcome targets.

### Measurable Outcomes

- **SC-001 — Accepted accuracy**: New deterministic matching achieves at least **99.5% accepted precision**, with a one-sided 95% lower bound of at least **99.0%**, overall and in each enabled primary workflow. Master-catalogue update associations achieve at least **99.9%** with a lower bound of at least **99.5%**. The AI-added subset independently meets the corresponding workflow thresholds; deterministic volume cannot hide poor AI precision.
- **SC-002 — Hard-conflict protection**: **100%** of labelled explicit strength, brand/combination, route/form/release, identifier contradiction, and required pack/unit conflict cases avoid incorrect automatic acceptance through every tested acceptance path. Every adversarial model selection of an unauthorized/nonexistent target is rejected before application.
- **SC-003 — Deterministic usefulness**: Correct automatic coverage reaches at least **80% of resolvable rows overall** and **75% in every primary workflow**, while meeting SC-001/002. Remaining resolvable review workload falls at least **25% relative to the current deterministic baseline**. If baseline coverage already exceeds the minimum, preserve or improve it at the required precision. A zero baseline review denominator requires preserving zero, not inventing a percentage.
- **SC-004 — Retrieval and honest abstention**: Candidate recall at 20 reaches at least **98% overall** and **95%** in each adequately sampled language/script and major difficulty cohort. At least **99%** of known absent-target and inherently ambiguous cases remain unaccepted. **100%** of unavailable-data cases are distinguishable from “no eligible candidate found.” Each reported cohort needs at least 100 resolvable cases for recall; smaller cohorts are labelled insufficient and require added independent evidence for qualification.
- **SC-005 — Usable deterministic results**: At least 95% of single-run imports become reviewable within **5 seconds for 100 rows**, **15 seconds for 1,000**, **60 seconds for 10,000**, and **5 minutes for 50,000** under the reference workload. With five organizations concurrently processing the same size class, the 95th-percentile time stays within **twice** that class's single-run limit without starvation. A one-row correction refreshes deterministic suggestions within **1 second at the 95th percentile** on the same catalogue.
- **SC-006 — Reviewer effort**: In observed tasks with at least **eight representative users**, including customers, vendors, administrators, and Arabic-first users, at least **90% of assigned tasks** complete from upload through valid commit without facilitator intervention. Median active review time on matched-complexity files falls at least **30%** from baseline without increasing mistaken confirmations. All required actions are keyboard-completable in both languages; no critical identity fact/action is inaccessible at 360-pixel display width.
- **SC-007 — AI value**: Each enabled primary workflow achieves at least **20% net AI resolution gain** on the new deterministic engine's remaining resolvable cases while satisfying SC-001/002, without decreasing overall correct coverage. Reports separate cold first-use gain, reuse, confirmations, new errors, and lost correct answers. At least **two distinct configurations**, spanning different model capability/size profiles and including one without native constrained-output support, meet the applicable gates before model portability is complete.
- **SC-008 — Bounded AI time**: **100%** of runs end locally by their **30/90/180-second** class deadline plus at most **2 seconds** to publish terminal state. No remote attempt exceeds its **15-second** local deadline. For enabled cohorts, at least **95%** of ultimately accepted additions appear within **20/60/120 seconds**, respectively. Failing configurations stay disabled or have reduced qualified scope. Upstream cancellation support is not assumed; obsolete responses still have zero application effects.
- **SC-009 — Availability and protected work**: Every workflow passes all disabled, unconfigured, unauthorized, slow, malformed-output, and upstream-outage scenarios with a usable deterministic/manual path. Optional AI adds no more than **1 second at the 95th percentile** to deterministic reviewable time. Cancelled, superseded, manually edited, and committed rows receive **zero** unauthorized late changes in concurrency/recovery scenarios.
- **SC-010 — Isolation and correct commit**: All cross-tenant, source-namespace, role, effective-organization, and platform-memory negative tests pass with **zero unauthorized rows, private knowledge, proposals, or business changes**. Every tested commit revalidates identity/pack and owning-workflow eligibility. Monetary and converted-quantity expectations agree exactly.
- **SC-011 — Accounting and recovery**: **100%** of source rows reconcile to one primary terminal or recoverable outcome. Every interruption/retry/repeated-commit scenario preserves saved review work without duplicate business writes. Interrupted processing becomes visibly recoverable or terminal within **60 seconds of its last valid liveness indication**, under the declared recovery workload. A live progress display never overrides a saved terminal state.
- **SC-012 — Safe memory**: On unchanged repeat files, at least **95%** of applicable human-confirmed questions resolve without renewed review or remote AI. **100%** of scoped-rejection, revocation, changed-identity, changed-policy, and cross-organization tests block stale/unauthorized acceptance. AI-only caches contribute zero automatic decisions with AI off.
- **SC-013 — Consumer consistency**: **100%** of equal-context replay cases agree on deterministic identity and reasons across all five workflows and inventoried entry points. Stricter-profile differences are explicit and never weaken hard constraints. End-to-end evidence covers comparison grouping and alternate savings paths.
- **SC-014 — Honest value and spending**: Every remote attempt, including retry/split/verification, counts against shared ceilings, and tested runs exceed none of them. At least **90% of accepted deterministic questions** avoid remote calls in ordinary enhancement; any remaining audits are separately enabled and measured. Before blind evaluation, every configuration has a declared maximum cost per verified additional correct resolution and meets it; unknown cost cannot pass. Every zero-gain run reports zero new resolutions and a reason distribution.
- **SC-015 — Reversible release**: A rehearsal disables failing AI or restores a qualified matching policy within **5 minutes**, preserving all source rows, review history, and committed records. Broad rollout requires at least **seven consecutive days** of limited-cohort operation without a confirmed hard-identity violation or unresolved breach of applicable precision/time/cost gates. Low volume requires extending observation rather than assuming success.

### Release evidence

Deliver an entry-point coverage record, baseline/candidate evaluation reports, reviewed identity vocabulary/rules, prompt/configuration qualification reports, measured load/deadline results, review-task observations, failure/isolation/recovery evidence, memory migration/revalidation assessment, updated module documentation, and demonstrated rollback. Name unmet criteria, unsupported cohorts, and unknown measurements. No live gateway benchmark, production-data evaluation, or user study was performed by this specification-writing task.

## Assumptions

- The current platform is authoritative. The existing implementation supplies a measurement baseline, not a requirement to preserve defects; retired Laravel behavior is not the acceptance standard.
- Existing modular boundaries, centralized capability gateway, exact money handling, tenant isolation, bilingual presentation, and compatible migration rules remain binding. This spec does not authorize a new service architecture or provider dependency in business modules.
- The constitution file is an unfilled template. Repository agent instructions and accepted architecture decisions provide operative constraints; this task does not invent or ratify a constitution.
- Matching means commercial catalogue identity, not clinical interchangeability. Domain reviewers label facts; the system does not invent therapeutic equivalence or substitute identifiable brands, forms, strengths, releases, or selling packs.
- Incomplete pack metadata may still allow a useful product-level suggestion, but dependent ordering/updating/comparison remains blocked for that row until valid evidence resolves the distinction. A full taxonomy rewrite is not assumed.
- Existing entitlements, roles, upload handling, and commit boundaries remain. Reviewers can correct source interpretation; canonical correction/publication requires privileged roles.
- Authorized de-identified representative data and knowledgeable reviewers will be available during implementation. Otherwise synthetic fixtures establish rule behavior only and real-data/AI/usability gates remain unverified.
- Adequately budgeted access to two model configurations is a qualification dependency. No model is promised to meet targets before evaluation; gateway limitations can block AI qualification without blocking deterministic work.
- The AI deadlines and accuracy/coverage figures are deliberate proposed improvements, binding until explicitly revised with evidence. They are not claims about existing service capacity.
- New runs default AI off unless a qualified policy is deliberately enabled. Existing visible opt-in preferences may be migrated, but changed automatic-acceptance configurations require qualification.
- Organization/operator spending ceilings and maximum acceptable cost per verified correct addition are set before qualification using actual accounting. This specification authorizes no purchase, budget increase, or production AI traffic.
- Existing retention policy governs. Planning resolves a missing duration for new diagnostic payloads before collection rather than inventing a legal retention rule.
- This is one coordinated rework delivered in independently testable workflow slices. Correct deterministic behavior comes first; AI cannot mask an unqualified foundation. This task creates specification artifacts, not implementation tasks or production changes.
