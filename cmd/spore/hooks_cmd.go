package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/versality/spore/internal/fleet"
	"github.com/versality/spore/internal/hooks"
	"github.com/versality/spore/internal/task"
)

func runHooks(args []string) int {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, hooksUsage)
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		fmt.Print(hooksUsage)
		return 0
	case "install":
		return runHooksInstall(rest)
	case "commit-msg":
		return runHooksCommitMsg(rest)
	case "pretooluse":
		return runHooksPreToolUse()
	case "stop":
		return runHooksStop()
	case "settings":
		return runHooksSettings()
	case "watch-inbox":
		return runHooksWatchInbox(rest)
	case "notify-coordinator":
		return runHooksNotifyCoordinator(rest)
	default:
		fmt.Fprintf(os.Stderr, "spore hooks: unknown subcommand %q\n\n%s", sub, hooksUsage)
		return 2
	}
}

func runHooksInstall(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "spore hooks install: takes no args")
		return 2
	}
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks install:", err)
		return 1
	}
	dir, err := hooks.Install(root, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks install:", err)
		return 1
	}
	fmt.Println(dir)
	return 0
}

func runHooksCommitMsg(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "spore hooks commit-msg: usage: commit-msg <file>")
		return 2
	}
	if err := hooks.CommitMsg(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks commit-msg:", err)
		return 1
	}
	return 0
}

func runHooksPreToolUse() int {
	req, err := readHookRequest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks pretooluse:", err)
		return 1
	}
	resp := hooks.PreToolUse(req, hooks.DefaultForbidden())
	return writeHookResponse(resp)
}

func runHooksStop() int {
	req, err := readHookRequest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks stop:", err)
		return 1
	}
	resp := hooks.Stop(req)
	return writeHookResponse(resp)
}

func readHookRequest() (hooks.Request, error) {
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return hooks.Request{}, fmt.Errorf("read stdin: %w", err)
	}
	var req hooks.Request
	if err := json.Unmarshal(body, &req); err != nil {
		return hooks.Request{}, fmt.Errorf("unmarshal request: %w", err)
	}
	return req, nil
}

func writeHookResponse(resp hooks.Response) int {
	enc, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks: marshal response:", err)
		return 1
	}
	if _, err := os.Stdout.Write(enc); err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks: write response:", err)
		return 1
	}
	os.Stdout.Write([]byte{'\n'})
	return 0
}

// settingsInput is the JSON schema read from stdin by `spore hooks settings`.
type settingsInput struct {
	Events map[string][]settingsInputBin `json:"events"`
}

