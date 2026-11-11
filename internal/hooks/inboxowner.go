package hooks

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ownerFile sits in watcherDir, below the inbox, so writing it never
// wakes a watcher on the inbox itself.
const ownerFile = "owner"

// RecordInboxOwner notes the tmux pane spore spawned the inbox's agent
// in. ForeignSession reads it back.
func RecordInboxOwner(inboxDir, paneID string) error {
	dir := filepath.Join(inboxDir, watcherDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ownerFile), []byte(paneID+"\n"), 0o644)
}

// ForeignSession reports whether the Stop hook calling it belongs to a
// session other than the agent spore spawned for inboxDir. Any process
// started inside an agent's pane inherits SPORE_TASK_INBOX, so a
// `claude -p` the agent runs from its Bash tool would otherwise claim
// the agent's tells and ack them into read/.
//
// The hook's session is foreign when it runs in another pane, when it
// is not under the owner pane's process (it outlived the agent that
// started it), or when its Claude Code process was itself started by a
// Claude Code process above it in the pane: Claude Code sets CLAUDE_PID
// for every tool it runs. Without a live owner record nothing is
// foreign, so sessions spawned by older releases keep working.
func ForeignSession(inboxDir string) bool {
	b, err := os.ReadFile(filepath.Join(inboxDir, watcherDir, ownerFile))
	if err != nil {
		return false
	}
	pane := strings.TrimSpace(string(b))
	panePid, ok := livePanePid(pane)
	if !ok {
		return false
	}
	if os.Getenv("TMUX_PANE") != pane {
		return true
	}
	if _, ok := parentPid(panePid); !ok {
		// Not visible from here (another pid namespace): no evidence.
		return false
	}
	session := sessionProcess(os.Getppid())
	chain, reached, complete := ancestorsUpTo(session, panePid)
	if !reached {
		return complete
	}
	if z, ok := environValue(session, "CLAUDE_PID"); ok {
		if pid, err := strconv.Atoi(z); err == nil {
			for _, a := range chain[1:] {
				if a == pid {
					return true
				}
			}
		}
	}
	return false
}

func livePanePid(pane string) (int, bool) {
	if pane == "" {
		return 0, false
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#{pane_pid}").Output()
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 1 {
		return 0, false
	}
	return pid, true
}

var shellComms = map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "fish": true}

// sessionProcess walks up from pid past any shell a hook runner put in
// between, to the process that ran the hook.
func sessionProcess(pid int) int {
	for pid > 1 {
		comm, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
		if err != nil || !shellComms[strings.TrimSpace(string(comm))] {
			return pid
		}
		ppid, ok := parentPid(pid)
		if !ok {
			return pid
		}
		pid = ppid
	}
	return pid
}

// ancestorsUpTo returns pid and its ancestors up to and including top.
// reached says the walk met top; complete says it otherwise ran to the
// root rather than stopping on an unreadable /proc entry.
func ancestorsUpTo(pid, top int) (chain []int, reached, complete bool) {
	for {
		chain = append(chain, pid)
		if pid == top {
			return chain, true, true
		}
		if pid <= 1 {
			return chain, false, true
		}
		ppid, ok := parentPid(pid)
		if !ok {
			return chain, false, false
		}
		pid = ppid
	}
}

func parentPid(pid int) (int, bool) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil {
		return 0, false
	}
	return ppid, true
}

func environValue(pid int, key string) (string, bool) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		return "", false
	}
	for _, kv := range bytes.Split(b, []byte{0}) {
		if v, ok := bytes.CutPrefix(kv, []byte(key+"=")); ok {
			return string(v), true
		}
	}
	return "", false
}
