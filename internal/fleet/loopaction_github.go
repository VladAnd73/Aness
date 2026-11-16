package fleet

import (
	"fmt"

	"github.com/versality/spore/internal/ghapp"
)

// actionPushAndOpenPR pushes the task worktree as the GitHub App and,
// on the first visit that finds no PR in github.json, opens one (or
// adopts the PR already open for the branch). Later visits push only.
// Step params: label (optional), draft ("true" or "false", default
// "false"). It never merges and never marks a PR ready.
func actionPushAndOpenPR(ctx actionCtx) error {
	var draft bool
	switch v := ctx.Step.Params["draft"]; v {
	case "", "false":
	case "true":
		draft = true
	default:
		return fmt.Errorf("param draft = %q, want \"true\" or \"false\"", v)
	}
	cfg, err := ghapp.LoadConfig(ctx.ProjectRoot)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.OperatorLogin == "" {
		return fmt.Errorf("github.toml: missing operator_login")
	}

	unlock, ok := flockBeside(githubStatePath(ctx.ProjectRoot, ctx.Slug))
	if !ok {
		return errActionBusy
	}
	defer unlock()
	m, err := newGitHubMirror(ctx.ProjectRoot, ctx.TasksDir, ctx.Slug)
	if err != nil {
		return err
	}
	m.cfg, m.loop = cfg, ctx.Def.Name
	if err := m.connect(); err != nil {
		return err
	}
	if err := m.push(); err != nil {
		return err
	}
	return m.openPR(ctx.Step.Params["label"], draft)
}
