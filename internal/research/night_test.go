package research

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTryEditorBreaksTargetIsCrash(t *testing.T) {
	breaks := map[string]func(f fixture) func(string) error{
		"deleted": func(fixture) func(string) error { return os.Remove },
		"emptied": func(fixture) func(string) error {
			return func(p string) error { return os.WriteFile(p, nil, 0o600) }
		},
		"renamed": func(fixture) func(string) error {
			return func(p string) error { return os.Rename(p, p+".old") }
		},
		"made a directory": func(fixture) func(string) error {
			return func(p string) error { os.Remove(p); return os.Mkdir(p, 0o700) }
		},
		"frozen edited in scratch": func(fixture) func(string) error {
			return func(p string) error {
				return os.WriteFile(filepath.Join(filepath.Dir(p), "frozen", "reference.md"), []byte("changed\n"), 0o600)
			}
		},
		"frozen edited in the repo": func(f fixture) func(string) error {
			return func(p string) error {
				return os.WriteFile(filepath.Join(f.repoDir(), "reference.md"), []byte("changed\n"), 0o600)
			}
		},
		"file added to the repo": func(f fixture) func(string) error {
			return func(p string) error { return os.WriteFile(filepath.Join(f.repoDir(), "extra.md"), nil, 0o600) }
		},
		"editor failed": func(fixture) func(string) error {
			return func(string) error { return errors.New("editor process died") }
		},
	}
	for name, mk := range breaks {
		t.Run(name, func(t *testing.T) {
			f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 2 })
			ed := &fakeEditor{edits: []func(string) error{mk(f), appendBytes("fine\n")}}
			j := &fakeJudge{scores: []float64{50}}
			if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
				t.Fatalf("RunNight: %v", err)
			}
			rows := f.resultRows(t)
			if len(rows) != 3 || rows[1]["status"] != "crash" || rows[1]["score"] != "-" {
				t.Errorf("rows = %v, want baseline, crash (score -), then a second try", rows)
			}
			if ed.calls != 2 {
				t.Errorf("editor calls = %d, want 2: the night goes on after a crash", ed.calls)
			}
			if j.calls != 3 {
				t.Errorf("judge calls = %d, want 3: a crashed try is never judged", j.calls)
			}
			f.wantBaselineHead(t, f.load(t))
			if clean, err := f.store.repo(f.name).Clean(); !clean || err != nil {
				t.Errorf("repo not clean after the night: %v", err)
			}
			if b, _ := os.ReadFile(filepath.Join(f.repoDir(), "reference.md")); string(b) != "frozen\n" {
				t.Errorf("frozen sibling = %q", b)
			}
		})
	}
}

// slowJudgeScript scores the first two calls (the baseline) 50 and
// blocks the third in a sleep, touching entered first.
const slowJudgeScript = `out=""
prev=""
for a in "$@"; do
  [ "$prev" = "--out" ] && out="$a"
  prev="$a"
done
dir=$(dirname "$0")
n=$(cat "$dir/count" 2>/dev/null || echo 0)
n=$((n+1))
echo $n > "$dir/count"
if [ $n -le 2 ]; then
  echo '{"score": 50, "runs": 3, "passed": 1}' > "$out"
  exit 0
fi
sleep 60 &
echo $! > "$dir/sleeper.pid"
touch "$dir/entered"
wait
`

