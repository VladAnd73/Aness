package fleet

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/versality/spore/internal/task"
)

// maxAgentToolCallForTest is the longest a single agent tool call may
// run on this harness: the Bash tool's ceiling. Measured on real
// transcripts, the longest observed assistant-to-tool-result gap was
// 120.2s (the default Bash timeout) and 0.97% of calls exceeded 30s.
// Nothing is written to the transcript for the whole of such a call,
// so any grace below this ceiling will eventually declare a working
// agent dead.
const maxAgentToolCallForTest = 600 * time.Second

// Finding 5. A coordinator sitting inside a long tool call writes
// nothing to its transcript for the duration, so "no transcript
// activity since the claim" is not evidence of a lost wakeup until the
// longest possible tool call has gone by.
func TestInboxWatchdog_IgnoresAgentInsideALongToolCall(t *testing.T) {
	if inboxWatchdogGrace <= maxAgentToolCallForTest {
		t.Errorf("grace %v is not above the %v tool-call ceiling: a mid-turn agent can be declared dead",
			inboxWatchdogGrace, maxAgentToolCallForTest)
	}

	h := newWatchdogHarness(t)
	h.claim(t, "100-1-1.json", "new thread")
	// The agent entered a tool call just before the claim landed and is
	// still inside it one tool-call ceiling later.
	h.activity = h.claimedAt.Add(-time.Second)
	h.after = maxAgentToolCallForTest

	res, err := runInboxWatchdog(h.cfg(), 3, h.deps)
	if err != nil {
		t.Fatalf("runInboxWatchdog: %v", err)
	}
	if res.Requeued != 0 || res.Rearmed || res.Escalated || res.Strikes != 0 {
		t.Errorf("acted on an agent inside a long tool call: %+v", res)
	}
	if h.spawned != 0 || len(h.nudges) != 0 {
		t.Errorf("spawned=%d nudges=%v, want 0/none", h.spawned, h.nudges)
	}
}

// Finding 8. The claiming watcher's pid is on disk. While that process
// is still running the delivery has not failed yet: it has not even
// finished printing.
func TestInboxWatchdog_SkipsWhileTheClaimingWatcherIsAlive(t *testing.T) {
	h := newWatchdogHarness(t)
	h.claim(t, "100-1-1.json", "new thread")
	h.claimer = true

	res, err := runInboxWatchdog(h.cfg(), 3, h.deps)
	if err != nil {
		t.Fatalf("runInboxWatchdog: %v", err)
	}
	if res.Requeued != 0 || res.Rearmed || res.Escalated {
		t.Errorf("acted while the claiming watcher is still running: %+v", res)
	}
}

// Finding 6. Standing down for want of a liveness signal is the safe
// choice, but a watchdog that is silently inert forever is worse than
// no watchdog: the operator believes they are covered. Say it once.
func TestInboxWatchdog_SaysSoOnceWhenItCannotTellWhetherTheAgentIsAlive(t *testing.T) {
	h := newWatchdogHarness(t)
	h.claim(t, "100-1-1.json", "new thread")
	h.activeOK = false

	for tick := 1; tick <= 3; tick++ {
		if _, err := runInboxWatchdog(h.cfg(), 3, h.deps); err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
	}
	said := 0
	for _, l := range h.logs {
		if strings.Contains(l, "liveness") {
			said++
		}
	}
	if said != 1 {
		t.Fatalf("logged the inert watchdog %d time(s) across 3 ticks, want exactly 1: %v", said, h.logs)
	}
}

// Finding 6. The transcript lookup hardcoded ~/.claude. A project
// running under its own CLAUDE_CONFIG_DIR has its transcripts
// elsewhere, and a watchdog that cannot find them stands down forever.
func TestAgentTranscriptActivity_HonoursClaudeConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, "elsewhere", "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	root := filepath.Join(home, "proj")
	writeTranscript(t, filepath.Join(cfgDir, "projects", claudeProjectDir(root)))

	if _, ok := agentTranscriptActivity(root); !ok {
		t.Fatal("no liveness signal found under CLAUDE_CONFIG_DIR")
	}
}

// Finding 6. Claude Code folds every character that is not a letter,
// digit or hyphen into a hyphen. A project root holding a dot -- every
// worktree under .worktrees/ -- encoded to a directory that does not
// exist, so the probe answered "cannot tell" and the watchdog was a
// permanent no-op for exactly the sessions it was written for.
func TestAgentTranscriptActivityFindsADottedProjectRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))

	root := filepath.Join(home, "spore", ".worktrees", "inbox-wake-ack")
	// Fold every non [A-Za-z0-9-] byte, not just / and .: a CI runner's
	// TMPDIR holds an underscore (_temp) that Claude Code also folds.
	encoded := regexp.MustCompile(`[^A-Za-z0-9-]`).ReplaceAllString(root, "-")
	writeTranscript(t, filepath.Join(home, ".claude", "projects", encoded))

	if _, ok := agentTranscriptActivity(root); !ok {
		t.Fatalf("no liveness signal for a worktree root: looked for something other than %q", encoded)
	}
}

