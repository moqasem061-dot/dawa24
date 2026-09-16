# Browser support

The baseline this application is built and tested against, and the reasoning
behind it. A feature outside the baseline either carries a fallback or is not
used.

## Baseline

| Browser | Minimum |
|---|---|
| Chrome / Edge (Chromium) | 111 |
| Firefox | 113 |
| Safari (macOS) | 16.4 |
| Safari (iOS / iPadOS) | 16.4 |
| Samsung Internet | 22 |

Egypt's pharmacy counters run a wide spread of hardware, so the floor is set by
the oldest release that still receives security updates on a device someone is
plausibly still using — not by the newest feature we would like.

## What decides the floor

Three CSS features, in order of how badly their absence breaks the page.

**`@layer` — the hard floor.** Every stylesheet except `app.css` and
`invoice_printable.css` is wrapped in a cascade layer, and `make
check-css-layered` enforces that. A browser that does not understand `@layer`
does not degrade: it discards the entire block, and the application renders as
unstyled HTML. There is no fallback for this short of shipping a second,
unlayered build. Safari 15.4 and Firefox 97 are where support begins; the floor
above is comfortably past both.

**`oklch()` — 148 declarations.** Every one is preceded by a computed `rgb()`
or `rgba()` fallback in the same rule, so a browser without OKLCH takes the
sRGB value and the page keeps its colours. The fallbacks are generated from the
OKLCH values themselves rather than eyeballed, so the two cannot drift apart in
hue.

**`color-mix()` — 50 declarations.** Each is preceded by a fallback naming the
nearest solid token: a tint like `color-mix(in srgb, var(--success) 10%,
transparent)` falls back to `var(--success-subtle)`, and a border mix falls back
to `var(--success-border)`. Falling back to the *un-mixed* colour would have put
dark green text on solid green; the subtle tokens exist for exactly this
intensity and are what the mix approximates.

## Other features and how they are handled

| Feature | Handling |
|---|---|
| `dvh` units | Every use is preceded by the same value in `vh`. `dvh` measures the viewport without the mobile browser chrome; `vh` includes it, which is slightly wrong rather than broken. |
| `<dialog>` + `showModal()` | Used by `components.Modal` with no fallback. Supported from Safari 15.4 and Firefox 98, both below the floor. |
| `:has()` | Seven uses, all cosmetic. A browser without it loses polish, not function. |
| `text-wrap: balance` / `pretty` | Eight uses, purely typographic. |
| `backdrop-filter` | **Not used.** Every surface that had one carries a semi-opaque background of its own; the blur was removed rather than prefixed, because Safari honours it only behind `-webkit-` and older Firefox not at all, so most target browsers never rendered it anyway. `make check-backdrop-filter` keeps it at zero. |
| `scrollbar-gutter` | Two uses, cosmetic. |
| `e.explicitOriginalTarget` | One use in `app.js`, Firefox-only, behind a `\|\|` fallback. |
| Optional chaining (`?.`) | One use. Safari 13.1+. |
| `AbortController` | Used by the combobox, guarded by a `typeof` check. |

## Geolocation

`navigator.geolocation` is only available in a **secure context**. Over plain
HTTP it is absent in Safari and Firefox on every host, and in Chrome on
everything except `localhost`. The "موقعي الحالي" button detects this and says
so rather than reporting a generic failure — but the fix is to serve the
application over HTTPS, not to change the button.

Acquisition is two-stage: a coarse fix first (`enableHighAccuracy: false`,
8s), then a precise one in the background. A single high-accuracy request with
`maximumAge: 0` — what the button used to send — routinely outlasts its own
timeout on a cold receiver in Safari, on macOS and iOS both, and reported
failure on a device that would have answered a second later.

The button is bound by one delegated listener on `document`, so a button
rendered later (inside a modal, an Alpine template, an htmx swap) works without
re-initialisation. There are eight of them across seven templates and they all
run the same code.

## Testing matrix

Run before any release that touches layout, and after any change to
`tokens.css`:

- Windows: Chrome, Edge, Firefox
- macOS: Safari, Chrome
- iOS Safari, Android Chrome

Pages: `/`, `/auth/register`, `/catalog`, `/cart`, `/checkout`, `/orders/{id}`,
`/customer/branches`, `/customer/team`, `/vendor/products`, `/vendor/ads`,
`/vendor/organization`, `/market-discounts`, `/settings`.

Each in both themes (light, dark) and both directions (RTL, LTR). RTL is the
primary direction; LTR is the translation.
