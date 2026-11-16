package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/versality/spore/internal/task"
	"github.com/versality/spore/internal/task/frontmatter"
)

// DriveLoop advances the task's loop (its `loop:` key, resolved by
// LoadLoop) by one tick. Idempotent; all state lives in .spore/<slug>/.
// A tick:
//
//   - caches the spec and ensures the task worktree, like upstream's
//     role-loop driver;
//   - derives the loop state, running the action the cursor stands on
//     once per visit (see advanceLoop);
//   - done: writes the summary (when the loop asks for one), the ready
//     marker and one coordinator notice, and reaps every step's pane;
//   - escalated: writes the escalation marker and one notice, and
//     touches no pane;
//   - otherwise keeps the right panes alive and wakes them (see
//     keepLoopPanes, wakeLoopPanes);
//   - mirrors to the draft PR when the loop and github.toml ask for it;
//   - bounces invalid step outputs and checks the active step for a
//     stall.
//
// An action error parks the loop on the action step; the rest of the
// tick still runs and the error is returned.
func DriveLoop(projectRoot, tasksDir, slug string) (LoopState, error) {
	name, err := task.RequireLoop(tasksDir, slug)
	if err != nil {
		return LoopState{}, fmt.Errorf("drive %s: %w", slug, err)
	}
	def, err := LoadLoop(projectRoot, name)
	if err != nil {
		return LoopState{}, fmt.Errorf("drive %s: %w", slug, err)
	}
	st, _, err := driveLoopDef(projectRoot, tasksDir, slug, def)
	return st, err
}

func driveLoopDef(projectRoot, tasksDir, slug string, def LoopDef) (LoopState, map[string][]StepOutput, error) {
	if err := task.CacheSpecFromTaskFile(projectRoot, tasksDir, slug); err != nil {
		return LoopState{}, nil, fmt.Errorf("drive %s: cache spec: %w", slug, err)
	}
	if err := task.EnsureLegacyMarker(projectRoot, slug); err != nil {
		return LoopState{}, nil, fmt.Errorf("drive %s: legacy marker: %w", slug, err)
	}
	// The panes cwd into .worktrees/<slug>/; the generic fleet's
	// task.Ensure never runs for a looped slug.
	if _, err := task.EnsureWorktree(tasksDir, slug); err != nil {
		return LoopState{}, nil, fmt.Errorf("drive %s: ensure worktree: %w", slug, err)
	}
	st, outs, actionErr := advanceLoop(projectRoot, tasksDir, slug, def)
	if st.Step == "" {
		return st, outs, fmt.Errorf("drive %s: %w", slug, actionErr)
	}

	prFlow := def.GitHubMirror && githubFlow(projectRoot)
	switch st.Terminal {
	case "done":
		if err := finishLoop(projectRoot, slug, def, prFlow); err != nil {
			return st, outs, fmt.Errorf("drive %s: %w", slug, err)
		}
	case "escalated":
		if err := escalateLoop(projectRoot, slug, def, st); err != nil {
			return st, outs, fmt.Errorf("drive %s: %w", slug, err)
		}
	default:
		if err := keepLoopPanes(projectRoot, tasksDir, slug, def, st); err != nil {
			return st, outs, fmt.Errorf("drive %s: %w", slug, err)
		}
		if err := wakeLoopPanes(projectRoot, slug, def, st, outs); err != nil {
			return st, outs, fmt.Errorf("drive %s: wake: %w", slug, err)
		}
	}

	if prFlow {
		mirrorGitHub(projectRoot, tasksDir, slug, snapshotFromLoopState(def, st, outs))
	}
	if err := bounceInvalidRoleFiles(projectRoot, slug, loopStepFiles(def)...); err != nil {
		return st, outs, fmt.Errorf("drive %s: bounce invalid files: %w", slug, err)
	}
	if err := checkLoopStall(projectRoot, slug, def, st, outs); err != nil {
		return st, outs, fmt.Errorf("drive %s: stall check: %w", slug, err)
	}
	if actionErr != nil {
		return st, outs, fmt.Errorf("drive %s: %w", slug, actionErr)
	}
	return st, outs, nil
}

