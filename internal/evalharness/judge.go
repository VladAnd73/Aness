package evalharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/versality/spore/internal/research/fakemcp"
	"github.com/versality/spore/internal/statefile"
)

// AllowUnsandboxedEnv must be "1" for Isolation "none": without it a
// judge never runs an agent under test with the host's view of the disk.
const AllowUnsandboxedEnv = "SPORE_RESEARCH_ALLOW_UNSANDBOXED"

// JudgeOptions configures one judge run over one scenario set. Zero
// Repeats, Parallel and Timeout take the defaults 3, 3 and 5 minutes.
// Spawner and Grader default to ClaudeSpawner on AgentBinary. Services
// starts a scenario's fake services; nil refuses a set that declares any.
type JudgeOptions struct {
	SetDir, Candidate, FrozenDir, SkillName, Model, AgentBinary, Isolation, SandboxParent string
	Repeats, Parallel                                                                     int
	Timeout                                                                               time.Duration
	Spawner                                                                               AgentSpawner
	Grader                                                                                AgentSpawner
	Services                                                                              ServiceStarter
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
		o.SandboxParent = os.TempDir()
	}
	if o.Spawner == nil {
		o.Spawner = ClaudeSpawner{Binary: o.AgentBinary}
	}
	if o.Grader == nil {
		o.Grader = ClaudeSpawner{Binary: o.AgentBinary}
	}
	if err := checkIsolation(o.Isolation); err != nil {
		return JudgeReport{}, err
	}
	set, err := LoadSet(o.SetDir)
	if err != nil {
		return JudgeReport{}, err
	}
	for _, sc := range set {
		if len(sc.Services) > 0 && o.Services == nil {
			return JudgeReport{}, fmt.Errorf("evalharness: judge: scenario %s declares fake services and no service starter is configured", sc.Name)
		}
	}
	if err := checkSkillName(o.SkillName); err != nil {
		return JudgeReport{}, err
	}
	if _, err := os.ReadFile(o.Candidate); err != nil {
		return JudgeReport{}, fmt.Errorf("evalharness: judge: candidate: %w", err)
	}

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
				out, err := runOnce(ctx, o, sc)
				mu.Lock()
				defer mu.Unlock()
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("evalharness: judge: scenario %s run %d: %w", sc.Name, ri+1, err)
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

func checkIsolation(mode string) error {
	switch mode {
	case "none":
		if os.Getenv(AllowUnsandboxedEnv) != "1" {
			return fmt.Errorf("evalharness: judge: isolation none runs the agent under test unsandboxed; set %s=1 to allow it", AllowUnsandboxedEnv)
		}
		return nil
	case "", "bwrap":
		return fmt.Errorf("evalharness: judge: isolation bwrap is not implemented yet; use isolation none with %s=1", AllowUnsandboxedEnv)
	default:
		return fmt.Errorf("evalharness: judge: unknown isolation %q (want bwrap or none)", mode)
	}
}

func runOnce(ctx context.Context, o JudgeOptions, sc DataScenario) (runOutcome, error) {
	sb, err := NewSandbox(o.SandboxParent)
	if err != nil {
		return runOutcome{}, err
	}
	// The run dir is its own repository so the agent's project root,
	// where it looks for .claude/skills, is the run dir itself.
	if err := gitInitNoRemote(sb.RunWD); err != nil {
		return runOutcome{}, err
	}
	if err := InjectSkill(sb.RunWD, o.FrozenDir, o.SkillName, o.Candidate); err != nil {
		return runOutcome{}, err
	}
	files := filepath.Join(sc.Dir, "files")
	if _, err := os.Stat(files); err == nil {
		if err := copyTree(files, sb.RunWD); err != nil {
			return runOutcome{}, fmt.Errorf("fixture files: %w", err)
		}
	}

	args := append(append([]string{}, SessionSpawnArgs...), "--strict-mcp-config")
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	run := RunReport{SandboxRoot: sb.Root, TranscriptPath: filepath.Join(sb.Root, "transcript.jsonl")}
	if len(sc.Services) > 0 {
		run.CallsLog = filepath.Join(sb.Root, "calls.jsonl")
		cfg, stop, err := o.Services(ctx, sc.Services, run.CallsLog)
		if err != nil {
			return runOutcome{}, fmt.Errorf("fake services: %w", err)
		}
		defer stop()
		cfgPath := filepath.Join(sb.Root, "mcp.json")
		if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
			return runOutcome{}, err
		}
		args = append(args, "--mcp-config", cfgPath)
	}

	rctx, cancel := context.WithTimeout(ctx, o.Timeout)
	res, spawnErr := o.Spawner.Spawn(rctx, SpawnRequest{Prompt: sc.Prompt, WorkDir: sb.RunWD, Env: sb.Env(), ExtraArgs: args})
	timedOut := errors.Is(rctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
	cancel()
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
		run.Reason = "spawn: " + spawnErr.Error()
	case !stream.SawResult:
		run.Reason = "no result line"
	}
	if run.Reason != "" {
		run.Crashed = true
		out.report = run
		return out, nil
	}

	a := RunArtifacts{RunWD: sb.RunWD, Stream: stream}
	if run.CallsLog != "" {
		if a.Calls, err = fakemcp.ReadCalls(run.CallsLog); err != nil {
			return runOutcome{}, err
		}
	}
	var code, judged []ScenarioCheck
	for _, c := range sc.Checks {
		if c.Kind == "llm-judge" {
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
			ok, err := gradeLLM(ctx, o, sb, filepath.Join(sc.Dir, c.Rubric), stream.FinalReply)
			if err != nil {
				run.Crashed, run.Reason = true, "grader: "+err.Error()
				out.report = run
				return out, nil
			}
			if !ok {
				pass = false
				failed = append(failed, "llm-judge")
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

func WriteJudgeReport(path string, r JudgeReport) error {
	return statefile.WriteJSONAtomic(path, "judge-result", r)
}

// ProbeIsolated runs argv under the same isolation RunJudge gives the
// agent under test and returns its combined output.
func ProbeIsolated(ctx context.Context, o JudgeOptions, argv []string) (string, error) {
	return "", nil
}
