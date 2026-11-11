package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startFakeAgent starts task <slug> through the CLI with a fake agent that
// plays Claude Code: script is the body of the pane's process, $HOOK runs
// the real Stop hook (watch-inbox), and $OUT collects what the hook saw.
// The agent waits a second first, as a real one is busy with its first
// turn before any Stop hook fires.
func startFakeAgent(t *testing.T, script string) (bin, root, out, inbox string, env []string) {
	t.Helper()
	requireToolchain(t)
	bin = sharedSporeBin(t)
	root = gitInitProject(t)
	project := filepath.Base(root)
	killProjectSessions(t, project)
	out = t.TempDir()

	hook := filepath.Join(out, "hook.sh")
	writeFile(t, hook, `#!/bin/sh
printf '%s' '{"hook_event_name":"Stop","session_id":"'$1'"}' | WATCH_TIMEOUT=30 `+bin+` hooks watch-inbox >`+out+`/$1.out 2>&1
echo $? >`+out+`/$1.code
`, 0o755)
	nested := filepath.Join(out, "nested-claude.sh")
	writeFile(t, nested, "#!/bin/sh\n"+hook+" nested\n", 0o755)
	agent := filepath.Join(out, "agent.sh")
	body := strings.NewReplacer("$HOOK", hook, "$NESTED", nested).Replace(script)
	writeFile(t, agent, "#!/bin/sh\nsleep 1\n"+body+"\nexec sleep 300\n", 0o755)

	env = fleetEnv(t, agent)
	writeFile(t, filepath.Join(root, "tasks", "w.md"), taskFile("w", "draft"), 0o644)
	if res := runCLI(t, bin, root, env, "", "task", "start", "w"); res.code != 0 {
		t.Fatalf("task start: %s%s", res.stdout, res.stderr)
	}
	inbox = filepath.Join(envValue(env, "XDG_STATE_HOME"), "spore", project, "w", "inbox")
	return bin, root, out, inbox, env
}

func waitFile(path string, within time.Duration) string {
	deadline := time.Now().Add(within)
	for {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b))
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func unreadTells(t *testing.T, inbox string) int {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(inbox, "*.json"))
	return len(m)
}

// The normal case must keep working: the agent the pane was spawned for
// gets the tell through its own Stop hook.
func TestWatchInboxTheSpawnedAgentStillGetsItsTell(t *testing.T) {
	bin, root, out, _, env := startFakeAgent(t, "$HOOK owner &")
	time.Sleep(2 * time.Second)
	if res := runCLI(t, bin, root, env, "", "task", "tell", "w", "hello owner"); res.code != 0 {
		t.Fatalf("tell: %s", res.stderr)
	}
	if code := waitFile(filepath.Join(out, "owner.code"), 10*time.Second); code != "2" {
		b, _ := os.ReadFile(filepath.Join(out, "owner.out"))
		t.Fatalf("owner watcher exit %q, want 2 (wake); output:\n%s", code, b)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "owner.out")); !strings.Contains(string(b), "hello owner") {
		t.Fatalf("owner watcher did not print the tell:\n%s", b)
	}
}

// Bug 2: a `claude -p` the agent runs from its Bash tool inherits
// SPORE_TASK_INBOX. Its own Stop hook used to claim the agent's tells and
// ack them into read/, so the agent never saw them. Claude Code puts
// CLAUDE_PID in its tools' environment, so the nested claude carries its
// parent agent's pid; the orphan case is the same claude after the agent
// that started it went away.
func TestWatchInboxANestedClaudeDoesNotTakeTheAgentsTells(t *testing.T) {
	for _, tc := range []struct{ name, script string }{
		{"nested under the agent", "CLAUDE_PID=$$ $NESTED &"},
		{"orphaned", "(CLAUDE_PID=$$ $NESTED &)"},
		{"in another window of the agent's session", "tmux new-window -d $NESTED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, root, out, inbox, env := startFakeAgent(t, tc.script)
			waitFile(filepath.Join(out, "nested.code"), 4*time.Second)
			if res := runCLI(t, bin, root, env, "", "task", "tell", "w", "for the agent only"); res.code != 0 {
				t.Fatalf("tell: %s", res.stderr)
			}
			time.Sleep(2 * time.Second)
			code := waitFile(filepath.Join(out, "nested.code"), 0)
			if code != "0" {
				b, _ := os.ReadFile(filepath.Join(out, "nested.out"))
				t.Fatalf("nested session's watcher exit %q, want 0 without claiming; output:\n%s", code, b)
			}
			if n := unreadTells(t, inbox); n != 1 {
				t.Fatalf("%d unread tell(s) left in the agent's inbox, want 1", n)
			}
		})
	}
}
