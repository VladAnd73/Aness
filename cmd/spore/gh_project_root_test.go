package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// newGitRepo makes <parent>/<name> a git repo with one commit, so
// worktrees can be added to it.
func newGitRepo(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "init", "-q")
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	return dir
}

// ghRootFixture gives two repos, alpha (A) and bravo (B), plus a temp
// XDG_STATE_HOME so no real github.toml is ever touched.
func ghRootFixture(t *testing.T) (state, a, b string) {
	t.Helper()
	state = t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	parent := t.TempDir()
	return state, newGitRepo(t, parent, "alpha"), newGitRepo(t, parent, "bravo")
}

func ghTomlFor(state, project string) string {
	return filepath.Join(state, "spore", project, "github.toml")
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s exists (stat err %v), want absent", path, err)
	}
}

func assertFileHas(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read %s: %v", path, err)
		return
	}
	if !strings.Contains(string(b), want) {
		t.Errorf("%s = %q, want it to contain %q", path, b, want)
	}
}

// Scenario 1: env names A, cwd is inside B: refuse, name both roots,
// write nothing.
func TestGHConfigRefusesWhenCwdRepoDiffersFromEnv(t *testing.T) {
	state, a, b := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(b)

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--set", "operator_login=x"})
	})
	if code == 0 {
		t.Errorf("exit = 0, want non-zero")
	}
	for _, root := range []string{a, b, "--project"} {
		if !strings.Contains(stderr, root) {
			t.Errorf("stderr %q does not mention %q", stderr, root)
		}
	}
	assertNoFile(t, ghTomlFor(state, "alpha"))
	assertNoFile(t, ghTomlFor(state, "bravo"))
}