func TestNightCancelledMidTryLeavesCleanRepo(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 5 })
	j := scriptJudge(t, slowJudgeScript)
	jdir := filepath.Dir(j.Argv[0])
	ed := &fakeEditor{edits: []func(string) error{appendBytes("half done\n")}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			if _, err := os.Stat(filepath.Join(jdir, "entered")); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	d := f.deps(ed, j)
	start := time.Now()
	sum, err := RunNight(ctx, f.store, f.name, d)
	if err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if time.Since(start) > 20*time.Second {
		t.Errorf("RunNight took %v: the judge was not killed", time.Since(start))
	}
	if _, err := os.Stat(filepath.Join(jdir, "entered")); err != nil {
		t.Fatalf("the judge never reached the try: the test did not interrupt mid-judge")
	}
	if sum.StopReason != "interrupted" {
		t.Errorf("StopReason = %q, want interrupted", sum.StopReason)
	}
	if clean, err := f.store.repo(f.name).Clean(); !clean || err != nil {
		t.Errorf("repo not clean: %v", err)
	}
	f.wantBaselineHead(t, f.load(t))
	unlock, _, err := Lock(d.LockPath, "next")
	if err != nil {
		t.Errorf("lock still held: %v", err)
	} else {
		unlock()
	}
	text := f.readArtifact(t, "results.tsv")
	if !strings.HasSuffix(text, "\n") {
		t.Errorf("results.tsv ends with a half line: %q", text)
	}
	if rows := f.resultRows(t); len(rows) != 1 || rows[0]["status"] != "baseline" {
		t.Errorf("rows = %v, want only the baseline (no row for the interrupted try)", rows)
	}
	pid, _ := os.ReadFile(filepath.Join(jdir, "sleeper.pid"))
	if stat, err := os.ReadFile("/proc/" + strings.TrimSpace(string(pid)) + "/stat"); err == nil && !strings.Contains(string(stat), ") Z ") {
		t.Errorf("the judge's child %s outlived the night", strings.TrimSpace(string(pid)))
	}
	d2 := f.deps(&fakeEditor{}, &fakeJudge{scores: []float64{50}})
	if _, err := RunNight(context.Background(), f.store, f.name, d2); err != nil {
		t.Errorf("next night refused: %v", err)
	}
}

func TestNightQuotaReadFailureStops(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 5 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n")}}
	d := f.deps(ed, &fakeJudge{scores: []float64{50}})
	reads := 0
	d.WeeklyUsed = func(context.Context) (float64, error) {
		reads++
		if reads == 1 {
			return 20, nil
		}
		return 0, errors.New("token expired")
	}
	sum, err := RunNight(context.Background(), f.store, f.name, d)
	if err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if sum.StopReason != "quota-read-failed" || ed.calls != 1 {
		t.Errorf("stop %q after %d editor calls, want quota-read-failed after 1", sum.StopReason, ed.calls)
	}
}

type clockEditor struct {
	fakeEditor
	now  *time.Time
	step time.Duration
}

func (e *clockEditor) Propose(ctx context.Context, req EditRequest) (EditResult, error) {
	*e.now = e.now.Add(e.step)
	return e.fakeEditor.Propose(ctx, req)
}

func TestNightHourCap(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 20; c.MaxHoursPerNight = 8 })
	now := time.Date(2026, 11, 10, 22, 0, 0, 0, time.UTC)
	ed := &clockEditor{now: &now, step: 3 * time.Hour}
	d := f.deps(ed, &fakeJudge{scores: []float64{50}})
	d.Now = func() time.Time { return now }
	sum, err := RunNight(context.Background(), f.store, f.name, d)
	if err != nil {
		t.Errorf("RunNight: %v", err)
	}
	if sum.StopReason != "max-hours" || ed.calls != 3 || sum.Tries != 3 {
		t.Errorf("stop %q after %d tries (%d editor calls), want max-hours after 3", sum.StopReason, sum.Tries, ed.calls)
	}
}

// flakyJudge fails main call number failOn and otherwise defers to fakeJudge.
type flakyJudge struct {
	fakeJudge
	failOn int
	n      int
	mu     sync.Mutex
}

func (j *flakyJudge) Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error) {
	j.mu.Lock()
	j.n++
	n := j.n
	j.mu.Unlock()
	if n == j.failOn {
		return JudgeResult{}, errors.New("judge timed out")
	}
	return j.fakeJudge.Run(ctx, candidate, set, outPath)
}

