package fleet

import "testing"

// Given steps held by a file condition and by a verdict condition,
// when the driver ticks past them repeatedly, then the coordinator is
// told once per held step and round, not never and not every tick.
func TestBlockedStepNotifiesOnce(t *testing.T) {
	f := newLoopFixture(t, "gated", "gated.toml", "alpha", "beta")
	f.tick(t)
	f.write(t, "a/round-1.json", `{"result":"x"}`)
	f.tick(t)
	f.tick(t)
	if n := countContaining(inboxBodies(t, f.inbox), "step a round 1"); n != 1 {
		t.Errorf("notices for a held by its file condition = %d, want 1: %v", n, inboxBodies(t, f.inbox))
	}

	f.write(t, "a/extra-1.txt", "")
	f.tick(t)
	f.write(t, "b/round-1.json", verdictJSON("b", "request_changes"))
	f.tick(t)
	f.tick(t)
	if n := countContaining(inboxBodies(t, f.inbox), "step b round 1"); n != 1 {
		t.Errorf("notices for b held by its verdict condition = %d, want 1: %v", n, inboxBodies(t, f.inbox))
	}
}