// The encoding itself, pinned against directory names measured on this
// host next to the cwd recorded inside their own transcripts.
func TestClaudeProjectDir(t *testing.T) {
	cases := map[string]string{
		"/home/spore/spore":                        "-home-spore-spore",
		"/home/spore/assistant/.worktrees/arcloop": "-home-spore-assistant--worktrees-arcloop",
		"/home/spore/marketer-frontend_nb":         "-home-spore-marketer-frontend-nb",
	}
	for root, want := range cases {
		if got := claudeProjectDir(root); got != want {
			t.Errorf("claudeProjectDir(%q) = %q, want %q", root, got, want)
		}
	}
}

func writeTranscript(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Finding 7. The draft probe took the lowest line starting with a
// prompt glyph, wherever it was. A selection dialog puts a cursor glyph
// in the middle of the pane; binding to it reads the dialog's selected
// row as the operator's input line, and a row that renders empty after
// the glyph would be read as "no draft, safe to type" -- into a dialog
// where Enter picks an option.
func TestDraftFromPane_WillNotMistakeAMenuCursorForTheInputLine(t *testing.T) {
	pane := strings.Join([]string{
		"  Which option?",
		"",
		"❯ ",
		"  2. Hold as-is",
		"  3. Rework something first",
		"",
		"  Press enter to confirm",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	}, "\n")

	_, known := draftFromPane(pane)
	if known {
		t.Fatal("bound to a dialog cursor and reported a usable input line")
	}
}

// The real shape of an agent pane: the input line sits a few rows from
// the bottom and the glyph is padded with a non-breaking space when the
// operator has typed nothing.
func TestDraftFromPane_ReadsTheRealInputLine(t *testing.T) {
	blanks := strings.Repeat("\n", 40)

	empty := blanks + "❯ \n  ? for shortcuts\n"
	if hasDraft, known := draftFromPane(empty); !known || hasDraft {
		t.Errorf("empty input line: hasDraft=%v known=%v, want false/true", hasDraft, known)
	}

	typed := blanks + "❯ what happened to the deploy\n  ? for shortcuts\n"
	if hasDraft, known := draftFromPane(typed); !known || !hasDraft {
		t.Errorf("unsent draft: hasDraft=%v known=%v, want true/true", hasDraft, known)
	}
}

// Finding 11. spawnObserver started the process and only then resolved
// where to write its pidfile, so a pidfile it could not write left a
// live, untracked observer behind and the next tick spawned another.
func TestSpawnObserver_StartsNothingWhenThePidfileCannotBeWritten(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	state := filepath.Join(home, "state")
	t.Setenv("XDG_STATE_HOME", state)

	root := filepath.Join(home, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// The project's state directory is occupied by a file, so the
	// pidfile cannot be created.
	project, err := task.ProjectName(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(state, "spore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "spore", project), []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(home, "observer-ran")
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(bin, "spore"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	if err := spawnObserver(root, []string{filepath.Join(home, "inbox")}); err == nil {
		t.Fatal("spawnObserver reported success with an unwritable pidfile")
	}
	for range 20 {
		if _, err := os.Stat(marker); err == nil {
			t.Fatal("an untracked observer was started and left running")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// The escalate-after knob has to mean the same thing at its smallest
// setting as at its default. With N=1 the first qualifying tick both
// repairs the inbox and tells the operator; with N=3 it stays silent
// until the third. Either way the nudge happens exactly once.
func TestInboxWatchdog_EscalateAfterHoldsAtOneAndAtTheDefault(t *testing.T) {
	for _, n := range []int{1, DefaultInboxWatchdogEscalateAfter} {
		h := newWatchdogHarness(t)
		h.claim(t, "100-1-1.json", "new thread")

		for tick := 1; tick <= n+2; tick++ {
			res, err := runInboxWatchdog(h.cfg(), n, h.deps)
			if err != nil {
				t.Fatalf("n=%d tick %d: %v", n, tick, err)
			}
			if res.Escalated != (tick == n) {
				t.Errorf("n=%d tick %d: Escalated=%v, want %v",
					n, tick, res.Escalated, tick == n)
			}
		}
		if len(h.nudges) != 1 {
			t.Errorf("n=%d: nudges = %v, want exactly 1", n, h.nudges)
		}
		if _, err := os.Stat(filepath.Join(h.inbox, "100-1-1.json")); err != nil {
			t.Errorf("n=%d: envelope is not in the inbox the nudge points at: %v", n, err)
		}
	}
}
