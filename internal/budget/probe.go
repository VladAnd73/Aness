package budget

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	probeSocket  = "spore-budget-probe"
	probeSession = "probe"
	probePrompt  = "Reply with the single word ok."
)

type commandRunner func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)

type probe struct {
	run         commandRunner
	dir         string
	captureFile string
	socket      string
	timeout     time.Duration
	killTimeout time.Duration
	poll        time.Duration
	environ     func() []string
}

type statusRateWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

type statusInput struct {
	RateLimits *struct {
		FiveHour *statusRateWindow `json:"five_hour"`
		SevenDay *statusRateWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

func execRunner(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return out, nil
}

func newProbe(dir string) probe {
	return probe{
		run:         execRunner,
		dir:         dir,
		captureFile: filepath.Join(dir, "statusline.json"),
		socket:      probeSocket,
		timeout:     90 * time.Second,
		killTimeout: 10 * time.Second,
		poll:        time.Second,
		environ:     os.Environ,
	}
}

func probeReading(ctx context.Context, now time.Time) (Reading, error) {
	d, err := stateDir()
	if err != nil {
		return Reading{}, fmt.Errorf("probe: %w", err)
	}
	r, err := newProbe(filepath.Join(d, "probe")).reading(ctx, now)
	if err != nil {
		return Reading{}, fmt.Errorf("probe: %w", err)
	}
	return r, nil
}

// probeEnv drops SPORE_* so spore's hooks do not treat the probe as a
// fleet agent, and CLAUDECODE so a run started from inside a Claude
// session is not refused as nested. It also drops every credential
// variable, so claude falls back to the same credentials file the
// endpoint reads and both sources measure one account.
func probeEnv(base []string) []string {
	drop := []string{"SPORE_", "CLAUDECODE=", "CLAUDE_CODE_OAUTH_TOKEN=", "ANTHROPIC_API_KEY=", "ANTHROPIC_AUTH_TOKEN="}
	out := make([]string, 0, len(base))
	for _, kv := range base {
		if slices.ContainsFunc(drop, func(p string) bool { return strings.HasPrefix(kv, p) }) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func claudeConfigDir(env []string) string {
	if d := envValue(env, "CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	if home := envValue(env, "HOME"); home != "" {
		return filepath.Join(home, ".claude")
	}
	return ""
}

func envValue(env []string, key string) string {
	v := ""
	for _, kv := range env {
		if k, val, ok := strings.Cut(kv, "="); ok && k == key {
			v = val
		}
	}
	return v
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// claudeProjectDir is where claude keeps transcripts for sessions run in
// dir: its config dir's projects/ plus dir with every non-alphanumeric
// byte turned into '-'. Empty when no config dir can be resolved, so a
// caller never deletes relative to the working directory.
func claudeProjectDir(env []string, dir string) string {
	base := claudeConfigDir(env)
	if base == "" || !filepath.IsAbs(base) || !filepath.IsAbs(dir) {
		return ""
	}
	return filepath.Join(base, "projects", nonAlnum.ReplaceAllString(dir, "-"))
}

func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// removeTranscript deletes the probe session's transcript, and nothing
// else: --no-session-persistence only works with --print, and every
// daily probe would otherwise leave one behind.
func (p probe) removeTranscript(env []string, sessionID string) {
	project := claudeProjectDir(env, p.dir)
	if project == "" || sessionID == "" {
		return
	}
	for _, path := range []string{filepath.Join(project, sessionID+".jsonl"), filepath.Join(project, sessionID)} {
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		fmt.Fprintf(dailyStderr, "spore budget daily: removing probe transcript %s\n", path)
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintf(dailyStderr, "spore budget daily: warning: %v\n", err)
		}
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (p probe) writeSettings() (string, error) {
	if err := os.MkdirAll(p.dir, stateDirMode); err != nil {
		return "", err
	}
	if err := os.Remove(p.captureFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	tmp := shellQuote(p.captureFile + ".tmp")
	// A default ACL on the state dir overrides the umask, so the temp
	// file is made private with chmod before any content goes in.
	settings := map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "umask 077; : > " + tmp + " && chmod 600 " + tmp + " && cat > " + tmp + " && mv " + tmp + " " + shellQuote(p.captureFile),
		},
	}
	b, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	path := filepath.Join(p.dir, "settings.json")
	return path, os.WriteFile(path, b, stateFileMode)
}

func (p probe) reading(ctx context.Context, now time.Time) (Reading, error) {
	settings, err := p.writeSettings()
	if err != nil {
		return Reading{}, err
	}
	sessionID, err := newSessionID()
	if err != nil {
		return Reading{}, err
	}
	env := probeEnv(p.environ())
	defer p.removeTranscript(env, sessionID)
	tmux := func(ctx context.Context, args ...string) ([]byte, error) {
		return p.run(ctx, env, "tmux", append([]string{"-L", p.socket, "-f", "/dev/null"}, args...)...)
	}
	// Bounded and detached from ctx: teardown must still run, and finish,
	// after a signal has cancelled the run.
	killServer := func() {
		c, cancel := context.WithTimeout(context.Background(), p.killTimeout)
		defer cancel()
		_, _ = tmux(c, "kill-server")
	}
	killServer()
	defer killServer()

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	// remain-on-exit keeps a dead claude's pane, so its exit status and
	// last output can be reported instead of waiting out the timeout.
	// User settings hold spore's Stop hook, which spawns a stray
	// coordinator outside a project root. disableAllHooks cannot be used
	// instead: it also turns off the status line.
	if _, err := tmux(ctx, "start-server", ";", "set-option", "-g", "remain-on-exit", "on", ";",
		"new-session", "-d", "-s", probeSession, "-c", p.dir, "-x", "200", "-y", "50",
		"claude", "--model", "haiku", "--session-id", sessionID,
		"--setting-sources", "project,local", "--settings", settings); err != nil {
		return Reading{}, fmt.Errorf("start claude: %w", err)
	}
	dead := func(ctx context.Context) error {
		out, err := tmux(ctx, "display-message", "-p", "-t", probeSession, "#{pane_dead} #{pane_dead_status}")
		if err != nil {
			return fmt.Errorf("probe session gone: %w", err)
		}
		isDead, status, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
		if isDead != "1" {
			return nil
		}
		return fmt.Errorf("claude exited with status %s: %s", status, p.screen(ctx, tmux))
	}
	if _, err := p.waitCapture(ctx, dead, func(statusInput) bool { return true }); err != nil {
		if ctx.Err() != nil {
			return Reading{}, fmt.Errorf("status line never ran in %s (folder not trusted?): %w; screen: %s", p.dir, err, p.screen(context.Background(), tmux))
		}
		return Reading{}, err
	}
	if _, err := tmux(ctx, "send-keys", "-t", probeSession, "-l", probePrompt); err != nil {
		return Reading{}, err
	}
	if _, err := tmux(ctx, "send-keys", "-t", probeSession, "Enter"); err != nil {
		return Reading{}, err
	}
	in, err := p.waitCapture(ctx, dead, func(in statusInput) bool {
		return in.RateLimits != nil && in.RateLimits.SevenDay != nil &&
			in.RateLimits.SevenDay.UsedPercentage != nil && in.RateLimits.SevenDay.ResetsAt != nil
	})
	if err != nil {
		return Reading{}, fmt.Errorf("no rate_limits in the status line: %w", err)
	}
	w := in.RateLimits.SevenDay
	r := Reading{
		Source:         "probe",
		WeeklyUsed:     *w.UsedPercentage,
		WeeklyResetsAt: time.Unix(int64(*w.ResetsAt), 0).UTC(),
	}
	if d := claudeConfigDir(env); d != "" {
		r.Credentials = filepath.Join(d, ".credentials.json")
	}
	if f := in.RateLimits.FiveHour; f != nil {
		if f.UsedPercentage != nil {
			r.FiveHourUsed = *f.UsedPercentage
		}
		if f.ResetsAt != nil {
			r.FiveHourResetsAt = time.Unix(int64(*f.ResetsAt), 0).UTC()
		}
	}
	return r, nil
}

// screen returns the last non-blank lines of the probe pane, history
// included: a claude that dies at once leaves its output above the
// visible area.
func (p probe) screen(ctx context.Context, tmux func(context.Context, ...string) ([]byte, error)) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := tmux(ctx, "capture-pane", "-p", "-S", "-", "-t", probeSession)
	if err != nil {
		return fmt.Sprintf("(capture failed: %v)", err)
	}
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return strings.Join(lines, " | ")
}

func (p probe) waitCapture(ctx context.Context, dead func(context.Context) error, ok func(statusInput) bool) (statusInput, error) {
	t := time.NewTicker(p.poll)
	defer t.Stop()
	for {
		if b, err := os.ReadFile(p.captureFile); err == nil {
			var in statusInput
			if json.Unmarshal(b, &in) == nil && ok(in) {
				return in, nil
			}
		}
		if err := dead(ctx); err != nil && ctx.Err() == nil {
			return statusInput{}, err
		}
		select {
		case <-ctx.Done():
			return statusInput{}, ctx.Err()
		case <-t.C:
		}
	}
}