// advanceLoop derives the loop state. While the cursor stands on an
// action step whose visit has no marker, it runs the action, records
// the visit with an O_EXCL marker and derives again, at most
// len(def.Steps) times per tick. A failing action leaves no marker, so
// the next tick retries it.
func advanceLoop(projectRoot, tasksDir, slug string, def LoopDef) (LoopState, map[string][]StepOutput, error) {
	for n := 0; ; n++ {
		outs, actions, err := observeLoop(projectRoot, slug, def)
		if err != nil {
			return LoopState{}, nil, err
		}
		st := deriveLoopState(def, outs, actions, fileGate(projectRoot, slug, def, nil))
		st.Slug = slug
		i := def.StepIndex(st.Step)
		if st.Terminal != "" || i < 0 || def.Steps[i].Kind != "action" || n == len(def.Steps) {
			return st, outs, nil
		}
		s := def.Steps[i]
		visit := st.Round + 1
		run, ok := loopActions[s.Action]
		if !ok {
			return st, outs, fmt.Errorf("step %s: unknown action %q", s.Name, s.Action)
		}
		ctx := actionCtx{ProjectRoot: projectRoot, TasksDir: tasksDir, Slug: slug, Def: def, Step: s, Visit: visit, State: st}
		if err := run(ctx); err != nil {
			notifyActionError(projectRoot, slug, s.Name, err)
			return st, outs, fmt.Errorf("action %s (%s) visit %d: %w", s.Name, s.Action, visit, err)
		}
		marker := actionMarker(projectRoot, slug, s.Name, visit)
		if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
			return st, outs, err
		}
		if _, err := createMarkerExcl(marker); err != nil {
			return st, outs, err
		}
	}
}

// StepSpec builds the pane spec of an agent step. Mode, effort and
// agent are the step's overrides; the spawn layers them over the
// reviewer slot the step names (see RoleSpawnSpec.resolve).
func StepSpec(projectRoot, slug string, def LoopDef, s Step) RoleSpawnSpec {
	spec := RoleSpawnSpec{
		ProjectRoot:      projectRoot,
		Slug:             slug,
		Role:             s.Role,
		ReviewerInstance: task.ReviewerInstance(s.Slot),
		Session:          s.Session,
		Loop:             def.Name,
		Step:             s.Name,
		Label:            s.Label,
		Skills:           s.Skills,
		Mode:             s.Mode,
		Effort:           s.Effort,
		StepAgent:        s.Agent,
	}
	if s.Output != "" {
		spec.OutputTemplate = filepath.Join(task.RoleTaskDir(projectRoot, slug), filepath.FromSlash(s.Output))
	}
	return spec
}

// classifyLoop resolves a task's `loop:` key to a loop name: "" for a
// task without one, "dev" for the legacy "role". A name LoadLoop does
// not know returns an error wrapping ErrUnknownLoop; a loop file that
// fails to load or validate returns its error.
func classifyLoop(projectRoot string, m frontmatter.Meta) (string, error) {
	if m.Loop == "" {
		return "", nil
	}
	def, err := LoadLoop(projectRoot, m.Loop)
	if err != nil {
		return "", err
	}
	return def.Name, nil
}

// keepLoopPanes keeps alive the current step's pane and the persist
// panes of steps at or before the cursor. When the cursor reached its
// step forward, the non-persist panes of earlier steps are reaped; a
// step that sent the cursor back keeps its pane for the re-check.
func keepLoopPanes(projectRoot, tasksDir, slug string, def LoopDef, st LoopState) error {
	cur := def.StepIndex(st.Step)
	keep := map[string]bool{}
	var ensure []Step
	for i, s := range def.Steps {
		if s.Kind == "agent" && (i == cur || (s.Persist && i < cur)) {
			keep[s.Session] = true
			ensure = append(ensure, s)
		}
	}
	if st.From == "" && st.ReturnFrom == "" {
		for i, s := range def.Steps {
			if s.Kind != "agent" || i >= cur || s.Persist || keep[s.Session] {
				continue
			}
			if err := reapRolePane(StepSpec(projectRoot, slug, def, s)); err != nil {
				return fmt.Errorf("reap %s: %w", s.Name, err)
			}
		}
	}
	for _, s := range ensure {
		spec := StepSpec(projectRoot, slug, def, s)
		spec.TasksDir = tasksDir
		if err := ensureRolePane(spec, st.Rounds[s.Name]); err != nil {
			return fmt.Errorf("ensure %s: %w", s.Name, err)
		}
	}
	return nil
}

// reapRolePane kills spec's session when it exists, through the
// liveness seams.
func reapRolePane(spec RoleSpawnSpec) error {
	session, err := spec.SessionName()
	if err != nil {
		return err
	}
	if p, err := probeRolePane(session); err == nil && p.Exists {
		killRoleSession(session)
	}
	return nil
}

