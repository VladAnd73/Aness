package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/versality/spore/internal/evalharness"
)

const evalUsage = `usage: spore eval <list|run|judge> [flags]

  list
         List every registered eval scenario: its name, which dream
         brief it exercises, and what it checks.
  run <scenario> [--brief-override PATH] [--sandbox-parent DIR] [--agent-binary PATH]
      run --all [--sandbox-parent DIR] [--agent-binary PATH]
         Spawn a real, sandboxed Claude Code agent against one
         scenario (or every registered scenario with --all) and grade
         the result. Exits non-zero if any scenario fails or errors.

  judge --scenarios-root DIR --skill NAME --candidate PATH --set main|holdout --out PATH
        [--frozen DIR] [--model M] [--repeats 3] [--parallel 3] [--timeout 5m]
        [--agent-binary claude] [--isolation bwrap|none] [--keep DIR]
         Score a candidate SKILL.md on the scenario set DIR/<set>: every
         scenario runs --repeats times, each in its own sandbox with the
         candidate installed as skill NAME over the --frozen folder. Writes
         the JSON score to --out and exits 0 whatever the score; exits 1 when
         the judge itself failed. --isolation bwrap (the default) is
         refused until bwrap isolation exists; --isolation none runs
         unsandboxed and needs SPORE_RESEARCH_ALLOW_UNSANDBOXED=1. Sandboxes are deleted unless
         --keep names a directory to leave them (transcripts included) in.

--brief-override replaces the embedded proposer or reviewer brief (the
one the scenario's own Kind uses) with the file's contents, for
comparing a baseline brief against a candidate. Not valid with --all,
since a single override file cannot target scenarios of both kinds.

--agent-binary runs that executable instead of claude on PATH; it gets
the same headless claude arguments.

--sandbox-parent overrides where each scenario's throwaway sandbox
directory is created (default: the OS temp directory). Every run is
fully sandboxed regardless: no git remote, no GitHub token, and its
own state directory - see internal/evalharness.
`

func runEval(args []string) int { return evalMain(os.Stdout, os.Stderr, args, nil) }

