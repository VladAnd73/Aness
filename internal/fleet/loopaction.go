package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/versality/spore/internal/task"
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

// errActionBusy is an action that found another drive mid-way through
// the same work; the next tick retries it without a notice.
var errActionBusy = errors.New("another drive holds the action's lock")

// notifyActionError tells the coordinator about a failing action once
// per step and error key (stderr detail cut, see mirrorErrorKey); the
// driver retries the action every tick regardless.
func notifyActionError(projectRoot, slug, step string, err error) {
	if errors.Is(err, errActionBusy) {
		return
	}
	sum := sha256.Sum256([]byte(mirrorErrorKey(err)))
	stateDir := filepath.Join(task.RoleTaskDir(projectRoot, slug), "state")
	if os.MkdirAll(stateDir, 0o755) != nil {
		return
	}
	created, mErr := createMarkerExcl(filepath.Join(stateDir, fmt.Sprintf("action-error-%s-%s", step, hex.EncodeToString(sum[:6]))))
	if mErr != nil || !created {
		return
	}
	notifyRoleLoop(projectRoot, fmt.Sprintf("%s: loop step %s failed, retrying each tick: %v", slug, step, err))
}
