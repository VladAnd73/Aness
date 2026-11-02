---
name: analyzing-test-tickets
description: Use when a tester wants a full test-analysis brief for a {{PROJECT_NAME}} feature or bug starting from EITHER a {{TRACKER}} issue (e.g. "{{TRACKER_PROJECT_KEY}}-123", "analyze this ticket for testing", "what should I test") OR a {{CHAT_TOOL}} thread (a thread URL/permalink, "build test cases from this thread").
---

# Analyzing test tickets (tracker-first)

You are helping a tester build a complete picture of a feature or bug so they can write thorough,
well-informed test cases. Gather **all relevant data** - from {{TRACKER}} (primary), {{CHAT_TOOL}}, and
{{INTENT_SOURCE}} - and synthesise it into a **Feature Analysis Report**.

The input is one of two entry points; both converge on the same shared gathering + report body:

- **A {{TRACKER}} issue** (identifier like `{{TRACKER_PROJECT_KEY}}-123`, or an issue URL).
- **A {{CHAT_TOOL}} thread** (permalink, channel + thread timestamp, or a description of the thread).

Don't dump interim tool output to the user - show a brief "Gathering data..." status while working,
then present only the final report.

## Systems of record & tools (verify names before calling)

- **{{TRACKER}} = primary.** Use `{{TRACKER_TOOLS}}`. If your harness defers tool schemas, load them
  first (e.g. a tool search for the tracker) before calling. You need: get issue, list comments, list
  issues (filtered), search documentation, get project, list/get documents.
