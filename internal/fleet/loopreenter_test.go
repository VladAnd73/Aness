package fleet

import (
	"strings"
	"testing"
	"time"
)

// Given a step reaped on forward exit and re-entered after a later step
// sent work back, when its pane is spawned fresh at round 2, then its
// step note names round 2's path as the file to write now, not round 1.
func TestReenteredStepNoteNamesTheRoundToWrite(t *testing.T) {
	f := newLoopFixture(t, "reenter", "reenter.toml", "author", "reviewer")
	review := f.session("review")

	f.tick(t)
	f.write(t, "author/round-1.json", `{"notes":"r1"}`)
	f.tick(t)
	f.write(t, "review/round-1.json", `{"notes":"ok"}`)
	f.tick(t)
	f.write(t, "comp/round-1.json", verdictJSON("comp", "request_changes"))
	f.tick(t)
	f.write(t, "author/round-2.json", `{"notes":"r2"}`)
	f.panes.now = f.panes.now.Add(quickDeathWindow + time.Minute)
	f.tick(t)

	notes := f.panes.notes[review]
	if len(notes) != 2 {
		t.Fatalf("review spawned %d times, want 2 (fresh at round 1, re-entered at round 2): %v", len(notes), f.panes.spawns)
	}
	if want := f.path("review/round-1.json"); !strings.Contains(notes[0], "Write round 1 now: "+want) {
		t.Errorf("first review note lacks %q:\n%s", "Write round 1 now: "+want, notes[0])
	}
	if want := f.path("review/round-2.json"); !strings.Contains(notes[1], "Write round 2 now: "+want) {
		t.Errorf("re-entered review note lacks %q:\n%s", "Write round 2 now: "+want, notes[1])
	}
}

// Given a persisted step idling behind the cursor, when its pane is
// respawned, then its note names its next round but says to wait for a
// wake instead of writing now.
func TestIdlePersistedStepNoteSaysWait(t *testing.T) {
	f := newLoopFixture(t, "reenter", "reenter.toml", "author", "reviewer")
	author := f.session("author")
	f.tick(t)
	f.write(t, "author/round-1.json", `{"notes":"r1"}`)
	f.tick(t)
	killLoopFakePane(f, author)
	f.panes.now = f.panes.now.Add(quickDeathWindow + time.Minute)
	f.tick(t)

	notes := f.panes.notes[author]
	if len(notes) != 2 {
		t.Fatalf("author spawned %d times, want 2: %v", len(notes), f.panes.spawns)
	}
	if want := "next output is round 2: " + f.path("author/round-2.json"); !strings.Contains(notes[1], want) || !strings.Contains(notes[1], "wait") {
		t.Errorf("respawned idle author note lacks %q and a wait instruction:\n%s", want, notes[1])
	}
}

func killLoopFakePane(f *loopFixture, session string) {
	delete(f.panes.panes, session)
}