func TestNightJudgeCrashIsLoggedAndReset(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 2 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n"), appendBytes("two\n")}}
	j := &flakyJudge{fakeJudge: fakeJudge{scores: []float64{50}}, failOn: 3}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	rows := f.resultRows(t)
	if len(rows) != 3 || rows[1]["status"] != "crash" || rows[1]["score"] != "-" || rows[2]["status"] != "reset" {
		t.Errorf("rows = %v, want baseline, crash, reset", rows)
	}
	f.wantBaselineHead(t, f.load(t))
}

func TestKeptTryAdvancesBest(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("better\n")}}
	j := &fakeJudge{scores: []float64{50, 50, 80, 90}}
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	cfg := f.load(t)
	if cfg.BestScore != 85 || sum.Kept != 1 || sum.EndScore != 85 || cfg.NoGainNights != 0 {
		t.Errorf("best %v, kept %d, end %v, no-gain %d", cfg.BestScore, sum.Kept, sum.EndScore, cfg.NoGainNights)
	}
	head := f.repoHead(t)
	if head == cfg.StartCommit || head != cfg.BestCommit {
		t.Errorf("HEAD %q, best %q, start %q", head, cfg.BestCommit, cfg.StartCommit)
	}
	if cfg.BestBytes != 107 {
		t.Errorf("BestBytes = %d, want 107", cfg.BestBytes)
	}
	if b, _ := os.ReadFile(filepath.Join(f.repoDir(), "SKILL.md")); !bytes.HasSuffix(b, []byte("better\n")) {
		t.Errorf("repo target = %q, want the kept edit", b)
	}
	if row := f.lastRow(t); row["status"] != "kept" || row["score"] != "85.0" || row["bytes"] != "107" {
		t.Errorf("row = %v", row)
	}
	if live, _ := os.ReadFile(f.liveTarget); !bytes.Equal(live, f.original) {
		t.Errorf("a night touched the live target")
	}
}

func TestMaxNightsSwitchesOff(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 1; c.MaxNights = 1 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n")}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, &fakeJudge{scores: []float64{50}})); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if cfg := f.load(t); cfg.Status != StatusOff || cfg.StopReason != "max-nights" || cfg.Nights != 1 {
		t.Errorf("status %q, reason %q, nights %d; want off, max-nights, 1", cfg.Status, cfg.StopReason, cfg.Nights)
	}
}

// leakyJudge writes a full evalharness-style report with scenario names
// and failed-check reasons into its result file.
type leakyJudge struct{ fakeJudge }

func (j *leakyJudge) Run(ctx context.Context, candidate, set, outPath string) (JudgeResult, error) {
	res, err := j.fakeJudge.Run(ctx, candidate, set, outPath)
	if err != nil {
		return res, err
	}
	report := `{"score": 50, "scenarios": [{"name": "SECRET-SCENARIO", "runs": [{"pass": false, "reason": "SECRET-REASON check 0 file-exists"}]}]}`
	return res, os.WriteFile(outPath, []byte(report), 0o600)
}

func TestEditorSeesScoreOnly(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 2 })
	var seen [][]string
	ed := &fakeEditor{edits: []func(string) error{
		func(p string) error {
			entries, _ := os.ReadDir(filepath.Dir(p))
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			seen = append(seen, names)
			return appendBytes("one\n")(p)
		},
		appendBytes("two\n"),
	}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, &leakyJudge{fakeJudge{scores: []float64{50}}})); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if len(seen) != 1 || !slices.Equal(seen[0], []string{"SKILL.md", "frozen", "goal.md", "results.tsv"}) {
		t.Errorf("scratch entries = %v, want SKILL.md frozen goal.md results.tsv", seen)
	}
	scratches := 0
	filepath.WalkDir(filepath.Join(f.store.Dir(f.name), "nights"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(path, string(filepath.Separator)+"scratch"+string(filepath.Separator)) {
			return err
		}
		scratches++
		b, _ := os.ReadFile(path)
		for _, secret := range []string{"SECRET-SCENARIO", "SECRET-REASON", "file-exists", "out.txt", "m1", "h1", "Follow the demo skill"} {
			if strings.Contains(string(b), secret) {
				t.Errorf("scratch file %s holds %q", path, secret)
			}
		}
		return nil
	})
	if scratches == 0 {
		t.Errorf("no scratch files found to check")
	}
	if strings.Contains(f.readArtifact(t, "results.tsv"), "SECRET") {
		t.Errorf("results.tsv holds judge detail")
	}
}

