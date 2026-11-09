package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	VerdictRoom    = "room"
	VerdictNoRoom  = "no-room"
	VerdictUnknown = "unknown"
)

type Reading struct {
	Source           string
	WeeklyUsed       float64
	WeeklyResetsAt   time.Time
	FiveHourUsed     float64
	FiveHourResetsAt time.Time
	Extra            map[string]float64
	Warnings         []string
}

type DailyRecord struct {
	TakenAt          time.Time          `json:"taken_at"`
	Source           string             `json:"source"`
	WeeklyUsed       float64            `json:"weekly_used"`
	WeeklyResetsAt   time.Time          `json:"weekly_resets_at"`
	Pace             float64            `json:"pace"`
	Headroom         float64            `json:"headroom"`
	Margin           float64            `json:"margin"`
	Verdict          string             `json:"verdict"`
	FiveHourUsed     float64            `json:"five_hour_used"`
	FiveHourResetsAt time.Time          `json:"five_hour_resets_at"`
	Extra            map[string]float64 `json:"extra,omitempty"`
	Error            string             `json:"error,omitempty"`
	Errors           []string           `json:"errors,omitempty"`
}

// MarshalJSON leaves the numbers out of an unknown record, so a failed
// day in the history cannot be mistaken for a 0% reading.
func (r DailyRecord) MarshalJSON() ([]byte, error) {
	type plain DailyRecord
	if r.Verdict != VerdictUnknown {
		return json.Marshal(plain(r))
	}
	return json.Marshal(struct {
		TakenAt time.Time `json:"taken_at"`
		Margin  float64   `json:"margin"`
		Verdict string    `json:"verdict"`
		Error   string    `json:"error,omitempty"`
		Errors  []string  `json:"errors,omitempty"`
	}{r.TakenAt, r.Margin, r.Verdict, r.Error, r.Errors})
}

type ReadingSource func(ctx context.Context, now time.Time) (Reading, error)

const weeklyWindow = 7 * 24 * time.Hour

func computeVerdict(r Reading, margin float64, now time.Time) DailyRecord {
	start := r.WeeklyResetsAt.Add(-weeklyWindow)
	elapsed := float64(now.Sub(start)) / float64(weeklyWindow)
	elapsed = math.Min(math.Max(elapsed, 0), 1)
	pace := elapsed * 100
	headroom := pace - r.WeeklyUsed
	verdict := VerdictNoRoom
	if headroom >= margin {
		verdict = VerdictRoom
	}
	return DailyRecord{
		TakenAt:          now.UTC(),
		Source:           r.Source,
		WeeklyUsed:       r.WeeklyUsed,
		WeeklyResetsAt:   r.WeeklyResetsAt.UTC(),
		Pace:             pace,
		Headroom:         headroom,
		Margin:           margin,
		Verdict:          verdict,
		FiveHourUsed:     r.FiveHourUsed,
		FiveHourResetsAt: r.FiveHourResetsAt.UTC(),
		Extra:            r.Extra,
	}
}

