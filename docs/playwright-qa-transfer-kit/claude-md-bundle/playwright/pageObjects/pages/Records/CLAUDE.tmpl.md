# pages/Records - Record list & detail (main area)

Parent: [`../CLAUDE.md`](../CLAUDE.md).

## What's here
- <list page object (`/main/records`): class name, grid/list views and page size, create/search/
  filter/sort/remove/open methods and the GraphQL ops they wait for, its `validateXList` helper, the
  per-row state validator (and which view it works in), fixture name>
- <detail page object: class name, field validators, state-change action, the report-app link helpers,
  the two independently paginated sub-tables it holds, fixture name>

## Conventions & gotchas
- **A search helper waits for the query it triggers - except below the minimum length**, where no query
  fires; there it only races the wait instead of hanging on it.
- **After the UI changes a value, assert it with a single-field oracle that reads the page**, not with a
  whole-object validator that reads the expected value off the seed object - the seed object is stale
  after the change.
- **Two validators with overlapping locators but different contracts** (one takes literal expected
  strings and can poll for an async write-back; one derives its expectations from the seed object) may
  co-exist. Collapsing them changes what existing specs assert, so do it as its own change, not inside an
  unrelated one.
- A row-state validator that only one view renders must say so; the other view has no such cell.
- <pagination mode and page size; grid vs list column selectors; the report-app URL pattern>

## Cross-links
Parent [`../CLAUDE.md`](../CLAUDE.md) | drawer [`../../components/drawers/record/`](../../components/drawers/record/CLAUDE.md) | specs [`../../../tests/main/record/`](../../../tests/main/record/CLAUDE.md).

> **Maintenance:** when you add, move, rename, or delete a file in this folder, update this CLAUDE.md in the same change.