- **{{CHAT_TOOL}} = wired.** Use `{{CHAT_TOOLS}}`: read a full thread; search messages.
- **Secondary tracker = read-only FALLBACK only** (if your project still has one, use it only when an ID
  isn't in {{TRACKER}}). Use whatever read path is known to work for it; never write to it.
- **Product intent** comes via the **`{{INTENT_SKILL}}`** skill (see Phase C) - never a local product
  doc.

**Project / workspace assumptions:** {{TRACKER}} project key **`{{TRACKER_PROJECT_KEY}}`**; {{CHAT_TOOL}}
channels **`{{CHAT_DEV_CHANNEL}}`** (development discussion) and **`{{CHAT_QA_CHANNEL}}`** (QA reports
and bugs).

---

## Phase A - Resolve the entry point

### A1. If the input is a {{TRACKER}} issue
If no identifier was supplied, ask for one. Then fetch it (get issue, passing the identifier, e.g.
`{{TRACKER_PROJECT_KEY}}-123`). Capture: title, description (incl. acceptance criteria), type/estimate,
state, priority, assignee, creator, labels, project/cycle, parent, and any relations (blocks /
blocked-by / related / duplicate). Fetch its comments.

### A2. If the input is a {{CHAT_TOOL}} thread
Resolve the thread identifier:
- **Permalink** - derive the channel id and the thread timestamp from it, following {{CHAT_TOOL}}'s
  permalink format.
- **channel + thread timestamp** - use directly.
- **A description** ("the thread about member scoping in the dev channel") - locate it first with a
  message search, confirm with the user if ambiguous.

Read the full thread. For **every** message (incl. the root) capture author, timestamp, full text
(preserve error strings / field names / repro steps verbatim), reactions (high-reaction = important),
and attachments. Then extract:
- **Ticket IDs** - every `{{TRACKER_PROJECT_KEY}}-<n>` reference (capture all).
- **Feature / component** area (map to {{PROJECT_NAME}} domains).
- **Key claims / decisions**, **bug reports**, **open questions**, and any explicit "we should test X".
- **Author role signals** (dev / PO / QA / designer) where identifiable.

For each ticket ID found, fetch it as in A1 (issue + comments).

---

## Phase B - Gather the issue neighbourhood ({{TRACKER}} primary)

For the target issue (and any issues surfaced from a thread), map the neighbourhood:

- **Parent / project** - fetch the parent issue and the project for the broader story.
- **Relations** - for each related / blocking / blocked-by / duplicate issue, fetch it and note
  summary, state, type, and how it relates. Flag any **blocker still open / in progress** - it may
  affect testability.
  **Read each linked issue's ACCEPTANCE CRITERIA, not just its title and status.** A linked ticket's AC
  is an **inherited constraint on the change under test**, not background reading. Copy any AC that
  constrains the target change into section 3 and 6b verbatim.
  **The rule:** if a neighbour's AC names a population, state or behaviour that the target change does
  **not explicitly exclude**, that becomes a **mandatory negative scenario** in section 10, phrased as
  *"...and <the complement population> is unchanged."*
  *Why:* a constraint written as an AC on a sibling ticket - already phrased as a testable negative, and
  cited by name from the introducing ticket - is exactly the one nobody turns into a test unless this
  rule forces it.
- **Sub-issues** - fetch each; capture summary + state.
- **Additional related issues** - list issues scoped to the `{{TRACKER_PROJECT_KEY}}` project/team,
  filtered by label/state, to surface issues not explicitly linked (same component, recent open bugs =
  regression surface). Skim; pull full detail only for directly relevant ones.
- **Tracker docs** - if the area has a spec, a documentation or document search may surface it.

If an ID cannot be found in {{TRACKER}}, fall back to the secondary tracker (read-only) for that ID.

---

## Phase C - Load product domain context ({{INTENT_SOURCE}})

When the analysis turns on how the feature is *supposed* to behave - item phases and what each means,
unarchive rules, bundle instance statuses, trigger types, contact/notification flags, permission edges,
glossary terms - get the intended behavior from **{{INTENT_SOURCE}}** via the **`{{INTENT_SKILL}}`**
skill. That skill is the access path (it loads the right tools and navigates the KB: product map,
personas, glossary, user flows) and it carries the **authority rule**: code = how it *does*
behave (ground truth); KB = how it *should* behave (intent); **on a conflict, FLAG the gap - never
silently follow either.**

Use this to state **concrete expected values** and to surface risk areas the ticket/thread didn't
mention (e.g. a member-scoping fix that also touches analytics scoping). Note known complexity areas:
multi-tenant scoping, upstream system-of-record ({{VENDOR_B}}) integration, async polling, permissions.

---

## Phase D - Search {{CHAT_TOOL}} for context

Search the two project channels for discussion about the issue (exclude bots; about 10 results per
query).

- **By ticket ID:** the ticket ID scoped to `{{CHAT_DEV_CHANNEL}}`, then to `{{CHAT_QA_CHANNEL}}`.
- **By key terms:** 2-3 significant keywords from the title (avoid generic "add"/"fix"/"update").
  Only run keyword searches if the ID searches returned fewer than 3 results (avoids noise).

(If the entry point was already a {{CHAT_TOOL}} thread, this phase supplements it - skip searches that
just re-find the same thread.)

For each relevant message capture author, channel, approximate date, and a one-line summary. Focus on
QA observations / repro steps, dev decisions not reflected in the ticket, edge cases/workarounds, and
verify/re-open confirmations. Skip bots, CI noise, and tangential keyword hits.
`{{CHAT_QA_CHANNEL}}` often holds informal repro steps never captured in the tracker - treat it as high
value.

---

## Phase E - Synthesise the Feature Analysis Report

```
==========================================================================
FEATURE ANALYSIS REPORT - <TICKET-ID or THREAD TOPIC>
Source: <{{TRACKER}} issue {{TRACKER_PROJECT_KEY}}-NNN | {{CHAT_TOOL}} thread <channel>, date>
Generated for QA / Testing purposes
==========================================================================

## 1. Overview
- Source      : <tracker issue / chat thread>
- ID / Topic  : <{{TRACKER_PROJECT_KEY}}-NNN and title, or thread topic>
- Type        : <Story / Bug / Task / ...>
- Status      : <state>  |  Priority: <priority>
- Assignee    : <assignee>  |  Creator: <creator>
- Project     : <project/cycle>  |  Labels: <labels>
(For a chat thread: channel, date, participants, root message, thread length, and a 2-4 sentence
summary of what was decided/discovered + open questions.)

## 2. Feature Description
<Full description. Preserve acceptance criteria verbatim if present; otherwise extract implicit
requirements. For a Bug, include a "Steps to Reproduce" subsection.>

## 3. Acceptance Criteria
<Bullet list. Pull from the AC field/section if present; else derive from description + comments +
thread. Tag chat-only items "(from thread)" and derived items "(inferred)".
Then a second sub-list, **Inherited constraints** - ACs from LINKED tickets (Phase B) that constrain
this change, each tagged with its source ID, e.g. "({{TRACKER_PROJECT_KEY}}-NNN AC)". These are testable
requirements on this ticket even though they are written on another one. **Never leave this sub-list
off; write "none found" if there are none, so the reader knows it was checked.**>

## 4. Broader Context (Project / Epic / Parent)
<Where this issue sits in the larger story. Omit if none.>

## 5. Product Domain Context
<From {{INTENT_SOURCE}} (Phase C): which domain(s) are involved, relevant data-model relationships,
known complexities, and how this expands the test surface beyond the ticket/thread. If code and intent
disagree, FLAG the gap here.>

## 6. Related Tickets
### 6a. Sub-issues            | ID | Summary | Status |
### 6b. Linked / Relations    | ID | Relation | Summary | Status | **AC constraining this change** | Notes |
<The AC column is REQUIRED - quote the constraining criterion verbatim, or "-" if none. A row whose AC
column was never opened is an unread ticket, and an unread neighbour's AC is how a constraint ships
untested.>
### 6c. Same Project/Component | ID | Summary | Status | Relevance |
### 6d. Open Bugs, Same Area (Regression Watch) | ID | Summary | Priority | Status |

## 7. Dependency & Blocker Status
<Issues that block / are blocked by this one. Highlight any not-yet-done - they affect completeness.>

## 8. Key Decisions & Clarifications
### From ticket comments  <decision, scope change, QA note - commenter + approx date>
### From chat             <decision - author + approx date; summarise debates to the final call>

## 9. Testing Considerations
**In-Scope:** ...
**Out-of-Scope / Excluded:** ...
**Risk Areas / Edge Cases:** ...
**Domain-Specific Risks (from intent KB):** <phase transitions, trigger conditions, unarchive rules,
permission edges, state changes driven by the upstream system of record - whichever apply>
**Dependencies to Verify Before Testing:** ...
**Regression Surface:** ...

## 10. Suggested Test Scenarios (High-Level)
<5-12 scenarios derived from AC + related tickets + intent context. Starting points, not final cases:
happy path, negative/error, boundary/edge, multi-tenant/scoping, upstream integration, permissions,
regression.

**Two coverage rules, both mandatory:**
1. **No AC left untested - including inherited ones (section 3).** Every AC maps to at least one
   scenario, and the scenario asserts the AC's actual behaviour, not that a control exists. If an AC
   genuinely cannot be tested at this layer, say so explicitly and name the layer that can.
2. **Every AC naming a population gets its complement tested.** "X does Y for premium records" silently
   asserts "...and standard records are unaffected". Testing only the named population proves the
   feature works; it cannot detect the change bleeding into everything else. **Testing the feature's own
   population is not testing the change's blast radius** - the named-population specs can exist and
   pass while the complement breaks.>

==========================================================================
END OF REPORT
==========================================================================
```

---

## Phase F - Save the report

After displaying the report, save it (no confirmation needed) under `.test-cases/` in the current
working directory (create the folder if missing):

- **Tracker entry:** `.test-cases/<TICKET-ID>-analysis.md` (e.g.
  `.test-cases/{{TRACKER_PROJECT_KEY}}-1234-analysis.md`).
- **Chat entry:** `.test-cases/thread-<YYYY-MM-DD>-<topic-slug>-analysis.md`.

Add a metadata header at the very top:
```markdown
---
source: <tracker-issue | chat-thread>
id: <{{TRACKER_PROJECT_KEY}}-NNN or thread topic slug>
generated: <ISO 8601 date-time>
tool: analyzing-test-tickets skill
linked_tickets: [{{TRACKER_PROJECT_KEY}}-NNN, {{TRACKER_PROJECT_KEY}}-MMM]
---
```
Use the `Write` tool. Confirm with one line: `Report saved to .test-cases/<filename>.md`.

---

## Suggested intent-KB update (read-only - operator decides)

{{INTENT_SOURCE}} is **operator-maintained and human-gated**. This skill **must never edit the KB or
any product/source doc.** If the analysis revealed something the KB seems to miss or contradict,
**surface a suggested KB update to the operator** as a short read-only note appended after the report:

```
KB update suggestion (for operator review - NOT applied):
- Section: <which KB area - e.g. product map, personas, glossary, user flows>
- Observation: <what the ticket/thread revealed>
- Proposed change: <add / correct / clarify ...>
- Evidence: <{{TRACKER_PROJECT_KEY}}-NNN comment, thread quote, code path>
```

Never write to {{INTENT_SOURCE}}. If nothing qualifies, output: `KB update suggestion: none.` A
code-vs-intent gap is a **finding to flag**, per `{{INTENT_SKILL}}` - not something to fix by editing
the spec.

## Developer tickets are STRICTLY read-only (HARD RULE)

This skill **reads** tracker tickets; it **never mutates them**. A developer's ticket (any ticket NOT
owned/assigned to the operator) is **strictly read-only - never edit AND never comment**: no
description/status/assignee/label/field changes and no comments, by anyone including the coordinator.
You may only ever edit tickets owned/assigned to the operator. When the analysis warrants a change to a
developer-owned ticket, capture it in the report (or on the operator's own QA ticket) and surface it for
the operator - the most that may associate our work with a dev ticket is a "related to" relation FROM a
QA ticket assigned to the operator. Never write to the developer's ticket.

---

## Error handling

| Situation | Action |
|-----------|--------|
| No ID / thread supplied | Ask for a {{TRACKER}} identifier or a {{CHAT_TOOL}} permalink before proceeding |
| ID not in {{TRACKER}} | Fall back to the secondary tracker (read-only) for that ID; note the source in the report |
| Tracker tools not loaded | Load the deferred tool schemas first |
| Chat thread unresolvable | Ask for channel ID + thread timestamp directly |
| {{CHAT_TOOL}} unavailable | Skip Phase D; note "chat search skipped" in section 8 |
| Intent KB unreachable | Note it in section 5; proceed with ticket + chat data |
| No relations / comments | Proceed with what's available; note gaps |
| Search returns 0 | Note "no additional related tickets/messages found" |

## Related skills

- **`{{INTENT_SKILL}}`** - Phase C, the intended-behavior oracle.
- **`reviewing-e2e-coverage`** - sibling audit of EXISTING specs; reuses Phase B's inherited-AC rule.
- **`testing-a-feature-end-to-end`** - the full arc this analysis is the first step of.
- **`new-test`** - turning the suggested scenarios into specs.
- **`{{BUG_FILING_SKILL}}`** - filing a bug the analysis surfaces.
