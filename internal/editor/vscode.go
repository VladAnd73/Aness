package editor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// VSCode drives a Remote-SSH window through the remote `code` CLI and
// the window's CLI IPC socket. Window choice: newest live socket
// first. The CLI cannot cheaply tell which window a socket belongs to
// (`code --status` lists every client window, not the socket's own),
// so with one window per project the newest live one is the pick.
type VSCode struct {
	Home       string
	RuntimeDir string
	Timeout    time.Duration
}

func NewVSCode() *VSCode {
	home, _ := os.UserHomeDir()
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		rt = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return &VSCode{Home: home, RuntimeDir: rt, Timeout: 5 * time.Second}
}

func (v *VSCode) Name() string { return "vscode" }

func (v *VSCode) Open(loc Location) error {
	cli, err := FindCLI(v.Home)
	if err != nil {
		return err
	}
	socks, err := LiveSockets(v.RuntimeDir)
	if err != nil {
		return err
	}
	var errs []string
	for _, sock := range socks {
		err := v.run(cli, sock, loc)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(sock), err))
	}
	return fmt.Errorf("%w: every live window refused (%s)", ErrNoEditor, strings.Join(errs, "; "))
}

func (v *VSCode) run(cli, sock string, loc Location) error {
	timeout := v.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, "--reuse-window", "--goto", loc.String())
	cmd.Env = append(os.Environ(), "VSCODE_IPC_HOOK_CLI="+sock)
	// The remote-cli `code` is a sh wrapper that runs node without
	// exec. Killing only sh leaves node holding the output pipe, so
	// kill the whole group and cap the pipe drain.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%v: %s", err, msg)
		}
		return err
	}
	return nil
}

// FindCLI returns the remote `code` CLI of the newest VS Code server
// install whose CLI exists. Partial installs leave server dirs without
// one.
func FindCLI(home string) (string, error) {
	dirs, _ := filepath.Glob(filepath.Join(home, ".vscode-server", "cli", "servers", "Stable-*"))
	type cand struct {
		path string
		mod  time.Time
	}
	var cands []cand
	for _, d := range dirs {
		cli := filepath.Join(d, "server", "bin", "remote-cli", "code")
		fi, err := os.Stat(cli)
		if err != nil || fi.IsDir() {
			continue
		}
		di, err := os.Stat(d)
		if err != nil {
			continue
		}
		cands = append(cands, cand{cli, di.ModTime()})
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("%w: no VS Code server CLI under %s", ErrNoEditor, filepath.Join(home, ".vscode-server"))
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })
	return cands[0].path, nil
}

// LiveSockets returns the VS Code CLI IPC sockets that accept a
// connection, newest mtime first. Every window reload leaves the old
// socket file behind (connect gets ECONNREFUSED), so mtime alone
// picks dead ones.
func LiveSockets(runtimeDir string) ([]string, error) {
	paths, _ := filepath.Glob(filepath.Join(runtimeDir, "vscode-ipc-*.sock"))
	type cand struct {
		path string
		mod  time.Time
	}
	var live []cand
	stale := 0
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || fi.Mode()&os.ModeSocket == 0 {
			continue
		}
		if !Alive(p) {
			stale++
			continue
		}
		live = append(live, cand{p, fi.ModTime()})
	}
	if len(live) == 0 {
		return nil, fmt.Errorf("%w: no live VS Code IPC socket in %s (%d stale)", ErrNoEditor, runtimeDir, stale)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].mod.After(live[j].mod) })
	out := make([]string, len(live))
	for i, c := range live {
		out[i] = c.path
	}
	return out, nil
}

func Alive(sock string) bool {
	c, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
