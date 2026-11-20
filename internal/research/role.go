package research

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/versality/spore/internal/evalharness"
	"github.com/versality/spore/internal/fleet"
)

// RoleOptions names the loop step a role target runs as. An empty Loop
// searches every loop file of the target's project for Step.
type RoleOptions struct{ Step, Loop string }

// ContextDir holds a role campaign's frozen context in its repo: the
// loop file, the pinned step and the step's skills. The editor sees it
// under frozen/, the judge reads it as --frozen.
const ContextDir = "context"

// roleSetup is what CreateRole resolved before any campaign file
// exists: the step, its loop file and each found skill's folder.
type roleSetup struct {
	step     evalharness.RoleStep
	loopFile string
	skills   map[string]string
}

// CreateRole starts a draft campaign on a role body: the target alone
// is the editable file, and the step's loop file and skills are copied
// as frozen context. missing names the step's skills found neither in
// the project's nor the user's .claude/skills, such as a plugin's.
func (s Store) CreateRole(name, target, goalFile, objective string, ro RoleOptions) (cfg Config, missing []string, err error) {
	if filepath.Ext(target) != ".md" {
		return Config{}, nil, fmt.Errorf("research: a role target must be a markdown body ending in .md, got %s", filepath.Base(target))
	}
	if ro.Step == "" {
		return Config{}, nil, errors.New("research: a role target needs the loop step it runs as (--step)")
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return Config{}, nil, err
	}
	setup, missing, err := resolveRole(target, ro)
	if err != nil {
		return Config{}, nil, err
	}
	cfg, err = s.create(name, target, goalFile, objective, &setup)
	return cfg, missing, err
}

func resolveRole(target string, ro RoleOptions) (roleSetup, []string, error) {
	loops := findLoopsDir(filepath.Dir(target))
	if loops == "" {
		return roleSetup{}, nil, fmt.Errorf("research: no bootstrap/loops folder above %s", target)
	}
	files, err := filepath.Glob(filepath.Join(loops, "*.toml"))
	if err != nil {
		return roleSetup{}, nil, err
	}
	if ro.Loop != "" {
		files = []string{filepath.Join(loops, ro.Loop+".toml")}
	}
	role := strings.TrimSuffix(filepath.Base(target), ".md")
	var found []roleSetup
	others := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return roleSetup{}, nil, fmt.Errorf("research: loop: %w", err)
		}
		def, err := fleet.ParseLoopDef(f, b)
		if err != nil {
			return roleSetup{}, nil, fmt.Errorf("research: %w", err)
		}
		for _, st := range def.Steps {
			switch {
			case st.Name != ro.Step || st.Kind != "agent":
			case st.Role != role:
				others[st.Role] = true
			default:
				found = append(found, roleSetup{step: roleStepFor(def.Name, st), loopFile: f})
			}
		}
	}
	switch {
	case len(found) == 0 && len(others) > 0:
		return roleSetup{}, nil, fmt.Errorf("research: step %q runs role %s, not %s", ro.Step, strings.Join(slices.Sorted(maps.Keys(others)), ", "), role)
	case len(found) == 0:
		return roleSetup{}, nil, fmt.Errorf("research: no agent step %q in %s", ro.Step, loops)
	case len(found) > 1:
		return roleSetup{}, nil, fmt.Errorf("research: step %q runs role %s in more than one loop; name one with --loop", ro.Step, role)
	}
	setup := found[0]
	if setup.step.Output == "" {
		return roleSetup{}, nil, fmt.Errorf("research: step %q writes no output to grade", ro.Step)
	}
	project := filepath.Dir(filepath.Dir(loops))
	home, _ := os.UserHomeDir()
	setup.skills = map[string]string{}
	var missing []string
	for _, sk := range setup.step.Skills {
		dir := ""
		for _, root := range []string{filepath.Join(project, ".claude", "skills"), filepath.Join(home, ".claude", "skills")} {
			if p, err := filepath.EvalSymlinks(filepath.Join(root, sk)); err == nil && dirExists(p) {
				dir = p
				break
			}
		}
		if dir == "" {
			missing = append(missing, sk)
			continue
		}
		setup.skills[sk] = dir
	}
	return setup, missing, nil
}

