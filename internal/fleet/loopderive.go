package fleet

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/versality/spore/internal/task"
)

// StepOutput is one valid output of an agent step on disk.
type StepOutput struct {
	Round   int
	Path    string
	Verdict string
}

// LoopState is where a task stands in its loop, derived only from the
// files under .spore/<slug>/.
type LoopState struct {
	Slug, Loop  string
	Step, Phase string // current step; Phase per Step.Phase / Transition.Phase rules
	Terminal    string // "" | "done" | "escalated"
	Round       int    // valid outputs consumed for Step (agent) or completed visits (action)
	ReturnFrom  string // sender step while serving a return, else ""
	SenderRound int    // round number of the sender's output that requested changes
	From        string // step whose output jumped the cursor back to Step without a return, else ""
	FromRound   int    // round number of From's output
	LastVerdict string // last verdict consumed; outputs without one keep it
	Rounds      map[string]int
	Blocked     string // non-empty when the current output exists but a done gate fails
}

// deriveLoopState walks def from step 0, consuming outputs in order. It
// is pure: gate reports whether step's conditions other than its output
// and verdict hold for round, the output's round number on disk (nil
// means they do). Every pass through the
// walk consumes one output or one action visit, so it terminates.
func deriveLoopState(def LoopDef, outs map[string][]StepOutput, actionsDone map[string]int, gate func(step string, round int) bool) LoopState {
	st := LoopState{Loop: def.Name, Rounds: map[string]int{}}
	if len(def.Steps) == 0 {
		return st
	}
	type frame struct {
		sender, target string
		round          int
	}
	var stack []frame
	i, phase := 0, def.Steps[0].Phase
	var from string
	var fromRound int
	for {
		s := def.Steps[i]
		st.Step, st.Phase, st.Round = s.Name, phase, st.Rounds[s.Name]
		st.ReturnFrom, st.SenderRound = "", 0
		st.From, st.FromRound = from, fromRound
		from, fromRound = "", 0
		serving := len(stack) > 0 && stack[len(stack)-1].target == s.Name
		if serving {
			st.ReturnFrom, st.SenderRound = stack[len(stack)-1].sender, stack[len(stack)-1].round
		}

		var verdict string
		fileRound := st.Round + 1
		if s.Kind == "action" {
			if st.Round >= actionsDone[s.Name] {
				return st
			}
		} else {
			o := outs[s.Name]
			if st.Round >= len(o) {
				return st
			}
			verdict, fileRound = o[st.Round].Verdict, o[st.Round].Round
			if verdict != "" {
				st.LastVerdict = verdict
			}
		}
		st.Rounds[s.Name]++
		st.Round++
		if s.Kind == "agent" && (!verdictConditionsHold(s, verdict) || (gate != nil && !gate(s.Name, fileRound))) {
			st.Blocked = fmt.Sprintf("step %s round %d: done conditions do not hold", s.Name, fileRound)
			return st
		}

		if serving {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			i = def.StepIndex(top.sender)
			phase = def.Steps[i].Phase
			continue
		}

		t, ok := pickTransition(s, verdict)
		if !ok {
			if i+1 == len(def.Steps) {
				st.Terminal, st.Phase = "done", "done"
				return st
			}
			i++
			phase = def.Steps[i].Phase
			continue
		}
		if t.To == "done" {
			st.Terminal, st.Phase = "done", "done"
			return st
		}
		j := def.StepIndex(t.To)
		if j < 0 {
			return st
		}
		if j <= i && s.MaxRounds > 0 && st.Round >= s.MaxRounds {
			st.Terminal, st.Phase = "escalated", "escalated"
			return st
		}
		if j <= i && !t.Return {
			from, fromRound = s.Name, fileRound
		}
		if t.Return {
			stack = append(stack, frame{sender: s.Name, target: t.To, round: fileRound})
			phase = t.Phase
		} else {
			phase = def.Steps[j].Phase
		}
		i = j
	}
}

// pickTransition returns the transition matching verdict, else the
// default one; false means "the following step".
func pickTransition(s Step, verdict string) (Transition, bool) {
	for _, t := range s.Next {
		if t.On != "" && t.On == verdict {
			return t, true
		}
	}
	for _, t := range s.Next {
		if t.On == "" {
			return t, true
		}
	}
	return Transition{}, false
}

// verdictConditionsHold checks s's verdict conditions against the
// output's verdict, the only field a StepOutput carries.
func verdictConditionsHold(s Step, verdict string) bool {
	for _, c := range s.Done {
		if c.Kind == "verdict" && verdict != c.Equals {
			return false
		}
	}
	return true
}

// loopStepFiles describes the outputs of def's agent steps for
// validation, listing and bounces.
func loopStepFiles(def LoopDef) []task.StepFiles {
	var out []task.StepFiles
	for _, s := range def.Steps {
		if s.Kind != "agent" {
			continue
		}
		out = append(out, stepFiles(s))
	}
	return out
}

func stepFiles(s Step) task.StepFiles {
	sf := task.StepFiles{
		Step: s.Name, Output: s.Output, Shape: s.Shape,
		Role: s.Role, Session: s.Session, Required: s.Required,
	}
	switch s.Shape {
	case "engineer-response":
		sf.Author = task.EngineerAuthor
	case "verdict":
		sf.Author = s.Label
		if sf.Author == "" {
			sf.Author = s.Name
		}
	}
	return sf
}

