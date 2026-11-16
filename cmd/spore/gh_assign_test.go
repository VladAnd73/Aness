package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const fakePRURL = "https://github.com/o/r/pull/7"

// fakeGHScript records each call's argv as one line in $FAKE_GH_LOG and
// acts like gh on github.com: `pr create` prints the new PR's URL on
// stdout, and so does `pr view` (as `--json url -q .url` would). FAKE_GH_EDIT_FAIL makes `pr edit` fail the way an assignee
// rejection would.
const fakeGHScript = `#!/bin/sh
echo "$*" >> "$FAKE_GH_LOG"
if [ "$1 $2" = "pr create" ] || [ "$1 $2" = "pr new" ]; then
	echo "$GH_TOKEN" > "$FAKE_GH_LOG.token"
	echo ` + fakePRURL + `
	exit 0
fi
if [ "$1 $2" = "pr view" ]; then
	echo ` + fakePRURL + `
fi
if [ "$1 $2" = "pr edit" ] && [ -n "$FAKE_GH_EDIT_FAIL" ]; then
	echo "could not assign user: 'alice' not found" >&2
	exit 1
fi
exit 0
`

// fakeGHOnPath puts the fake gh first on PATH and returns its call log.
func fakeGHOnPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(fakeGHScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	log := filepath.Join(t.TempDir(), "gh.log")
	t.Setenv("FAKE_GH_LOG", log)
	return log
}

func ghCalls(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("fake gh never ran: %v", err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func runGHExecFake(t *testing.T, args ...string) int {
	t.Helper()
	return runGH(append([]string{"exec", "--"}, args...))
}

func TestGHExecPRCreateAssignsOperator(t *testing.T) {
	ghTestProject(t)
	log := fakeGHOnPath(t)
	if code := runGH([]string{"config", "--set", "operator_login=alice"}); code != 0 {
		t.Fatalf("config exit %d", code)
	}
	if code := runGHExecFake(t, "gh", "pr", "create", "--title", "t", "--body", "b"); code != 0 {
		t.Fatalf("exec exit %d, want 0", code)
	}
	want := []string{"pr create --title t --body b", "pr edit " + fakePRURL + " --add-assignee alice"}
	if got := ghCalls(t, log); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("gh calls = %q, want %q", got, want)
	}
}

func TestGHExecPRCreateWithoutOperatorLoginUnchanged(t *testing.T) {
	ghTestProject(t)
	log := fakeGHOnPath(t)
	if code := runGHExecFake(t, "gh", "pr", "create", "--title", "t", "--body", "b"); code != 0 {
		t.Fatalf("exec exit %d, want 0", code)
	}
	want := []string{"pr create --title t --body b"}
	if got := ghCalls(t, log); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("gh calls = %q, want %q", got, want)
	}
}

func TestGHExecPRCreateExplicitAssigneeWins(t *testing.T) {
	for _, flag := range [][]string{
		{"--assignee", "bob"},
		{"--assignee=bob"},
		{"-a", "bob"},
		{"-abob"},
		{"-da", "bob"},
	} {
		t.Run(strings.Join(flag, " "), func(t *testing.T) {
			ghTestProject(t)
			log := fakeGHOnPath(t)
			if code := runGH([]string{"config", "--set", "operator_login=alice"}); code != 0 {
				t.Fatalf("config exit %d", code)
			}
			args := append([]string{"gh", "pr", "create", "--title", "t"}, flag...)
			if code := runGHExecFake(t, args...); code != 0 {
				t.Fatalf("exec exit %d, want 0", code)
			}
			want := []string{strings.Join(args[1:], " ")}
			if got := ghCalls(t, log); strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("gh calls = %q, want %q", got, want)
			}
		})
	}
}

// A flag value that looks like -a is not an assignee: "-ta" is --title
// "a", "-Ro/a" is --repo "o/a", and a long flag takes the next arg as
// its value. The operator is still added.
func TestGHExecPRCreateFlagValueIsNotAssignee(t *testing.T) {
	for _, flag := range [][]string{
		{"-ta"},
		{"-Ro/a"},
		{"--title", "-a"},
		{"--body", "-abob", "-t", "t"},
	} {
		t.Run(strings.Join(flag, " "), func(t *testing.T) {
			ghTestProject(t)
			log := fakeGHOnPath(t)
			if code := runGH([]string{"config", "--set", "operator_login=alice"}); code != 0 {
				t.Fatalf("config exit %d", code)
			}
			args := append([]string{"gh", "pr", "new", "-b", "b"}, flag...)
			if code := runGHExecFake(t, args...); code != 0 {
				t.Fatalf("exec exit %d, want 0", code)
			}
			want := []string{strings.Join(args[1:], " "), "pr edit " + fakePRURL + " --add-assignee alice"}
			if got := ghCalls(t, log); strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("gh calls = %q, want %q", got, want)
			}
		})
	}
}

