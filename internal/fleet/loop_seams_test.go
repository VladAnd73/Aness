package fleet

import (
	"errors"
	"time"

	"github.com/versality/spore/internal/task"
)

// Placeholder seams for loop_acceptance_test.go. Upstream's
// rolespawn.go, roleliveness.go, roledrive.go and rolebounce.go declare
// these same names; the task that ports them deletes this file, and the
// acceptance helpers then drive the real seams unchanged.

type RoleSpawnSpec struct {
	ProjectRoot      string
	Slug             string
	Role             string
	ReviewerInstance task.ReviewerInstance
	RoleBodyPath     string
	Cwd              string
	TasksDir         string
	Agent            string
}

func (s RoleSpawnSpec) SessionName() (string, error) {
	return "", errors.New("placeholder RoleSpawnSpec has no session name")
}

type paneProbe struct {
	Exists  bool
	Dead    bool
	Command string
	Created time.Time
	Content string
}

type wakeOutcome int

const (
	wakePending wakeOutcome = iota
	wakeDelivered
	wakeNoSession
)

const MaxRoleRespawns = 3

var (
	spawnRole       = func(RoleSpawnSpec) (string, bool, error) { return "", false, nil }
	probeRolePane   = func(string) (paneProbe, error) { return paneProbe{}, nil }
	killRoleSession = func(string) {}
	nowFunc         = time.Now
	wakeRole        = func(RoleSpawnSpec, string, string) (wakeOutcome, error) { return wakePending, nil }
)
