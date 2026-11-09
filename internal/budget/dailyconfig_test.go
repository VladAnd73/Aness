package budget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDailyConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "budget.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPORE_BUDGET_CONFIG", p)
	return p
}

func TestLoadDailyConfigDefault(t *testing.T) {
	t.Setenv("SPORE_BUDGET_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	cfg, err := loadDailyConfig()
	if err != nil {
		t.Fatalf("loadDailyConfig: %v", err)
	}
	if cfg.Margin != 10 {
		t.Errorf("margin: got %v want 10", cfg.Margin)
	}
}

func TestLoadDailyConfigMargin(t *testing.T) {
	writeDailyConfig(t, "# headroom tuning\n\nmargin = 5 # points\n")
	cfg, err := loadDailyConfig()
	if err != nil {
		t.Fatalf("loadDailyConfig: %v", err)
	}
	if cfg.Margin != 5 {
		t.Errorf("margin: got %v want 5", cfg.Margin)
	}
}

func TestLoadDailyConfigRejectsBad(t *testing.T) {
	for _, body := range []string{
		"margin = abc\n",
		"margin = -3\n",
		"margin = NaN\n",
		"marign = 5\n",
		"margin 5\n",
		"[daily]\nmargin = 5\n",
	} {
		p := writeDailyConfig(t, body)
		_, err := loadDailyConfig()
		if err == nil {
			t.Errorf("%q: want error", body)
			continue
		}
		if !strings.Contains(err.Error(), p) {
			t.Errorf("%q: error %q does not name %s", body, err, p)
		}
	}
}

func TestDailyConfigPathFollowsXDG(t *testing.T) {
	t.Setenv("SPORE_BUDGET_CONFIG", "")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if got, want := dailyConfigPath(), filepath.Join(dir, "spore", "budget.toml"); got != want {
		t.Errorf("path: got %s want %s", got, want)
	}
}