// evalMain is evalharness's CLI entry point. spawner is nil in normal
// use (RunScenario then defaults to a real ClaudeSpawner); tests pass a
// fake so this wiring can be exercised without starting a real agent.
func evalMain(out, errOut io.Writer, args []string, spawner evalharness.AgentSpawner) int {
	if len(args) < 1 {
		fmt.Fprint(errOut, evalUsage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(out, evalUsage)
		return 0
	case "list":
		return evalList(out)
	case "run":
		return evalRun(out, errOut, args[1:], spawner)
	case "judge":
		return evalJudge(out, errOut, args[1:], spawner)
	default:
		fmt.Fprintf(errOut, "spore eval: unknown subcommand %q\n\n%s", args[0], evalUsage)
		return 2
	}
}

func evalList(out io.Writer) int {
	for _, sc := range evalharness.Scenarios {
		fmt.Fprintf(out, "%s [%s]\n  %s\n", sc.Name, sc.Kind, sc.Description)
	}
	return 0
}

func evalRun(out, errOut io.Writer, args []string, spawner evalharness.AgentSpawner) int {
	fs := flag.NewFlagSet("eval run", flag.ContinueOnError)
	fs.SetOutput(errOut)
	all := fs.Bool("all", false, "run every registered scenario")
	briefOverride := fs.String("brief-override", "", "path to a brief file that replaces the scenario's embedded brief")
	sandboxParent := fs.String("sandbox-parent", "", "directory each scenario's sandbox is created under (default: OS temp dir)")
	agentBinary := fs.String("agent-binary", "", "agent executable to spawn (default: claude on PATH)")
	if err := fs.Parse(reorderFlagsFirst(fs, args)); err != nil {
		return 2
	}

	var scenarios []evalharness.Scenario
	if *all {
		if *briefOverride != "" {
			fmt.Fprintln(errOut, "spore eval run: --brief-override is not valid with --all: it cannot target scenarios of both kinds at once")
			return 2
		}
		scenarios = evalharness.Scenarios
	} else {
		if fs.NArg() != 1 {
			fmt.Fprint(errOut, "spore eval run: exactly one <scenario> is required, or pass --all\n\n"+evalUsage)
			return 2
		}
		sc, ok := evalharness.Find(fs.Arg(0))
		if !ok {
			fmt.Fprintf(errOut, "spore eval run: unknown scenario %q\n\n%s", fs.Arg(0), evalUsage)
			return 2
		}
		scenarios = []evalharness.Scenario{sc}
	}

	var overrideText string
	if *briefOverride != "" {
		b, err := os.ReadFile(*briefOverride)
		if err != nil {
			fmt.Fprintf(errOut, "spore eval run: --brief-override: %v\n", err)
			return 2
		}
		overrideText = string(b)
	}

	if spawner == nil && *agentBinary != "" {
		spawner = evalharness.ClaudeSpawner{Binary: *agentBinary}
	}
	allPass := true
	for _, sc := range scenarios {
		opts := evalharness.RunOptions{Spawner: spawner, SandboxParent: *sandboxParent}
		switch sc.Kind {
		case evalharness.KindProposer:
			opts.ProposerBriefOverride = overrideText
		case evalharness.KindReviewer:
			opts.ReviewerBriefOverride = overrideText
		}
		outcome, err := evalharness.RunScenario(context.Background(), sc, opts)
		if err != nil {
			fmt.Fprintf(errOut, "%s: ERROR: %v\n", sc.Name, err)
			allPass = false
			continue
		}
		status := "PASS"
		if !outcome.Pass {
			status = "FAIL"
			allPass = false
		}
		fmt.Fprintf(out, "%s: %s (%s)\n  sandbox: %s\n", sc.Name, status, outcome.Reason, outcome.SandboxRoot)
	}
	if !allPass {
		return 1
	}
	return 0
}

func evalJudge(out, errOut io.Writer, args []string, spawner evalharness.AgentSpawner) int {
	fs := flag.NewFlagSet("eval judge", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("scenarios-root", "", "directory holding the main and holdout scenario sets")
	set := fs.String("set", "", "main or holdout")
	candidate := fs.String("candidate", "", "candidate SKILL.md")
	frozen := fs.String("frozen", "", "skill folder the candidate is installed over")
	skill := fs.String("skill", "", "skill name the agent under test loads it as")
	outPath := fs.String("out", "", "result JSON path")
	model := fs.String("model", "", "model for the agent under test (empty: the CLI default)")
	repeats := fs.Int("repeats", 3, "runs per scenario")
	parallel := fs.Int("parallel", 3, "scenario runs at once")
	timeout := fs.Duration("timeout", 5*time.Minute, "limit per scenario run")
	agentBinary := fs.String("agent-binary", "claude", "agent CLI to run")
	isolation := fs.String("isolation", "bwrap", "bwrap or none")
	keep := fs.String("keep", "", "leave every run's sandbox under this directory")
	if err := fs.Parse(reorderFlagsFirst(fs, args)); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *root == "" || *candidate == "" || *skill == "" || *outPath == "" {
		fmt.Fprint(errOut, "spore eval judge: --scenarios-root, --skill, --candidate, --set and --out are required\n\n"+evalUsage)
		return 2
	}
	if *set != "main" && *set != "holdout" {
		fmt.Fprintf(errOut, "spore eval judge: --set must be main or holdout, got %q\n", *set)
		return 2
	}

	parent := *keep
	if parent == "" {
		tmp, err := os.MkdirTemp("", "spore-judge-")
		if err != nil {
			fmt.Fprintf(errOut, "spore eval judge: %v\n", err)
			return 1
		}
		defer os.RemoveAll(tmp)
		parent = tmp
	} else if err := os.MkdirAll(parent, 0o700); err != nil {
		fmt.Fprintf(errOut, "spore eval judge: --keep: %v\n", err)
		return 1
	}

	rep, err := evalharness.RunJudge(context.Background(), evalharness.JudgeOptions{
		SetDir:        filepath.Join(*root, *set),
		Candidate:     *candidate,
		FrozenDir:     *frozen,
		SkillName:     *skill,
		Model:         *model,
		AgentBinary:   *agentBinary,
		Isolation:     *isolation,
		SandboxParent: parent,
		Repeats:       *repeats,
		Parallel:      *parallel,
		Timeout:       *timeout,
		Spawner:       spawner,
	})
	if err != nil {
		fmt.Fprintf(errOut, "spore eval judge: %v\n", err)
		return 1
	}
	if err := evalharness.WriteJudgeReport(*outPath, rep); err != nil {
		fmt.Fprintf(errOut, "spore eval judge: write result: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "score %.1f: %d of %d runs passed, %d crashed\n", rep.Score, rep.Passed, rep.Runs, rep.Crashes)
	return 0
}
