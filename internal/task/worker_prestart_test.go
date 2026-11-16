package task

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versality/spore/internal/task/frontmatter"
)

// Given [fleet] pre_start, when a homogeneous worker's launch command
// runs, then the hook runs before the agent, as it does for loop panes.
func TestWorkerLaunchRunsPreStart(t *testing.T) {
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock not on PATH")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(AgentBinaryEnv, "printf agent-ran")
	flag := filepath.Join(t.TempDir(), "hook-ran")
	dir := writeProjectTOML(t, "[fleet]\npre_start = \"touch "+flag+"\"\n")

	cmd, err := workerLaunchCommand(dir, "feat", frontmatter.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	if err != nil {
		t.Fatalf("run %q: %v: %s", cmd, err, out)
	}
	if _, err := os.Stat(flag); err != nil {
		t.Errorf("pre_start hook never ran for a worker: %v (command %q)", err, cmd)
	}
	if string(out) != "agent-ran" {
		t.Errorf("agent output = %q, want %q", out, "agent-ran")
	}
}

// Given pre_start and isolate_network, when a worker's launch command
// is built, then the hook runs inside the network namespace with the
// agent, so services it starts are on the loopback the agent sees.
func TestWorkerLaunchRunsPreStartInsideNetns(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(AgentBinaryEnv, "sleep 30")
	defer stubLookPath(func(string) (string, error) { return "/nix/store/x/bin/pasta", nil })()
	dir := writeProjectTOML(t, "[worker]\nisolate_network = true\n[fleet]\npre_start = \"bin/up\"\n")

	cmd, err := workerLaunchCommand(dir, "feat", frontmatter.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cmd, "pasta ") || !strings.Contains(cmd, "flock") {
		t.Errorf("command %q: want the pre-start hook (flock) inside the pasta wrapper", cmd)
	}
	if !strings.Contains(cmd, "env IS_SANDBOX=1 sleep 30") {
		t.Errorf("command %q: want IS_SANDBOX=1 on the agent", cmd)
	}
}

// Given no pre_start, the worker launch command is unchanged.
func TestWorkerLaunchWithoutPreStartUnchanged(t *testing.T) {
	t.Setenv(AgentBinaryEnv, "sleep 30")
	defer stubLookPath(func(string) (string, error) { return "/nix/store/x/bin/pasta", nil })()
	dir := writeProjectTOML(t, "[worker]\nisolate_network = true\n")
	got, err := workerLaunchCommand(dir, "feat", frontmatter.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	if want := "pasta --config-net -T none -U none -t none -u none -- env IS_SANDBOX=1 sleep 30"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
