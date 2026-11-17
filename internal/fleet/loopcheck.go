package fleet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/versality/spore/internal/task"
)

const checkOutputTail = 4 << 10

// runLoopCommand runs run under sh -c in dir. exit is -1 when the
// command did not finish (timeout); err covers only failures to start
// it. The whole process group is killed on cancel so a backgrounded
// grandchild cannot hold the output pipe open past the timeout.
var runLoopCommand = func(ctx context.Context, dir string, env []string, run string) (exit int, out string, err error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", run)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdout, cmd.Stderr = &buf, &buf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		return -1, buf.String(), nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), buf.String(), nil
	}
	if err != nil {
		return -1, buf.String(), err
	}
	return 0, buf.String(), nil
}

// checkFingerprint keys a check result on what it judged: the step
// output's bytes, the worktree's HEAD and the command itself.
func checkFingerprint(outputPath, worktree, run string) (string, error) {
	b, err := os.ReadFile(outputPath)
	if err != nil {
		return "", err
	}
	head, err := exec.Command("git", "-C", worktree, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %w", worktree, err)
	}
	h := sha256.New()
	for _, part := range [][]byte{b, bytes.TrimSpace(head), []byte(run)} {
		fmt.Fprintf(h, "%d:", len(part))
		h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type checkResult struct {
	Exit   int    `json:"exit"`
	Output string `json:"output"`
	Run    string `json:"run"`
}

func checkResultPath(projectRoot, slug, step string, round int, fp string) string {
	return filepath.Join(task.RoleTaskDir(projectRoot, slug), "state", fmt.Sprintf("check-%s-r%d-%s.json", step, round, fp[:12]))
}

// evalCommand judges command condition c of step s at round. A result
// is cached per fingerprint and never rerun. Once any fingerprint of
// (step, round, run) passed, the round stays passed: a later HEAD must
// not reopen work the loop already moved past.
func evalCommand(projectRoot, slug string, s Step, round int, c Condition) (passed bool, out string, fp string, err error) {
	if cachedCommandPass(projectRoot, slug, s.Name, round, c.Run) {
		return true, "", "", nil
	}
	output := task.StepOutputPath(projectRoot, slug, s.Output, round)
	worktree := filepath.Join(projectRoot, ".worktrees", slug)
	fp, err = checkFingerprint(output, worktree, c.Run)
	if err != nil {
		return false, "", "", err
	}
	path := checkResultPath(projectRoot, slug, s.Name, round, fp)
	if r, ok := readCheckResult(path); ok {
		return r.Exit == 0, r.Output, fp, nil
	}

	timeout := time.Duration(c.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCommandTimeoutSeconds * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	env := append(os.Environ(), "SPORE_TASK_SLUG="+slug, "SPORE_STEP_OUTPUT="+output)
	exit, out, err := runLoopCommand(ctx, worktree, env, c.Run)
	if err != nil {
		return false, "", fp, fmt.Errorf("run check %q: %w", c.Run, err)
	}
	if ctx.Err() != nil {
		out = strings.TrimRight(out, "\n")
		if out != "" {
			out += "\n"
		}
		out += fmt.Sprintf("timed out after %s", timeout)
	}
	if len(out) > checkOutputTail {
		out = out[len(out)-checkOutputTail:]
	}
	r := checkResult{Exit: exit, Output: out, Run: c.Run}
	if err := writeCheckResult(path, r); err != nil {
		return false, out, fp, err
	}
	return exit == 0, out, fp, nil
}

func readCheckResult(path string) (checkResult, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return checkResult{}, false
	}
	var r checkResult
	if json.Unmarshal(b, &r) != nil {
		return checkResult{}, false
	}
	return r, true
}

func writeCheckResult(path string, r checkResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// cachedCommandPass reports whether any cached result of run for
// (step, round) passed. The file name does not carry the run, so the
// result records it.
func cachedCommandPass(projectRoot, slug, step string, round int, run string) bool {
	dir := filepath.Join(task.RoleTaskDir(projectRoot, slug), "state")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	prefix := fmt.Sprintf("check-%s-r%d-", step, round)
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".json") {
			continue
		}
		if r, ok := readCheckResult(filepath.Join(dir, n)); ok && r.Exit == 0 && r.Run == run {
			return true
		}
	}
	return false
}

// checkFailure is the failing command condition that blocks a step.
type checkFailure struct {
	Step, Run, Out, FP, Result string
	Round                      int
}

// commandGate evaluates a step's command conditions for the
// interpreter and remembers the last failure, which is the one that
// blocks the walk, and the last error.
type commandGate struct {
	projectRoot, slug string
	def               LoopDef
	outs              map[string][]StepOutput
	failed            *checkFailure
	err               error
}

func (g *commandGate) check(step string, round int) bool {
	s := g.def.Steps[g.def.StepIndex(step)]
	newer := hasLaterRound(g.outs[step], round)
	for _, c := range s.Done {
		if c.Kind != "command" {
			continue
		}
		// A round with a newer output is judged by its cached result
		// alone. Re-running it on a new HEAD could pass and consume it,
		// leaving the newer round, the agent's answer, unread.
		if newer {
			if !cachedCommandPass(g.projectRoot, g.slug, step, round, c.Run) {
				return false
			}
			continue
		}
		passed, out, fp, err := evalCommand(g.projectRoot, g.slug, s, round, c)
		if err != nil {
			g.err = fmt.Errorf("step %s round %d: %w", step, round, err)
			return false
		}
		if !passed {
			g.failed = &checkFailure{Step: step, Round: round, Run: c.Run, Out: out, FP: fp,
				Result: checkResultPath(g.projectRoot, g.slug, step, round, fp)}
			return false
		}
	}
	return true
}

func hasLaterRound(outs []StepOutput, round int) bool {
	for _, o := range outs {
		if o.Round > round {
			return true
		}
	}
	return false
}

const checkWakeTail = 1 << 10

// reportCheckFailure wakes the step's pane with the check output once
// per fingerprint and tells the coordinator once per fingerprint. The
// wake text is one line: send-keys would submit at the first newline.
func reportCheckFailure(projectRoot, slug string, def LoopDef, f checkFailure) error {
	s := def.Steps[def.StepIndex(f.Step)]
	fp12 := f.FP[:12]
	out := strings.TrimSpace(f.Out)
	if len(out) > checkWakeTail {
		out = out[len(out)-checkWakeTail:]
	}
	oneLine := strings.Join(strings.Fields(strings.ReplaceAll(out, "\n", " | ")), " ")
	next := f.Round + 1
	msg := fmt.Sprintf("Check failed for %s round %d: %s Output: %s (full result: %s). The check reruns only when your step output or the task branch changes: after fixing, commit your work and write your next output, or rewrite this round's file. Write round %d now: %s.",
		f.Step, f.Round, f.Run, oneLine, f.Result, next, task.StepOutputPath(projectRoot, slug, s.Output, next))
	if _, err := wakeRole(StepSpec(projectRoot, slug, def, s), fmt.Sprintf("woken-check-%s-%s", f.Step, fp12), msg); err != nil {
		return err
	}
	marker := filepath.Join(task.RoleTaskDir(projectRoot, slug), "state", fmt.Sprintf("check-notified-%s-r%d-%s", f.Step, f.Round, fp12))
	created, err := createMarkerExcl(marker)
	if err != nil {
		return err
	}
	if created {
		notifyRoleLoop(projectRoot, fmt.Sprintf("%s: check failed at %s round %d: %s\n%s", slug, f.Step, f.Round, f.Run, f.Out))
	}
	return nil
}