// observeLoop reads the valid outputs of every agent step of def and
// the completed visits of every action step. Files of steps def does
// not name are never looked at.
func observeLoop(projectRoot, slug string, def LoopDef) (map[string][]StepOutput, map[string]int, error) {
	outs := map[string][]StepOutput{}
	actions := map[string]int{}
	for _, s := range def.Steps {
		if s.Kind == "action" {
			n, err := countActionVisits(projectRoot, slug, s.Name)
			if err != nil {
				return nil, nil, err
			}
			actions[s.Name] = n
			continue
		}
		sf := stepFiles(s)
		rounds, err := task.StepOutputRounds(projectRoot, slug, s.Output)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range rounds {
			verdict, err := task.ReadStepOutput(projectRoot, slug, sf, r)
			if errors.Is(err, task.ErrInvalidRoleFile) || errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, nil, fmt.Errorf("read %s round %d: %w", s.Name, r, err)
			}
			outs[s.Name] = append(outs[s.Name], StepOutput{
				Round: r, Path: task.StepOutputPath(projectRoot, slug, s.Output, r), Verdict: verdict,
			})
		}
	}
	return outs, actions, nil
}

// actionMarker is the state file recording visit n of an action step.
func actionMarker(projectRoot, slug, step string, n int) string {
	return filepath.Join(task.RoleTaskDir(projectRoot, slug), "state", fmt.Sprintf("action-%s-v%d", step, n))
}

func countActionVisits(projectRoot, slug, step string) (int, error) {
	n := 0
	for {
		_, err := os.Lstat(actionMarker(projectRoot, slug, step, n+1))
		if errors.Is(err, fs.ErrNotExist) {
			return n, nil
		}
		if err != nil {
			return 0, err
		}
		n++
	}
}

// DeriveLoopState observes the task tree and derives its loop state.
// A step's `file` conditions are checked here; gate covers the rest
// (command conditions) and may be nil.
func DeriveLoopState(projectRoot, slug string, def LoopDef, gate func(string, int) bool) (LoopState, error) {
	outs, actions, err := observeLoop(projectRoot, slug, def)
	if err != nil {
		return LoopState{}, err
	}
	st := deriveLoopState(def, outs, actions, fileGate(projectRoot, slug, def, gate))
	st.Slug = slug
	return st, nil
}

func fileGate(projectRoot, slug string, def LoopDef, gate func(string, int) bool) func(string, int) bool {
	return func(step string, round int) bool {
		s := def.Steps[def.StepIndex(step)]
		for _, c := range s.Done {
			if c.Kind != "file" {
				continue
			}
			if _, err := os.Stat(task.StepOutputPath(projectRoot, slug, c.Path, round)); err != nil {
				return false
			}
		}
		return gate == nil || gate(step, round)
	}
}

// DeriveLoopSnapshot is DeriveLoopState in upstream's role-loop terms.
// It only reads: a command condition holds when a cached result of it
// passed, and is never run here.
func DeriveLoopSnapshot(projectRoot, slug string, def LoopDef) (Snapshot, error) {
	st, outs, err := readLoopState(projectRoot, slug, def)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFromLoopState(def, st, outs), nil
}

// readLoopState derives the state without running anything: a command
// condition holds when a cached result of it passed.
func readLoopState(projectRoot, slug string, def LoopDef) (LoopState, map[string][]StepOutput, error) {
	outs, actions, err := observeLoop(projectRoot, slug, def)
	if err != nil {
		return LoopState{}, nil, err
	}
	cached := func(step string, round int) bool {
		for _, c := range def.Steps[def.StepIndex(step)].Done {
			if c.Kind == "command" && !cachedCommandPass(projectRoot, slug, step, round, c.Run) {
				return false
			}
		}
		return true
	}
	st := deriveLoopState(def, outs, actions, fileGate(projectRoot, slug, def, cached))
	st.Slug = slug
	return st, outs, nil
}

// LoopStatus is a task's loop position for `spore task status`.
// Snapshot is set for the dev loop only, in upstream's role-loop terms.
type LoopStatus struct {
	State    LoopState
	Snapshot *Snapshot
}

// ReadLoopStatus reads the loop the task's own `loop:` key names. It
// never spawns, wakes or runs a command check.
func ReadLoopStatus(projectRoot, tasksDir, slug string) (LoopStatus, error) {
	name, err := task.RequireLoop(tasksDir, slug)
	if err != nil {
		return LoopStatus{}, err
	}
	def, err := LoadLoop(projectRoot, name)
	if err != nil {
		return LoopStatus{}, err
	}
	st, outs, err := readLoopState(projectRoot, slug, def)
	if err != nil {
		return LoopStatus{}, err
	}
	ls := LoopStatus{State: st}
	if def.Name == "dev" {
		snap := snapshotFromLoopState(def, st, outs)
		ls.Snapshot = &snap
	}
	return ls, nil
}

// snapshotFromLoopState maps a loop state onto upstream's Snapshot:
// the first step is the engineer, and the reviewer is the verdict step
// the state stands at, is serving a return for, or ended on.
func snapshotFromLoopState(def LoopDef, st LoopState, outs map[string][]StepOutput) Snapshot {
	snap := Snapshot{Slug: st.Slug, Phase: Phase(st.Phase)}
	if len(def.Steps) > 0 {
		if eng := outs[def.Steps[0].Name]; len(eng) > 0 {
			snap.EngineerRound = eng[len(eng)-1].Round
		}
	}
	rev := st.Step
	if st.Terminal == "" && st.ReturnFrom != "" {
		rev = st.ReturnFrom
	}
	i := def.StepIndex(rev)
	if i < 0 || def.Steps[i].Shape != "verdict" {
		return snap
	}
	snap.CurrentReviewer = task.ReviewerInstance(stepFiles(def.Steps[i]).Author)
	snap.ReviewerRound = st.Rounds[rev]
	if n := snap.ReviewerRound; n > 0 && n <= len(outs[rev]) {
		snap.LastVerdict = outs[rev][n-1].Verdict
	}
	return snap
}
