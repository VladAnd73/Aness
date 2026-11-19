package evalharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/versality/spore/internal/isolate"
	"github.com/versality/spore/internal/research/fakemcp"
	"github.com/versality/spore/internal/statefile"
)

// AllowUnsandboxedEnv must be "1" for Isolation "none": without it a
// judge never runs an agent under test with the host's view of the disk.
const AllowUnsandboxedEnv = "SPORE_RESEARCH_ALLOW_UNSANDBOXED"

// JudgeOptions configures one judge run over one scenario set. Zero
// Repeats, Parallel and Timeout take the defaults 3, 3 and 5 minutes.
// Spawner and Grader default to IsolatedSpawner on AgentBinary
// (ClaudeSpawner for Isolation "none"). An empty SandboxParent means a
// temp dir removed when the judge returns, transcripts included.
// Services starts a scenario's fake services and defaults to
// FakeServices.
type JudgeOptions struct {
	SetDir, Candidate, FrozenDir, SkillName, Model, AgentBinary, Isolation, SandboxParent string
	Repeats, Parallel                                                                     int
	Timeout                                                                               time.Duration
	Spawner                                                                               AgentSpawner
	Grader                                                                                AgentSpawner
	Services                                                                              ServiceStarter
	Role                                                                                  *RoleStep
}

// ServiceStarter starts the fake MCP servers for one run, logging every
// call to callsLog, and returns the --mcp-config JSON that reaches them.
type ServiceStarter func(ctx context.Context, svcs []fakemcp.Tool, callsLog string) (mcpConfigJSON []byte, stop func() error, err error)

// JudgeReport carries the same JSON fields as research.JudgeResult.
// evalharness must not import research: research imports evalharness.
type JudgeReport struct {
	Score        float64          `json:"score"`
	Runs         int              `json:"runs"`
	Passed       int              `json:"passed"`
	Crashes      int              `json:"crashes"`
	CostUSD      float64          `json:"cost_usd"`
	InputTokens  int64            `json:"input_tokens"`
	OutputTokens int64            `json:"output_tokens"`
	Scenarios    []ScenarioReport `json:"scenarios"`
}

type ScenarioReport struct {
	Name string      `json:"name"`
	Runs []RunReport `json:"runs"`
}

type RunReport struct {
	Pass           bool   `json:"pass"`
	Crashed        bool   `json:"crashed"`
	Reason         string `json:"reason"`
	TranscriptPath string `json:"transcript_path"`
	CallsLog       string `json:"calls_log"`
	SandboxRoot    string `json:"sandbox_root"`
	// Quotes are the output lines a bug-caught grader matched, kept so a
	// human can audit each catch.
	Quotes []string `json:"quotes,omitempty"`
}

type runOutcome struct {
	report RunReport
	stream StreamResult
}

