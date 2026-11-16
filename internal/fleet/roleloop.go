package fleet

import (
	"errors"
	"fmt"

	"github.com/versality/spore/internal/task"
)

// Phase is the current step in a task's role-loop. Derived from the
// artifacts on disk under <projectRoot>/.spore/<slug>/.
type Phase string

const (
	// PhaseEngineerInitial: no engineer response yet. Coordinator
	// should spawn the engineer pane (or wait for the existing one to
	// write its first response).
	PhaseEngineerInitial Phase = "engineer-initial"

	// PhaseReviewA: engineer has shipped a response; A has not
	// written a verdict for it yet. The A pane (still alive if mid-
	// loop, else freshly spawned) owns the next file write.
	PhaseReviewA Phase = "review-A"

	// PhaseEngineerReviseA: A's latest verdict is request_changes and
	// the engineer has not responded to it. The engineer owns the
	// next file write.
	PhaseEngineerReviseA Phase = "engineer-revise-A"

	// PhaseReviewB: A approved; B is either not yet spawned (no B
	// verdict on disk) or alive and processing the engineer's latest
	// response. The B pane owns the next file write.
	PhaseReviewB Phase = "review-B"

	// PhaseEngineerReviseB: B's latest verdict is request_changes and
	// the engineer has not responded to it.
	PhaseEngineerReviseB Phase = "engineer-revise-B"

	// PhaseDone: B approved. The branch is ready for the operator.
	PhaseDone Phase = "done"

	// PhaseEscalated: the current reviewer issued request_changes at
	// round MaxReviewerRounds. Panes are left alive; the operator
	// decides whether to clarify spec, force-approve, kill the task,
	// or hand back for another loop.
	PhaseEscalated Phase = "escalated"
)

// MaxReviewerRounds caps each reviewer phase. A request_changes
// verdict at this round escalates instead of looping for round N+1.
const MaxReviewerRounds = 3

// Snapshot is the current view of a task's role-loop state.
type Snapshot struct {
	Slug string
	// Phase is the derived loop step.
	Phase Phase
	// EngineerRound is the highest engineer-round-N.json present
	// (== count, since rounds are dense). Zero before the engineer
	// has shipped anything.
	EngineerRound int
	// CurrentReviewer is the reviewer instance whose phase we are
	// in. Empty in PhaseEngineerInitial.
	CurrentReviewer task.ReviewerInstance
	// ReviewerRound is the number of verdicts the current reviewer
	// has written. Zero before the first verdict of the phase.
	ReviewerRound int
	// LastVerdict is the verdict on the latest reviewer round in
	// the current phase, or empty when none has been written yet.
	LastVerdict string
}

// DeriveSnapshot reads `<projectRoot>/.spore/<slug>/` and returns the
// implied state: the bundled `dev` loop run by the loop interpreter. A
// missing tree returns a PhaseEngineerInitial snapshot (no error), so
// callers can drive the loop forward without first preparing the tree.
// Files that fail validation are not counted: a bad file must neither
// wedge the loop nor skew the round math. DriveRoleLoop bounces them
// back to their author.
func DeriveSnapshot(projectRoot, slug string) (Snapshot, error) {
	def, err := bundledDevLoop()
	if err != nil {
		return Snapshot{}, err
	}
	return DeriveLoopSnapshot(projectRoot, slug, def)
}

func validEngineerRounds(projectRoot, slug string) ([]int, error) {
	rounds, err := task.ListEngineerRounds(projectRoot, slug)
	if err != nil {
		return nil, err
	}
	out := rounds[:0]
	for _, r := range rounds {
		_, err := task.ReadEngineerResponse(projectRoot, slug, r)
		if errors.Is(err, task.ErrInvalidRoleFile) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read engineer round %d: %w", r, err)
		}
		out = append(out, r)
	}
	return out, nil
}
