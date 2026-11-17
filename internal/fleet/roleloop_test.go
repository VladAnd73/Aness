package fleet

import (
	"context"
	"testing"
	"time"

	"github.com/versality/spore/internal/task"
)

// stepFn mutates the artifact tree to advance the role loop by one
// step. Used to script the state-machine walk in tests.
type stepFn func(t *testing.T, root, slug string)

func writeEng(round int) stepFn {
	return func(t *testing.T, root, slug string) {
		t.Helper()
		if err := task.WriteEngineerResponse(root, slug, round, task.EngineerResponse{
			Addressed: []string{"placeholder"},
		}); err != nil {
			t.Fatalf("WriteEngineerResponse round=%d: %v", round, err)
		}
	}
}

func writeRev(instance task.ReviewerInstance, round int, verdict string) stepFn {
	return func(t *testing.T, root, slug string) {
		t.Helper()
		if err := task.WriteReview(root, slug, instance, round, task.Review{
			Verdict: verdict,
			Summary: "test verdict",
		}); err != nil {
			t.Fatalf("WriteReview %s round=%d: %v", instance, round, err)
		}
	}
}

func TestDeriveSnapshotHappyPath(t *testing.T) {
	root := t.TempDir()
	slug := "demo"

	check := func(stepIdx int, want Snapshot) {
		t.Helper()
		got, err := DeriveSnapshot(root, slug)
		if err != nil {
			t.Fatalf("step %d: DeriveSnapshot: %v", stepIdx, err)
		}
		got.Slug = "" // do not assert on slug in each row
		if got != want {
			t.Errorf("step %d: snapshot mismatch\n want: %+v\n got:  %+v", stepIdx, want, got)
		}
	}

	// 0. Fresh tree, no artifacts: engineer-initial.
	check(0, Snapshot{Phase: PhaseEngineerInitial})

	// 1. Engineer ships initial response.
	writeEng(1)(t, root, slug)
	check(1, Snapshot{
		Phase:           PhaseReviewA,
		EngineerRound:   1,
		CurrentReviewer: task.ReviewerA,
	})

	// 2. A approves round 1 -> review-B (B not yet spawned).
	writeRev(task.ReviewerA, 1, task.VerdictApprove)(t, root, slug)
	check(2, Snapshot{
		Phase:           PhaseReviewB,
		EngineerRound:   1,
		CurrentReviewer: task.ReviewerB,
	})

	// 3. B approves round 1 -> done.
	writeRev(task.ReviewerB, 1, task.VerdictApprove)(t, root, slug)
	check(3, Snapshot{
		Phase:           PhaseDone,
		EngineerRound:   1,
		CurrentReviewer: task.ReviewerB,
		ReviewerRound:   1,
		LastVerdict:     task.VerdictApprove,
	})
}

func TestDeriveSnapshotReviseLoop(t *testing.T) {
	root := t.TempDir()
	slug := "revise"

	writeEng(1)(t, root, slug)
	writeRev(task.ReviewerA, 1, task.VerdictRequestChanges)(t, root, slug)

	got, err := DeriveSnapshot(root, slug)
	if err != nil {
		t.Fatalf("DeriveSnapshot: %v", err)
	}
	want := Snapshot{
		Slug:            slug,
		Phase:           PhaseEngineerReviseA,
		EngineerRound:   1,
		CurrentReviewer: task.ReviewerA,
		ReviewerRound:   1,
		LastVerdict:     task.VerdictRequestChanges,
	}
	if got != want {
		t.Errorf("after A round 1 request_changes\n want: %+v\n got:  %+v", want, got)
	}

	// Engineer responds (round 2): back to review-A waiting on A round 2.
	writeEng(2)(t, root, slug)
	got, _ = DeriveSnapshot(root, slug)
	want = Snapshot{
		Slug:            slug,
		Phase:           PhaseReviewA,
		EngineerRound:   2,
		CurrentReviewer: task.ReviewerA,
		ReviewerRound:   1,
		LastVerdict:     task.VerdictRequestChanges,
	}
	if got != want {
		t.Errorf("after engineer round 2\n want: %+v\n got:  %+v", want, got)
	}

	// A round 2 approves -> review-B (engineer is still at round 2; B
	// hasn't run yet).
	writeRev(task.ReviewerA, 2, task.VerdictApprove)(t, root, slug)
	got, _ = DeriveSnapshot(root, slug)
	want = Snapshot{
		Slug:            slug,
		Phase:           PhaseReviewB,
		EngineerRound:   2,
		CurrentReviewer: task.ReviewerB,
	}
	if got != want {
		t.Errorf("after A round 2 approve\n want: %+v\n got:  %+v", want, got)
	}

	// B requests changes round 1.
	writeRev(task.ReviewerB, 1, task.VerdictRequestChanges)(t, root, slug)
	got, _ = DeriveSnapshot(root, slug)
	want = Snapshot{
		Slug:            slug,
		Phase:           PhaseEngineerReviseB,
		EngineerRound:   2,
		CurrentReviewer: task.ReviewerB,
		ReviewerRound:   1,
		LastVerdict:     task.VerdictRequestChanges,
	}
	if got != want {
		t.Errorf("after B round 1 request_changes\n want: %+v\n got:  %+v", want, got)
	}

	// Engineer round 3 (revising B), then B approves round 2 -> done.
	writeEng(3)(t, root, slug)
	writeRev(task.ReviewerB, 2, task.VerdictApprove)(t, root, slug)
	got, _ = DeriveSnapshot(root, slug)
	want = Snapshot{
		Slug:            slug,
		Phase:           PhaseDone,
		EngineerRound:   3,
		CurrentReviewer: task.ReviewerB,
		ReviewerRound:   2,
		LastVerdict:     task.VerdictApprove,
	}
	if got != want {
		t.Errorf("after B round 2 approve\n want: %+v\n got:  %+v", want, got)
	}
}