// RunJudge runs every scenario in o.SetDir o.Repeats times, each run in
// its own sandbox, at most o.Parallel at once. Score is the percent of
// runs that passed. A run that times out, exits non-zero or never
// reaches a result line is a crash: a failed run, never a score of its
// own. An error means the judge itself failed and there is no score.
func RunJudge(ctx context.Context, o JudgeOptions) (JudgeReport, error) {
	if o.Repeats <= 0 {
		o.Repeats = 3
	}
	if o.Parallel <= 0 {
		o.Parallel = 3
	}
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Minute
	}
	if o.SandboxParent == "" {
		tmp, err := os.MkdirTemp("", "spore-judge-")
		if err != nil {
			return JudgeReport{}, fmt.Errorf("evalharness: judge: %w", err)
		}
		defer os.RemoveAll(tmp)
		o.SandboxParent = tmp
	}
	if o.Spawner == nil {
		if o.Isolation == "none" {
			o.Spawner = ClaudeSpawner{Binary: o.AgentBinary}
		} else {
			o.Spawner = IsolatedSpawner{Binary: o.AgentBinary}
		}
	}
	if o.Services == nil {
		o.Services = FakeServices()
	}
	if o.Grader == nil {
		if o.Isolation == "none" {
			o.Grader = ClaudeSpawner{Binary: o.AgentBinary}
		} else {
			o.Grader = IsolatedSpawner{Binary: o.AgentBinary}
		}
	}
	if err := checkIsolation(o.Isolation); err != nil {
		return JudgeReport{}, err
	}
	set, err := LoadSet(o.SetDir)
	if err != nil {
		return JudgeReport{}, err
	}
	if err := checkJudgeKind(o, set); err != nil {
		return JudgeReport{}, err
	}
	if _, err := os.ReadFile(o.Candidate); err != nil {
		return JudgeReport{}, fmt.Errorf("evalharness: judge: candidate: %w", err)
	}

	// The first judge error cancels the runs still waiting: they could
	// only spend quota on a judgement that is already void.
	jctx, cancel := context.WithCancel(ctx)
	defer cancel()
	outcomes := make([][]runOutcome, len(set))
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		sem      = make(chan struct{}, o.Parallel)
	)
	for si, sc := range set {
		outcomes[si] = make([]runOutcome, o.Repeats)
		for ri := range o.Repeats {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if jctx.Err() != nil {
					return
				}
				out, err := runOnce(jctx, o, sc)
				mu.Lock()
				defer mu.Unlock()
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("evalharness: judge: scenario %s run %d: %w", sc.Name, ri+1, err)
					cancel()
				}
				outcomes[si][ri] = out
			}()
		}
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return JudgeReport{}, fmt.Errorf("evalharness: judge: %w", err)
	}
	if firstErr != nil {
		return JudgeReport{}, firstErr
	}

	var rep JudgeReport
	for si, sc := range set {
		sr := ScenarioReport{Name: sc.Name}
		for _, out := range outcomes[si] {
			rep.Runs++
			if out.report.Pass {
				rep.Passed++
			}
			if out.report.Crashed {
				rep.Crashes++
			}
			rep.CostUSD += out.stream.CostUSD
			rep.InputTokens += out.stream.InputTokens
			rep.OutputTokens += out.stream.OutputTokens
			sr.Runs = append(sr.Runs, out.report)
		}
		rep.Scenarios = append(rep.Scenarios, sr)
	}
	if rep.Runs > 0 {
		rep.Score = 100 * float64(rep.Passed) / float64(rep.Runs)
	}
	return rep, nil
}

// checkJudgeKind refuses a set of the other kind: a role run without
// its step, or a skill run without its skill name, measures nothing.
func checkJudgeKind(o JudgeOptions, set []DataScenario) error {
	want := ""
	if o.Role != nil {
		want = "role"
		if err := o.Role.validate(); err != nil {
			return fmt.Errorf("evalharness: judge: role step: %w", err)
		}
	} else if err := checkSkillName(o.SkillName); err != nil {
		return err
	}
	for _, sc := range set {
		if sc.Kind != want {
			return fmt.Errorf("evalharness: judge: scenario %s is kind %q, the judge runs kind %q", sc.Name, sc.Kind, want)
		}
	}
	return nil
}

func checkIsolation(mode string) error {
	switch mode {
	case "none":
		if os.Getenv(AllowUnsandboxedEnv) != "1" {
			return fmt.Errorf("evalharness: judge: isolation none runs the agent under test unsandboxed; set %s=1 to allow it", AllowUnsandboxedEnv)
		}
		return nil
	case "", "bwrap":
		if err := isolate.HostSupported(); err != nil {
			return fmt.Errorf("evalharness: judge: %w; to run unsandboxed pass --isolation none and set %s=1", err, AllowUnsandboxedEnv)
		}
		return nil
	default:
		return fmt.Errorf("evalharness: judge: unknown isolation %q (want bwrap or none)", mode)
	}
}

