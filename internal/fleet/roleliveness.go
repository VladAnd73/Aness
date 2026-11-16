package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/versality/spore/internal/sporetoml"
	"github.com/versality/spore/internal/task"
)

// Seams for the liveness pass. Tests swap them to drive pane states
// without tmux or a live agent.
var (
	spawnRole       = SpawnRole
	probeRolePane   = probeRolePaneTmux
	killRoleSession = func(session string) { _ = exec.Command("tmux", "kill-session", "-t", session).Run() }
	nowFunc         = time.Now
)

// MaxRoleRespawns caps respawns per role per round. Past the cap the
// driver stops respawning, leaves any dead pane for inspection, and
// tells the coordinator once.
const MaxRoleRespawns = 3

// DefaultRoleStallMinutes is the stall window when spore.toml sets no
// `[fleet] role_stall_minutes`.
const DefaultRoleStallMinutes = 20

// spawnGrace is how long a fresh session may show a shell before it
// counts as dead: the launch line runs under sh until it execs the
// agent.
const spawnGrace = 30 * time.Second

// paneProbe is what the liveness pass needs to know about a role
// session. Content is the visible pane text, used as the activity
// signal for stall detection.
type paneProbe struct {
	Exists  bool
	Dead    bool
	Command string
	Created time.Time
	Content string
}

func probeRolePaneTmux(session string) (paneProbe, error) {
	if !hasSession(session) {
		return paneProbe{}, nil
	}
	out, err := exec.Command("tmux", "display-message", "-p", "-t", session,
		"#{pane_dead}\t#{pane_current_command}\t#{session_created}").Output()
	if err != nil {
		return paneProbe{}, fmt.Errorf("tmux display-message %s: %w", session, err)
	}
	fields := strings.SplitN(strings.TrimRight(string(out), "\n"), "\t", 3)
	if len(fields) != 3 {
		return paneProbe{}, fmt.Errorf("tmux display-message %s: unexpected output %q", session, out)
	}
	p := paneProbe{Exists: true, Dead: fields[0] == "1", Command: fields[1]}
	if secs, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
		p.Created = time.Unix(secs, 0)
	}
	p.Content, _ = capturePane(session)
	return p, nil
}

// agentGone reports why the agent in a live session is no longer
// running, or "" when it looks alive. A shell in the foreground means
// the agent exited back to it (a hand-started role, or a launch that
// never reached exec); the grace window keeps the pre-exec sh of a
// fresh spawn from counting.
func agentGone(p paneProbe, now time.Time) string {
	if p.Dead {
		return "pane dead"
	}
	if isShellCommand(p.Command) && now.Sub(p.Created) >= spawnGrace {
		return "agent exited to " + p.Command
	}
	return ""
}

// preStartRunning reports whether spec's pane is still in its
// pre-start hook (or queued on the slug lock). A marker older than the
// session is a leftover from a pane killed mid-hook and does not
// count; session_created has 1s resolution, hence the slack.
func preStartRunning(spec RoleSpawnSpec, created time.Time) bool {
	fi, err := os.Stat(task.PreStartMarkerPath(spec.ProjectRoot, spec.Slug, roleKey(spec)))
	if err != nil {
		return false
	}
	return !fi.ModTime().Before(created.Add(-time.Second))
}

func isShellCommand(cmd string) bool {
	switch strings.TrimPrefix(filepath.Base(cmd), "-") {
	case "sh", "bash", "zsh", "fish", "dash", "ksh", "mksh", "tcsh", "csh", "nu":
		return true
	}
	return false
}

// roleKey names a role pane in state markers: "engineer", "reviewer-A".
func roleKey(spec RoleSpawnSpec) string {
	if spec.Role == "reviewer" {
		return "reviewer-" + string(spec.ReviewerInstance)
	}
	return spec.Role
}

// ensureRolePane keeps the agent for spec running. A healthy pane is
// left alone. A missing session is spawned silently and never counts
// toward the cap: a token-monitor wrap kills its own session on
// purpose and must always recover. A session whose agent is gone is
// killed and respawned; each such respawn claims a marker
// `respawn-<key>-r<round>-<n>` under state/ and notifies the
// coordinator, and once MaxRoleRespawns are used the pass stops and
// escalates once instead. round is the role's completed-round count,
// so a new round re-arms the cap.
func ensureRolePane(spec RoleSpawnSpec, round int) error {
	session, err := spec.SessionName()
	if err != nil {
		return err
	}
	p, err := probeRolePane(session)
	if err != nil {
		// Unknown pane state: neither kill nor spawn on a guess.
		fmt.Fprintf(os.Stderr, "role-loop probe %s: %v\n", session, err)
		return nil
	}
	if !p.Exists {
		_, _, err := spawnRole(spec)
		return err
	}
	reason := agentGone(p, nowFunc())
	if reason == "" || (!p.Dead && preStartRunning(spec, p.Created)) {
		return nil
	}

	stateDir := filepath.Join(task.RoleTaskDir(spec.ProjectRoot, spec.Slug), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	key := roleKey(spec)
	stem := fmt.Sprintf("respawn-%s-r%d-", key, round)
	respawns, err := countMarkers(stateDir, stem)
	if err != nil {
		return err
	}
	if respawns >= MaxRoleRespawns {
		created, err := createMarkerExcl(filepath.Join(stateDir, fmt.Sprintf("respawn-capped-%s-r%d", key, round)))
		if err != nil {
			return err
		}
		if created {
			notifyRoleLoop(spec.ProjectRoot, fmt.Sprintf(
				"%s: escalated: %s agent died again after %d respawns in round %d (%s); respawn stopped, pane left for inspection",
				spec.Slug, key, respawns, round+1, reason))
		}
		return nil
	}
	killRoleSession(session)
	if _, spawned, err := spawnRole(spec); err != nil || !spawned {
		return err
	}
	n := respawns + 1
	created, err := createMarkerExcl(filepath.Join(stateDir, stem+strconv.Itoa(n)))
	if err != nil {
		return err
	}
	if created {
		notifyRoleLoop(spec.ProjectRoot, fmt.Sprintf("%s: respawned %s (%s; round %d, respawn %d/%d)",
			spec.Slug, key, reason, round+1, n, MaxRoleRespawns))
	}
	return nil
}

func countMarkers(dir, prefix string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			n++
		}
	}
	return n, nil
}