// wakeLoopPanes nudges the current step's pane. Arrival: a step that
// sent the cursor back (with or without return) hands over its output.
// Re-entry: a step whose last output sent the cursor back is told the
// step it sent work to has answered. A step at round 0 starts from its
// role body and gets neither.
func wakeLoopPanes(projectRoot, slug string, def LoopDef, st LoopState, outs map[string][]StepOutput) error {
	cur := def.StepIndex(st.Step)
	s := def.Steps[cur]
	if s.Kind != "agent" {
		return nil
	}
	spec := StepSpec(projectRoot, slug, def, s)
	from, fromRound := st.ReturnFrom, st.SenderRound
	if from == "" {
		from, fromRound = st.From, st.FromRound
	}
	if from != "" {
		x := def.Steps[def.StepIndex(from)]
		path := task.StepOutputPath(projectRoot, slug, x.Output, fromRound)
		msg := fmt.Sprintf("%s requested changes at round %d. Read %s and start the revision.", x.Title, fromRound, path)
		if _, err := wakeRole(spec, fmt.Sprintf("woken-%s-%d", labelOrName(x), fromRound), msg+skillsHint(s)); err != nil {
			return err
		}
	}
	if st.Round < 1 || st.Blocked != "" || st.Round > len(outs[s.Name]) {
		return nil
	}
	t, ok := pickTransition(s, outs[s.Name][st.Round-1].Verdict)
	j := def.StepIndex(t.To)
	if !ok || j < 0 || j > cur || def.Steps[j].Kind != "agent" {
		return nil
	}
	target := def.Steps[j]
	n := st.Rounds[target.Name]
	if n < 1 || n > len(outs[target.Name]) {
		return nil
	}
	resp := outs[target.Name][n-1]
	msg := fmt.Sprintf("%s responded to your round %d verdict. Read %s and review round %d.",
		target.Title, st.Round, resp.Path, st.Round+1)
	_, err := wakeRole(spec, fmt.Sprintf("woken-%s-%d", s.Session, resp.Round), msg+skillsHint(s))
	return err
}

func labelOrName(s Step) string {
	if s.Label != "" {
		return s.Label
	}
	return s.Name
}

func skillsHint(s Step) string {
	if len(s.Skills) == 0 {
		return ""
	}
	return " Use skills: " + strings.Join(s.Skills, ", ") + "."
}

// finishLoop is the done transition: summary (when the loop keeps
// one), escalation markers cleared, ready marker, one notice unless
// the PR mirror sends it, and every pane reaped.
func finishLoop(projectRoot, slug string, def LoopDef, prFlow bool) error {
	if def.Summary {
		if err := writeSummary(projectRoot, slug); err != nil {
			return fmt.Errorf("write summary: %w", err)
		}
	}
	if err := clearEscalationMarkers(projectRoot, slug); err != nil {
		return fmt.Errorf("clear escalation markers: %w", err)
	}
	created, err := writeReadyMarker(projectRoot, slug)
	if err != nil {
		return fmt.Errorf("write ready marker: %w", err)
	}
	if created && !prFlow {
		notifyRoleLoop(projectRoot, fmt.Sprintf("%s: done", slug))
	}
	for _, s := range def.Steps {
		if s.Kind == "agent" {
			_ = reapRolePane(StepSpec(projectRoot, slug, def, s))
		}
	}
	return nil
}

// escalateLoop drops state/escalated-<label or name>-<round> and tells
// the coordinator once. Panes are left for the operator.
func escalateLoop(projectRoot, slug string, def LoopDef, st LoopState) error {
	s := def.Steps[def.StepIndex(st.Step)]
	stateDir := filepath.Join(task.RoleTaskDir(projectRoot, slug), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	created, err := createMarkerExcl(filepath.Join(stateDir, fmt.Sprintf("escalated-%s-%d", labelOrName(s), st.Round)))
	if err != nil {
		return fmt.Errorf("write escalation marker: %w", err)
	}
	if !created {
		return nil
	}
	who := s.Name
	if s.Label != "" {
		who = s.Role + " " + s.Label
	}
	notifyRoleLoop(projectRoot, fmt.Sprintf("%s: escalated (%s, round %d)", slug, who, st.Round))
	return nil
}

// checkLoopStall runs checkRoleStall for the current agent step.
func checkLoopStall(projectRoot, slug string, def LoopDef, st LoopState, outs map[string][]StepOutput) error {
	if st.Terminal != "" || st.Blocked != "" {
		return nil
	}
	s := def.Steps[def.StepIndex(st.Step)]
	if s.Kind != "agent" {
		return nil
	}
	next := 1
	if n := st.Round; n > 0 && n <= len(outs[s.Name]) {
		next = outs[s.Name][n-1].Round + 1
	}
	expected := task.StepOutputPath(projectRoot, slug, s.Output, next)
	return checkRoleStall(StepSpec(projectRoot, slug, def, s), st.Round, expected, roleStallWindow(projectRoot))
}