func TestEditorResultsHoldOnlyScoreColumns(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 2 })
	var tsv string
	ed := &fakeEditor{edits: []func(string) error{
		appendBytes("one\n"),
		func(p string) error {
			b, _ := os.ReadFile(filepath.Join(filepath.Dir(p), "results.tsv"))
			tsv = string(b)
			return nil
		},
	}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, &fakeJudge{scores: []float64{50, 50, 40}})); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	want := "try\tscore\tstatus\tbytes\tdescription\n0\t50.0\tbaseline\t100\tbaseline\n1\t40.0\treset\t104\ttry 1\n"
	if tsv != want {
		t.Errorf("editor results.tsv =\n%q\nwant\n%q", tsv, want)
	}
}

func TestNightReportSection(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 2 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n"), appendBytes("two\n")}}
	d := f.deps(ed, &fakeJudge{scores: []float64{50, 50, 80, 80, 10}})
	reads := 0
	d.WeeklyUsed = func(context.Context) (float64, error) { reads++; return 20 + float64(reads), nil }
	if _, err := RunNight(context.Background(), f.store, f.name, d); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	report := f.readArtifact(t, "report.md")
	for _, want := range []string{"## Night 1", "stop reason: max-tries", "tries: 2", "kept: 1", "score: 50.0 -> 80.0", "quota used: 2.0 points"} {
		if !strings.Contains(report, want) {
			t.Errorf("report.md lacks %q:\n%s", want, report)
		}
	}
	if cfg := f.load(t); cfg.QuotaUsed != 2 {
		t.Errorf("cfg.QuotaUsed = %v, want 2", cfg.QuotaUsed)
	}
}

func TestNightReportsEachRowToProgress(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 2 })
	ed := &fakeEditor{edits: []func(string) error{appendBytes("x\n"), appendBytes("y\n")}}
	d := f.deps(ed, &fakeJudge{scores: []float64{50, 50, 80, 80, 40}})
	var got []string
	d.Progress = func(r Row) { got = append(got, fmt.Sprintf("%d:%s", r.Try, r.Status)) }
	if _, err := RunNight(context.Background(), f.store, f.name, d); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if want := "0:baseline 1:kept 2:reset"; strings.Join(got, " ") != want {
		t.Errorf("progress = %v, want %s", got, want)
	}
}

// off while a night runs must stop the night and stay off, and it is a
// pause: a kept change does not trigger the end-of-campaign holdout.
func TestOffDuringNightStopsItAndSticks(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 3 })
	off := func(path string) error {
		if err := f.store.SetOn(f.name, false); err != nil {
			return err
		}
		return appendBytes("better\n")(path)
	}
	ed := &fakeEditor{edits: []func(string) error{off, appendBytes("two\n"), appendBytes("three\n")}}
	j := &fakeJudge{scores: []float64{50, 50, 80, 90}, holdout: func([]byte) float64 { return 70 }}
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	cfg := f.load(t)
	if cfg.Status != StatusOff || cfg.StopReason != "manual" {
		t.Errorf("status %q reason %q after off mid-night, want off manual", cfg.Status, cfg.StopReason)
	}
	if ed.calls != 1 || sum.StopReason != "switched-off" {
		t.Errorf("night ran %d editor calls, stop %q; want 1 and switched-off", ed.calls, sum.StopReason)
	}
	if cfg.BestScore != 85 || j.holdoutCalls != 0 {
		t.Errorf("best %v, holdout calls %d; want the kept 85 and no holdout", cfg.BestScore, j.holdoutCalls)
	}
}

