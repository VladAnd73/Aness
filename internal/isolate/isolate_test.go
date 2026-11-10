package isolate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// binds returns every (flag, src, dst) bind triple in a bwrap argv.
func binds(args []string) [][3]string {
	var out [][3]string
	for i := 0; i+2 < len(args); i++ {
		switch args[i] {
		case "--bind", "--ro-bind", "--ro-bind-try", "--bind-try", "--dev-bind":
			out = append(out, [3]string{args[i], args[i+1], args[i+2]})
			i += 2
		}
	}
	return out
}

func testSpec(t *testing.T) Spec {
	home := filepath.Join(t.TempDir(), "host-home")
	t.Setenv("HOME", home)
	root := t.TempDir()
	return Spec{
		Work:        filepath.Join(root, "work"),
		Home:        filepath.Join(root, "agent-home"),
		Credentials: filepath.Join(home, ".claude", ".credentials.json"),
		Extra:       []Bind{{Src: filepath.Join(root, "mcp.json"), Dst: filepath.Join(root, "mcp.json")}},
		Env:         []string{"PATH=/bin"},
	}
}

func TestArgsNeverBindHostHome(t *testing.T) {
	s := testSpec(t)
	home, _ := os.UserHomeDir()
	args := Args(s)
	for _, b := range binds(args) {
		src := b[1]
		if src == home || strings.HasPrefix(src, home+string(filepath.Separator)) {
			t.Errorf("argv binds %s %s -> %s from the host home", b[0], src, b[2])
		}
	}
	if slices.Contains(args, s.Credentials) {
		t.Errorf("argv names the host credentials file %s; want only a copy fed through a descriptor", s.Credentials)
	}
	dst := filepath.Join(s.Home, ".claude", ".credentials.json")
	i := slices.Index(args, dst)
	if i < 2 || args[i-2] != "--ro-bind-data" {
		t.Errorf("credentials copy is not mounted read-only at %s: %v", dst, args)
	}
}

func TestArgsBindsWorkHomeAndExtras(t *testing.T) {
	s := testSpec(t)
	got := binds(Args(s))
	for _, want := range [][3]string{
		{"--bind", s.Work, s.Work},
		{"--bind", s.Home, s.Home},
		{"--ro-bind", s.Extra[0].Src, s.Extra[0].Dst},
		{"--ro-bind", "/nix", "/nix"},
		{"--ro-bind", "/etc", "/etc"},
	} {
		if !slices.Contains(got, want) {
			t.Errorf("argv lacks %v; binds = %v", want, got)
		}
	}
	args := Args(s)
	for _, flag := range []string{"--unshare-pid", "--unshare-ipc", "--unshare-uts", "--die-with-parent", "--new-session", "--clearenv"} {
		if !slices.Contains(args, flag) {
			t.Errorf("argv lacks %s", flag)
		}
	}
	if i := slices.Index(args, "--chdir"); i < 0 || args[i+1] != s.Work {
		t.Errorf("argv does not chdir into the work dir: %v", args)
	}
	if len(args) == 0 || args[len(args)-1] != "--" {
		t.Errorf("argv does not end with --: %v", args)
	}
}

func TestArgsMountHostsOverResolvedEtcHosts(t *testing.T) {
	real, err := filepath.EvalSymlinks("/etc/hosts")
	if err != nil {
		t.Skipf("no /etc/hosts: %v", err)
	}
	args := Args(testSpec(t))
	i := slices.Index(args, real)
	if i < 2 || args[i-2] != "--ro-bind-data" {
		t.Errorf("hosts override not mounted at %s (bwrap refuses a symlink destination): %v", real, args)
	}
}

func TestBaseEnvCarriesNoSecrets(t *testing.T) {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", "ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "SPORE_PROJECT_ROOT", "NOTION_TOKEN"} {
		t.Setenv(k, "secret-"+k)
	}
	env := BaseEnv("/sandbox/home")
	for _, kv := range env {
		if strings.Contains(kv, "secret-") {
			t.Errorf("BaseEnv leaks %q", kv)
		}
	}
	for _, want := range []string{"HOME=/sandbox/home", "TMPDIR=/tmp", "IS_SANDBOX=1", "ENABLE_CLAUDEAI_MCP_SERVERS=false"} {
		if !slices.Contains(env, want) {
			t.Errorf("BaseEnv lacks %s: %v", want, env)
		}
	}
	allowed := map[string]bool{"PATH": true, "LANG": true, "TERM": true, "HOME": true, "TMPDIR": true, "IS_SANDBOX": true, "ENABLE_CLAUDEAI_MCP_SERVERS": true}
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); !allowed[k] {
			t.Errorf("BaseEnv carries unexpected %s", k)
		}
	}
}

func TestBaseEnvPathDropsHostHomeEntries(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("PATH", filepath.Join(home, "bin")+":/run/current-system/sw/bin:/nix/store/x-go/bin")
	if env := BaseEnv("/h"); !slices.Contains(env, "PATH=/run/current-system/sw/bin:/nix/store/x-go/bin") {
		t.Errorf("BaseEnv = %v, want PATH with only the entries the sandbox can see", env)
	}
}

func TestHostsFileBlocksEveryHostBothFamilies(t *testing.T) {
	got := string(HostsFile([]byte("127.0.0.1 localhost\n")))
	if !strings.HasPrefix(got, "127.0.0.1 localhost\n") {
		t.Errorf("host lines dropped: %q", got)
	}
	for _, h := range BlockedHosts {
		for _, ip := range []string{"0.0.0.0", "::"} {
			if !strings.Contains(got, ip+" "+h+"\n") {
				t.Errorf("hosts file lacks %s %s", ip, h)
			}
		}
	}
}

func TestCredentialsCopyDropsRefreshToken(t *testing.T) {
	in := []byte(`{"claudeAiOauth":{"accessToken":"at","refreshToken":"rt","expiresAt":5,"scopes":["a"]},"mcpOAuth":{"notion":{"accessToken":"mcp-secret"}}}`)
	out, err := CredentialsCopy(in)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["claudeAiOauth"]["accessToken"] != "at" || got["claudeAiOauth"]["expiresAt"] != float64(5) {
		t.Errorf("access token or expiry lost: %s", out)
	}
	if strings.Contains(string(out), "rt") || strings.Contains(string(out), "mcp-secret") {
		t.Errorf("copy keeps the refresh token or connector tokens: %s", out)
	}
}

func TestCredentialsCopyRefusesNoAccessToken(t *testing.T) {
	if _, err := CredentialsCopy([]byte(`{"mcpOAuth":{}}`)); err == nil {
		t.Error("CredentialsCopy accepted a file with no claude.ai login")
	}
}

func TestCredentialsPathLookup(t *testing.T) {
	t.Setenv("AGENT_BUDGET_CREDS", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "/c")
	if got := CredentialsPath(); got != "/c/.credentials.json" {
		t.Errorf("with CLAUDE_CONFIG_DIR: %q", got)
	}
	t.Setenv("AGENT_BUDGET_CREDS", "/x.json")
	if got := CredentialsPath(); got != "/x.json" {
		t.Errorf("override: %q", got)
	}
}