func runOnce(ctx context.Context, o JudgeOptions, sc DataScenario) (runOutcome, error) {
	sb, err := NewSandbox(o.SandboxParent)
	if err != nil {
		return runOutcome{}, err
	}
	prompt := sc.Prompt
	var role roleRun
	if o.Role != nil {
		// No git init: a role run's snapshot must carry no history.
		if role, err = prepareRoleRun(sb.RunWD, o, sc); err != nil {
			return runOutcome{}, err
		}
		prompt = role.prompt
	} else if err := prepareSkillRun(sb.RunWD, o, sc); err != nil {
		return runOutcome{}, err
	}

	args := append(append([]string{}, SessionSpawnArgs...), "--strict-mcp-config")
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	run := RunReport{SandboxRoot: sb.Root, TranscriptPath: filepath.Join(sb.Root, "transcript.jsonl")}
	cfg := fakemcp.MCPConfig(nil)
	stopServices := func() error { return nil }
	if len(sc.Services) > 0 {
		run.CallsLog = filepath.Join(sb.Root, "calls.jsonl")
		var stop func() error
		cfg, stop, err = o.Services(ctx, sc.Services, run.CallsLog)
		if err != nil {
			return runOutcome{}, fmt.Errorf("fake services: %w", err)
		}
		stopServices = sync.OnceValue(stop)
		defer stopServices()
	}
	cfgPath := filepath.Join(sb.Root, "mcp.json")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		return runOutcome{}, err
	}
	args = append(args, "--mcp-config", cfgPath)
	req := SpawnRequest{Prompt: prompt, WorkDir: sb.RunWD, Env: append(sb.Env(), role.env...), ExtraArgs: args}
	if o.Isolation != "none" {
		spec := agentSpec(sb, cfgPath)
		spec.Env = append(spec.Env, role.env...)
		req.Env, req.Isolate = spec.Env, &spec
	}

	rctx, cancel := context.WithTimeout(ctx, o.Timeout)
	res, spawnErr := o.Spawner.Spawn(rctx, req)
	timedOut := errors.Is(rctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
	cancel()
	servicesErr := stopServices()
	if err := ctx.Err(); err != nil {
		return runOutcome{}, err
	}
	if err := os.WriteFile(run.TranscriptPath, []byte(res.Stdout), 0o600); err != nil {
		return runOutcome{}, err
	}
	stream := ParseStream(res.Stdout)
	out := runOutcome{stream: stream}

	switch {
	case timedOut:
		run.Reason = "timeout"
	case res.ExitCode != 0:
		run.Reason = fmt.Sprintf("exit %d", res.ExitCode)
	case spawnErr != nil:
		// Exit 0 with an error means no agent ever ran: a missing binary,
		// login or sandbox fails the judge instead of scoring a loss.
		return runOutcome{}, fmt.Errorf("spawn: %w", spawnErr)
	case !stream.SawResult:
		run.Reason = "no result line"
	case servicesErr != nil:
		run.Reason = "fake services: " + servicesErr.Error()
	}
	if run.Reason != "" {
		run.Crashed = true
		out.report = run
		return out, nil
	}

	a := RunArtifacts{RunWD: sb.RunWD, Stream: stream}
	material := stream.FinalReply
	if o.Role != nil {
		a.Role, a.TaskDir, a.Output, a.Snapshot = o.Role, role.taskDir, role.output, filepath.Join(sc.Dir, "snapshot")
		if material, err = roleMaterial(a.Snapshot, sb.RunWD, role.taskDir); err != nil {
			return runOutcome{}, err
		}
	}
	if run.CallsLog != "" {
		if a.Calls, err = fakemcp.ReadCalls(run.CallsLog); err != nil {
			return runOutcome{}, err
		}
	}
	var code, judged []ScenarioCheck
	for _, c := range sc.Checks {
		if c.Kind == "llm-judge" || c.Kind == "bug-caught" {
			judged = append(judged, c)
		} else {
			code = append(code, c)
		}
	}
	pass, failed, err := GradeChecks(a, code)
	if err != nil {
		return runOutcome{}, err
	}
	if pass {
		for _, c := range judged {
			var ok bool
			var crash string
			if c.Kind == "bug-caught" {
				var quote string
				ok, quote, crash, err = gradeCaught(ctx, o, sb, filepath.Join(sc.Dir, c.Repro), material)
				if ok {
					run.Quotes = append(run.Quotes, quote)
				}
			} else {
				ok, crash, err = gradeLLM(ctx, o, sb, filepath.Join(sc.Dir, c.Rubric), material)
			}
			if err != nil {
				return runOutcome{}, err
			}
			if crash != "" {
				run.Crashed, run.Reason = true, "grader: "+crash
				out.report = run
				return out, nil
			}
			if !ok {
				pass = false
				failed = append(failed, c.Kind)
			}
		}
	}
	run.Pass = pass
	run.Reason = "pass"
	if !pass {
		run.Reason = strings.Join(failed, ", ")
	}
	out.report = run
	return out, nil
}