// checkRoleStall tells the coordinator once when the active role has
// gone quiet: its expected output file is absent and neither the role
// files nor the pane's transcript changed for the stall window. The
// activity fingerprint and the time it last changed live in
// state/activity-<key>-r<round>; a changed fingerprint resets the
// clock, which re-arms the notice (the stall marker is keyed on it).
// Never kills: a stalled agent may be mid-thought or waiting on a
// dialog only the operator can answer.
func checkRoleStall(spec RoleSpawnSpec, round int, expected string, window time.Duration) error {
	if window <= 0 {
		return nil
	}
	if _, err := os.Stat(expected); err == nil {
		return nil
	}
	session, err := spec.SessionName()
	if err != nil {
		return err
	}
	p, err := probeRolePane(session)
	if err != nil || !p.Exists || agentGone(p, nowFunc()) != "" {
		return nil
	}
	taskDir := task.RoleTaskDir(spec.ProjectRoot, spec.Slug)
	fp, err := activityFingerprint(taskDir, p.Content)
	if err != nil {
		return err
	}
	key := roleKey(spec)
	stateDir := filepath.Join(taskDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	actPath := filepath.Join(stateDir, fmt.Sprintf("activity-%s-r%d", key, round))
	now := nowFunc()
	prevFP, since, ok := readActivity(actPath)
	if !ok || prevFP != fp {
		return os.WriteFile(actPath, []byte(fmt.Sprintf("%s %d\n", fp, now.Unix())), 0o644)
	}
	quiet := now.Sub(since)
	if quiet < window {
		return nil
	}
	created, err := createMarkerExcl(filepath.Join(stateDir,
		fmt.Sprintf("stalled-%s-r%d-%d", key, round, since.Unix())))
	if err != nil {
		return err
	}
	if created {
		notifyRoleLoop(spec.ProjectRoot, fmt.Sprintf(
			"%s: stalled: %s has not written %s and shows no activity for %d min (pane %s)",
			spec.Slug, key, filepath.Base(expected), int(quiet.Minutes()), session))
	}
	return nil
}

func readActivity(path string) (string, time.Time, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, false
	}
	fields := strings.Fields(string(b))
	if len(fields) != 2 {
		return "", time.Time{}, false
	}
	secs, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	return fields[0], time.Unix(secs, 0), true
}

// activityFingerprint hashes the role files (name, size, mtime; state/
// excluded so the liveness markers do not count as activity) plus the
// pane transcript above the input prompt. The status line and input
// box are cut: a status line clock would otherwise read as activity
// forever, while a working agent's spinner line sits above the prompt.
func activityFingerprint(taskDir, pane string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(taskDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if d.Name() == "state" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %d %d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	h.Write([]byte(transcriptPart(pane)))
	return hex.EncodeToString(h.Sum(nil)[:12]), nil
}

// transcriptPart returns the pane text above the last input prompt
// line, minus the rule line framing the input box. Without a prompt
// line the whole capture counts.
func transcriptPart(pane string) string {
	lines := strings.Split(pane, "\n")
	prompt := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), string(inputPromptRune)) {
			prompt = i
		}
	}
	if prompt == -1 {
		return pane
	}
	end := prompt
	if end > 0 && isRuleLine(lines[end-1]) {
		end--
	}
	return strings.Join(lines[:end], "\n")
}

// LoadRoleStallMinutes reads `[fleet] role_stall_minutes = N` from
// spore.toml. 0 disables stall notices. A missing key, file, or bad
// value falls back to DefaultRoleStallMinutes; a bad value also
// returns an error for the caller to surface.
func LoadRoleStallMinutes(projectRoot string) (int, error) {
	entries, err := sporetoml.ReadSection(projectRoot, "fleet")
	if err != nil {
		return DefaultRoleStallMinutes, err
	}
	for _, e := range entries {
		if e.Key != "role_stall_minutes" {
			continue
		}
		n, err := strconv.Atoi(e.Value)
		if err != nil || n < 0 {
			return DefaultRoleStallMinutes, fmt.Errorf("line %d: role_stall_minutes: want integer >= 0, got %q", e.Line, e.Value)
		}
		return n, nil
	}
	return DefaultRoleStallMinutes, nil
}

func roleStallWindow(projectRoot string) time.Duration {
	n, err := LoadRoleStallMinutes(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "role-loop spore.toml: %v\n", err)
	}
	return time.Duration(n) * time.Minute
}
