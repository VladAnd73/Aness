package task

// Compile stub for the configurable-loops acceptance tests. The task
// that ports upstream's internal/task/roletask.go replaces this file.

type ReviewerInstance string

const (
	ReviewerA ReviewerInstance = "A"
	ReviewerB ReviewerInstance = "B"
)

const (
	VerdictApprove        = "approve"
	VerdictRequestChanges = "request_changes"
)

const LoopRole = "role"

type EngineerResponse struct {
	Author    string   `json:"author"`
	Addressed []string `json:"addressed"`
	Pushback  []string `json:"pushback"`
	Notes     string   `json:"notes,omitempty"`
	Followups []string `json:"followups,omitempty"`
}

type Review struct {
	Author   string   `json:"author"`
	Verdict  string   `json:"verdict"`
	Summary  string   `json:"summary"`
	Comments []string `json:"comments"`
}

func WriteEngineerResponse(projectRoot, slug string, round int, r EngineerResponse) error {
	return nil
}

func WriteReview(projectRoot, slug string, inst ReviewerInstance, round int, v Review) error {
	return nil
}