// A cut-short night is no evidence of no gain, but it spent quota, so
// it still uses one of max_nights and gets its own night folder.
func TestCutShortNightsCountTowardMaxNightsNotNoGain(t *testing.T) {
	for _, reason := range []string{"interrupted", "quota-read-failed"} {
		t.Run(reason, func(t *testing.T) {
			f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 5 })
			for night := 1; night <= 2; night++ {
				ctx, cancel := context.WithCancel(context.Background())
				ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n")}}
				d := f.deps(ed, &fakeJudge{scores: []float64{50}})
				reads := 0
				d.WeeklyUsed = func(context.Context) (float64, error) {
					reads++
					if reads == 1 {
						return 20, nil
					}
					if reason == "interrupted" {
						cancel()
						return 0, ctx.Err()
					}
					return 0, errors.New("token expired")
				}
				sum, err := RunNight(ctx, f.store, f.name, d)
				cancel()
				if err != nil {
					t.Fatalf("night %d: RunNight: %v", night, err)
				}
				cfg := f.load(t)
				if sum.StopReason != reason || cfg.Nights != night || cfg.NoGainNights != 0 {
					t.Errorf("night %d: stop %q, nights %d, no-gain %d; want %s, nights %d, no-gain 0", night, sum.StopReason, cfg.Nights, cfg.NoGainNights, reason, night)
				}
				if _, err := os.Stat(filepath.Join(f.store.Dir(f.name), "nights", fmt.Sprint(night), "try-1")); err != nil {
					t.Errorf("night %d has no folder of its own: %v", night, err)
				}
			}
		})
	}
}

func offFirst(f fixture, then func(string) error) func(string) error {
	return func(path string) error {
		if err := f.store.SetOn(f.name, false); err != nil {
			return err
		}
		return then(path)
	}
}

func TestManualOffNightIsNotANoGainNight(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 3 })
	ed := &fakeEditor{edits: []func(string) error{offFirst(f, appendBytes("worse\n"))}}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, &fakeJudge{scores: []float64{50, 50, 40}})); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if cfg := f.load(t); cfg.Nights != 1 || cfg.NoGainNights != 0 || cfg.Status != StatusOff || cfg.StopReason != "manual" {
		t.Errorf("nights %d, no-gain %d, status %s %q; want 1, 0, off manual", cfg.Nights, cfg.NoGainNights, cfg.Status, cfg.StopReason)
	}
}

// Off during the last allowed night must not leave a campaign that can
// neither go back on nor have run its holdout.
func TestManualOffOnLastNightStillEndsCampaign(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 3; c.MaxNights = 1 })
	ed := &fakeEditor{edits: []func(string) error{offFirst(f, appendBytes("better\n"))}}
	j := &fakeJudge{scores: []float64{50, 50, 80, 90}, holdout: func([]byte) float64 { return 70 }}
	if _, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, j)); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if cfg := f.load(t); cfg.Status != StatusStopped || cfg.StopReason != "max-nights" || j.holdoutCalls == 0 {
		t.Errorf("status %s %q, holdout calls %d; want stopped max-nights with the holdout run", cfg.Status, cfg.StopReason, j.holdoutCalls)
	}
}

func TestManualOffNightStillCountsLastTryQuota(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 3 })
	ed := &fakeEditor{edits: []func(string) error{offFirst(f, appendBytes("worse\n"))}}
	d := f.deps(ed, &fakeJudge{scores: []float64{50, 50, 40}})
	reads := 0
	d.WeeklyUsed = func(context.Context) (float64, error) {
		reads++
		if reads == 1 {
			return 20, nil
		}
		return 26, nil
	}
	if _, err := RunNight(context.Background(), f.store, f.name, d); err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if cfg := f.load(t); cfg.QuotaUsed != 6 {
		t.Errorf("quota used %v, want the 6 points the try spent", cfg.QuotaUsed)
	}
}

