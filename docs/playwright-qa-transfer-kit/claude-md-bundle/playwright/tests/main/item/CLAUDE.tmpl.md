# tests/main/item - Item specs (main area)

Item creation, list, lifecycle, bundles, and destination operations. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
| Spec | Exercises | Seeds | @smoke |
|------|-----------|-------|--------|
| <spec file> | <one row per spec: list/search/lifecycle/ordering/filters; publication-status drawer and failure chain; create flows; bundle ordering and transitions; per-destination pause/resume; record-kind summaries; first-time publish payload. Mark every spec that asserts at the {{VENDOR_A}} mock.> | <seed paths, e.g. `seeds/main/item_bundle_live`> | <count / skipped> |

Subfolder: **[`details/`](details/CLAUDE.md)** - restricted-field visibility + item edit specs.

## Patterns & gotchas
- **If a row's click target depends on its state (e.g. a draft opens the editor, not the detail page),
  pick the row by state, never by index.** A seed that samples states at random turns an index pick into
  an intermittent flake.
- **Lifecycle end-states** are driven with `{{DRAIN_COMMAND}}` (often called multiple times to
  drain multi-step transitions) then a UI poll (`itemsPage.waitForItemPhase`).
- **{{VENDOR_A}} assertions are mandatory for publish/republish/pause/archive/activate**:
  `vendorAMock.purgeLogs()` -> action -> `verifyVendorReceived*`. These need {{MOCK_TOOL}} on
  `{{VENDOR_A_MOCK_PORT}}` + `{{VENDOR_A_MOCK_URL_ENV}}`. A seed that publishes while setting up puts its
  own traffic in the log before the test body runs, so a republish assertion must purge again
  immediately before the click.
- **Match mock-log entries by an id unique to this test's item, not just by path.** Backend jobs that
  retry on any failure can re-fire an earlier test's request; id matching keeps that from
  false-positiving a different item's assertion.
- **A test that needs a mock route the shared mock file lacks** depends on a {{MOCKS_HOME}} change; record
  the dependency here. Without it, requests may proxy to a real vendor and fail every verifier.
- **A seed that stamps an item "published" without going through the publish validators** produces an
  item whose republish is rejected. Use it only for the read surface; a test that needs a *working*
  republish needs a seed that completes every required link before marking the item published.
- **Timestamps that the backend compares must be seeded in the right order** (e.g. a failure stamp later
  than the publish stamp), or backend and frontend disagree about the item's state.
- **Column ordering is asserted on the sort KEY read off the RENDERED rows**, in row-index order, checking
  the key is monotonic: ties can't make it flap, it never matches a row back to a seeded record by a
  non-unique name, and it fails when a column's values are uniform, so a column that cannot discriminate
  cannot pass by construction. A column whose seeded values are all equal needs a seed with spread
  values before it can be covered.
- Several bundle/transition specs are `.skip`/fixme (documented flakes/known gaps) - reasons belong in
  the PR/ticket. Add `test.slow()` to full-lifecycle tests.
- **Gate on backend readiness before completing anything an async render creates.** A create helper that
  returns as soon as the mutation and list refetch land can let the next step act on an empty collection,
  link nothing, raise nothing, and surface far downstream as a misleading failure. The gate is readiness
  only; the spec's own assertions stay on the UI and the mock log.
- **A first-time publish payload is asserted on both the per-entry count and the server-deduplicated
  asset count**, so N identical assets rendered as N entries still fails.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | child [`details/`](details/CLAUDE.md) | page objects [`../../../pageObjects/pages/Item/`](../../../pageObjects/pages/Item/CLAUDE.md) | mock helper [`../../../support/`](../../../support/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
