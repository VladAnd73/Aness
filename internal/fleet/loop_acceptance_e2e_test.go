package fleet

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/task"
)

// Acceptance scenario 10: a QA-shaped loop end to end through
// Reconcile, with fake panes and a fake GitHub.

// loopFakeGHScript is upstream rolegithub_test.go's fake gh, trimmed:
// it logs each call with its token and answers pr list / create / view.
const loopFakeGHScript = `#!/bin/sh
{ printf 'ARGS:'; for a in "$@"; do printf ' %s' "$a"; done; printf '\nTOKEN: %s\n' "$GH_TOKEN"; } >> "$FAKE_GH_LOG"
case "$1 $2" in
  "pr list") echo '[]' ;;
  "pr create") echo "https://github.com/acme/widget/pull/42" ;;
  "pr view") echo '{"number":42,"url":"https://github.com/acme/widget/pull/42","state":"OPEN"}' ;;
esac
`

type loopGitHub struct {
	log, bare string
}

// loopFakeGitHub puts a fake gh on PATH, points origin at a local bare
// repo through an https insteadOf, and writes a github.toml whose token
// mint is an httptest server. Mirrors upstream's newMirrorFixture.
func loopFakeGitHub(t *testing.T, root string) *loopGitHub {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	g := &loopGitHub{
		log:  filepath.Join(t.TempDir(), "gh.log"),
		bare: filepath.Join(t.TempDir(), "widget.git"),
	}
	git := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git(filepath.Dir(g.bare), "init", "-q", "--bare", g.bare)
	git(root, "remote", "add", "origin", "https://github.com/acme/widget.git")
	git(root, "config", "url."+g.bare+".insteadOf", "https://github.com/acme/widget.git")
	git(root, "push", "-q", "origin", "main")
	git(root, "fetch", "-q", "origin")
	git(root, "remote", "set-head", "origin", "main")

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(loopFakeGHScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_LOG", g.log)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemPath := filepath.Join(t.TempDir(), "app.pem")
	if err := os.WriteFile(pemPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_app", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	}))
	t.Cleanup(srv.Close)
	cfg := "app_id = 1\ninstallation_id = 2\nprivate_key = \"" + pemPath +
		"\"\noperator_login = \"octo\"\ncommit_name = \"Octo Cat\"\ncommit_email = \"octo@example.com\"\napi_url = \"" + srv.URL + "\"\n"
	stateDir, err := task.StateDirForProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "github.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return g
}

// calls returns each logged gh call as "<args>\x00<token>".
func (g *loopGitHub) calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(g.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if a, ok := strings.CutPrefix(l, "ARGS: "); ok {
			tok := ""
			if i+1 < len(lines) {
				tok, _ = strings.CutPrefix(lines[i+1], "TOKEN: ")
			}
			out = append(out, a+"\x00"+tok)
		}
	}
	return out
}

// hasFlag reports whether args carries flag with value, as "--f v",
// "--f=v" or a comma list value.
func hasFlag(args, flag, value string) bool {
	f := strings.Fields(args)
	for i, a := range f {
		var v string
		switch {
		case a == flag && i+1 < len(f):
			v = f[i+1]
		case strings.HasPrefix(a, flag+"="):
			v = strings.TrimPrefix(a, flag+"=")
		default:
			continue
		}
		for _, part := range strings.Split(v, ",") {
			if part == value {
				return true
			}
		}
	}
	return false
}

func TestAcceptanceS10QALoopEndToEnd(t *testing.T) {
	requireToolchain(t)
	dirs := newTestDirs(t)
	gitInit(t, dirs.project)
	mustEnable(t)
	t.Setenv("SPORE_AGENT_BINARY", "sleep 30")
	coordState := t.TempDir()
	t.Setenv("SPORE_COORDINATOR_STATE_DIR", coordState)
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "Test")
		t.Setenv(k+"_EMAIL", "test@example.com")
	}
	project := filepath.Base(dirs.project)
	t.Cleanup(func() { killLoopTestSessions(project) })
	root, slug := dirs.project, "qa"
	gh := loopFakeGitHub(t, root)

	wt := filepath.Join(root, ".worktrees", slug)
	if out, err := exec.Command("git", "-C", root, "worktree", "add", "-q", "-b", "wt/"+slug, wt).CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	copyLoopFixture(t, "qa-standin.toml", filepath.Join(root, "bootstrap", "loops", "qa-standin.toml"))
	writeLoopRoleBodies(t, root, "analyst", "author", "reviewer", "completeness", "reporter")
	writeTaskWithExtra(t, dirs.tasks, slug, "active", "loop", "qa-standin")
	panes := installLoopPanes(t)
	f := &loopFixture{root: root, tasks: dirs.tasks, slug: slug, project: project, panes: panes}

	rounds := map[string]int{}
	act := func(step string) {
		rounds[step]++
		n := rounds[step]
		rel := fmt.Sprintf("%s/round-%d.json", step, n)
		switch step {
		case "author":
			name := fmt.Sprintf("author-%d.txt", n)
			if err := os.WriteFile(filepath.Join(wt, name), []byte(name+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"add", name}, {"commit", "-qm", name}} {
				if out, err := exec.Command("git", append([]string{"-C", wt}, args...)...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
			}
			f.write(t, rel, `{"notes":"authored"}`)
		case "review":
			v := task.VerdictApprove
			if n == 1 {
				v = task.VerdictRequestChanges
			}
			f.write(t, rel, verdictJSON(step, v))
		case "completeness":
			f.write(t, rel, verdictJSON(step, task.VerdictApprove))
		default:
			f.write(t, rel, `{"notes":"done"}`)
		}
	}

	// An agent works once each time its pane is spawned or woken.
	sessionPrefix := f.session("")
	seenSpawns, seenWakes := 0, 0
	var tickErrs []string
	for tick := 1; tick <= 30 && !f.exists("state/ready"); tick++ {
		if _, err := Reconcile(Config{TasksDir: dirs.tasks, ProjectRoot: root, MaxWorkers: 3}); err != nil {
			tickErrs = append(tickErrs, fmt.Sprintf("tick %d: %v", tick, err))
		}
		active := map[string]bool{}
		for _, s := range panes.spawns[seenSpawns:] {
			active[s] = true
		}
		for _, w := range panes.wakes[seenWakes:] {
			active[w.session] = true
		}
		seenSpawns, seenWakes = len(panes.spawns), len(panes.wakes)
		for _, step := range []string{"intake", "author", "review", "completeness", "report"} {
			if active[sessionPrefix+step] {
				act(step)
			}
		}
	}
	if len(tickErrs) > 0 {
		t.Logf("Reconcile errors: %v", tickErrs)
	}

	if !f.exists("state/ready") {
		t.Errorf("qa stand-in loop did not reach done in 30 ticks; spawns = %v, agent rounds = %v", panes.spawns, rounds)
	}
	var creates []string
	for _, c := range gh.calls(t) {
		args, tok, _ := strings.Cut(c, "\x00")
		if strings.Contains(args, "merge") {
			t.Errorf("gh call merges: %q", args)
		}
		if strings.HasPrefix(args, "pr create") {
			creates = append(creates, args)
			if tok != "ghs_app" {
				t.Errorf("pr create ran with token %q, want the app token ghs_app", tok)
			}
		}
	}
	if len(creates) != 1 {
		t.Fatalf("pr create calls = %q, want exactly 1", creates)
	}
	labelled, assigned := hasFlag(creates[0], "--label", "QA"), hasFlag(creates[0], "--assignee", "octo")
	for _, c := range gh.calls(t) {
		args, _, _ := strings.Cut(c, "\x00")
		if strings.HasPrefix(args, "pr edit") && strings.Contains(args, "42") {
			labelled = labelled || hasFlag(args, "--add-label", "QA")
			assigned = assigned || hasFlag(args, "--add-assignee", "octo")
		}
	}
	if !labelled {
		t.Errorf("PR not labelled QA (create: %q)", creates[0])
	}
	if !assigned {
		t.Errorf("PR not assigned to operator_login octo (create: %q)", creates[0])
	}

	head, err := exec.Command("git", "-C", wt, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	pushed, _ := exec.Command("git", "-C", gh.bare, "rev-parse", "refs/heads/wt/"+slug).Output()
	if strings.TrimSpace(string(pushed)) != strings.TrimSpace(string(head)) {
		t.Errorf("remote wt/%s = %q, want worktree HEAD %q (the second open-pr visit must push)",
			slug, strings.TrimSpace(string(pushed)), strings.TrimSpace(string(head)))
	}

	var gs struct {
		PRNumber int `json:"pr_number"`
	}
	b, _ := os.ReadFile(f.path("github.json"))
	_ = json.Unmarshal(b, &gs)
	if gs.PRNumber != 42 {
		t.Errorf("github.json pr_number = %d, want 42 (body %q)", gs.PRNumber, b)
	}
}