// prepareSkillRun installs the candidate skill over its frozen folder.
// The run dir is its own repository so the agent's project root, where
// it looks for .claude/skills, is the run dir itself. Fixtures go in
// first so the injected skill always wins a collision.
func prepareSkillRun(runWD string, o JudgeOptions, sc DataScenario) error {
	if err := gitInitNoRemote(runWD); err != nil {
		return err
	}
	files := filepath.Join(sc.Dir, "files")
	if _, err := os.Stat(files); err == nil {
		if err := copyTree(files, runWD); err != nil {
			return fmt.Errorf("fixture files: %w", err)
		}
	}
	return InjectSkill(runWD, o.FrozenDir, o.SkillName, o.Candidate)
}

func WriteJudgeReport(path string, r JudgeReport) error {
	return statefile.WriteJSONAtomic(path, "judge-result", r)
}

// ProbeIsolated runs argv under the same isolation RunJudge gives the
// agent under test and returns its combined output.
func ProbeIsolated(ctx context.Context, o JudgeOptions, argv []string) (string, error) {
	if err := checkIsolation(o.Isolation); err != nil {
		return "", err
	}
	if len(argv) == 0 {
		return "", errors.New("evalharness: probe: empty argv")
	}
	parent := o.SandboxParent
	if parent == "" {
		parent = os.TempDir()
	}
	sb, err := NewSandbox(parent)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(sb.Root)
	var cmd *exec.Cmd
	if o.Isolation == "none" {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir, cmd.Env = sb.RunWD, sb.Env()
	} else if cmd, err = isolate.Command(ctx, agentSpec(sb, ""), argv); err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	for _, f := range cmd.ExtraFiles {
		f.Close()
	}
	return string(out), err
}

// agentSpec is the sandbox for one agent under test: the run dir and a
// fresh home, the MCP config read-only, nothing from the scenario store.
func agentSpec(sb *Sandbox, mcpConfig string) isolate.Spec {
	s := isolate.Spec{
		Work:        sb.RunWD,
		Home:        sb.AgentHome,
		Credentials: isolate.CredentialsPath(),
		Env:         isolate.BaseEnv(sb.AgentHome),
	}
	if mcpConfig != "" {
		s.Extra = []isolate.Bind{{Src: mcpConfig, Dst: mcpConfig}}
	}
	return s
}

// FakeServices starts the scenario's tools on a fakemcp server outside
// the sandbox, so the agent can neither read the canned answers nor
// write the calls log.
func FakeServices() ServiceStarter {
	return func(ctx context.Context, svcs []fakemcp.Tool, callsLog string) ([]byte, func() error, error) {
		urls, stop, err := fakemcp.Start(ctx, svcs, callsLog)
		if err != nil {
			return nil, nil, err
		}
		return fakemcp.MCPConfig(urls), stop, nil
	}
}
