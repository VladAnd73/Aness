package research

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConfigRoundTrip(t *testing.T) {
	s := Store{Root: t.TempDir()}
	want := Config{
		Name: "demo", Target: "/live/demo/SKILL.md", SkillName: "demo", Objective: "maximize-score",
		Model: "opus", GoalFile: "/x/goal.md", AgentBinary: "/bin/claude",
		JudgeArgv:        []string{"/x/bin/spore", "eval", "judge", `with "quote", comma`, ""},
		MaxTriesPerNight: 20, MaxNights: 7, StopAfterNightsWithoutGain: 3, Parallel: 3, Repeats: 3,
		MaxHoursPerNight: 8, QuotaCeilingPoints: 15.5, RunTimeout: 5 * time.Minute, Isolation: "bwrap",
		Status: StatusOn, StopReason: "manual", StartBytes: 100, BestBytes: 90,
		StartScore: 66.66666666666667, BestScore: 80,
		BestCommit: "abc", StartCommit: "def", MainHash: "sha256:1", HoldoutHash: "sha256:2",
		BinaryHash: "sha256:3", LiveFolderHash: "sha256:4", Nights: 2, NoGainNights: 1,
		HoldoutVerdict: "pass", QuotaUsed: 12.5, JudgeCostUSD: 3.25,
	}
	if err := os.MkdirAll(s.Dir("demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load("demo")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, want)
	}
	fi, err := os.Stat(filepath.Join(s.Dir("demo"), "campaign.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("campaign.toml mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestConfigUnknownKeyErrors(t *testing.T) {
	s := Store{Root: t.TempDir()}
	os.MkdirAll(s.Dir("demo"), 0o700)
	body := "name = \"demo\"\nbogus_key = 1\n"
	os.WriteFile(filepath.Join(s.Dir("demo"), "campaign.toml"), []byte(body), 0o600)
	_, err := s.Load("demo")
	if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "bogus_key") {
		t.Errorf("Load = %v, want an error naming line 2 and the key", err)
	}
}
