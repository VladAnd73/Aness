package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
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

type dailyUsageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

type dailyUsageBody struct {
	FiveHour *dailyUsageWindow `json:"five_hour"`
	SevenDay *dailyUsageWindow `json:"seven_day"`
	Limits   []struct {
		Kind    string   `json:"kind"`
		Percent *float64 `json:"percent"`
		Scope   *struct {
			Model *struct {
				DisplayName string `json:"display_name"`
			} `json:"model"`
		} `json:"scope"`
	} `json:"limits"`
}

func endpointReading(ctx context.Context, now time.Time) (Reading, error) {
	body, err := fetchUsageBody(ctx, now)
	if err != nil {
		return Reading{}, fmt.Errorf("endpoint: %w", err)
	}
	var ub dailyUsageBody
	if err := json.Unmarshal(body, &ub); err != nil {
		return Reading{}, fmt.Errorf("endpoint: decode: %w", err)
	}
	w := ub.SevenDay
	if w == nil || w.Utilization == nil || w.ResetsAt == nil || *w.ResetsAt == "" {
		return Reading{}, errors.New("endpoint: response has no seven_day utilization and resets_at")
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
	for _, l := range ub.Limits {
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
	cfg, err := loadDailyConfig()
	if err != nil {
		return failDaily(now, defaultDailyMargin, err)
	}
	var errs []error
	for _, src := range sources {
		r, err := src(ctx, now)
		if err == nil {
			err = checkWeeklyReset(r, now)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rec := computeVerdict(r, cfg.Margin, now)
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
func checkWeeklyReset(r Reading, now time.Time) error {
	if !r.WeeklyResetsAt.After(now) || r.WeeklyResetsAt.After(now.Add(weeklyWindow+time.Hour)) {
		return fmt.Errorf("%s: weekly resets_at %s is not within the coming week", r.Source, r.WeeklyResetsAt.UTC().Format(time.RFC3339))
	}
	return nil
}

var atomicWrite = func(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, stateFileMode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

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
	f, err := os.OpenFile(filepath.Join(d, "daily-history.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, stateFileMode)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func Daily() error {
	rec, err := runDaily(context.Background(), time.Now(), []ReadingSource{endpointReading, probeReading})
	if rec.Verdict != "" {
		fmt.Println(roomLine(rec.Verdict, rec, ""))
	}
	return err
}