func findLoopsDir(dir string) string {
	for {
		if p := filepath.Join(dir, "bootstrap", "loops"); dirExists(p) {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// roleStepFor pins a loop step as the judge runs it.
func roleStepFor(loop string, st fleet.Step) evalharness.RoleStep {
	rs := evalharness.RoleStep{Loop: loop, Step: st.Name, Role: st.Role, Output: st.Output, Shape: st.Shape,
		Required: st.Required, Skills: st.Skills}
	switch st.Shape {
	case "engineer-response":
		rs.Author = "engineer"
	case "verdict":
		rs.Author = st.Label
		if rs.Author == "" {
			rs.Author = st.Name
		}
	}
	for _, t := range st.Next {
		if t.On != "" {
			rs.Verdicts = append(rs.Verdicts, t.On)
		}
	}
	return rs
}

// writeRoleRepo fills a fresh role campaign repo: the target at its
// root, the frozen context beside it.
func writeRoleRepo(repoDir, target string, setup *roleSetup) error {
	ctx := filepath.Join(repoDir, ContextDir)
	if err := os.MkdirAll(filepath.Join(ctx, "skills"), 0o700); err != nil {
		return err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repoDir, filepath.Base(target)), body, 0o600); err != nil {
		return err
	}
	loop, err := os.ReadFile(setup.loopFile)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ctx, "loop.toml"), loop, 0o600); err != nil {
		return err
	}
	step, err := json.MarshalIndent(setup.step, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ctx, evalharness.RoleStepFile), append(step, '\n'), 0o600); err != nil {
		return err
	}
	for name, dir := range setup.skills {
		if err := copyFolder(dir, filepath.Join(ctx, "skills", name)); err != nil {
			return fmt.Errorf("skill %s: %w", name, err)
		}
	}
	return nil
}

func (cfg Config) isRole() bool { return cfg.Kind == "role" }

// RoleContextDir is the role campaign's frozen context, the judge's
// --frozen folder.
func (s Store) RoleContextDir(name string) string { return filepath.Join(s.repo(name).dir, ContextDir) }

func (cfg Config) roleStep() evalharness.RoleStep {
	return evalharness.RoleStep{Loop: cfg.Loop, Step: cfg.Step, Role: cfg.SkillName, Output: cfg.StepOutput,
		Shape: cfg.StepShape, Author: cfg.StepAuthor, Required: cfg.StepRequired, Verdicts: cfg.StepVerdicts, Skills: cfg.StepSkills}
}

// checkRoleStep refuses a role campaign whose campaign.toml step differs
// from context/step.json, the copy the judge scores with.
func (s Store) checkRoleStep(cfg Config) error {
	if !cfg.isRole() {
		return nil
	}
	pinned, err := evalharness.LoadRoleStep(filepath.Join(s.RoleContextDir(cfg.Name), evalharness.RoleStepFile))
	if err != nil {
		return err
	}
	a, _ := json.Marshal(pinned)
	b, _ := json.Marshal(cfg.roleStep())
	if !bytes.Equal(a, b) {
		return fmt.Errorf("the step in campaign.toml differs from %s/%s, which the judge scores with", ContextDir, evalharness.RoleStepFile)
	}
	return nil
}

func (cfg *Config) setRoleStep(rs evalharness.RoleStep) {
	cfg.Kind, cfg.Loop, cfg.Step, cfg.SkillName = "role", rs.Loop, rs.Step, rs.Role
	cfg.StepOutput, cfg.StepShape, cfg.StepAuthor = rs.Output, rs.Shape, rs.Author
	cfg.StepRequired, cfg.StepVerdicts, cfg.StepSkills = rs.Required, rs.Verdicts, rs.Skills
}

// gitIgnored reports whether git ignores path in its checkout: accept
// cuts the research branch from HEAD, which never holds such a file.
func gitIgnored(path string) bool {
	return exec.Command("git", "-C", filepath.Dir(path), "check-ignore", "-q", "--", path).Run() == nil
}
