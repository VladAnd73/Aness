package budget

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultDailyMargin = 10

type DailyConfig struct {
	Margin float64
}

func dailyConfigPath() string {
	if p := os.Getenv("SPORE_BUDGET_CONFIG"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "spore", "budget.toml")
}

func loadDailyConfig() (DailyConfig, error) {
	cfg := DailyConfig{Margin: defaultDailyMargin}
	path := dailyConfigPath()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("budget config %s: %w", path, err)
	}
	for i, raw := range strings.Split(string(b), "\n") {
		line := raw
		if j := strings.IndexByte(line, '#'); j >= 0 {
			line = line[:j]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return cfg, fmt.Errorf("budget config %s line %d: want key = value, got %q", path, i+1, raw)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "margin":
			m, err := strconv.ParseFloat(val, 64)
			if err != nil || math.IsNaN(m) || math.IsInf(m, 0) || m < 0 {
				return cfg, fmt.Errorf("budget config %s line %d: margin must be a number >= 0, got %q", path, i+1, val)
			}
			cfg.Margin = m
		default:
			return cfg, fmt.Errorf("budget config %s line %d: unknown key %q", path, i+1, key)
		}
	}
	return cfg, nil
}
