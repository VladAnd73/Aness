package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBudgetRoomExitCodes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_BUDGET_STATE_DIR", dir)
	if code := runBudgetCmd("room", nil); code != 2 {
		t.Errorf("no daily.json: got %d want 2", code)
	}
	resets := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	taken := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	for verdict, want := range map[string]int{"room": 0, "no-room": 1, "unknown": 2} {
		body := `{"taken_at":"` + taken + `","verdict":"` + verdict + `","weekly_resets_at":"` + resets + `","source":"endpoint"}`
		if err := os.WriteFile(filepath.Join(dir, "daily.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if code := runBudgetCmd("room", nil); code != want {
			t.Errorf("%s: got %d want %d", verdict, code, want)
		}
	}
	if code := runBudgetCmd("room", []string{"extra"}); code != 2 {
		t.Errorf("room with args: got %d want 2", code)
	}
}

func TestBudgetUsageListsDailyAndRoom(t *testing.T) {
	for _, want := range []string{"spore budget daily", "spore budget room", "daily.json", "daily-history.jsonl", "budget.toml"} {
		if !strings.Contains(budgetUsage, want) {
			t.Errorf("usage text lacks %q", want)
		}
	}
}

func TestBudgetUsageDailyExitRule(t *testing.T) {
	for _, want := range []string{"non-zero when no fresh verdict was saved", "history append failure only warns"} {
		if !strings.Contains(strings.Join(strings.Fields(budgetUsage), " "), want) {
			t.Errorf("usage text lacks %q", want)
		}
	}
}
