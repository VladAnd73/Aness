package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/versality/spore/internal/task"
)

// wakeRole is the wake seam bounceInvalidRoleFiles delivers through.
// Tests swap it to observe bounces without tmux.
var wakeRole = wakeSession

const (
	reviewKeys   = `"author", "verdict" ("approve" or "request_changes"), "summary", "comments"`
	engineerKeys = `"author", "addressed", "pushback", "notes", "followups"`
)

// bounceInvalidRoleFiles hands every role file that fails validation
// back to its author. Per file:
//
//   - The coordinator is told once (O_EXCL marker keyed on the file,
//     its quarantine count, and its content hash).
//   - The authoring pane is woken with the validation error. Only once
//     the wake is confirmed is the file moved to `<name>.invalid`
//     (kept for debugging), so a pane still booting gets the error on
//     a later tick. Until then DeriveSnapshot already ignores it.
//   - When the authoring pane is not running (a file written out of
//     turn, e.g. under a reviewer whose phase is not active), the file
//     is moved aside at once; the coordinator notice carries the error.
func bounceInvalidRoleFiles(projectRoot, slug string, steps ...task.StepFiles) error {
	files, err := task.ListRoleFiles(projectRoot, slug, steps...)
	if err != nil {
		return err
	}
	for _, f := range files {
		verr := task.ValidateRoleFile(projectRoot, slug, f)
		if verr == nil {
			continue
		}
		var inv *task.InvalidRoleFileError
		if !errors.As(verr, &inv) {
			return verr
		}
		if err := bounceRoleFile(projectRoot, slug, f, oneLine(inv.Reason)); err != nil {
			return err
		}
	}
	return nil
}

func bounceRoleFile(projectRoot, slug string, f task.RoleFile, reason string) error {
	body, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	dest, prior, err := task.QuarantinePath(f.Path)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s-%d-%s", strings.ReplaceAll(f.Rel, "/", "_"), prior, hex.EncodeToString(sum[:6]))

	spec := EngineerSpec(projectRoot, slug)
	keys := engineerKeys
	if f.Instance != "" {
		spec = ReviewerSpec(projectRoot, slug, f.Instance)
		keys = reviewKeys
	}
	shape := fmt.Sprintf("only the keys %s, and \"author\": %q", keys, f.Author())
	if f.Step != nil {
		spec = RoleSpawnSpec{ProjectRoot: projectRoot, Slug: slug, Role: f.Step.Role, Session: f.Step.Session}
		switch f.Step.Shape {
		case "verdict":
			shape = fmt.Sprintf("only the keys %s, and \"author\": %q", reviewKeys, f.Author())
		case "json":
			shape = fmt.Sprintf("non-null values for the keys %s", quotedList(f.Step.Required))
		}
	}
	stateDir := filepath.Join(task.RoleTaskDir(projectRoot, slug), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	created, err := createMarkerExcl(filepath.Join(stateDir, "invalid-"+key))
	if err != nil {
		return err
	}
	if created {
		to := f.Author()
		if f.Step != nil {
			to = f.Step.Step
		}
		notifyRoleLoop(projectRoot, fmt.Sprintf("%s: invalid %s (%s); bounced to %s",
			slug, f.Rel, reason, to))
	}

	msg := fmt.Sprintf("Your file %s failed validation: %s. It was moved to %s and does not count. "+
		"Rewrite %s with the Write tool (never a shell heredoc or cat >): one JSON object with %s.",
		f.Path, reason, filepath.Base(dest), f.Path, shape)
	outcome, err := wakeRole(spec, "bounced-"+key, msg)
	if err != nil {
		return err
	}
	if outcome == wakePending {
		return nil
	}
	return os.Rename(f.Path, dest)
}

func quotedList(keys []string) string {
	q := make([]string, len(keys))
	for i, k := range keys {
		q[i] = fmt.Sprintf("%q", k)
	}
	return strings.Join(q, ", ")
}

// oneLine flattens s for a single tmux send-keys line: an embedded
// newline would submit the wake early.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 300
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}
