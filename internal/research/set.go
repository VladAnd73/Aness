package research

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SettableKeys are the campaign.toml keys an operator may change with
// Set; every other key is the engine's own bookkeeping.
var SettableKeys = []string{
	"model", "agent_binary", "isolation", "max_tries_per_night", "max_hours_per_night",
	"quota_ceiling_points", "max_nights", "stop_after_nights_without_gain",
	"parallel", "repeats", "run_timeout",
}

// Set changes one operator key of a campaign that is not running. A
// judge argv that is still the default one is rebuilt, so model,
// repeats and the like reach the judge as well.
func (s Store) Set(name, key, value, lockPath string) error {
	if !slices.Contains(SettableKeys, key) {
		return fmt.Errorf("research: %q is not a settable key (want one of %s)", key, strings.Join(SettableKeys, ", "))
	}
	unlock, holder, err := Lock(lockPath, name)
	if err != nil {
		if holder != "" {
			return fmt.Errorf("research: campaign %s: a night is running (lock held by %s); set it after the night ends", name, holder)
		}
		return err
	}
	defer unlock()
	cfg, err := s.Load(name)
	if err != nil {
		return err
	}
	switch cfg.Status {
	case StatusDraft, StatusReady, StatusOn, StatusOff:
	default:
		return fmt.Errorf("research: campaign %s is %s; its settings are final", name, cfg.Status)
	}
	wasDefault := s.isDefaultJudgeArgv(cfg)
	var ptr any
	for _, f := range configFields(&cfg) {
		if f.key == key {
			ptr = f.ptr
		}
	}
	raw := value
	switch ptr.(type) {
	case *string, *time.Duration:
		if _, err := strconv.Unquote(value); err != nil {
			raw = strconv.Quote(value)
		}
	}
	if err := decodeValue(ptr, raw); err != nil {
		return fmt.Errorf("research: %s = %q: %v", key, value, err)
	}
	if err := checkSetting(cfg, key); err != nil {
		return err
	}
	if wasDefault {
		cfg.JudgeArgv = defaultJudgeArgv(cfg, s.pinnedBinary(name), filepath.Join(s.Dir(name), "judge"), s.repo(name).dir)
	}
	return s.Save(cfg)
}

func checkSetting(cfg Config, key string) error {
	bad := func(why string) error { return fmt.Errorf("research: %s: %s", key, why) }
	switch key {
	case "isolation":
		if cfg.Isolation != "bwrap" && cfg.Isolation != "none" {
			return bad("want bwrap or none")
		}
	case "max_tries_per_night", "max_nights", "stop_after_nights_without_gain", "parallel", "repeats":
		var n int
		for _, f := range configFields(&cfg) {
			if f.key == key {
				n = *f.ptr.(*int)
			}
		}
		if n < 1 {
			return bad("want a whole number of at least 1")
		}
	case "max_hours_per_night":
		if cfg.MaxHoursPerNight <= 0 {
			return bad("want a number above 0")
		}
	case "quota_ceiling_points":
		if cfg.QuotaCeilingPoints <= 0 {
			return bad("want a number above 0")
		}
	case "run_timeout":
		if cfg.RunTimeout <= 0 {
			return bad("want a duration above 0, such as 5m")
		}
	}
	return nil
}

func (s Store) pinnedBinary(name string) string { return filepath.Join(s.Dir(name), "bin", "spore") }

func (s Store) isDefaultJudgeArgv(cfg Config) bool {
	a := cfg.JudgeArgv
	return len(a) >= 3 && a[0] == s.pinnedBinary(cfg.Name) && a[1] == "eval" && a[2] == "judge"
}
