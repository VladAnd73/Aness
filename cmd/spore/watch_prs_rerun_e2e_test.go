package main

import (
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// Bug 4: a CI rerun puts the failing check back to pending. That poll saw
// no failing check, pruned the commit's seen keys, and the next failure
// on the same commit alerted the coordinator again.
func TestWatchPRsDoesNotRealertTheSameCommitAfterACIRerun(t *testing.T) {
	bin := sharedSporeBin(t)
	env := e2eEnv(t)
	project := "rerunproj"
	root := filepath.Join(t.TempDir(), project)
	writeFile(t, filepath.Join(root, ".keep"), "", 0o644)
	cfg := envValue(env, "XDG_CONFIG_HOME")
	writeFile(t, filepath.Join(cfg, "spore", project, "watch.toml"), "enabled = true\nchecks = [\"e2e\"]\n", 0o644)

	counter := filepath.Join(t.TempDir(), "round")
	gh := filepath.Join(t.TempDir(), "gh")
	writeFile(t, gh, `#!/bin/sh
case "$2" in
list) echo '[{"number":7,"headRefName":"feat-x","headRefOid":"sha1","isDraft":false,"url":"u7"}]' ;;
checks)
  n=$(cat `+counter+` 2>/dev/null || echo 0)
  if [ "$n" = 1 ]; then echo '[{"name":"e2e-run","bucket":"pending","link":"l"}]'; exit 8; fi
  echo '[{"name":"e2e-run","bucket":"fail","link":"https://github.com/o/r/actions/runs/11/job/2"}]'; exit 8 ;;
esac
`, 0o755)
	env = append(env, "SPORE_GH_BINARY="+gh, "SPORE_COORDINATOR_STATE_DIR="+filepath.Join(t.TempDir(), "coord"))

	alertsRE := regexp.MustCompile(`(\d+) alert\(s\)`)
	total := 0
	for round, state := range []string{"fail", "pending", "fail"} {
		writeFile(t, counter, strconv.Itoa(round), 0o644)
		res := runCLI(t, bin, root, env, "", "watch", "prs", "--project-root", root)
		m := alertsRE.FindStringSubmatch(res.stdout)
		if res.code != 0 || m == nil {
			t.Fatalf("round %d (%s): exit %d:\n%s%s", round, state, res.code, res.stdout, res.stderr)
		}
		n, _ := strconv.Atoi(m[1])
		total += n
	}
	if total != 1 {
		t.Fatalf("one failing check on one commit alerted %d times across fail, rerun-pending, fail; want 1", total)
	}
}
