package research

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeEditor struct {
	edits []func(path string) error
	calls int
}

func (e *fakeEditor) Propose(ctx context.Context, req EditRequest) (EditResult, error) {
	e.calls++
	if e.calls <= len(e.edits) {
		if err := e.edits[e.calls-1](filepath.Join(req.ScratchDir, req.TargetRel)); err != nil {
			return EditResult{ExitCode: 1}, err
		}
	}
	return EditResult{Description: fmt.Sprintf("try %d", e.calls)}, nil
}

func appendBytes(s string) func(string) error {
	return func(path string) error {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(s)
		return err
	}
}

func writeBytes(n int) func(string) error {
	return func(path string) error {
		return os.WriteFile(path, bytes.Repeat([]byte("x"), n), 0o600)
	}
}

// fakeJudge serves scores to "main" runs in order, repeating the last
// one once the list runs out. holdout, when set, scores "holdout" runs
// from the candidate's bytes.
type fakeJudge struct {
	mu           sync.Mutex
	scores       []float64
	holdout      func(candidate []byte) float64
	calls        int
	holdoutCalls int
	err          error
}

func (j *fakeJudge) Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.err != nil {
		return JudgeResult{}, j.err
	}
	var score float64
	if set == "holdout" {
		j.holdoutCalls++
		b, err := os.ReadFile(candidate)
		if err != nil {
			return JudgeResult{}, err
		}
		if j.holdout != nil {
			score = j.holdout(b)
		}
	} else {
		j.calls++
		if len(j.scores) > 0 {
			score = j.scores[min(j.calls, len(j.scores))-1]
		}
	}
	res := JudgeResult{Score: score, Runs: 3, Passed: int(score * 3 / 100)}
	b, _ := json.Marshal(res)
	if err := os.WriteFile(outPath, b, 0o600); err != nil {
		return JudgeResult{}, err
	}
	return res, nil
}

type fixture struct {
	store      Store
	name       string
	liveDir    string
	liveTarget string
	original   []byte
	approved   Config
	notes      *[]string
}

const fixtureName = "demo"

// newCampaign builds a campaign on a 100-byte SKILL.md with one frozen
// sibling, approves the testdata/drafts judge sets, switches it on,
// and applies cfgEdits to the saved config.
func newCampaign(t *testing.T, cfgEdits func(*Config)) fixture {
	t.Helper()
	root := t.TempDir()
	store := Store{Root: filepath.Join(root, "state", "spore", "research")}
	liveDir := filepath.Join(root, "skills", fixtureName)
	if err := os.MkdirAll(liveDir, 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(strings.Repeat("a", 99) + "\n")
	target := filepath.Join(liveDir, "SKILL.md")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "reference.md"), []byte("frozen\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goal := filepath.Join(root, "goal.md")
	if err := os.WriteFile(goal, []byte("Make the skill pass more scenarios.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "spore-binary")
	if err := os.WriteFile(binary, []byte("not a real binary\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(fixtureName, target, goal, "maximize-score"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	approved, err := store.ApproveJudge(fixtureName, "testdata/drafts", binary, []string{binary, "eval", "judge"})
	if err != nil {
		t.Fatalf("ApproveJudge: %v", err)
	}
	if err := store.SetOn(fixtureName, true); err != nil {
		t.Fatalf("SetOn: %v", err)
	}
	cfg, err := store.Load(fixtureName)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfgEdits != nil {
		cfgEdits(&cfg)
		if err := store.Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	return fixture{store: store, name: fixtureName, liveDir: liveDir, liveTarget: target, original: original, approved: approved, notes: new([]string)}
}

func (f fixture) deps(ed Editor, j Judge) Deps {
	return Deps{
		Editor:     ed,
		Judge:      j,
		Room:       func() (int, string) { return 0, "room" },
		WeeklyUsed: func(context.Context) (float64, error) { return 20.0, nil },
		Notify: func(msg string) error {
			*f.notes = append(*f.notes, msg)
			return nil
		},
		LockPath: filepath.Join(filepath.Dir(f.store.Root), "research.lock"),
	}
}

func (f fixture) repoDir() string { return filepath.Join(f.store.Dir(f.name), "repo") }

func (f fixture) load(t *testing.T) Config {
	t.Helper()
	cfg, err := f.store.Load(f.name)
	if err != nil {
		t.Errorf("Load: %v", err)
	}
	return cfg
}

// repoHead reports a missing or broken campaign repo as a test error
// and returns "" so the caller's comparisons still run.
func (f fixture) repoHead(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "-C", f.repoDir(), "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Errorf("campaign repo HEAD: %v: %s", err, strings.TrimSpace(string(out)))
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (f fixture) readArtifact(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.store.Dir(f.name), rel))
	if err != nil {
		t.Errorf("read %s: %v", rel, err)
	}
	return string(b)
}

// resultRows parses results.tsv by its header line, so the tests do
// not pin a column order.
func (f fixture) resultRows(t *testing.T) []map[string]string {
	t.Helper()
	text := f.readArtifact(t, "results.tsv")
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) < 2 {
		t.Errorf("results.tsv: want a header and at least one row, got %q", text)
		return nil
	}
	header := strings.Split(lines[0], "\t")
	var rows []map[string]string
	for _, line := range lines[1:] {
		row := map[string]string{}
		for i, cell := range strings.Split(line, "\t") {
			if i < len(header) {
				row[header[i]] = cell
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (f fixture) lastRow(t *testing.T) map[string]string {
	t.Helper()
	rows := f.resultRows(t)
	if len(rows) == 0 {
		return map[string]string{}
	}
	return rows[len(rows)-1]
}

// wantBaselineHead checks the campaign repo sits on the starting
// commit, which must be recorded and must be the best commit.
func (f fixture) wantBaselineHead(t *testing.T, cfg Config) {
	t.Helper()
	head := f.repoHead(t)
	if cfg.StartCommit == "" {
		t.Errorf("cfg.StartCommit is empty: the baseline commit was never recorded")
	}
	if head != cfg.StartCommit {
		t.Errorf("repo HEAD = %q, want the baseline commit %q", head, cfg.StartCommit)
	}
	if head != cfg.BestCommit {
		t.Errorf("repo HEAD = %q, want cfg.BestCommit %q", head, cfg.BestCommit)
	}
}
