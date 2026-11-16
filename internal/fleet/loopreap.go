package fleet

import (
	"os/exec"
	"strings"

	"github.com/versality/spore/internal/task"
)

// reapFinishedLoopPanes kills the loop panes of this project's tasks
// that are done, deleted or in an unknown state, keeping those of
// active, paused, blocked and unparseable tasks: the rule generic
// workers follow. DriveLoop only visits active tasks, so nothing else
// would ever reap them. Returns the slugs reaped.
func reapFinishedLoopPanes(projectRoot string, statusBySlug map[string]string) ([]string, error) {
	project, err := task.ProjectName(projectRoot)
	if err != nil {
		return nil, err
	}
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil, nil
	}
	prefix := "spore-role/" + project + "/"
	seen := map[string]bool{}
	var reaped []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), prefix)
		if !ok {
			continue
		}
		slug, _, _ := strings.Cut(rest, "/")
		switch statusBySlug[slug] {
		case "active", "paused", "blocked", statusUnparseable:
			continue
		}
		killRoleSession(strings.TrimSpace(line))
		if !seen[slug] {
			seen[slug] = true
			reaped = append(reaped, slug)
		}
	}
	return reaped, nil
}
