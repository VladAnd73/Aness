package fleet

import "github.com/versality/spore/internal/task"

// Compile stub for the configurable-loops acceptance tests. The task
// that ports upstream's internal/fleet/roleloop.go replaces this file.

type Phase string

const (
	PhaseEngineerInitial Phase = "engineer-initial"
	PhaseReviewA         Phase = "review-A"
	PhaseEngineerReviseA Phase = "engineer-revise-A"
	PhaseReviewB         Phase = "review-B"
	PhaseEngineerReviseB Phase = "engineer-revise-B"
	PhaseDone            Phase = "done"
	PhaseEscalated       Phase = "escalated"
)

const MaxReviewerRounds = 3

type Snapshot struct {
	Slug            string
	Phase           Phase
	EngineerRound   int
	CurrentReviewer task.ReviewerInstance
	ReviewerRound   int
	LastVerdict     string
}

func DeriveSnapshot(projectRoot, slug string) (Snapshot, error) {
	return Snapshot{}, nil
}