// An interrupted night that reaches max_nights cannot run the holdout,
// so it must say how to finish the campaign instead of leaving it stuck.
func TestInterruptedLastNightTellsHowToFinish(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 5; c.MaxNights = 1 })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ed := &fakeEditor{edits: []func(string) error{appendBytes("better\n")}}
	d := f.deps(ed, &fakeJudge{scores: []float64{50, 50, 80, 90}, holdout: func([]byte) float64 { return 70 }})
	reads := 0
	d.WeeklyUsed = func(context.Context) (float64, error) {
		if reads++; reads == 1 {
			return 20, nil
		}
		cancel()
		return 0, ctx.Err()
	}
	sum, err := RunNight(ctx, f.store, f.name, d)
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	cfg := f.load(t)
	if sum.StopReason != "interrupted" || cfg.Status != StatusOff || cfg.StopReason != "max-nights" || !cfg.hasKeptChange() {
		t.Fatalf("stop %q, status %s %q, kept %v; want an interrupted last night with a kept change", sum.StopReason, cfg.Status, cfg.StopReason, cfg.hasKeptChange())
	}
	if len(*f.notes) != 1 || !strings.Contains((*f.notes)[0], "spore research stop "+f.name) {
		t.Errorf("notes %q; want one telling the coordinator to run spore research stop %s", *f.notes, f.name)
	}
	if !strings.Contains(f.readArtifact(t, "report.md"), "spore research stop "+f.name) {
		t.Errorf("report lacks the stop advice")
	}
	if err := f.store.SetOn(f.name, true); err == nil || !strings.Contains(err.Error(), "spore research stop "+f.name) {
		t.Errorf("on after the interrupted last night = %v; want a refusal naming spore research stop %s", err, f.name)
	}
}

// A judge that fails before scoring anything (missing grader, short
// credentials) fails every try the same way: two in a row end the night.
func TestJudgeSetupFailureEndsTheNight(t *testing.T) {
	f := newCampaign(t, func(c *Config) {
		c.MaxTriesPerNight = 5
		c.BestCommit, c.StartScore, c.BestScore, c.Nights = c.StartCommit, 50, 50, 1
	})
	script := filepath.Join(t.TempDir(), "judge")
	os.WriteFile(script, []byte("#!/bin/sh\necho 'credentials expire too soon' >&2\nexit 1\n"), 0o700)
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n"), appendBytes("two\n"), appendBytes("three\n")}}
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, CommandJudge{Argv: []string{script}}))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	cfg := f.load(t)
	if sum.StopReason != "judge-failed" || ed.calls != 2 || cfg.Nights != 2 || cfg.NoGainNights != 0 {
		t.Errorf("stop %q, editor calls %d, nights %d, no-gain %d; want judge-failed after 2 tries, nights 2, no-gain 0", sum.StopReason, ed.calls, cfg.Nights, cfg.NoGainNights)
	}
	if row := f.lastRow(t); row["status"] != "crash" {
		t.Errorf("last row %v, want the try logged as a crash", row)
	}
}

// One failure (an OOM kill, a transient spawn error) is a crashed try,
// not a broken judge.
func TestOneJudgeFailureDoesNotEndTheNight(t *testing.T) {
	f := newCampaign(t, func(c *Config) {
		c.MaxTriesPerNight = 3
		c.BestCommit, c.StartScore, c.BestScore, c.Nights = c.StartCommit, 50, 50, 1
	})
	dir := t.TempDir()
	script := filepath.Join(dir, "judge")
	os.WriteFile(script, []byte("#!/bin/sh\nm="+filepath.Join(dir, "failed-once")+"\n[ -e $m ] || { touch $m; exit 1; }\n"+
		"while [ $# -gt 0 ]; do [ \"$1\" = --out ] && out=$2; shift; done\necho '{\"score\":40,\"runs\":3}' > \"$out\"\n"), 0o700)
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n"), appendBytes("two\n"), appendBytes("three\n")}}
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, CommandJudge{Argv: []string{script}}))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if sum.StopReason != "max-tries" || ed.calls != 3 {
		t.Errorf("stop %q after %d tries; want max-tries after 3", sum.StopReason, ed.calls)
	}
}

