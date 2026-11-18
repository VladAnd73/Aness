package fleet

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/versality/spore/internal/task"
)

// LoopDef is one parsed loop file (bootstrap/loops/<name>.toml).
type LoopDef struct {
	Name         string
	Summary      bool // write upstream's summary.md on done
	GitHubMirror bool // run the draft-PR mirror when pr_flow = "draft"
	PRFooter     bool // end a PR body with the dispatch/approval footer; on unless pr_footer = false
	Steps        []Step
	Source       string // file path or "embedded:<name>", for errors
}

type Step struct {
	Name, Kind          string // kind: "agent" | "action"
	Role, Session       string
	Title, Label        string // Title for wake text; Label for markers and SPORE_REVIEWER_INSTANCE
	Phase               string // phase name when entered forward
	Persist             bool   // session survives forward exit
	Output              string // path under .spore/<slug>/ with "{round}"
	Shape               string
	Required            []string // shape "json" only
	Skills              []string
	Slot                string // reviewer slot: base mode/agent/effort from [fleet.reviewers.<slot>]
	Mode, Agent, Effort string // override the slot
	MaxRounds           int
	Done                []Condition
	Next                []Transition
	Action              string
	Params              map[string]string
}

type Condition struct {
	Kind           string // "file" | "verdict" | "command"
	Path           string // file: relative to .spore/<slug>/, "{round}" allowed
	Field, Equals  string // verdict: field of the step output
	Run            string // command: sh -c in the task worktree
	TimeoutSeconds int
}

type Transition struct {
	On     string // output verdict value; "" = default
	To     string // step name or "done"
	Return bool
	Phase  string // phase of the target while serving this return
}

var ErrUnknownLoop = errors.New("unknown loop")

const defaultCommandTimeoutSeconds = 600

var (
	loopStepKinds      = map[string]bool{"agent": true, "action": true}
	loopShapes         = map[string]bool{"engineer-response": true, "verdict": true, "json": true}
	loopConditionKinds = map[string]bool{"file": true, "verdict": true, "command": true}
)

// ParseLoopDef parses, defaults and validates one loop file. Every
// error is prefixed with source.
func ParseLoopDef(source string, b []byte) (LoopDef, error) {
	doc, err := parseLoopTOML(b)
	if err != nil {
		return LoopDef{}, fmt.Errorf("%s: %w", source, err)
	}
	def, err := loopDefFromTOML(doc)
	if err != nil {
		return LoopDef{}, fmt.Errorf("%s: %w", source, err)
	}
	def.Source = source
	if err := def.Validate(); err != nil {
		return LoopDef{}, err
	}
	return def, nil
}

// tomlReader decodes one table, recording the first type error and
// rejecting keys nobody asked for.
type tomlReader struct {
	t    tomlTable
	seen map[string]bool
	err  error
}

func newTOMLReader(t tomlTable) *tomlReader {
	return &tomlReader{t: t, seen: map[string]bool{}}
}

func (r *tomlReader) get(key, want string) (any, bool) {
	r.seen[key] = true
	v, ok := r.t[key]
	if !ok {
		return nil, false
	}
	var good bool
	switch want {
	case "string":
		_, good = v.v.(string)
	case "integer":
		_, good = v.v.(int)
	case "boolean":
		_, good = v.v.(bool)
	case "array of strings":
		_, good = v.v.([]string)
	}
	if !good {
		if r.err == nil {
			r.err = fmt.Errorf("line %d: %s must be a %s", v.line, key, want)
		}
		return nil, false
	}
	return v.v, true
}

func (r *tomlReader) str(key string) string {
	v, _ := r.get(key, "string")
	s, _ := v.(string)
	return s
}

func (r *tomlReader) integer(key string) int {
	v, _ := r.get(key, "integer")
	n, _ := v.(int)
	return n
}

func (r *tomlReader) boolean(key string) bool {
	v, _ := r.get(key, "boolean")
	b, _ := v.(bool)
	return b
}

