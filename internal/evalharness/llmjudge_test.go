package evalharness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLLMJudgeCheckUsesRubric(t *testing.T) {
	set := t.TempDir()
	dir := filepath.Join(set, "judged")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "rubric.md"), []byte("RUBRIC-TEXT: the reply must be polite"), 0o644)
	os.WriteFile(filepath.Join(dir, "scenario.json"), []byte(`{"prompt":"pass a","llm_judge":true,"checks":[{"kind":"llm-judge","rubric":"rubric.md"}]}`), 0o644)

	for _, verdict := range []string{"VERDICT: PASS", "VERDICT: FAIL", "looks fine"} {
		var graderPrompt string
		grader := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
			graderPrompt = req.Prompt
			return SpawnResult{Stdout: "thinking it over\n" + verdict + "\n"}, nil
		}}
		agent := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
			return SpawnResult{Stdout: resultLine("FINAL-REPLY-TEXT", 0)}, nil
		}}
		o := judgeOpts(t, set, agent)
		o.Repeats, o.Grader = 1, grader

		rep, err := RunJudge(context.Background(), o)
		if err != nil {
			t.Fatalf("RunJudge: %v", err)
		}
		if !strings.Contains(graderPrompt, "RUBRIC-TEXT") || !strings.Contains(graderPrompt, "FINAL-REPLY-TEXT") {
			t.Errorf("grader prompt lacks the rubric or the reply:\n%s", graderPrompt)
		}
		if want := verdict == "VERDICT: PASS"; (rep.Passed == 1) != want {
			t.Errorf("verdict %q: passed = %d, want pass %v", verdict, rep.Passed, want)
		}
	}
}

func TestLLMJudgeGraderNonZeroExitIsCrash(t *testing.T) {
	set := t.TempDir()
	dir := filepath.Join(set, "judged")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "rubric.md"), []byte("be polite"), 0o644)
	os.WriteFile(filepath.Join(dir, "scenario.json"), []byte(`{"prompt":"pass a","llm_judge":true,"checks":[{"kind":"llm-judge","rubric":"rubric.md"}]}`), 0o644)
	grader := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		return SpawnResult{Stdout: "VERDICT: PASS\n", ExitCode: 1}, nil
	}}
	agent := &scriptSpawner{fn: func(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
		return SpawnResult{Stdout: resultLine("reply", 0)}, nil
	}}
	o := judgeOpts(t, set, agent)
	o.Repeats, o.Grader = 1, grader

	rep, err := RunJudge(context.Background(), o)
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	run := firstRun(t, rep)
	if run.Pass || !run.Crashed || rep.Passed != 0 {
		t.Errorf("grader exited 1: run = %+v, passed %d; want a crash, never a pass", run, rep.Passed)
	}
}