// Scenario 2: --project B wins over env and cwd.
func TestGHConfigProjectFlagOverridesEnv(t *testing.T) {
	state, a, b := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(b)

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--project", b, "--set", "operator_login=x"})
	})
	if code != 0 {
		t.Errorf("exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "bravo"), `operator_login = "x"`)
	assertNoFile(t, ghTomlFor(state, "alpha"))

	code, _, stderr = captureFn(t, func() int {
		return runGH([]string{"config", "--set=operator_login=y", "--project=" + b})
	})
	if code != 0 {
		t.Errorf("--project= after --set: exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "bravo"), `operator_login = "y"`)
	assertNoFile(t, ghTomlFor(state, "alpha"))
}

// Scenario 3: a worktree of A is the same project, not a mismatch.
func TestGHConfigAcceptsWorktreeOfEnvProject(t *testing.T) {
	state, a, _ := ghRootFixture(t)
	wt := filepath.Join(t.TempDir(), "alpha-wt")
	gitT(t, a, "worktree", "add", "-q", wt)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(wt)

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--set", "operator_login=x"})
	})
	if code != 0 {
		t.Errorf("exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "alpha"), `operator_login = "x"`)
}

// A symlinked SPORE_PROJECT_ROOT must not read as a mismatch against
// the real path git reports from inside a worktree.
func TestGHConfigAcceptsSymlinkedEnvRoot(t *testing.T) {
	state, a, _ := ghRootFixture(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Dir(a), link); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "alpha-wt")
	gitT(t, a, "worktree", "add", "-q", wt)
	t.Setenv("SPORE_PROJECT_ROOT", filepath.Join(link, "alpha"))
	t.Chdir(wt)

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--set", "operator_login=x"})
	})
	if code != 0 {
		t.Errorf("exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "alpha"), `operator_login = "x"`)
}

// Scenario 4: a cwd outside any git repo is not a mismatch.
func TestGHConfigAcceptsCwdOutsideGit(t *testing.T) {
	state, a, _ := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(t.TempDir())

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--set", "operator_login=x"})
	})
	if code != 0 {
		t.Errorf("exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "alpha"), `operator_login = "x"`)
}

// Scenario 5: with no env, the cwd repo is the project, as before.
func TestGHConfigUsesCwdWhenEnvUnset(t *testing.T) {
	state, _, b := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", "")
	t.Chdir(b)

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--set", "operator_login=x"})
	})
	if code != 0 {
		t.Errorf("exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "bravo"), `operator_login = "x"`)
	assertNoFile(t, ghTomlFor(state, "alpha"))
}

// token and exec take --project before the child command; after "--"
// it belongs to the child and is passed through untouched.
func TestGHTokenAndExecTakeProjectFlag(t *testing.T) {
	root := ghTestProject(t)
	gitT(t, root, "init", "-q")
	b := newGitRepo(t, t.TempDir(), "bravo")
	t.Setenv("SPORE_PROJECT_ROOT", filepath.Join(t.TempDir(), "elsewhere"))
	t.Chdir(b)

	code, stdout, stderr := captureFn(t, func() int {
		return runGH([]string{"token", "--project", root})
	})
	if code != 0 || strings.TrimSpace(stdout) != "ghs_cli" {
		t.Errorf("token --project: exit %d stdout %q stderr %q, want 0 ghs_cli", code, stdout, stderr)
	}

	code, stdout, stderr = captureFn(t, func() int {
		return runGH([]string{"exec", "--project", root, "--", "sh", "-c", `printf '%s|' "$GH_TOKEN" "$@"`, "x", "--project", "child"})
	})
	if code != 0 || stdout != "ghs_cli|--project|child|" {
		t.Errorf("exec --project: exit %d stdout %q stderr %q, want 0 %q", code, stdout, stderr, "ghs_cli|--project|child|")
	}
}

// Scenario 6: the built binary, against temp repos and a temp
// XDG_STATE_HOME, refuses the mismatch and honours --project.
func TestGHBinaryProjectRootMismatchEndToEnd(t *testing.T) {
	bin := buildSporeBin(t)
	parent := t.TempDir()
	a := newGitRepo(t, parent, "alpha")
	b := newGitRepo(t, parent, "bravo")
	state := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "SPORE_") || strings.HasPrefix(kv, "XDG_STATE_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "XDG_STATE_HOME="+state, "SPORE_PROJECT_ROOT="+a)

	run := func(args ...string) (int, string) {
		cmd := exec.Command(bin, append([]string{"gh"}, args...)...)
		cmd.Dir = b
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return code, string(out)
	}

	code, out := run("config", "--set", "operator_login=x")
	if code == 0 {
		t.Errorf("mismatch: exit 0, want non-zero; output %q", out)
	}
	if !strings.Contains(out, a) || !strings.Contains(out, b) {
		t.Errorf("mismatch output %q does not name both %s and %s", out, a, b)
	}
	assertNoFile(t, ghTomlFor(state, "alpha"))
	assertNoFile(t, ghTomlFor(state, "bravo"))

	code, out = run("config", "--project", b, "--set", "operator_login=x")
	if code != 0 {
		t.Errorf("--project: exit %d, want 0; output %q", code, out)
	}
	assertFileHas(t, ghTomlFor(state, "bravo"), `operator_login = "x"`)
	assertNoFile(t, ghTomlFor(state, "alpha"))

	code, out = run("config", "--project", ".", "--set", "operator_login=y")
	if code != 0 {
		t.Errorf("--project .: exit %d, want 0; output %q", code, out)
	}
	assertFileHas(t, ghTomlFor(state, "bravo"), `operator_login = "y"`)
	assertNoFile(t, filepath.Join(state, "spore", "github.toml"))

	missing := filepath.Join(parent, "brovo")
	code, out = run("config", "--project", missing, "--set", "operator_login=x")
	if code == 0 || !strings.Contains(out, missing) {
		t.Errorf("--project missing: exit %d output %q, want non-zero naming %s", code, out, missing)
	}
	assertNoFile(t, ghTomlFor(state, "brovo"))
}

// The bot PR recipe: env names project A, the cwd is a throwaway clone
// (a fresh repo with an https origin). exec must proceed with A's
// token; only config and token refuse a cwd/env mismatch.
func TestGHExecIgnoresMismatchForThrowawayClone(t *testing.T) {
	root := ghTestProject(t)
	gitT(t, root, "init", "-q")
	clone := t.TempDir()
	gitT(t, clone, "init", "-q")
	gitT(t, clone, "remote", "add", "origin", "https://github.com/example/repo.git")
	t.Chdir(clone)

	code, stdout, stderr := captureFn(t, func() int {
		return runGH([]string{"exec", "--", "sh", "-c", `printf '%s' "$GH_TOKEN"`})
	})
	if code != 0 || stdout != "ghs_cli" {
		t.Errorf("exec from clone: exit %d stdout %q stderr %q, want 0 ghs_cli", code, stdout, stderr)
	}
}

// assertNoGithubToml fails if any github.toml exists under state.
func assertNoGithubToml(t *testing.T, state string) {
	t.Helper()
	_ = filepath.WalkDir(state, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == "github.toml" {
			t.Errorf("github.toml written at %s, want none", p)
		}
		return nil
	})
}

// --project . is the cwd's repo, never the shared state root.
func TestGHConfigProjectDotIsCwdRepo(t *testing.T) {
	state, a, b := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(b)

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--project", ".", "--set", "operator_login=x"})
	})
	if code != 0 {
		t.Errorf("exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "bravo"), `operator_login = "x"`)
	assertNoFile(t, filepath.Join(state, "spore", "github.toml"))
	assertNoFile(t, ghTomlFor(state, "alpha"))
}

// A relative --project resolves against the cwd.
func TestGHConfigRelativeProjectFlag(t *testing.T) {
	state, a, _ := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(a)

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--project", "../bravo", "--set", "operator_login=x"})
	})
	if code != 0 {
		t.Errorf("exit = %d, want 0; stderr %q", code, stderr)
	}
	assertFileHas(t, ghTomlFor(state, "bravo"), `operator_login = "x"`)
	assertNoFile(t, ghTomlFor(state, "alpha"))
}