type usageLimit struct {
	Kind    string   `json:"kind"`
	Percent *float64 `json:"percent"`
	Scope   *struct {
		Model *struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

func endpointReading(ctx context.Context, now time.Time) (Reading, error) {
	body, err := fetchUsageBody(ctx, now)
	if err != nil {
		return Reading{}, fmt.Errorf("endpoint: %w", err)
	}
	ub, err := decodeUsageBody(body)
	if err != nil {
		return Reading{}, fmt.Errorf("endpoint: %w", err)
	}
	w := ub.SevenDay
	if w.ResetsAt == nil || *w.ResetsAt == "" {
		return Reading{}, errors.New("endpoint: response has no seven_day resets_at")
	}
	resets, err := parseUsageTimestamp(*w.ResetsAt)
	if err != nil {
		return Reading{}, fmt.Errorf("endpoint: seven_day resets_at: %w", err)
	}
	r := Reading{Source: "endpoint", WeeklyUsed: *w.Utilization, WeeklyResetsAt: resets}
	if f := ub.FiveHour; f != nil {
		if f.Utilization != nil {
			r.FiveHourUsed = *f.Utilization
		}
		if f.ResetsAt != nil {
			if t, err := parseUsageTimestamp(*f.ResetsAt); err == nil {
				r.FiveHourResetsAt = t
			}
		}
	}
	if len(ub.Limits) == 0 || string(ub.Limits) == "null" {
		return r, nil
	}
	var limits []usageLimit
	if err := json.Unmarshal(ub.Limits, &limits); err != nil {
		r.Warnings = append(r.Warnings, fmt.Sprintf("endpoint: limits ignored: %v", err))
		return r, nil
	}
	for _, l := range limits {
		if l.Kind != "weekly_scoped" || l.Percent == nil || l.Scope == nil || l.Scope.Model == nil || l.Scope.Model.DisplayName == "" {
			continue
		}
		if r.Extra == nil {
			r.Extra = map[string]float64{}
		}
		r.Extra[l.Scope.Model.DisplayName] = *l.Percent
	}
	return r, nil
}

func runDaily(ctx context.Context, now time.Time, sources []ReadingSource) (DailyRecord, error) {
	unlock, err := lockDaily(ctx)
	if err != nil {
		return DailyRecord{}, err
	}
	defer unlock()
	cfg, err := loadDailyConfig()
	if err != nil {
		return failDaily(now, defaultDailyMargin, err)
	}
	var errs []error
	for _, src := range sources {
		r, err := src(ctx, now)
		if err == nil {
			err = checkReading(r, now)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rec := computeVerdict(r, cfg.Margin, now)
		for _, e := range errs {
			rec.Errors = append(rec.Errors, e.Error())
		}
		rec.Errors = append(rec.Errors, r.Warnings...)
		for _, e := range rec.Errors {
			fmt.Fprintf(dailyStderr, "spore budget daily: %s\n", e)
		}
		if err := saveDaily(rec); err != nil {
			return failDaily(now, cfg.Margin, append(errs, fmt.Errorf("save %s reading: %w", r.Source, err))...)
		}
		return rec, nil
	}
	if len(errs) == 0 {
		errs = append(errs, errors.New("no reading sources"))
	}
	return failDaily(now, cfg.Margin, errs...)
}

// failDaily records unknown so a failed run never leaves an earlier
// room live. When even that save fails (a full disk still allows an
// unlink), daily.json is removed, which room also reads as unknown.
func failDaily(now time.Time, margin float64, errs ...error) (DailyRecord, error) {
	failed := errors.Join(errs...)
	rec := DailyRecord{TakenAt: now.UTC(), Margin: margin, Verdict: VerdictUnknown, Error: failed.Error()}
	if err := saveDaily(rec); err != nil {
		failed = errors.Join(failed, fmt.Errorf("save unknown record: %w", err))
		if rerr := removeDaily(); rerr != nil {
			failed = errors.Join(failed, fmt.Errorf("remove daily.json: %w", rerr))
		}
	}
	return rec, failed
}

func removeDaily() error {
	d, err := stateDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(d, "daily.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// A reset already past would clamp pace to 100 and read as room from
// the previous week's usage.
func checkReading(r Reading, now time.Time) error {
	if !validPercent(r.WeeklyUsed) {
		return fmt.Errorf("%s: weekly used %v is not a percentage in 0..100", r.Source, r.WeeklyUsed)
	}
	if !r.WeeklyResetsAt.After(now) || r.WeeklyResetsAt.After(now.Add(weeklyWindow+time.Hour)) {
		return fmt.Errorf("%s: weekly resets_at %s is not within the coming week", r.Source, r.WeeklyResetsAt.UTC().Format(time.RFC3339))
	}
	return nil
}

var dailyStderr io.Writer = os.Stderr

var atomicWrite = func(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// lockDaily serialises daily runs: two at once would share the probe's
// tmux server, each killing the other's probe. Like spore dream's lock
// it is an flock, so a crashed holder never wedges later runs. It
// blocks: the holder is bounded by the probe timeout.
func lockDaily(ctx context.Context) (unlock func(), err error) {
	d, err := stateDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d, stateDirMode); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(d, "daily.lock"), os.O_CREATE|os.O_RDWR, stateFileMode)
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("lock %s: %w", f.Name(), err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("waiting for another daily run: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// saveDaily replaces daily.json, then appends to the history. A history
// failure is only a warning: the fresh verdict is already live.
func saveDaily(rec DailyRecord) error {
	d, err := stateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, stateDirMode); err != nil {
		return err
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if err := atomicWrite(filepath.Join(d, "daily.json"), line); err != nil {
		return err
	}
	hist := filepath.Join(d, "daily-history.jsonl")
	if err := appendLine(hist, line); err != nil {
		fmt.Fprintf(dailyStderr, "spore budget daily: warning: %s not appended: %v\n", hist, err)
	}
	return nil
}

func appendLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, stateFileMode)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// dailyContext turns Ctrl-C and a systemd stop into a cancelled run, so
// the probe's deferred teardown kills its tmux server and claude.
func dailyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func Daily() error {
	ctx, stop := dailyContext()
	defer stop()
	rec, err := runDaily(ctx, time.Now(), []ReadingSource{endpointReading, probeReading})
	if rec.Verdict != "" {
		fmt.Println(roomLine(rec.Verdict, rec, ""))
	}
	return err
}
