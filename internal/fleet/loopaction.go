package fleet

import (
	"errors"
	"fmt"
)

// loopAction runs one visit of an action step. It must be safe to run
// again after an error: the visit is recorded only on success.
type loopAction func(ctx actionCtx) error

type actionCtx struct {
	ProjectRoot, TasksDir, Slug string
	Def                         LoopDef
	Step                        Step
	Visit                       int
	State                       LoopState
}

// loopActions is the action registry; loop validation accepts exactly
// these names.
var loopActions = map[string]loopAction{
	"notify-coordinator": actionNotifyCoordinator,
	"push-and-open-pr":   actionPushAndOpenPR,
}

// actionNotifyCoordinator tells the coordinator "<slug>: <message>".
func actionNotifyCoordinator(ctx actionCtx) error {
	msg := ctx.Step.Params["message"]
	if msg == "" {
		msg = "reached " + ctx.Step.Name
	}
	notifyRoleLoop(ctx.ProjectRoot, fmt.Sprintf("%s: %s", ctx.Slug, msg))
	return nil
}

// actionPushAndOpenPR is a placeholder until the push-and-open-pr task
// lands: it parks the loop on the step.
func actionPushAndOpenPR(actionCtx) error {
	return errors.New("push-and-open-pr is not implemented yet")
}