func TestGHConfigRefusesMissingProject(t *testing.T) {
	state, a, _ := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(a)
	missing := filepath.Join(filepath.Dir(a), "brovo")

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--project", missing, "--set", "operator_login=x"})
	})
	if code == 0 {
		t.Errorf("exit = 0, want non-zero")
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr %q does not name %s", stderr, missing)
	}
	assertNoGithubToml(t, state)
}

func TestGHConfigRefusesNonGitProject(t *testing.T) {
	state, a, _ := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(a)
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := captureFn(t, func() int {
		return runGH([]string{"config", "--project", plain, "--set", "operator_login=x"})
	})
	if code == 0 {
		t.Errorf("exit = 0, want non-zero")
	}
	if !strings.Contains(stderr, plain) {
		t.Errorf("stderr %q does not name %s", stderr, plain)
	}
	assertNoGithubToml(t, state)
}

// exec --project . reads the token of the cwd's project, by absolute root.
func TestGHExecProjectDotResolvesAbsolute(t *testing.T) {
	root := ghTestProject(t)
	gitT(t, root, "init", "-q")
	t.Setenv("SPORE_PROJECT_ROOT", newGitRepo(t, t.TempDir(), "alpha"))
	t.Chdir(root)

	code, stdout, stderr := captureFn(t, func() int {
		return runGH([]string{"exec", "--project", ".", "--", "sh", "-c", `printf '%s' "$GH_TOKEN"`})
	})
	if code != 0 || stdout != "ghs_cli" {
		t.Errorf("exec --project .: exit %d stdout %q stderr %q, want 0 ghs_cli", code, stdout, stderr)
	}
}

// An empty --project must not fall back to the env project.
func TestGHConfigRefusesEmptyProject(t *testing.T) {
	state, a, _ := ghRootFixture(t)
	t.Setenv("SPORE_PROJECT_ROOT", a)
	t.Chdir(a)

	for _, args := range [][]string{
		{"config", "--project=", "--set", "operator_login=x"},
		{"config", "--project", "", "--set", "operator_login=x"},
	} {
		code, _, stderr := captureFn(t, func() int { return runGH(args) })
		if code == 0 || !strings.Contains(stderr, "--project") {
			t.Errorf("%v: exit %d stderr %q, want non-zero naming --project", args, code, stderr)
		}
	}
	assertNoGithubToml(t, state)
}
