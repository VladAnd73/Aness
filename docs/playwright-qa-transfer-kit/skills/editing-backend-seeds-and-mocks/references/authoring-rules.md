# Authoring rules: backend seeds and mocks

Rules for writing test-only backend seeds (the `{{SEED_MECHANISM}}` commands under `{{SEED_DIR}}`) and
`{{MOCK_TOOL}}` mock environments (in `{{MOCKS_HOME}}`). Not about the app's own development seed data or
load-test fixtures. Governance - where you may edit, and the ack you need first - is in
[../SKILL.md](../SKILL.md).

The reference stack was a Rails backend with factory-built records and Mockoon mocks; the factory, trait and
helper syntax below is that stack's. Keep the rules and re-bind the syntax to your own stack.

---

## Backend seed rules

### Where seeds live

```
{{SEED_DIR}}/
  <area>/         setup seeds, one subfolder per product area
  transitions/    state advancement only (go live, drain async jobs, ...)
```

Transition seeds that advance state belong in `transitions/`, not next to setup seeds.

### How tests call seeds

Playwright calls a seed by path through the support layer (`server.seed('seeds/<area>/<name>')`, prefix
included). Every test auto-runs a clean and then the admin login; domain data is seeded in `beforeEach`.
The admin user comes from a factory on login, not from domain seeds.

### Authoring

1. Follow the backend's file header conventions (e.g. the frozen-string-literal magic comment in Ruby).
2. Enable **feature flags before any data creation** - only the flags the scenario needs.
3. Always **return serialized JSON** at the end.
4. Use **factories** for all records - no raw model `create!`. For multiple records use the list form:
   ```ruby
   units = FactoryBot.create_list(:unit, 5, parent: parent)
   ```
5. Prefer **traits** over magic strings and integers (`:live`, `:with_owner`).
6. **Navigate factory associations** - do not create orphaned duplicate tenants or records.
7. Use **model constants** where they exist.
8. Bypass validation (`save!(validate: false)`, `update_columns`) only for otherwise unreachable state.
9. Calling **services and jobs directly** in seeds is normal.
10. Read test params from the seed's **options hash** (`fetch` for required, a default for optional).
11. The returned hash exposes **only what the test needs**, with clear key names.
12. **Serializing a record returns its columns only** - associations are omitted unless added explicitly.
    Use the **splat-and-extend pattern**:
    ```ruby
    { **record.attributes, product: record.product,
      integration_configuration: record.product.integration_configuration }
    ```
    Prefer this over `as_json(include: ...)` for consistency.
13. **Know which associations are polymorphic** - do not assume two entities share a child row because
    their factories look alike.

### Naming and organisation

14. Name files after **what they set up**, not the spec that uses them (`item_bundle_live.rb`, not
    `test_for_foo_spec.rb`).
15. Use subfolders by area and specificity.
16. **Reuse** a seed when its preconditions are fully covered (an optional param is fine).
17. **Create a new seed** when flags, lifecycle state or relationships differ meaningfully.
18. **Prefer a focused new seed over conditional branching** in one file.

### Seed file template

```ruby
# frozen_string_literal: true

FeatureFlag.enable!(:relevant_flag)

tenant = FactoryBot.create(:tenant, :with_defaults, name: 'seeded tenant')
record = FactoryBot.create(:record, tenant:)

{
  tenant:,
  record:
}.to_json
```

### Domain rules

When seeding entities with a lifecycle:

- **Lifecycle states** (the reference's item phases: pending, scheduled, live, paused, finished,
  archived) are set with factory traits, never raw integers.
- **Timestamps that drive transitions** - e.g. a publish timestamp that decides where an unarchived item
  lands - are set or left nil deliberately, never incidentally.
- **Derived statuses** (a bundle instance's status derived from cancellation, change and child-phase
  timestamps) are produced by setting the inputs, not by writing the derived value.
- **Recipients and stakeholders** are created explicitly when the test asserts a notification.
- **Activation flags** that put a record into a polling loop are set only when the test needs the loop.

