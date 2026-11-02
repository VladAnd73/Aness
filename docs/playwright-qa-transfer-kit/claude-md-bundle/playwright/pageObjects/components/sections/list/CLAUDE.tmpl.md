# sections/list - Table machinery (Tables / ListTables / GridTables)

The most-reused code in the suite: the generic list/grid table objects that every list page and the
document-download drawer delegate to. Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- **Tables.ts** - abstract base (`extends BasePage`). Constructor `(page, drawer?)` - pass a `drawer`
  locator to scope the table inside a drawer. Provides view switching, list validation, pagination
  (`switchToView`, `validateItemsList`, `validateItemsMainInfo`, `navigateToNextPage`/`PreviousPage`,
  `validatePaginationVisible(visible)` - with `validatePaginationDoesntExist()` kept as its negative
  shorthand for existing callers) and declares the abstract row operations.
- **ListTables.ts** - `ListTables extends Tables`: the **list/row** view. Implements the row operations
  (`editItem`, `viewItem`, `deleteItem`, `archiveItem`/`unarchiveItem`, `pauseItem`/`resumeItem`,
  a few entity-specific row actions, `clickItemCheckbox`, `verifyListedItemsCount`, and the generic
  `chooseItemAction(item, option)` for a context-menu option with no dedicated method - reach for that
  before adding another one-line sibling) and the rich per-cell assertions incl. the
  integration-status cells (`assertIntegrationStatusDotColor`, `assertIntegrationStatusTooltip`,
  `assertSourceBadgeText/Color`, `validateIntegrationInfo`) and the by-name column oracles
  `waitForColumnTextByName` (polls with reload) / `validateColumnTextByName` (single assert; takes a
  string or a `RegExp`, for cells the app renders formatted), which resolve the row via
  `getRowIdByName`. `getColumnValuesInRowOrder(column)` returns one column's
  rendered cell texts ordered by `data-row-index` - the oracle for a sorted list, since locator order
  alone doesn't survive column pinning - and `waitForColumnValuesPopulated(column)` polls until none
  of them is blank.

  Constructor `(page, drawer?, pageSize?)` - the optional 3rd arg feeds `waitForPage`'s math (below); a
  top-level list page omits it. `ListTables` also holds the isolation-proof methods for an entity
  **detail** page that renders more than one independently-paginated table on the same page (e.g. a
  member's Records + Items; a record's Items + Contacts), where each table's own pagination/sort controls
  share the SAME fixed testids (`pagination-next-button`/`pagination-prev-button`/`pagination-page-N`)
  regardless of the table's own `testId` prop, so they can only be told apart by scoping the instance to
  a wrapping container via `drawer` (e.g.
  `new ListTables(page, page.getByTestId('records-container'), 12)`): `sortByColumn(headerName)`
  (clicks the `columnheader` role by name, scoped to `contextLocators`), `getVisibleRowsText()` (each
  visible row's cell text, in row order, read via `data-row-index` - a content/order snapshot that
  doesn't depend on knowing tanstack's derived column ids), `waitForRowCount(n)` /
  `waitForPage(pageNumber, totalCount)` (polls until the row count settles at the expected value for
  that page AND every row has non-empty text - a table renders its loading skeleton through the same
  row markup with the same `data-row-index`-bearing cells, and a page whose real row count happens to
  equal its `skeletonProps.count` would otherwise satisfy a count-only poll on placeholder rows;
  `waitForPage`'s math needs the ctor's `pageSize`), and the paired oracles
  `assertRowsChangedSince(previous)` (polls - the row's own action, which triggers a refetch) /
  `assertRowsUnchangedSince(previous)` (checked over a ~1s stability window, not a single read - the
  query hooks these tables sit on all use React Query's `placeholderData: keepPreviousData`, so a slow,
  real regression on the sibling could still be mid-refetch, rendering its old-but-correct rows, at the
  instant of a single-shot check). `assertUrlHasNoSharedPaginationParams()` asserts the current URL
  carries none of `page`/`perPage`/`sortBy`/`sortOrder` - only meaningful for `isolateFromUrl=true`
  embedded-table consumers like these (a top-level list page's `usePagination`/`useSort` default to
  `isolateFromUrl=false` and deliberately want that state in the URL for deep-linkability) - the
  isolation fix these embedded tables depend on is exactly "don't touch these URL params," so call this
  after every action as the direct, mechanism-level check (the row-content assertions above are a
  downstream proxy for the same claim, one step removed). To advance a page, use the inherited
  `Tables.navigateToNextPage()` directly - there is no `ListTables`-local next-page method.
- **GridTables.ts** - `GridTables extends Tables`: the **card/grid** view. Implements the card-based
  equivalents; a few list-only methods (`validateComposedItemInfo`, `clickItemCheckbox`,
  `validateColumnVisible`, `hasColumn`) throw "Not implemented".

## How to use it
- List page objects hold a `table: Tables` (constructed as `GridTables` by default) and reassign it on
  `switchToView(view)`. Row actions delegate to the table (`subunitsList.deleteSubunit()` -> `table.deleteItem()`).
- **Row-id pattern (key idiom):** getters can't take arguments, so row-scoped selectors are private
  **methods** taking a `rowId`. `ListTables` resolves the row id from the identifier cell
  (`getItemRowId` etc.), then `getColumnSelector(columnName, rowId)` -> `data-testid="${rowId}_${columnName}"`.
  Follow this when adding a row helper.
- **Table in a drawer:** `new ListTables(page, drawerLocator)` scopes everything to the drawer (see
  `DownloadDocsDrawer`).

## Conventions & gotchas
- **Read a whole column in ONE `evaluateAll` snapshot, never cell-by-cell.** `getColumnValuesInRowOrder`
  does exactly that: a per-cell read loop interleaves with the table's re-render after a sort or a page
  change and returns a mix of old and new rows (observed as the first 1-3 values being stale, or blank
  zero-width placeholders, on an otherwise correctly sorted list). Keep any new column read atomic, and
  drop cells with no `data-row-index` - those are layout ghosts, not rows.
- **Action-menu labels nest as substrings, and the two match paths differ.** `clickOnOption` (behind
  `chooseItemAction` and the dedicated `archiveItem`/`pauseItem`/... helpers) filters by `hasText`, a
  case-insensitive *substring*: `"Activate"` is inside `"Deactivate"`, `"Archive"` inside
  `"Unarchive"`. Pass an anchored `RegExp` whenever the sibling label could also be on the menu.
  `verifyItemActionNotAvailable` instead resolves the accessible name with `exact: true` and takes a
  string or a `RegExp` - a substring filter there would match the sibling option, so an "archive is no
  longer offered" assertion would fail on the very row that just got archived.
- **List vs Grid use different selector schemes** - `ListTables` keys off `data-testid="${rowId}_${col}"`;
  `GridTables` off card attributes like `-card-${uuid}`. Don't cross them.
- **`Tables.discoverItemType` duck-types the seed object**, so a new listable entity needs a branch
  there, an identifier in `ListTables.getItemIdentifier`, a page size in `Tables.getPageSize`, and its
  column names in `ListColumns`. Add the branch to the FIRST key that is unique to that entity and
  check no earlier branch already claims it - the chain is ordered and the last arm falls through to
  a default entity, so a missed entity silently looks up the default entity's id key and matches nothing.
- Destructive ops wait for the relevant GraphQL response (`waitForGraphQLResponse`) before asserting.
- Imports `formatPrice`/`roundNumber` from `~/playwright/support/utils` and the integration variant logic
  from `~/playwright/support/integrationStatusFromSeed` - never `~/utils`.
- This is the canonical reference for the "no locators in specs / row helpers as private methods" rule
  cited by the `new-test` skill.

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | context menu [`../../modals/actionsMenu/`](../../modals/actionsMenu/CLAUDE.md) | confirm dialog [`../../modals/confirmation/`](../../modals/confirmation/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