func (r *tomlReader) strings(key string) []string {
	v, _ := r.get(key, "array of strings")
	s, _ := v.([]string)
	return s
}

// timeoutSeconds accepts an integer number of seconds or a Go
// duration string such as "30s".
func (r *tomlReader) timeoutSeconds(key string) int {
	r.seen[key] = true
	v, ok := r.t[key]
	if !ok {
		return 0
	}
	switch x := v.v.(type) {
	case int:
		if x > 0 {
			return x
		}
	case string:
		if d, err := time.ParseDuration(x); err == nil && d >= time.Second {
			return int(d / time.Second)
		}
	}
	if r.err == nil {
		r.err = fmt.Errorf("line %d: %s must be a positive number of seconds or a duration like \"30s\"", v.line, key)
	}
	return 0
}

func (r *tomlReader) finish(where string) error {
	if r.err != nil {
		return r.err
	}
	var unknown []string
	for k := range r.t {
		if !r.seen[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Slice(unknown, func(i, j int) bool { return r.t[unknown[i]].line < r.t[unknown[j]].line })
	k := unknown[0]
	return fmt.Errorf("line %d: unknown key %q in %s", r.t[k].line, k, where)
}

func loopDefFromTOML(doc *loopTOML) (LoopDef, error) {
	top := newTOMLReader(doc.top)
	def := LoopDef{
		Name:         top.str("name"),
		Summary:      top.boolean("summary"),
		GitHubMirror: top.boolean("github_mirror"),
		PRFooter:     true,
	}
	if v, ok := top.get("pr_footer", "boolean"); ok {
		def.PRFooter, _ = v.(bool)
	}
	if err := top.finish("the loop header"); err != nil {
		return LoopDef{}, err
	}
	for _, ts := range doc.steps {
		s, err := stepFromTOML(ts)
		if err != nil {
			return LoopDef{}, err
		}
		def.Steps = append(def.Steps, s)
	}
	return def, nil
}

func stepFromTOML(ts tomlStep) (Step, error) {
	r := newTOMLReader(ts.keys)
	s := Step{
		Name: r.str("name"), Kind: r.str("kind"),
		Role: r.str("role"), Session: r.str("session"),
		Title: r.str("title"), Label: r.str("label"), Phase: r.str("phase"),
		Persist: r.boolean("persist"), Output: r.str("output"),
		Shape: r.str("shape"), Required: r.strings("required"), Skills: r.strings("skills"),
		Slot: r.str("slot"), Mode: r.str("mode"), Agent: r.str("agent"), Effort: r.str("effort"),
		MaxRounds: r.integer("max_rounds"), Action: r.str("action"),
	}
	message := r.str("message")
	where := fmt.Sprintf("[[steps]] starting on line %d", ts.line)
	if err := r.finish(where); err != nil {
		return Step{}, err
	}
	if s.Title == "" {
		s.Title = s.Name
	}
	if s.Phase == "" {
		s.Phase = s.Name
	}
	if s.Session == "" && s.Kind == "agent" {
		s.Session = s.Name
	}

	// Action params may sit flat on the step (label, message) or in
	// [steps.params]; the table wins on a clash.
	params := map[string]string{}
	if s.Kind == "action" && s.Label != "" {
		params["label"] = s.Label
	}
	if message != "" {
		params["message"] = message
	}
	for k, v := range ts.params {
		str, ok := v.v.(string)
		if !ok {
			return Step{}, fmt.Errorf("line %d: params.%s must be a string", v.line, k)
		}
		params[k] = str
	}
	if len(params) > 0 {
		s.Params = params
	}

	for _, t := range ts.done {
		cr := newTOMLReader(t)
		c := Condition{
			Kind: cr.str("kind"), Path: cr.str("path"),
			Field: cr.str("field"), Equals: cr.str("equals"),
			Run: cr.str("run"), TimeoutSeconds: cr.timeoutSeconds("timeout"),
		}
		if err := cr.finish("[[steps.done]] of step " + strconv.Quote(s.Name)); err != nil {
			return Step{}, err
		}
		if c.Kind == "verdict" && c.Field == "" {
			c.Field = "verdict"
		}
		if c.Kind == "command" && c.TimeoutSeconds == 0 {
			c.TimeoutSeconds = defaultCommandTimeoutSeconds
		}
		s.Done = append(s.Done, c)
	}
	for _, t := range ts.next {
		nr := newTOMLReader(t)
		tr := Transition{On: nr.str("on"), To: nr.str("to"), Return: nr.boolean("return"), Phase: nr.str("phase")}
		if err := nr.finish("[[steps.next]] of step " + strconv.Quote(s.Name)); err != nil {
			return Step{}, err
		}
		if tr.Return && tr.Phase == "" {
			tr.Phase = tr.To + "-revise-" + s.Name
		}
		s.Next = append(s.Next, tr)
	}
	return s, nil
}

// Validate checks the cross-field rules a parse cannot. Every error is
// prefixed with d.Source.
func (d LoopDef) Validate() error {
	if err := d.validate(); err != nil {
		return fmt.Errorf("%s: %w", d.Source, err)
	}
	return nil
}

func (d LoopDef) validate() error {
	if len(d.Steps) == 0 {
		return errors.New("loop has no steps")
	}
	seen := map[string]bool{}
	for _, s := range d.Steps {
		if s.Name == "" {
			return errors.New("step needs name")
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate step %q", s.Name)
		}
		if s.Name == "done" {
			return errors.New(`step name "done" is reserved for the end of the loop`)
		}
		seen[s.Name] = true
	}
	for i, s := range d.Steps {
		if err := d.validateStep(i, s); err != nil {
			return err
		}
	}
	// Two steps on one output would read each other's rounds as their
	// own.
	byOutput := map[string]string{}
	for _, s := range d.Steps {
		if s.Output == "" {
			continue
		}
		o := path.Clean(s.Output)
		if prev, ok := byOutput[o]; ok {
			return fmt.Errorf("steps %q and %q have the same output %q", prev, s.Name, o)
		}
		byOutput[o] = s.Name
	}
	if d.GitHubMirror {
		if err := d.validateMirrorOutputs(); err != nil {
			return err
		}
	}
	return nil
}

// mirrorOutputs are the agent-step outputs the draft-PR mirror reads
// (mirrorEvents), whatever the loop or its steps are called.
var mirrorOutputs = map[string]string{
	"responses/engineer-round-{round}.json": "engineer-response",
	"reviews/A/round-{round}.json":          "verdict",
	"reviews/B/round-{round}.json":          "verdict",
}

func (d LoopDef) validateMirrorOutputs() error {
	got := map[string]string{}
	for _, s := range d.Steps {
		if s.Kind == "agent" {
			got[path.Clean(s.Output)] = s.Shape
		}
	}
	if !maps.Equal(got, mirrorOutputs) {
		return fmt.Errorf("github_mirror = true needs agent steps writing exactly %s (shape engineer-response), %s and %s (shape verdict): the mirror reads only those",
			"responses/engineer-round-{round}.json", "reviews/A/round-{round}.json", "reviews/B/round-{round}.json")
	}
	return nil
}

func (d LoopDef) validateStep(i int, s Step) error {
	if !loopStepKinds[s.Kind] {
		return fmt.Errorf("step %q: unknown kind %q", s.Name, s.Kind)
	}
	switch s.Kind {
	case "agent":
		if s.Role == "" {
			return fmt.Errorf("agent step %q needs role", s.Name)
		}
		if s.Output == "" {
			return fmt.Errorf("agent step %q needs output", s.Name)
		}
		if !loopShapes[s.Shape] {
			return fmt.Errorf("step %q: unknown shape %q", s.Name, s.Shape)
		}
		if len(s.Required) > 0 && s.Shape != "json" {
			return fmt.Errorf("step %q: required applies to shape \"json\" only", s.Name)
		}
	case "action":
		if _, ok := loopActions[s.Action]; !ok {
			return fmt.Errorf("step %q: unknown action %q", s.Name, s.Action)
		}
		// Only agent steps evaluate done conditions; an action step's
		// would be ignored and the cursor would move on regardless.
		if len(s.Done) > 0 {
			return fmt.Errorf("action step %q: [[steps.done]] applies to agent steps only", s.Name)
		}
	}
	if s.MaxRounds < 0 {
		return fmt.Errorf("step %q: max_rounds must not be negative", s.Name)
	}
	if escapesTaskDir(s.Output) {
		return fmt.Errorf("step %q: output %q escapes the task directory", s.Name, s.Output)
	}
	switch {
	case s.Slot != "" && s.Slot != string(task.ReviewerA) && s.Slot != string(task.ReviewerB):
		return fmt.Errorf("step %q: slot %q: want %q or %q", s.Name, s.Slot, task.ReviewerA, task.ReviewerB)
	case s.Mode != "" && s.Mode != ReviewerModeFleet && s.Mode != ReviewerModeCodeReview:
		return fmt.Errorf("step %q: mode %q: want %q or %q", s.Name, s.Mode, ReviewerModeFleet, ReviewerModeCodeReview)
	case s.Effort != "" && !slices.Contains(reviewEfforts, s.Effort):
		return fmt.Errorf("step %q: effort %q: want one of %v", s.Name, s.Effort, reviewEfforts)
	}
	for _, c := range s.Done {
		if !loopConditionKinds[c.Kind] {
			return fmt.Errorf("step %q: unknown condition kind %q", s.Name, c.Kind)
		}
		switch {
		case c.Kind == "file" && escapesTaskDir(c.Path):
			return fmt.Errorf("step %q: file condition path %q escapes the task directory", s.Name, c.Path)
		case c.Kind == "verdict" && c.Field != "verdict":
			return fmt.Errorf("step %q: verdict condition field %q: only \"verdict\" is supported", s.Name, c.Field)
		case c.Kind == "command" && strings.TrimSpace(c.Run) == "":
			return fmt.Errorf("step %q: command condition needs run", s.Name)
		case c.Kind == "file" && c.Path == "":
			return fmt.Errorf("step %q: file condition needs path", s.Name)
		case c.Kind == "verdict" && c.Equals == "":
			return fmt.Errorf("step %q: verdict condition needs equals", s.Name)
		}
	}
	for _, c := range s.Done {
		if c.Kind != "verdict" {
			continue
		}
		for _, t := range s.Next {
			if t.On != "" && t.On != c.Equals {
				return fmt.Errorf("step %q: verdict condition equals %q blocks next on %q: an output with that verdict could never route; drop the condition or the transition", s.Name, c.Equals, t.On)
			}
		}
	}
	for _, t := range s.Next {
		if t.To == "done" {
			if t.Return {
				return fmt.Errorf("step %q: return to \"done\"", s.Name)
			}
			continue
		}
		j := d.StepIndex(t.To)
		if j < 0 {
			return fmt.Errorf("step %q: next: unknown step %q", s.Name, t.To)
		}
		if t.Return && j > i {
			return fmt.Errorf("step %q: return to a later step %q", s.Name, t.To)
		}
	}
	return nil
}

// escapesTaskDir reports whether a path relative to .spore/<slug>/
// would land outside it.
func escapesTaskDir(p string) bool {
	if p == "" {
		return false
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return true
	}
	c := filepath.Clean(filepath.FromSlash(p))
	return c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator))
}

// StepIndex returns the index of the step called name, or -1.
func (d LoopDef) StepIndex(name string) int {
	for i, s := range d.Steps {
		if s.Name == name {
			return i
		}
	}
	return -1
}