// An off on the try that also crosses the quota ceiling is still a
// pause: it must not count as a no-gain night and auto-stop the campaign.
func TestOffOnQuotaCeilingTryIsNotANoGainNight(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 3; c.StopAfterNightsWithoutGain = 1 })
	ed := &fakeEditor{edits: []func(string) error{offFirst(f, appendBytes("worse\n"))}}
	d := f.deps(ed, &fakeJudge{scores: []float64{50, 50, 40}})
	reads := 0
	d.WeeklyUsed = func(context.Context) (float64, error) {
		if reads++; reads == 1 {
			return 20, nil
		}
		return 40, nil
	}
	sum, err := RunNight(context.Background(), f.store, f.name, d)
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	cfg := f.load(t)
	if sum.StopReason != "quota-ceiling" || cfg.NoGainNights != 0 || cfg.Status != StatusOff || cfg.StopReason != "manual" {
		t.Errorf("stop %q, no-gain %d, status %s %q; want quota-ceiling, 0, off manual", sum.StopReason, cfg.NoGainNights, cfg.Status, cfg.StopReason)
	}
}

// Only judge failures in a row count; a try lost to the editor breaks
// the run.
func TestEditorCrashBreaksAJudgeFailureRun(t *testing.T) {
	f := newCampaign(t, func(c *Config) {
		c.MaxTriesPerNight = 3
		c.BestCommit, c.StartScore, c.BestScore, c.Nights = c.StartCommit, 50, 50, 1
	})
	script := filepath.Join(t.TempDir(), "judge")
	os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	broken := func(string) error { return errors.New("editor broke") }
	ed := &fakeEditor{edits: []func(string) error{appendBytes("one\n"), broken, appendBytes("three\n")}}
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(ed, CommandJudge{Argv: []string{script}}))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if sum.StopReason != "max-tries" || ed.calls != 3 {
		t.Errorf("stop %q after %d tries; want max-tries after 3", sum.StopReason, ed.calls)
	}
}

// The baseline follows the same two-strike rule as a try.
func TestOneBaselineJudgeFailureIsRetried(t *testing.T) {
	f := newCampaign(t, func(c *Config) { c.MaxTriesPerNight = 0 })
	dir := t.TempDir()
	script := filepath.Join(dir, "judge")
	os.WriteFile(script, []byte("#!/bin/sh\nm="+filepath.Join(dir, "failed-once")+"\n[ -e $m ] || { touch $m; exit 1; }\n"+
		"while [ $# -gt 0 ]; do [ \"$1\" = --out ] && out=$2; shift; done\necho '{\"score\":60,\"runs\":3}' > \"$out\"\n"), 0o700)
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(&fakeEditor{}, CommandJudge{Argv: []string{script}}))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if cfg := f.load(t); sum.StopReason == "baseline-crash" || cfg.BestCommit != cfg.StartCommit || cfg.StartScore != 60 {
		t.Errorf("stop %q, best %q, start score %v; want the baseline recorded after one retry", sum.StopReason, cfg.BestCommit, cfg.StartScore)
	}
}

// Two in a row still end the baseline night.
func TestTwoBaselineJudgeFailuresEndTheNight(t *testing.T) {
	f := newCampaign(t, nil)
	script := filepath.Join(t.TempDir(), "judge")
	os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	sum, err := RunNight(context.Background(), f.store, f.name, f.deps(&fakeEditor{}, CommandJudge{Argv: []string{script}}))
	if err != nil {
		t.Fatalf("RunNight: %v", err)
	}
	if sum.StopReason != "baseline-crash" {
		t.Errorf("stop %q, want baseline-crash", sum.StopReason)
	}
}
