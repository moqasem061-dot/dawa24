# UI and static asset delivery

## Release gate repairs (2026-10-14)

Embedded role guides declare a CSP nonce placeholder in their style tags; the
handler replaces it with the current request's nonce before serving the page.
The static markup gate and runtime guide checks now agree. Matching-policy
rollback uses the shared bilingual `data-confirm` action instead of an Alpine
expression unsupported by the CSP build.

## Documents, map addresses and supplier guide (2026-10-14)

Official document preview handlers live in a static script and resolve the
current modal on every action, including after navigation swaps and history
restoration. Each image load owns a fresh node so late callbacks cannot hide
the next document. Native new-tab links bypass the universal preview handler.
Configured shared-storage failures refuse official document uploads before
registering metadata, rather than recording success with only a local copy.

Moving the map pin regenerates the detailed address after previous manual edits;
typing while the request is pending still protects that new text. The supplier
dashboard places a large platform-guide button in its directory section and
explains that it opens the usage instructions and workflow map.

## Official documents and tenant activity language (2026-10-12)

The organization document viewer detaches old media callbacks before clearing
sources, ignores stale image errors, and renders an explicit error instead of a
blank iframe when an image cannot decode. MIME metadata supports filenames
without extensions. Close and backdrop handlers bind after the modal markup,
including when that markup arrives through navigation; queued close events
cannot clear a reopened preview.

Vendor and pharmacy activity lists use a bilingual presentation projection.
Route patterns become business actions and sections; response outcomes become
readable text. Details contain only translated, allowlisted before/after fields,
not route URLs, request methods, machine action names or raw JSON. Unknown
actions have a readable fallback. Full technical audit data remains available
to administrators. Browser fixtures cover refresh, replacement metadata,
repeated viewing, image failure, both languages, detail dialogs and search.

## Product image preview lifecycle (2026-10-12)

The shared viewer respects explicit image metadata, including extensionless
image URLs. Closing clears only the closed preview's media, removes image sources
without requesting the current page, and ignores queued close events after a
reopen. Browser coverage exercises both close buttons, Escape, backdrop clicks
and immediate reopen while asserting product details and the buy box remain.

The Arabic-first templ UI uses shared public, pharmacy, supplier, and admin
shells. JavaScript runs under the nonce-based CSP and Trusted Types policy.

## Performance invariants

- Embedded CSS and JavaScript are minified once at startup with esbuild, targeting
  the browsers in `docs/BROWSER_SUPPORT.md`. Source files remain readable.
- Content hashes and ETags describe the minified response. Gzip is computed once.
  Matching version URLs receive a one-year immutable cache lifetime.
- Shared CSS is bundled in its existing cascade order. Public pages add the
  customer/public/marketing bundle. Individual stylesheets remain available.
- `security.js` runs inline with the request nonce, before page scripts, analytics,
  or image failures. It is never deferred or removed to improve a score.
- Readex Pro and Spline Sans Mono use local variable WOFF2 subsets with
  `font-display: optional`; the primary language's face is preloaded. Font licenses
  are beside the files. There is no Google Fonts request during rendering.
- The bundled logo uses responsive AVIF files; the mascot uses WebP files sized
  for its display. Images reserve their dimensions. Uploaded logos retain their
  configured URLs. Original PNGs remain available for external references.
- Only the anonymous homepage inlines generated critical CSS. Complete styles
  load asynchronously, with a JavaScript-disabled fallback. Authenticated pages
  and other entry routes retain full blocking styles. The complete public sheet
  remains available for boosted navigation.

## Regenerating homepage styles

Run `go run ./scripts/pagespeed` to serve the real templates and asset handler
at `http://127.0.0.1:8097` without a database. This is a local test fixture, not
a production server. In another terminal:

```sh
npm ci --prefix scripts/pagespeed
# On a host without Chrome/Edge: npx --prefix scripts/pagespeed playwright install chromium
npm run critical --prefix scripts/pagespeed
```

`PLAYWRIGHT_CHROMIUM_EXECUTABLE` can select an installed browser and
`PAGESPEED_ORIGIN` can select another local fixture port. Rebuild the fixture
before regenerating after a CSS change. The generator keeps cascade and media
rules, fonts, keyframes, and hidden/conditional navigation, ad, and modal styles.
It writes `internal/ui/static/critical/home.css` and a source fingerprint.
`TestHomepageCriticalCSSIsCurrent` prevents stale generated styles from shipping.
Do not edit the generated stylesheet manually.

Run `go test -short ./internal/ui/...` and the browser checks with:

```sh
npm test --prefix scripts/pagespeed
```

The browser checks compare the homepage with and without the complete sheets,
exercise navigation, and check mobile/desktop, both directions and both themes.
The `/review` fixture also renders the vendor import review with representative
reasons and large numeric values. Its browser checks verify every field fits
without horizontal scrolling and that quantity editing, selection, sorting and
the catalog picker remain usable.
Lighthouse should also run against the deployed site: the fixture cannot measure
database latency, CDN behavior, live advertisements, or analytics.

## October 2026 fixes

The pharmacy, supplier and delivery dashboard headers now render one maps
tutorial link each. Product purchase controls use proper conditional HTML
boolean attributes. `scripts/pagespeed/cart.test.cjs` verifies real HTMX
submission and cart update events; `address.test.cjs` covers geocoder script
loading, detailed formatting, stale responses and preservation of manual edits.

The homepage now uses sequential heading levels (including footer sections).
The hero uses `minmax(0, ...)` tracks so its comparison table cannot push the
headline and buttons outside a narrow viewport. A malformed utility-sheet
comment that prematurely closed at a wildcard slash was repaired. Toast entrance
motion no longer scales its 44px close button below the minimum touch target.

Rebuild image variants with `python scripts/pagespeed/optimize_images.py`
(Pillow 11.3+). Font files came from Google Fonts CSS2 for Readex Pro 300–700
(v27, Arabic and Latin) and Spline Sans Mono 400–700 (v13, Latin); their OFL
licenses are retained in `static/fonts/`.

## Verification, 2 October 2026

The local mobile Lighthouse 13.5 fixture improved from 87 to 93 performance,
98 to 100 accessibility, and retained 100 best practices and SEO. FCP improved
from 2.1s to 1.5s, LCP from 3.8s to 3.1s, with zero blocking time and zero layout
shift in the final audit. Minification, image delivery/dimensions, caching, and
render-blocking resource audits pass. These are local lab results, not a claim
of 100 performance on the production deployment.

`go test -short ./...`, the UI package suite, ten homepage browser checks, and
all eighteen existing UI/session browser tests passed. Repository lint/static
gates still report unrelated existing issues: swallowed handler errors, an
unsupported Alpine expression in `admin_match_policies.templ`, undefined class
ratchets, and noctx/staticcheck findings outside the changed delivery code.

## Discount display and previews (2026-10-09)

Discount comparison labels describe percentage comparisons. Display helpers and
market-discount filter URLs retain hundredths of a percentage point, including
0.25 and 12.34. Warehouse mapping previews use a 200-row sample instead of
materializing the entire sheet; full spreadsheet security checks remain enabled.
