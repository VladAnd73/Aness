package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
// session is not refused as nested.
func probeEnv(base []string) []string {
	out := make([]string, 0, len(base))
	for _, kv := range base {
		if strings.HasPrefix(kv, "SPORE_") || strings.HasPrefix(kv, "CLAUDECODE=") {
			continue
		}
		out = append(out, kv)
	}
	return out
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
	tmp := p.captureFile + ".tmp"
	settings := map[string]any{
		"statusLine": map[string]any{
			"type":    "command",
			"command": "umask 077; cat > " + shellQuote(tmp) + " && mv " + shellQuote(tmp) + " " + shellQuote(p.captureFile),
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
	env := probeEnv(p.environ())
	tmux := func(ctx context.Context, args ...string) ([]byte, error) {
		return p.run(ctx, env, "tmux", append([]string{"-L", p.socket, "-f", "/dev/null"}, args...)...)
	}
	_, _ = tmux(ctx, "kill-server")
	defer func() { _, _ = tmux(context.Background(), "kill-server") }()

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	// remain-on-exit keeps a dead claude's pane, so its exit status and
	// last output can be reported instead of waiting out the timeout.
	// User settings hold spore's Stop hook, which spawns a stray
	// coordinator outside a project root. disableAllHooks cannot be used
	// instead: it also turns off the status line.
	if _, err := tmux(ctx, "start-server", ";", "set-option", "-g", "remain-on-exit", "on", ";",
		"new-session", "-d", "-s", probeSession, "-c", p.dir, "-x", "200", "-y", "50",
		"claude", "--model", "haiku", "--setting-sources", "project,local", "--settings", settings); err != nil {
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
