package research

import "context"

// EditRequest names the editor's scratch folder. The folder holds the
// target at TargetRel, the campaign goal as goal.md and the score log
// as results.tsv, and nothing else the editor may read.
type EditRequest struct {
	ScratchDir, TargetRel string
	Try                   int
}

type EditResult struct {
	Description, Transcript string
	ExitCode                int
}

type Editor interface {
	Propose(ctx context.Context, req EditRequest) (EditResult, error)
}

// ClaudeEditor proposes edits with a real claude process. Empty Binary
// means "claude"; empty Model passes no --model flag.
type ClaudeEditor struct {
	Binary, Model, Isolation string
}

func (e ClaudeEditor) Propose(ctx context.Context, req EditRequest) (EditResult, error) {
	return EditResult{}, nil
}