type settingsInputBin struct {
	Command     string `json:"command"`
	Matcher     string `json:"matcher,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
	Async       bool   `json:"async,omitempty"`
	AsyncRewake bool   `json:"asyncRewake,omitempty"`
}

func runHooksSettings() int {
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks settings:", err)
		return 1
	}
	var input settingsInput
	if err := json.Unmarshal(body, &input); err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks settings: bad input:", err)
		return 1
	}
	events := make(map[string][]hooks.HookBin, len(input.Events))
	for name, bins := range input.Events {
		for _, b := range bins {
			events[name] = append(events[name], hooks.HookBin{
				BinPath:     b.Command,
				Matcher:     b.Matcher,
				Timeout:     b.Timeout,
				Async:       b.Async,
				AsyncRewake: b.AsyncRewake,
			})
		}
	}
	out, err := hooks.Settings(events)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks settings:", err)
		return 1
	}
	os.Stdout.Write(out)
	return 0
}

func runHooksWatchInbox(args []string) int {
	observe, wakeSession, rest := parseObserveFlags(args)
	if observe {
		return runHooksObserveInbox(wakeSession)
	}
	args = rest

	// Exit before claiming, acking or superseding anything: a session
	// that merely inherited the agent's SPORE_TASK_INBOX must not touch
	// the agent's tells.
	if inbox := os.Getenv("SPORE_TASK_INBOX"); inbox != "" && hooks.ForeignSession(inbox) {
		return 0
	}

	// A newer watcher of the same session sends SIGTERM. Exit 0 so
	// claude-code does not treat the handover as a wake or an error,
	// unless a batch is already claimed: then finish and exit 2.
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	go func() {
		<-term
		if hooks.MayExitOnSupersede() {
			hooks.ReleaseWatcher()
			os.Exit(0)
		}
	}()

	var err error
	switch len(args) {
	case 0:
		dirs := watchInboxDirsFromEnv()
		if len(dirs) == 0 {
			// Host-level Stop hooks fire in every claude session,
			// including non-spore ones. No slug and no inbox env
			// means there is nothing to watch; exit silently so
			// the hook produces no stderr noise.
			return 0
		}
		err = hooks.WatchInboxForSession(hooks.WatcherKey(stopHookSessionID(os.Stdin)), dirs)
	case 1:
		err = hooks.WatchInboxSlugForSession(hooks.WatcherKey(stopHookSessionID(os.Stdin)), args[0])
	default:
		fmt.Fprintln(os.Stderr, "usage: spore hooks watch-inbox [slug]")
		return 2
	}
	if err == hooks.ErrWake {
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks watch-inbox:", err)
		return 1
	}
	return 0
}

// parseObserveFlags pulls --observe and --wake-session out of the
// watch-inbox argument list, returning the remaining positionals.
func parseObserveFlags(args []string) (observe bool, wakeSession string, rest []string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--observe":
			observe = true
		case "--wake-session":
			if i+1 < len(args) {
				i++
				wakeSession = args[i]
			}
		default:
			rest = append(rest, args[i])
		}
	}
	return observe, wakeSession, rest
}

// runHooksObserveInbox arms the inbox from outside the agent. It claims
// nothing; on a new arrival it nudges the agent's pane so the Stop-hook
// watcher that does the real draining gets a turn to run in. Spawned by
// the fleet reconciler's lost-wakeup watchdog, never by a hook.
func runHooksObserveInbox(wakeSession string) int {
	dirs := watchInboxDirsFromEnv()
	if len(dirs) == 0 {
		return 0
	}
	err := hooks.ObserveInbox(dirs, func() error {
		if wakeSession == "" {
			return nil
		}
		return fleet.NudgeSession(wakeSession)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore hooks watch-inbox --observe:", err)
		return 1
	}
	return 0
}

// watchInboxDirsFromEnv resolves the inbox dirs the Stop-hook watcher
// should watch, from the env the harness sets per spawn. Returns nil when
// SPORE_TASK_INBOX is unset (non-spore host session -> no-op). For a
// coordinator ($SPORE_TASK_SLUG == "coordinator"), $SPORE_TASK_INBOX is
// the poke channel; the coordinator's message inbox (where worker/CLI
// `spore task tell coordinator` lands) is a different tree, so append it
// too. Any other slug is a worker whose $SPORE_TASK_INBOX already IS its
// message inbox, so it stays single-dir.
func watchInboxDirsFromEnv() []string {
	inbox := os.Getenv("SPORE_TASK_INBOX")
	if inbox == "" {
		return nil
	}
	dirs := []string{inbox}
	if os.Getenv("SPORE_TASK_SLUG") == "coordinator" {
		msg, err := task.InboxDirForProject(os.Getenv("SPORE_PROJECT_ROOT"), "coordinator")
		if err == nil && msg != inbox {
			dirs = append(dirs, msg)
		}
	}
	return dirs
}

func runHooksNotifyCoordinator(args []string) int {
	switch len(args) {
	case 0:
		if err := hooks.NotifyCoordinatorEnv(); err != nil {
			fmt.Fprintln(os.Stderr, "spore hooks notify-coordinator:", err)
			return 1
		}
		return 0
	case 1:
		if err := hooks.NotifyCoordinator(args[0]); err != nil {
			fmt.Fprintln(os.Stderr, "spore hooks notify-coordinator:", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: spore hooks notify-coordinator [project]")
		return 2
	}
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "-c", "safe.directory="+wd, "rev-parse", "--show-toplevel")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse: %w (%s)", err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// stopHookSessionID reads the Stop hook input from stdin and returns its
// session_id. A terminal or a stdin that never closes yields "".
func stopHookSessionID(f *os.File) string {
	if fi, err := f.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	got := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(io.LimitReader(f, 1<<20))
		got <- b
	}()
	select {
	case b := <-got:
		var req hooks.Request
		if json.Unmarshal(b, &req) != nil {
			return ""
		}
		return req.SessionID
	case <-time.After(2 * time.Second):
		return ""
	}
}
