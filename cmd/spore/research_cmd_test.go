package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/versality/spore/internal/research"
)

type researchEnv struct {
	t      *testing.T
	root   string
	store  research.Store
	target string
	goal   string
	deps   researchDeps
}

// newResearchEnv points the research store at a temp XDG_STATE_HOME
// and writes a one-file skill and a goal.
func newResearchEnv(t *testing.T) *researchEnv {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_RUNTIME_DIR", root)
	skill := filepath.Join(root, "skills", "demo")
	if err := os.MkdirAll(skill, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(skill, "SKILL.md")
	if err := os.WriteFile(target, []byte("status: todo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goal := filepath.Join(root, "goal.md")
	if err := os.WriteFile(goal, []byte("Record the run as done.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &researchEnv{t: t, root: root, store: research.Store{Root: filepath.Join(root, "state", "spore", "research")},
		target: target, goal: goal, deps: researchDeps{
			Room:       func() (int, string) { return 0, "room" },
			WeeklyUsed: func(context.Context) (float64, error) { return 20, nil },
			Notify:     func(string) error { return nil },
		}}
}

func (e *researchEnv) run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = researchMain(&out, &errOut, args, e.deps)
	return code, out.String(), errOut.String()
}

func (e *researchEnv) must(args ...string) string {
	e.t.Helper()
	code, out, errOut := e.run(args...)
	if code != 0 {
		e.t.Fatalf("spore research %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

func TestResearchUsage(t *testing.T) {
	e := newResearchEnv(t)
	if code, _, errOut := e.run(); code != 2 || !strings.Contains(errOut, "usage: spore research") {
		t.Errorf("no args: exit %d, stderr %q; want 2 and the usage", code, errOut)
	}
	code, out, _ := e.run("help")
	if code != 0 {
		t.Errorf("help: exit %d", code)
	}
	for _, sub := range []string{"new", "judge draft", "judge approve", "on", "off", "set", "night", "status", "report", "stop", "accept", "reset", "fake-mcp"} {
		if !strings.Contains(out, "  "+sub+" ") {
			t.Errorf("usage lacks %q:\n%s", sub, out)
		}
	}
	if code, _, errOut := e.run("frobnicate"); code != 2 || !strings.Contains(errOut, "frobnicate") {
		t.Errorf("unknown subcommand: exit %d, stderr %q", code, errOut)
	}
	if code, _, _ := e.run("night"); code != 2 {
		t.Errorf("night without a campaign: exit %d, want 2", code)
	}
}

func TestResearchNewRefusesNonSkillTarget(t *testing.T) {
	e := newResearchEnv(t)
	notes := filepath.Join(filepath.Dir(e.target), "notes.md")
	if err := os.WriteFile(notes, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := e.run("new", "c", "--target", notes, "--goal", e.goal)
	if code != 1 || !strings.Contains(errOut, "SKILL.md") {
		t.Errorf("exit %d, stderr %q; want 1 and a message naming SKILL.md", code, errOut)
	}
	if _, err := os.Stat(e.store.Dir("c")); err == nil {
		t.Errorf("a refused new still created the campaign")
	}
}

func TestResearchNightRefusedExitCode(t *testing.T) {
	e := newResearchEnv(t)
	t.Setenv("SPORE_RESEARCH_ALLOW_UNSANDBOXED", "1")
	e.must("new", "c", "--target", e.target, "--goal", e.goal)
	e.must("set", "c", "isolation", "none")
	code, out, errOut := e.run("night", "c")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if out != "" || strings.Count(errOut, "\n") != 1 || !strings.Contains(errOut, "night refused: campaign c is draft") {
		t.Errorf("stdout %q stderr %q; want one stderr line with the reason", out, errOut)
	}
}

func TestResearchNightRefusesUnsandboxedWithoutOptIn(t *testing.T) {
	e := newResearchEnv(t)
	t.Setenv("SPORE_RESEARCH_ALLOW_UNSANDBOXED", "")
	e.must("new", "c", "--target", e.target, "--goal", e.goal)
	e.must("set", "c", "isolation", "none")
	code, _, errOut := e.run("night", "c")
	if code != 1 || !strings.Contains(errOut, "SPORE_RESEARCH_ALLOW_UNSANDBOXED") {
		t.Errorf("exit %d, stderr %q; want 1 naming the opt-in variable", code, errOut)
	}
}

func TestResearchJudgeDraftMintsDraftTask(t *testing.T) {
	e := newResearchEnv(t)
	e.must("new", "c", "--target", e.target, "--goal", e.goal)
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	slug := strings.TrimSpace(e.must("judge", "draft", "c"))
	b, err := os.ReadFile(filepath.Join(project, "tasks", slug+".md"))
	if err != nil {
		t.Fatalf("task file for slug %q: %v", slug, err)
	}
	body := string(b)
	for _, want := range []string{"status: draft", e.target, "research-drafts/c/main", "research-drafts/c/holdout",
		"Record the run as done.", "broken copy", "spore research judge approve c --from"} {
		if !strings.Contains(body, want) {
			t.Errorf("task body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "status: active") {
		t.Errorf("judge draft started the task")
	}
}

func TestResearchStatusLists(t *testing.T) {
	e := newResearchEnv(t)
	e.must("new", "alpha", "--target", e.target, "--goal", e.goal)
	e.must("new", "beta", "--target", e.target, "--goal", e.goal)
	lines := strings.Split(strings.TrimSpace(e.must("status")), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "alpha") || !strings.HasPrefix(lines[1], "beta") {
		t.Fatalf("status lines = %q, want alpha then beta", lines)
	}
	if !regexp.MustCompile(`^alpha\s+draft\s+nights 0/7\s+best -\s+stop -$`).MatchString(lines[0]) {
		t.Errorf("status line %q, want name, status, nights, best score and stop reason", lines[0])
	}
	if one := strings.TrimSpace(e.must("status", "beta")); !strings.HasPrefix(one, "beta") || strings.Contains(one, "alpha") {
		t.Errorf("status beta = %q", one)
	}
	if code, _, _ := e.run("status", "gamma"); code != 1 {
		t.Errorf("status of a missing campaign: exit %d, want 1", code)
	}
}

func TestResearchSetRejectsUnknownKey(t *testing.T) {
	e := newResearchEnv(t)
	e.must("new", "c", "--target", e.target, "--goal", e.goal)
	if code, _, errOut := e.run("set", "c", "best_score", "100"); code != 1 || !strings.Contains(errOut, "not a settable key") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	e.must("set", "c", "model", "haiku")
	if cfg, err := e.store.Load("c"); err != nil || cfg.Model != "haiku" {
		t.Errorf("model after set = %q, %v", cfg.Model, err)
	}
}

func TestResearchResetCleansRepo(t *testing.T) {
	e := newResearchEnv(t)
	e.must("new", "c", "--target", e.target, "--goal", e.goal)
	repoTarget := filepath.Join(e.store.Dir("c"), "repo", "SKILL.md")
	if err := os.WriteFile(repoTarget, []byte("half\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.must("reset", "c")
	if b, _ := os.ReadFile(repoTarget); string(b) != "status: todo\n" {
		t.Errorf("repo SKILL.md after reset = %q", b)
	}
	if !strings.Contains(out, "reset c to ") {
		t.Errorf("reset stdout %q", out)
	}
}

func TestResearchNightPrintsOneLinePerTry(t *testing.T) {
	e := newResearchEnv(t)
	t.Setenv("SPORE_RESEARCH_ALLOW_UNSANDBOXED", "1")
	fixture, _ := filepath.Abs("testdata/research-e2e")
	e.deps.Editor = &scriptedEditor{}
	e.deps.SporeBinary = buildSporeBin(t)
	live := filepath.Join(e.root, "live", "e2e", "SKILL.md")
	os.MkdirAll(filepath.Dir(live), 0o700)
	orig, _ := os.ReadFile(filepath.Join(fixture, "skill", "SKILL.md"))
	os.WriteFile(live, orig, 0o600)
	e.must("new", "e2e", "--target", live, "--goal", e.goal)
	e.must("set", "e2e", "isolation", "none")
	e.must("set", "e2e", "agent_binary", filepath.Join(fixture, "fake-claude.sh"))
	e.must("set", "e2e", "max_tries_per_night", "2")
	e.must("judge", "approve", "e2e", "--from", filepath.Join(fixture, "drafts"))
	e.must("on", "e2e")
	out := e.must("night", "e2e")
	line := regexp.MustCompile(`(?m)^try (\d+)\s+(\S+)\s+score (\d+\.\d)\s+bytes (\d+)\s+elapsed \S+$`)
	got := line.FindAllStringSubmatch(out, -1)
	if len(got) != 3 || got[0][1] != "0" || got[0][2] != "baseline" || got[1][2] != "reset" || got[2][2] != "kept" || got[2][3] != "100.0" {
		t.Errorf("night stdout has tries %v, want baseline, reset, kept (100.0):\n%s", got, out)
	}
	if !strings.Contains(out, "stop reason: max-tries") {
		t.Errorf("night stdout lacks the stop reason:\n%s", out)
	}
}

// signalCampaign builds a campaign whose baseline is already recorded
// and whose judge blocks, so a try is mid-judge with a dirty commit
// when the test signals the night.
func signalCampaign(t *testing.T, root string) (store research.Store, entered string) {
	t.Helper()
	store = research.Store{Root: filepath.Join(root, "state", "spore", "research")}
	skill := filepath.Join(root, "skills", "sig")
	os.MkdirAll(skill, 0o700)
	target := filepath.Join(skill, "SKILL.md")
	os.WriteFile(target, []byte("status: todo\n"), 0o600)
	goal := filepath.Join(root, "goal.md")
	os.WriteFile(goal, []byte("goal\n"), 0o600)
	drafts, _ := filepath.Abs("testdata/research-e2e/drafts")
	if _, err := store.Create("sig", target, goal, "maximize-score"); err != nil {
		t.Fatal(err)
	}
	entered = filepath.Join(root, "entered")
	judge := filepath.Join(root, "judge.sh")
	os.WriteFile(judge, []byte("#!/bin/sh\ntouch "+entered+"\nexec sleep 60\n"), 0o700)
	editor := filepath.Join(root, "editor.sh")
	os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'status: done\\n' > SKILL.md\necho done > .try-description\n"), 0o700)
	if _, err := store.ApproveJudge("sig", drafts, judge, []string{judge}); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load("sig")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Status, cfg.Isolation, cfg.AgentBinary = research.StatusOn, "none", editor
	cfg.BestCommit, cfg.BestScore = cfg.StartCommit, 0
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return store, entered
}

// TestResearchNightHelperProcess is not a test: the signal test runs
// the test binary itself as a child that executes `spore research`
// with fake budget readers, so no real usage endpoint is reached.
func TestResearchNightHelperProcess(t *testing.T) {
	if os.Getenv("SPORE_RESEARCH_HELPER") != "1" {
		t.Skip("helper process for TestResearchNightSignalsLeaveCleanRepo")
	}
	deps := researchDeps{
		Room:       func() (int, string) { return 0, "room" },
		WeeklyUsed: func(context.Context) (float64, error) { return 20, nil },
		Notify:     func(string) error { return nil },
	}
	os.Exit(researchMain(os.Stdout, os.Stderr, strings.Fields(os.Getenv("SPORE_RESEARCH_HELPER_ARGS")), deps))
}

func TestResearchNightSignalsLeaveCleanRepo(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			root := t.TempDir()
			store, entered := signalCampaign(t, root)
			cmd := exec.Command(os.Args[0], "-test.run=^TestResearchNightHelperProcess$")
			cmd.Env = append(os.Environ(), "SPORE_RESEARCH_HELPER=1", "SPORE_RESEARCH_HELPER_ARGS=night sig",
				"XDG_STATE_HOME="+filepath.Join(root, "state"), "XDG_RUNTIME_DIR="+root,
				"SPORE_RESEARCH_ALLOW_UNSANDBOXED=1")
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(30 * time.Second)
			for {
				if _, err := os.Stat(entered); err == nil {
					break
				}
				if time.Now().After(deadline) {
					cmd.Process.Kill()
					cmd.Wait()
					t.Fatalf("the judge never started:\n%s", out.String())
				}
				time.Sleep(20 * time.Millisecond)
			}
			cmd.Process.Signal(sig)
			waited := make(chan error, 1)
			go func() { waited <- cmd.Wait() }()
			select {
			case <-waited:
			case <-time.After(20 * time.Second):
				cmd.Process.Kill()
				t.Fatalf("night did not exit after %v:\n%s", sig, out.String())
			}
			if !strings.Contains(out.String(), "stop reason: interrupted") {
				t.Errorf("night output lacks the interrupted stop reason:\n%s", out.String())
			}
			repoDir := filepath.Join(store.Dir("sig"), "repo")
			status, _ := exec.Command("git", "-C", repoDir, "status", "--porcelain").Output()
			head, _ := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()
			cfg, _ := store.Load("sig")
			if len(status) != 0 || strings.TrimSpace(string(head)) != cfg.BestCommit {
				t.Errorf("repo after %v: status %q head %s, want clean on %s", sig, status, head, cfg.BestCommit)
			}
			unlock, holder, err := research.Lock(filepath.Join(root, "spore-research.lock"), "next")
			if err != nil {
				t.Errorf("lock still held by %q after %v: %v", holder, sig, err)
			} else {
				unlock()
			}
		})
	}
}

func TestResearchFakeMCPPrintsURL(t *testing.T) {
	bin := buildSporeBin(t)
	dir := t.TempDir()
	services := filepath.Join(dir, "services.json")
	os.WriteFile(services, []byte(`[{"server":"claude_ai_Notion","name":"notion-search","response":{"results":[]}}]`), 0o600)
	cmd := exec.Command(bin, "research", "fake-mcp", "--services", services, "--calls-log", filepath.Join(dir, "calls.jsonl"))
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read first line: %v", err)
	}
	if !regexp.MustCompile(`^claude_ai_Notion http://127\.0\.0\.1:\d+/mcp/claude_ai_Notion\n$`).MatchString(line) {
		t.Errorf("first line %q, want `<server> <url>`", line)
	}
}
