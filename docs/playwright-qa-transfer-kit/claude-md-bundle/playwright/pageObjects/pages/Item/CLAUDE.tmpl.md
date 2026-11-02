# pages/Item - Item list, detail & editor (main area)

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **Items.ts** - `Items` (list, `/main/items`). Holds a `table: ListTables`.
  `validateVisible(isEmpty)`, `startAddItem()` (-> `CreateItemDrawer`), `clickOnItem`,
  lifecycle actions `archiveItem`/`unarchiveItem`/`pauseItem`/`resumeItem`,
  `waitForItemPhase(item | name, phase, { poll? })` (the end-state oracle; `poll: false` asserts
  once against the loaded list instead of reloading), `openFilters()` (-> `FiltersDrawer`),
  `searchForItem` (item or bare string) / `clearSearch()` / `validateNoResultsFor(keyword)`,
  the pagination trio `verifyPaginationPresented(presented)` / `goToNextPage()` / `goToPreviousPage()`, and
  the ordering pair `sortBy(column, direction)` / `validateItemsSortedBy(items, column,
  direction)`. Registered as the `itemsPage` fixture.
- **Item.ts** - `Item` (detail). `validateVisible()`, `verifyItemData`, `openCreateContactDrawer`,
  `hasRestrictedFieldVisible(visible)`, `goToOrderManagement()` (-> `OrderManagement`), `editItemDetails()`
  (-> `EditItem`), `openPublicationStatusDrawer()` (header menu -> the publication-status drawer),
  `openPublicationStatusDrawerFromFailure()` (the failure banner's "View error" CTA -> the same drawer),
  `validatePublicationFailure(present, { poll })` (the header warning icon, its tooltip and the error
  banner; `present: false` asserts the complement, `poll: true` reloads until the indicators settle after
  an async republish), `validateRepublishAvailable(available)`, and a pair of record-summary validators
  for the two record kinds (each asserts its own fields AND that the other kind's fields are
  absent). Registered as the `itemPage` fixture.
- **editItem.ts** - `EditItem`: the detail-page **editor drawer** (one section per editable area:
  content, destinations, schedule, details). Section start/save methods, content add/preview, the
  form editor, `editEndDate`, `validateItemDuration`, `verifyDestinationUrl(url, { destination?,
  derived? })` (`destination` switches the destinations tab first; `derived: true` additionally asserts
  the URL carries no member-only draft markers, for a destination the system resolved rather than the
  member typing it). Registered as the `editItemPage` fixture. Its constructor sets up the
  canned-response intercepts the editor needs.

## How to use it
Phase transitions each wait for the set-phase mutation + the list refresh, then the spec asserts
the phase via `waitForItemPhase`. `EditItem` is not a route - it's opened from the detail page.

## Conventions & gotchas
- **`sortBy` clicks the header until the URL says the sort landed, and never more than twice.** The
  list keeps its sort in `?sortBy=&sortOrder=`, and which direction the FIRST click yields differs per
  column, so counting clicks encodes a guess. Two clicks is also the hard cap: a third would enter the
  table's "no sort" state, where the list page has no column to fall back on.
- **One assertion method per field on this drawer.** A new variant of an existing check (another
  destination tab, a stricter comparison) is an optional parameter on the method that already owns that
  locator - e.g. `verifyDestinationUrl` for `destinationsUrlField` - never a second method beside it.
- **No comments** ([`../../CLAUDE.md`](../../CLAUDE.md)); never explain `src/`, and keep provenance in the
  PR or the ticket.
- Restricted-field visibility is role-gated (`hasRestrictedFieldVisible`) - see the restricted-field specs.
- Async lifecycle end-states are driven with `{{DRAIN_COMMAND}}` then a UI poll, and
  publish/pause/archive/activate touches are asserted at the {{VENDOR_A}} mock via `vendorAMock`.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | drawer [`../../components/drawers/item/`](../../components/drawers/item/CLAUDE.md) | specs [`../../../tests/main/item/`](../../../tests/main/item/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