// TestDeriveSnapshotEscalateAtRound3A drives the negative path called
// out in the spec: a reviewer A that never approves; the loop must
// land in PhaseEscalated on the round-3 request_changes verdict and
// stay there.
func TestDeriveSnapshotEscalateAtRound3A(t *testing.T) {
	root := t.TempDir()
	slug := "escalate-a"

	writeEng(1)(t, root, slug)
	writeRev(task.ReviewerA, 1, task.VerdictRequestChanges)(t, root, slug)
	writeEng(2)(t, root, slug)
	writeRev(task.ReviewerA, 2, task.VerdictRequestChanges)(t, root, slug)
	writeEng(3)(t, root, slug)
	writeRev(task.ReviewerA, 3, task.VerdictRequestChanges)(t, root, slug)

	got, err := DeriveSnapshot(root, slug)
	if err != nil {
		t.Fatalf("DeriveSnapshot: %v", err)
	}
	want := Snapshot{
		Slug:            slug,
		Phase:           PhaseEscalated,
		EngineerRound:   3,
		CurrentReviewer: task.ReviewerA,
		ReviewerRound:   3,
		LastVerdict:     task.VerdictRequestChanges,
	}
	if got != want {
		t.Fatalf("escalation snapshot mismatch\n want: %+v\n got:  %+v", want, got)
	}

	// Engineer can keep writing but phase stays escalated until A is
	// reset (or operator force-approves) - the loop should not be
	// fooled into looking like review-A round 4 is pending.
	writeEng(4)(t, root, slug)
	got, _ = DeriveSnapshot(root, slug)
	if got.Phase != PhaseEscalated {
		t.Errorf("phase after engineer round 4 = %s, want %s", got.Phase, PhaseEscalated)
	}
}

// TestDeriveSnapshotEscalateAtRound3B verifies the same cap applies
// independently to B after A has cleared.
func TestDeriveSnapshotEscalateAtRound3B(t *testing.T) {
	root := t.TempDir()
	slug := "escalate-b"

	writeEng(1)(t, root, slug)
	writeRev(task.ReviewerA, 1, task.VerdictApprove)(t, root, slug)
	writeRev(task.ReviewerB, 1, task.VerdictRequestChanges)(t, root, slug)
	writeEng(2)(t, root, slug)
	writeRev(task.ReviewerB, 2, task.VerdictRequestChanges)(t, root, slug)
	writeEng(3)(t, root, slug)
	writeRev(task.ReviewerB, 3, task.VerdictRequestChanges)(t, root, slug)

	got, err := DeriveSnapshot(root, slug)
	if err != nil {
		t.Fatalf("DeriveSnapshot: %v", err)
	}
	want := Snapshot{
		Slug:            slug,
		Phase:           PhaseEscalated,
		EngineerRound:   3,
		CurrentReviewer: task.ReviewerB,
		ReviewerRound:   3,
		LastVerdict:     task.VerdictRequestChanges,
	}
	if got != want {
		t.Errorf("B escalation snapshot mismatch\n want: %+v\n got:  %+v", want, got)
	}
}

func TestWatcherTickReportsChange(t *testing.T) {
	root := t.TempDir()
	slug := "tick"
	w := NewRoleWatcher(root, slug)

	// First tick: bootstrap = changed.
	snap, changed, err := w.Tick()
	if err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	if !changed {
		t.Errorf("first Tick changed=false, want true (bootstrap)")
	}
	if snap.Phase != PhaseEngineerInitial {
		t.Errorf("first Tick phase=%s, want %s", snap.Phase, PhaseEngineerInitial)
	}

	// No artifact change: changed=false.
	_, changed, err = w.Tick()
	if err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	if changed {
		t.Errorf("second Tick changed=true on unchanged tree")
	}

	// Engineer writes round 1: tick reports change.
	writeEng(1)(t, root, slug)
	snap, changed, err = w.Tick()
	if err != nil {
		t.Fatalf("third Tick: %v", err)
	}
	if !changed {
		t.Errorf("third Tick changed=false after artifact write")
	}
	if snap.Phase != PhaseReviewA {
		t.Errorf("third Tick phase=%s, want %s", snap.Phase, PhaseReviewA)
	}
}

func TestWatcherTerminalAfterDone(t *testing.T) {
	root := t.TempDir()
	slug := "terminal"
	w := NewRoleWatcher(root, slug)

	writeEng(1)(t, root, slug)
	writeRev(task.ReviewerA, 1, task.VerdictApprove)(t, root, slug)
	writeRev(task.ReviewerB, 1, task.VerdictApprove)(t, root, slug)

	if _, _, err := w.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if !w.Terminal() {
		t.Errorf("Terminal()=false after done snapshot")
	}
}

func TestWatcherRunEmitsAndClosesOnTerminal(t *testing.T) {
	root := t.TempDir()
	slug := "run"

	// Pre-seed a fully-approved tree so the first Tick lands in
	// PhaseDone and the Run loop emits + exits.
	writeEng(1)(t, root, slug)
	writeRev(task.ReviewerA, 1, task.VerdictApprove)(t, root, slug)
	writeRev(task.ReviewerB, 1, task.VerdictApprove)(t, root, slug)

	w := NewRoleWatcher(root, slug)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, errs := w.Run(ctx, 10*time.Millisecond)

	var got []Snapshot
	for snap := range events {
		got = append(got, snap)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	}
	if len(got) != 1 {
		t.Fatalf("events = %d, want 1 (terminal-on-bootstrap)", len(got))
	}
	if got[0].Phase != PhaseDone {
		t.Errorf("event phase = %s, want %s", got[0].Phase, PhaseDone)
	}
}
