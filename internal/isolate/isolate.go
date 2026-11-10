// Package isolate runs a command inside bubblewrap with a private HOME,
// the host's read-only system paths, and nothing else of the host's.
// The network is shared: the agent needs the model API, and every
// outside service it could reach needs credentials it does not have.
package isolate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/versality/spore/internal/budget"
)

type Bind struct {
	Src, Dst string
	RW       bool
}

// Spec is one sandbox. Work is bound read-write at its own path and is
// the cwd; Home is the sandbox HOME, bound read-write. Credentials names
// the host credentials file: a copy of it, refreshed and stripped, is
// mounted read-only at <Home>/.claude/.credentials.json. Env is the
// whole environment of the sandboxed process.
type Spec struct {
	Work        string
	Home        string
	Credentials string
	Extra       []Bind
	Env         []string
}

var BlockedHosts = []string{"api.notion.com", "mcp.notion.com", "api.linear.app", "mcp.linear.app", "slack.com", "api.slack.com", "mcp.slack.com"}

// MinCredentialLife is how long the copied access token must stay valid.
// The copy has no refresh token, so a run outliving it is logged out.
var MinCredentialLife = 30 * time.Minute

// RefreshCredentials is swapped by tests; the default may write the
// host credentials file, exactly as claude's own refresh does.
var RefreshCredentials = func(ctx context.Context, path string) error {
	return budget.EnsureFresh(ctx, path, MinCredentialLife)
}

const (
	hostsFD = 3
	credsFD = 4
)

func credentialsDst(home string) string {
	return filepath.Join(home, ".claude", ".credentials.json")
}

// hostsDst is where /etc/hosts really lives: on NixOS it is a symlink
// into the store, and bwrap refuses a symlink as a mount destination.
func hostsDst() string {
	if p, err := filepath.EvalSymlinks("/etc/hosts"); err == nil {
		return p
	}
	return "/etc/hosts"
}

// Args is the bwrap argv up to and including "--". The hosts override
// is read from fd 3 and the credentials copy from fd 4; Command
// supplies both.
func Args(s Spec) []string {
	a := []string{
		"--die-with-parent", "--new-session",
		"--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--clearenv",
		"--ro-bind", "/nix", "/nix",
		"--ro-bind", "/etc", "/etc",
		"--ro-bind-try", "/run/current-system", "/run/current-system",
		"--ro-bind-try", "/bin", "/bin",
		"--ro-bind-try", "/usr/bin", "/usr/bin",
		"--ro-bind-data", fmt.Sprint(hostsFD), hostsDst(),
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--bind", s.Home, s.Home,
		"--bind", s.Work, s.Work,
	}
	for _, b := range s.Extra {
		flag := "--ro-bind"
		if b.RW {
			flag = "--bind"
		}
		a = append(a, flag, b.Src, b.Dst)
	}
	if s.Credentials != "" {
		a = append(a, "--ro-bind-data", fmt.Sprint(credsFD), credentialsDst(s.Home))
	}
	for _, kv := range s.Env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			a = append(a, "--setenv", k, v)
		}
	}
	return append(a, "--chdir", s.Work, "--")
}

// Command returns an unstarted bwrap command running argv in s. Cancel
// kills bwrap's process group; --die-with-parent and the private PID
// namespace take everything inside down with it.
func Command(ctx context.Context, s Spec, argv []string) (*exec.Cmd, error) {
	bin, err := lookBwrap()
	if err != nil {
		return nil, err
	}
	hosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		hosts = []byte("127.0.0.1 localhost\n::1 localhost\n")
	}
	hf, err := unlinkedFile(HostsFile(hosts))
	if err != nil {
		return nil, err
	}
	files := []*os.File{hf}
	if s.Credentials != "" {
		cf, err := credentialsFile(ctx, s)
		if err != nil {
			hf.Close()
			return nil, err
		}
		files = append(files, cf)
	}
	cmd := exec.CommandContext(ctx, bin, append(Args(s), argv...)...)
	cmd.Env = s.Env
	cmd.ExtraFiles = files
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return cmd, nil
}

func credentialsFile(ctx context.Context, s Spec) (*os.File, error) {
	if err := RefreshCredentials(ctx, s.Credentials); err != nil {
		return nil, fmt.Errorf("isolate: refresh login: %w", err)
	}
	raw, err := os.ReadFile(s.Credentials)
	if err != nil {
		return nil, fmt.Errorf("isolate: credentials: %w", err)
	}
	cp, err := CredentialsCopy(raw)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(credentialsDst(s.Home)), 0o700); err != nil {
		return nil, err
	}
	return unlinkedFile(cp)
}

// unlinkedFile holds b in a temp file whose name is already gone, so
// nothing on disk outside the sandbox keeps a copy.
func unlinkedFile(b []byte) (*os.File, error) {
	f, err := os.CreateTemp("", "isolate-")
	if err != nil {
		return nil, err
	}
	os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func lookBwrap() (string, error) {
	bin, err := exec.LookPath("bwrap")
	if err != nil {
		return "", fmt.Errorf("isolate: bwrap is not on PATH: install bubblewrap (nix: nixpkgs#bubblewrap) or run with isolation none: %w", err)
	}
	return bin, nil
}

// Available reports whether bwrap is installed and can build a
// namespace here; the nix build sandbox, for one, cannot.
func Available() error {
	bin, err := lookBwrap()
	if err != nil {
		return err
	}
	if out, err := exec.Command(bin, "--ro-bind", "/", "/", "true").CombinedOutput(); err != nil {
		return fmt.Errorf("isolate: bwrap cannot run here: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BaseEnv is the sandbox environment: no host variable passes except
// the PATH entries the sandbox can see and LANG.
func BaseEnv(home string) []string {
	var path []string
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		for _, root := range []string{"/nix/", "/run/current-system/", "/bin", "/usr/bin"} {
			if p == strings.TrimSuffix(root, "/") || strings.HasPrefix(p, root) {
				path = append(path, p)
				break
			}
		}
	}
	if len(path) == 0 {
		path = []string{"/run/current-system/sw/bin", "/bin", "/usr/bin"}
	}
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "C.UTF-8"
	}
	return []string{
		"PATH=" + strings.Join(path, ":"),
		"LANG=" + lang,
		"TERM=dumb",
		"HOME=" + home,
		"TMPDIR=/tmp",
		"IS_SANDBOX=1",
		"ENABLE_CLAUDEAI_MCP_SERVERS=false",
	}
}

// CredentialsPath follows claude's own lookup, as internal/budget does.
func CredentialsPath() string {
	if p := os.Getenv("AGENT_BUDGET_CREDS"); p != "" {
		return p
	}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, ".credentials.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

// CredentialsCopy keeps only the claude.ai access token. Without the
// refresh token the sandbox cannot rotate it, which would log the host
// out; connector tokens (mcpOAuth) never enter at all.
func CredentialsCopy(raw []byte) ([]byte, error) {
	var file struct {
		OAuth map[string]any `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("isolate: credentials: %w", err)
	}
	if at, _ := file.OAuth["accessToken"].(string); at == "" {
		return nil, errors.New("isolate: credentials: no claude.ai access token; log in on the host first")
	}
	delete(file.OAuth, "refreshToken")
	return json.Marshal(file)
}