Read the backend's own subsystem docs for exact column names, nullable flags and the facades that set them,
and consult `{{INTENT_SOURCE}}` via `{{INTENT_SKILL}}` for what a state *means* (code = ground truth;
intent = how it should behave; flag conflicts, never silently follow either).

### TypeScript

- Seed types go in the suite's shared seed-types module - **never inline in test files**.
- Types must match the **actual JSON shape**; update them when a seed's structure changes.
- Helpers that derive expected state from seed data must read the **same paths** the seed exposes.

### Scope

- E2e fixes may edit seeds; do **not** change application source to make tests pass unless explicitly
  asked.
- Prefer fixing the **seed or the test**, not both, unless both are genuinely required.
- No dedicated unit tests for seed files; follow the backend's linter.

### Checklist

- [ ] Backend file header conventions
- [ ] Feature flags before data
- [ ] Factories + traits
- [ ] Associations navigated, not duplicated
- [ ] Validation bypasses only for unreachable state
- [ ] Options hash for params
- [ ] Minimal return hash, correct keys
- [ ] Correct subfolder and filename

---

## Mock rules

### Scope

A mock simulates an upstream HTTP service the app talks to in e2e - in the reference, `{{VENDOR_A}}` (a
distribution API whose pause/archive/activate calls the suite asserts) and `{{VENDOR_B}}` (an upstream
system of record feeding an async pipeline). The same authoring rules apply to every mock environment.

### Build routes from the third-party's OWN API docs - never from imagination

A mock is only useful if it matches the real upstream contract. **The endpoint path, method, request shape,
response schema, field names, types and status codes MUST come from the vendor's own API documentation
(`{{VENDOR_API_DOCS_URL}}`) - never invented or guessed.** If you cannot find the contract for a route in
the vendor docs (or a recorded cassette of the real service), do NOT fabricate one - stop and flag it.

Cite the doc section (or the cassette) you took each route's shape from, in the route's documentation
field, and quote the exact contract where it is surprising (e.g. a status callback that returns
**204 No Content, not 200**).

**If the app under test does NOT respect the vendor's documentation** - it calls a path the vendor doesn't
define, sends a shape the vendor rejects, or expects a response the vendor doesn't return - **that is a bug
in our code, and it must be flagged HIGHLY.** Do NOT bend the mock to match non-conformant code and call
it green: the mock follows the vendor spec; where our code diverges, the code is wrong.

### Dynamic data is mandatory

**Every route returns as much DYNAMIC data as possible.** A route that hardcodes ids, dates and echo-able
fields is a bug to fix, not a baseline to copy. Sources of dynamism, in priority order (Mockoon syntax):

**1. Echo what the caller sent.** If a value arrives in the request and the real service would return it,
read it back from the request - never hardcode it.

| Source | Helper | Example |
|--------|--------|---------|
| JSON body field (stringified) | `{{body 'path' 'default'}}` | `"external_id": "{{body 'external_id'}}"` |
| Body value with type/array/object preserved | `{{bodyRaw 'path'}}` | inside `{{#each}}`/`{{#if}}` |
| Named route param (`:recordId` in the endpoint) | `{{urlParam 'recordId'}}` | `"recordId": "{{urlParam 'recordId'}}"` |
| Query param | `{{queryParam 'name' 'default'}}` / `{{queryParamRaw 'name'}}` | |
| Header | `{{header 'Header-Name' 'default'}}` | |

Example: a status-update route receives `external_id` and `status`, so the response echoes them -
`{ "message": "Updated", "external_id": "{{body 'external_id'}}", "status": "{{body 'status'}}" }`.

**2. Faker for values the service ORIGINATES** (ids, names, numbers the caller did not supply):
`{{faker 'string.uuid'}}`, `{{faker 'number.int' min=1 max=9999}}`, `{{faker 'string.numeric' length=12}}`,
`{{faker 'person.firstName'}}`.

**3. Dates - use the mock tool's date helpers, NOT faker, when the format matters.** Faker's date methods
return a Date object rendered as a non-ISO string and take no format argument. For a formatted past or
future date:

