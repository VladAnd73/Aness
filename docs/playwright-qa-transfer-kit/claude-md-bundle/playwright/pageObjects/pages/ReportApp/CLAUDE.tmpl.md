# pages/ReportApp - The report app (separate SPA)

Parent: [`../CLAUDE.md`](../CLAUDE.md).

The report app is a **separate app** (its own repo and production host), not a {{PROJECT_NAME}} route. It
reads only from a public read API (a credentials call, then a set of per-report reads). Today its only
consumer is the production smoke ([`../../../prodSmoke/`](../../../prodSmoke/CLAUDE.md)), which runs
against `{{PROD_SMOKE_TARGET}}` using `{{CANARY_IDS}}`.

## What's here
- **reportApp.ts** - the report page object (route `/{tenantUuid}/{reportId}` on the report host).
  - `open(tenantUuid, reportId)` - installs a **read-only guard** (every non-GET/HEAD/OPTIONS request is
    aborted in the browser and recorded), starts recording every read (endpoint + status, or the network
    error) and checking the body shape of every 2xx required read, records every **error toast** as it
    appears, then navigates.
  - <one validator per rendered section: what it asserts (a header value, a set of numeric cards, at
    least one visible entry and no visible empty notice, ...)>
  - `validateReadsOk()` - **call it right after `open()`**, before any section check. Waits until
    every required read has answered, then requires every 2xx required read to have its expected body
    shape and every recorded read to be 2xx. A bad body or status is reported before "never answered",
    because a malformed upstream read stops the reads after it from being sent.
  - `validateNoErrorState()` - none of the app's error texts and no error toast was shown at any point.
  - `validateNoLateFailedReadOrWrite()` - **call it last.** Re-checks reads that arrived after
    `validateReadsOk()` (shape, then 2xx) and requires that no write was attempted.

## Conventions & gotchas
- **The production bundle carries almost no `data-testid`s.** The app's own test ids are stripped from
  the prod build; only a few shared-library ids survive. Locators therefore use roles and visible text,
  plus stable data attributes where needed.
- **Hidden views are in the DOM.** When a grid and a table view are both rendered and one is hidden, the
  hidden one holds its own empty-state notice. Entry and notice locators filter on `visible: true`.
- **Values that default to 0 on a failed or missing read prove only that a section rendered.** Whether
  the data actually arrived, in the expected shape, is proven by the read checks, not by the zeros.
- **Read failures are asserted before the sections.** A failed or malformed read makes a section check
  fail with a misleading "canary data changed?" message, so the read checks must come first and name
  the failing endpoint.
- **Body shapes are the live production shapes, dated when last checked**, kept to the fields the app
  needs. When the canary's arrays are empty, only the container shape is asserted. A rendered-but-empty
  chart is not proof a read worked; the shape check is.
- **Error toasts auto-close after a few seconds.** A point-in-time count at the end of a test misses
  them, so `open()` records them as they are added to the page. A `200` response carrying an error body
  may show only a toast, and only this check catches it.
- **There is no click method on purpose.** Anything that would trigger live third-party calls or a
  server-side refresh must never be driven against production.
- A section that only renders for some record kinds fails its validator by design when
  pointed at a canary outside that set; pick canaries that render every section.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | consumer [`../../../prodSmoke/`](../../../prodSmoke/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