func TestGHExecOtherCommandsUnchanged(t *testing.T) {
	for _, args := range [][]string{
		{"gh", "pr", "view", "1"},
		{"sh", "-c", `echo "$*" >> "$FAKE_GH_LOG"`, "git", "push", "origin", "b"},
	} {
		t.Run(args[1], func(t *testing.T) {
			ghTestProject(t)
			log := fakeGHOnPath(t)
			if code := runGH([]string{"config", "--set", "operator_login=alice"}); code != 0 {
				t.Fatalf("config exit %d", code)
			}
			if code := runGHExecFake(t, args...); code != 0 {
				t.Fatalf("exec exit %d, want 0", code)
			}
			got := ghCalls(t, log)
			want := strings.Join(args[1:], " ")
			if args[0] == "sh" {
				want = "push origin b"
			}
			if len(got) != 1 || got[0] != want {
				t.Fatalf("calls = %q, want only %q", got, want)
			}
		})
	}
}

// gh pr create on github.com opens the PR before it assigns, and an
// assignee it cannot set fails the run after the PR exists. Assigning
// in a follow-up call keeps that failure from turning a created PR into
// a failed create: the create's exit code stands and spore only warns.
func TestGHExecAssignFailureOnlyWarns(t *testing.T) {
	ghTestProject(t)
	log := fakeGHOnPath(t)
	t.Setenv("FAKE_GH_EDIT_FAIL", "1")
	if code := runGH([]string{"config", "--set", "operator_login=alice"}); code != 0 {
		t.Fatalf("config exit %d", code)
	}
	if code := runGHExecFake(t, "gh", "pr", "create", "--title", "t", "--body", "b"); code != 0 {
		t.Fatalf("exec exit %d, want 0 (PR was created; assign failure is a warning)", code)
	}
	got := ghCalls(t, log)
	if len(got) != 2 || !strings.HasPrefix(got[1], "pr edit "+fakePRURL) {
		t.Fatalf("gh calls = %q, want create then edit", got)
	}
}

// End to end: the built binary mints a token from a GitHub that checks
// the JWT, runs gh pr create with it, then assigns the operator.
func TestGHBinaryPRCreateAssignsEndToEnd(t *testing.T) {
	bin := buildSporeBin(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemPath := filepath.Join(t.TempDir(), "app.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(pemPath, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	const minted = "ghs_e2e_assign"
	var hits atomic.Int32
	srv := fakeGitHub(t, &key.PublicKey, "41", "/app/installations/42/access_tokens", minted, &hits)

	root := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/o/r.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	log := fakeGHOnPath(t)
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "SPORE_") || strings.HasPrefix(kv, "GH_") ||
			strings.HasPrefix(kv, "GIT_CONFIG_") || strings.HasPrefix(kv, "XDG_STATE_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "XDG_STATE_HOME="+t.TempDir())
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, append([]string{"gh"}, args...)...)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("spore gh %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("config", "--set", "app_id=41", "--set", "installation_id=42",
		"--set", "private_key="+pemPath, "--set", "api_url="+srv.URL, "--set", "operator_login=alice")

	out := run("exec", "--", "gh", "pr", "create", "--title", "t", "--body", "b")
	if !strings.Contains(out, fakePRURL) {
		t.Fatalf("exec output = %q, want the PR URL passed through", out)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("fake GitHub hit %d times, want 1", n)
	}
	tok, err := os.ReadFile(log + ".token")
	if err != nil || strings.TrimSpace(string(tok)) != minted {
		t.Fatalf("gh pr create saw GH_TOKEN %q (%v), want %q", tok, err, minted)
	}
	want := []string{"pr create --title t --body b", "pr edit " + fakePRURL + " --add-assignee alice"}
	if got := ghCalls(t, log); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("gh calls = %q, want %q", got, want)
	}
}
