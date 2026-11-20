package research

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// repo is a campaign's git repository. Every command ignores the host's
// global and system git config, so no host hook or signing rule runs.
type repo struct{ dir string }

const gitTimeout = 30 * time.Second

func (r repo) git(args ...string) (string, error) {
	out, err := r.gitRaw(args...)
	return strings.TrimSpace(string(out)), err
}

func (r repo) gitRaw(args ...string) ([]byte, error) {
	// A fresh context: cleanup after a cancelled night must still run.
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	full := append([]string{"-C", r.dir, "-c", "user.name=spore research", "-c", "user.email=research@localhost", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("research: git %s: %v: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (r repo) init() error {
	_, err := r.git("init", "--quiet", "--initial-branch=main")
	return err
}

func (r repo) Commit(msg string) (string, error) {
	if _, err := r.git("add", "--all"); err != nil {
		return "", err
	}
	if _, err := r.git("commit", "--quiet", "--allow-empty", "--no-verify", "-m", msg); err != nil {
		return "", err
	}
	return r.Head()
}

func (r repo) ResetHard(commit string) error {
	if _, err := r.git("reset", "--quiet", "--hard", commit); err != nil {
		return err
	}
	_, err := r.git("clean", "-fdxq")
	return err
}

func (r repo) Head() (string, error) { return r.git("rev-parse", "HEAD") }

func (r repo) Clean() (bool, error) {
	out, err := r.git("status", "--porcelain", "--untracked-files=all")
	return out == "", err
}

// changed lists paths that differ from HEAD, untracked ones included.
func (r repo) changed() ([]string, error) {
	out, err := r.gitRaw("status", "--porcelain", "-z", "--untracked-files=all", "--no-renames")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range strings.Split(string(out), "\x00") {
		if len(entry) > 3 {
			paths = append(paths, entry[3:])
		}
	}
	return paths, nil
}

func (r repo) show(commit, rel string) ([]byte, error) { return r.gitRaw("show", commit+":"+rel) }
