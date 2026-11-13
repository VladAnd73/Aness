package fleet

import (
	"testing"
	"time"
)

func TestSpawnBackoffDoublesPerQuickDeathAndResetsOnALongRun(t *testing.T) {
	b := &spawnBackoff{recs: map[string]*spawnRecord{}}
	t0 := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	active := map[string]bool{"w": true}

	if d := b.wait("w", t0); d != 0 {
		t.Fatalf("first spawn waits %s, want 0", d)
	}
	b.spawned("w", t0)
	if d := b.wait("w", t0.Add(30*time.Second)); d != 30*time.Second {
		t.Fatalf("after one quick death wait %s, want the rest of 1m", d)
	}
	if d := b.wait("w", t0.Add(30*time.Second)); d != 30*time.Second {
		t.Fatalf("re-checking the same death counted it twice: wait %s", d)
	}
	b.spawned("w", t0.Add(time.Minute))
	if d := b.wait("w", t0.Add(time.Minute)); d != 2*time.Minute {
		t.Fatalf("after two quick deaths wait %s, want 2m", d)
	}
	b.failed("w", t0.Add(3*time.Minute))
	if d := b.wait("w", t0.Add(3*time.Minute)); d != 4*time.Minute {
		t.Fatalf("after a failed spawn wait %s, want 4m", d)
	}

	b.spawned("w", t0.Add(7*time.Minute))
	b.prune(map[string]bool{"w": true}, active, t0.Add(20*time.Minute))
	if d := b.wait("w", t0.Add(20*time.Minute)); d != 0 {
		t.Fatalf("a worker that ran past the quick-death window still backs off %s", d)
	}
}
