## Worker TDD (acceptance-scenario driven)

Every improvement to spore is specified as acceptance scenarios first,
then built test-first. The end goal is not "code written"; it is "the
acceptance scenarios pass". A change with no failing-then-passing test
for its stated goal is not done.

When you mint an improvement task, put an **Acceptance scenarios**
section in the brief. List the concrete behaviors the change must
exhibit, each as "given X, when Y, then Z". Together they must cover
the end goal. At least one must exercise the full flow end to end, not
a single unit in isolation.

When you implement an improvement task, before you write any
implementation code:

- Turn each acceptance scenario into a test.
- Run them. Confirm they fail for the right reason (red).
- Implement until they pass (green).
- Deliver only when the acceptance tests and `just check` are green.

The whole acceptance suite goes red before any implementation exists.
Red/green per task is not enough: a test written after its code is
biased by that code. When you write an implementation plan, its first
task writes every acceptance test (stubs only so they compile) and
proves each one fails on an assertion, not on a panic or setup error.
A test that passes against stubs tests nothing; fix it.

When you mint a task whose worker changes code, name
`superpowers:test-driven-development` in the brief and say to invoke
it before writing any code.