| Need | Helper | Example (ISO 8601) |
|------|--------|--------------------|
| Now | `{{now 'format'}}` | `{{now "yyyy-MM-dd'T'HH:mm:ss'Z'"}}` |
| N days in the FUTURE | `{{dateTimeShift format='...' days=N}}` | `{{dateTimeShift format="yyyy-MM-dd'T'HH:mm:ss'Z'" days=30}}` |
| N days in the PAST | `{{dateTimeShift ... days=-N}}` | `{{dateTimeShift format="yyyy-MM-dd'T'HH:mm:ss'Z'" days=-30}}` |
| Random date in a range | `{{date 'from' 'to' 'format'}}` | `{{date '2024-01-01' (now 'yyyy-MM-dd') "yyyy-MM-dd'T'HH:mm:ss'Z'"}}` |

**4. Stateful callbacks - capture on the request, replay on the callback.** When the app POSTs a task and
the mock later fires a callback, store the inbound body in a data bucket and read it back so the callback
references real request values:

```
# on the task-creation route body:
{{setData 'set' 'VendorTasks' null (bodyRaw)}}
# in the callback body:
{{#with (dataRaw 'VendorTasks' '$')}}
{ "result": [ { "callback_reference": "{{entries.0.callback_reference}}",
                "external_id": "{{faker 'string.uuid'}}", "errors": [] } ],
  "reference": "{{task_reference}}" }
{{/with}}
```

### Dynamic-data gotchas (verify before you claim it works)

- **Templating must be enabled on the response** (`disableTemplating: false` in Mockoon), or every
  template tag is returned literally. Flip it, restart the mock, and confirm the rendered body over HTTP.
- **Templated output must stay valid JSON.** Quote string helpers; use the raw helper (unquoted) for
  numbers, booleans, arrays and objects; guard trailing commas in loops.
- **Dynamic is not random. Preserve the consumer contract.** Echo the SAME id the caller correlates on.
  Keep a date on the side the consumer requires (`days=-N` stays past, `days=N` stays future). Prefer
  relative dates over hardcoded years so fixtures don't age.
- **Prove it, don't assume it.** Restart the mock after any edit, hit the route, and read the rendered
  response. A template tag that appears verbatim means templating is off or the helper/path is wrong.

### Setup order

1. Start the mock servers **before** the backend test server.
2. Start the backend with its outbound vendor URL pointed at the mock
   (`{{VENDOR_A_OUTBOUND_URL_ENV}}=http://localhost:{{VENDOR_A_MOCK_PORT}}`) on the command line. The
   backend's test env file may default to a real remote; without the override, mock assertions fail.

### Proxy mode

If a mock environment proxies unmatched requests to a real upstream (e.g. the vendor's staging):

- Local routes return their status and appear in the mock's admin logs.
- Proxied requests may not match test assertions, and can break teardown flows.

**Rule:** for every endpoint a teardown or stop flow calls, the mock must define a local **2xx** route. A
missing route can make a transactional backend operation fail one vendor call and roll back the whole
teardown, leaving the item **live**.

### Playwright verification

- Assertions read the **mock's admin transaction log**, not backend logs.
- The per-test fixture purges the logs before every test; tests match entries **since the last purge**.
- The test-side destination-to-path mapping (in the vendor-A mock helper) must stay aligned with the mock
  environment's routes: when you add a destination or status route, update the mapping in the same change.
- Match on the specific entity (path + status in the body + the entity's identifier), never on the path
  alone.

---

## Seeds + mocks together

1. **The initial lifecycle state from the seed** decides whether the vendor mock receives archive, pause
   or activate calls on archive/unarchive flows.
2. **A flow that ends an item** needs every vendor route it calls to return 2xx locally.
3. **Transactional backend operations that raise on a vendor error** roll back entirely when one vendor
   call fails.
4. Typical flow: seed a live item -> UI action (or `{{DRAIN_COMMAND}}`) -> assert the lifecycle state in
   the UI -> assert the vendor call at the mock log.
