package evalharness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// RoleStep is the loop step a candidate role body runs as, pinned from
// the loop file when the campaign was created. Author is the verdict
// author a verdict step's output names.
type RoleStep struct {
	Loop     string   `json:"loop"`
	Step     string   `json:"step"`
	Role     string   `json:"role"`
	Output   string   `json:"output"`
	Shape    string   `json:"shape"`
	Author   string   `json:"author,omitempty"`
	Required []string `json:"required,omitempty"`
	Verdicts []string `json:"verdicts,omitempty"`
	Skills   []string `json:"skills,omitempty"`
}

// RoleStepFile is the file a role judge's frozen folder pins the step
// in, next to the skills/ folder.
const RoleStepFile = "step.json"

// replaySlug names the replayed task; its task dir is
// <run>/.spore/<replaySlug>, so the agent's snapshot and task files
// share the one writable bind.
const replaySlug = "replay"

func LoadRoleStep(path string) (RoleStep, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return RoleStep{}, fmt.Errorf("evalharness: role step: %w", err)
	}
	var rs RoleStep
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rs); err != nil {
		return RoleStep{}, fmt.Errorf("evalharness: role step %s: %w", path, err)
	}
	if err := rs.validate(); err != nil {
		return RoleStep{}, fmt.Errorf("evalharness: role step %s: %w", path, err)
	}
	return rs, nil
}

func (rs RoleStep) validate() error {
	if rs.Step == "" || rs.Role == "" {
		return fmt.Errorf("step and role are required")
	}
	if rs.Output == "" || !filepath.IsLocal(filepath.FromSlash(rs.Output)) {
		return fmt.Errorf("output %q must be a relative path inside the task dir", rs.Output)
	}
	for _, s := range rs.Skills {
		if err := checkSkillName(s); err != nil {
			return err
		}
	}
	return nil
}

// StepNote copies fleet's stepNote (internal/fleet/loopdrive.go), which
// evalharness must not import. research's TestStepNoteMatchesFleet pins
// the copy to the original.
func StepNote(rs RoleStep, output string, round int) string {
	var format string
	switch rs.Shape {
	case "engineer-response":
		format = `exactly {"author": "engineer", "addressed": [...], "pushback": [...], "notes": "...", "followups": [...]}, no other keys`
	case "verdict":
		format = fmt.Sprintf(`exactly {"author": %q, "verdict": %q or %q, "summary": "one line", "comments": ["one per concern"]}, no other keys`,
			rs.Author, verdictApprove, verdictRequestChanges)
	default:
		format = "one JSON object"
		if len(rs.Required) > 0 {
			format += " with these keys present and not null: " + quotedList(rs.Required)
		}
		if len(rs.Verdicts) > 0 {
			format += `, and "verdict" set to one of ` + quotedList(rs.Verdicts)
		}
	}
	next := ""
	if round > 0 {
		next = fmt.Sprintf(" Write round %d now: %s.", round, strings.ReplaceAll(output, "{round}", strconv.Itoa(round)))
	}
	return fmt.Sprintf("You are step %q of loop %q. Write each round's output to %s with {round} replaced by the round number.%s Each time the work comes back to you, write the next round. Format: %s. This path and format replace any output path or file format your role instructions below name; follow the rest of them as written.",
		rs.Step, rs.Loop, output, next, format)
}

const (
	verdictApprove        = "approve"
	verdictRequestChanges = "request_changes"
)

func quotedList(keys []string) string {
	q := make([]string, len(keys))
	for i, k := range keys {
		q[i] = strconv.Quote(k)
	}
	return strings.Join(q, ", ")
}

// roleRun is one role scenario's world, laid out in the run dir.
type roleRun struct {
	prompt, taskDir, output string
	env                     []string
}

// prepareRoleRun lays out what a loop step's pane would see: the
// pre-fix snapshot as the working directory, the spec at
// $SPORE_TASK_DIR/spec.md and the step's frozen skills. grader/ stays
// in the scenario store, which no sandbox binds.
func prepareRoleRun(runWD string, o JudgeOptions, sc DataScenario) (roleRun, error) {
	rs := *o.Role
	if snap := filepath.Join(sc.Dir, "snapshot"); dirExists(snap) {
		if err := copyTree(snap, runWD); err != nil {
			return roleRun{}, fmt.Errorf("snapshot: %w", err)
		}
	}
	r := roleRun{taskDir: filepath.Join(runWD, ".spore", replaySlug)}
	if err := os.MkdirAll(r.taskDir, 0o755); err != nil {
		return roleRun{}, err
	}
	spec, err := os.ReadFile(filepath.Join(sc.Dir, "spec.md"))
	if err != nil {
		return roleRun{}, err
	}
	if err := os.WriteFile(filepath.Join(r.taskDir, "spec.md"), spec, 0o644); err != nil {
		return roleRun{}, err
	}
	for _, name := range rs.Skills {
		src := filepath.Join(o.FrozenDir, "skills", name)
		if !dirExists(src) {
			continue
		}
		if err := copyTree(src, filepath.Join(runWD, ".claude", "skills", name)); err != nil {
			return roleRun{}, fmt.Errorf("skill %s: %w", name, err)
		}
	}
	body, err := os.ReadFile(o.Candidate)
	if err != nil {
		return roleRun{}, err
	}
	template := filepath.Join(r.taskDir, filepath.FromSlash(rs.Output))
	r.output = strings.ReplaceAll(template, "{round}", "1")
	r.prompt = StepNote(rs, template, 1) + "\n\n" + string(body)
	r.env = []string{
		"SPORE_TASK_DIR=" + r.taskDir, "SPORE_ROLE=" + rs.Role, "SPORE_LOOP=" + rs.Loop,
		"SPORE_STEP=" + rs.Step, "SPORE_STEP_OUTPUT=" + template, "SPORE_STEP_SKILLS=" + strings.Join(rs.Skills, ","),
	}
	return r, nil
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// maxMaterial bounds what a grader reads of one run.
const maxMaterial = 256 << 10

// roleMaterial is everything the agent wrote: each file in the run dir
// that is new or differs from the snapshot, skills, the spec and
// node_modules aside. Task files come first, then new files, then
// changed ones, so a large changed fixture cannot push a new test past
// the cap; a file over the cap on its own is named, not read.
func roleMaterial(snapshot, runWD, taskDir string) (string, error) {
	type entry struct {
		rel, body string
		rank      int
	}
	var entries []entry
	err := filepath.WalkDir(runWD, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(runWD, path)
		if d.IsDir() {
			if rel == ".claude" || d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if path == filepath.Join(taskDir, "spec.md") || !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		was, wasErr := os.ReadFile(filepath.Join(snapshot, rel))
		e := entry{rel: filepath.ToSlash(rel), rank: 2}
		switch {
		case strings.HasPrefix(path, taskDir+string(filepath.Separator)):
			e.rank = 0
		case wasErr != nil:
			e.rank = 1
		}
		if fi.Size() > maxMaterial {
			e.body = fmt.Sprintf("[%d bytes, too large to show]", fi.Size())
			entries = append(entries, e)
			return nil
		}
		now, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if wasErr == nil && bytes.Equal(was, now) {
			return nil
		}
		e.body = string(now)
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].rank < entries[j].rank })
	var b strings.Builder
	for _, e := range entries {
		section := fmt.Sprintf("=== %s ===\n%s\n", e.rel, e.body)
		if b.Len()+len(section) > maxMaterial {
			fmt.Fprintf(&b, "=== %s ===\n[left out: over the size cap]\n", e.rel)
			continue
		}
		b.WriteString(section)
	}
	return b.String(), nil
}
