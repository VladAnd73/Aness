package evalharness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveInjectedSkillIsTheOnlyOneWithItsName pins what a real run
// showed on 2026-11-10 (claude 2.1.296): under --setting-sources
// project a host skill in ~/.claude/skills is not loaded, so a
// candidate injected under the same name is the only skill by that name.
//
//	EVALHARNESS_LIVE=1 go test ./internal/evalharness/ -run TestLiveInjectedSkill -v
func TestLiveInjectedSkillIsTheOnlyOneWithItsName(t *testing.T) {
	if os.Getenv("EVALHARNESS_LIVE") == "" {
		t.Skip("set EVALHARNESS_LIVE=1 to run a real agent")
	}
	home, _ := os.UserHomeDir()
	entries, _ := os.ReadDir(filepath.Join(home, ".claude", "skills"))
	if len(entries) == 0 {
		t.Skip("no host user skill to collide with")
	}
	name := entries[0].Name()
	sb, err := NewSandbox(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := gitInitNoRemote(sb.RunWD); err != nil {
		t.Fatal(err)
	}
	cand := filepath.Join(t.TempDir(), "SKILL.md")
	os.WriteFile(cand, []byte("---\nname: "+name+"\ndescription: probe\n---\nReply with exactly: PROBE-DONE\n"), 0o644)
	if err := InjectSkill(sb.RunWD, "", name, cand); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	res, err := ClaudeSpawner{}.Spawn(ctx, SpawnRequest{
		Prompt: "Use the " + name + " skill now.", WorkDir: sb.RunWD, Env: sb.Env(),
		ExtraArgs: append(append([]string{}, SessionSpawnArgs...), "--strict-mcp-config", "--model", "haiku"),
	})
	if err != nil {
		t.Fatalf("spawn: %v\n%s", err, res.Stdout)
	}
	n := 0
	for _, line := range strings.Split(res.Stdout, "\n") {
		var ev struct {
			Type, Subtype string
			Skills        []string `json:"skills"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "system" || ev.Subtype != "init" {
			continue
		}
		for _, s := range ev.Skills {
			if s == name {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("init lists skill %q %d times, want once", name, n)
	}
	if !strings.Contains(res.Stdout, filepath.Join(sb.RunWD, ".claude", "skills", name)) {
		t.Errorf("the loaded skill's base directory is not the injected one:\n%s", res.Stdout)
	}
}
