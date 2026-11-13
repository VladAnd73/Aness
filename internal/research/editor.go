package research

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/versality/spore/internal/evalharness"
	"github.com/versality/spore/internal/isolate"
)

// EditRequest names the editor's scratch folder. The folder holds the
// target at TargetRel, the campaign goal as goal.md and the score log
// as results.tsv, and nothing else the editor may read.
type EditRequest struct {
	ScratchDir, TargetRel string
	Try                   int
}

type EditResult struct {
	Description, Transcript string
	ExitCode                int
}

type Editor interface {
	Propose(ctx context.Context, req EditRequest) (EditResult, error)
}

// ClaudeEditor proposes edits with a real claude process, a fresh
// session per try. Empty Binary means "claude"; empty Model passes no
// --model flag; empty Isolation means bwrap; zero Timeout means 10
// minutes.
type ClaudeEditor struct {
	Binary, Model, Isolation string
	Timeout                  time.Duration
	run                      func(*exec.Cmd) (string, int, error)
}

const descriptionFile = ".try-description"

const editorPrompt = `You improve one file, %[1]s, in this directory.
Read goal.md (what the file should achieve), results.tsv (the scores
of earlier tries) and %[1]s. Then make exactly one focused edit to
%[1]s. The frozen/ folder is reference material and cannot be changed.
Finally write a one-line description of your edit to %[2]s in this
directory.`

func (e ClaudeEditor) Propose(ctx context.Context, req EditRequest) (EditResult, error) {
	switch e.Isolation {
	case "", "bwrap":
	case "none":
		if os.Getenv(evalharness.AllowUnsandboxedEnv) != "1" {
			return EditResult{}, fmt.Errorf("research: editor: isolation none runs the editor unsandboxed; set %s=1 to allow it", evalharness.AllowUnsandboxedEnv)
		}
	default:
		return EditResult{}, fmt.Errorf("research: editor: unknown isolation %q (want bwrap or none)", e.Isolation)
	}
	bin := e.Binary
	if bin == "" {
		bin = "claude"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return EditResult{}, fmt.Errorf("research: editor: %w", err)
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	argv := []string{path, "-p", "--tools", "Read,Edit,Write", "--permission-mode", "acceptEdits",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "project",
		"--output-format", "stream-json", "--verbose"}
	if e.Model != "" {
		argv = append(argv, "--model", e.Model)
	}
	argv = append(argv, "--", fmt.Sprintf(editorPrompt, req.TargetRel, descriptionFile))

	var cmd *exec.Cmd
	switch e.Isolation {
	case "none":
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir = req.ScratchDir
	case "", "bwrap":
		home, err := os.MkdirTemp("", "research-editor-home-")
		if err != nil {
			return EditResult{}, fmt.Errorf("research: editor: %w", err)
		}
		defer os.RemoveAll(home)
		spec := isolate.Spec{
			Work:        req.ScratchDir,
			Home:        home,
			Credentials: isolate.CredentialsPath(),
			Env:         isolate.BaseEnv(home),
		}
		if frozen := filepath.Join(req.ScratchDir, "frozen"); dirExists(frozen) {
			spec.Extra = []isolate.Bind{{Src: frozen, Dst: frozen}}
		}
		if cmd, err = isolate.Command(ctx, spec, argv); err != nil {
			return EditResult{}, fmt.Errorf("research: editor: %w", err)
		}
		defer func() {
			for _, f := range cmd.ExtraFiles {
				f.Close()
			}
		}()
	}

	run := e.run
	if run == nil {
		run = evalharness.RunCommand
	}
	descPath := filepath.Join(req.ScratchDir, descriptionFile)
	os.Remove(descPath)
	out, code, runErr := run(cmd)
	res := EditResult{Transcript: out, ExitCode: code, Description: readDescription(descPath)}
	os.Remove(descPath)
	if err := ctx.Err(); err != nil {
		return res, fmt.Errorf("research: editor: %w", err)
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return res, fmt.Errorf("research: editor: %w", runErr)
	}
	return res, nil
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func readDescription(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(no description)"
	}
	line, _, _ := strings.Cut(string(b), "\n")
	line = strings.TrimSpace(line)
	if len(line) > 200 {
		line = line[:200]
	}
	if line == "" {
		return "(no description)"
	}
	return line
}
