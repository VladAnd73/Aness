package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/versality/spore/internal/research"
)

const researchUsage = `usage: spore research <subcommand> [flags]

An overnight loop that edits one skill file, scores each edit with a
locked judge, and keeps only confirmed improvements. See
docs/autoresearch.md.

  new <c> --target PATH/SKILL.md --goal FILE [--objective maximize-score] [--model M]
         Create draft campaign c from the skill folder holding PATH.
  judge draft <c>
         Mint a draft worker task (in ./tasks) that drafts the main and
         holdout scenario sets. Prints the slug; does not start it.
  judge approve <c> --from DIR
         Import DIR/main and DIR/holdout, pin this spore binary as the
         judge, lock all three by hash. The campaign becomes ready.
  on <c>
         Allow nights to run.
  off <c>
         Stop allowing nights (no holdout; on again later).
  set <c> <key> <value>
         Change a setting while no night runs. Keys: %s.
  night <c>
         Run one night in the foreground, one line per try. Ctrl-C,
         SIGTERM or a closed tmux window end it cleanly. Exit 0 when it
         ran, 1 when refused (reason on stderr).
  status [<c>]
         One line per campaign: status, nights, best score, stop reason.
  report <c>
         Print the campaign's report.md.
  stop <c>
         End the campaign: run the sealed holdout on a kept change,
         write the final report, tell the coordinator.
  accept <c> [--force]
         Land the winner of a stopped campaign (a branch when the skill
         lives in the spore repo). --force overrides an overfit holdout.
  reset <c>
         Put the campaign repo back on its best commit (after a kill -9
         left it dirty). Refused while a night runs.
  fake-mcp --services FILE --calls-log FILE [--listen 127.0.0.1:0]
         Serve canned MCP tools; prints "<server> <url>" lines, then
         serves until killed.
`

func runResearch(args []string) int {
	return researchMain(os.Stdout, os.Stderr, args, researchDeps{})
}

// researchDeps carries what a test swaps out of `spore research`.
// SporeBinary is the binary `judge approve` copies into the campaign as
// its pinned judge; empty means the running executable.
type researchDeps struct {
	Editor      research.Editor
	Room        func() (code int, line string)
	WeeklyUsed  func(ctx context.Context) (float64, error)
	Notify      func(msg string) error
	SporeBinary string
}

// errUsage marks a malformed command line: exit 2 with the usage.
var errUsage = errors.New("usage")

type researchCmd struct {
	out, errOut io.Writer
	d           researchDeps
	store       research.Store
	lockPath    string
}

func researchMain(out, errOut io.Writer, args []string, d researchDeps) int {
	if len(args) == 0 {
		fmt.Fprintf(errOut, researchUsage, strings.Join(research.SettableKeys, ", "))
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintf(out, researchUsage, strings.Join(research.SettableKeys, ", "))
		return 0
	}
	store, err := research.DefaultStore()
	if err != nil {
		fmt.Fprintln(errOut, "spore", err)
		return 1
	}
	c := &researchCmd{out: out, errOut: errOut, d: d, store: store, lockPath: research.DefaultLockPath()}
	sub, rest := args[0], args[1:]
	if sub == "judge" && len(rest) > 0 {
		sub, rest = "judge "+rest[0], rest[1:]
	}
	run := map[string]func([]string) error{
		"new": c.new, "judge draft": c.judgeDraft, "judge approve": c.judgeApprove,
		"on": c.onOff(true), "off": c.onOff(false), "set": c.set, "night": c.night,
		"status": c.status, "report": c.report, "stop": c.stop, "accept": c.accept,
		"reset": c.reset, "fake-mcp": c.fakeMCP,
	}[sub]
	if run == nil {
		fmt.Fprintf(errOut, "spore research: unknown subcommand %q\n\n", sub)
		fmt.Fprintf(errOut, researchUsage, strings.Join(research.SettableKeys, ", "))
		return 2
	}
	err = run(rest)
	var refused *research.RefusedError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		fmt.Fprintf(errOut, "spore research %s: %v\n", sub, err)
		return 2
	case errors.As(err, &refused):
		fmt.Fprintln(errOut, "spore", err)
		return 1
	default:
		fmt.Fprintf(errOut, "spore research %s: %v\n", sub, err)
		return 1
	}
}

// parse reads flags in any position and wants exactly npos positionals.
func parse(fs *flag.FlagSet, args []string, npos int, names string) ([]string, error) {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(reorderFlagsFirst(fs, args)); err != nil {
		return nil, fmt.Errorf("%w: %v", errUsage, err)
	}
	if fs.NArg() != npos {
		return nil, fmt.Errorf("%w: want %s", errUsage, names)
	}
	return fs.Args(), nil
}

func (c *researchCmd) new(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	target := fs.String("target", "", "the skill's SKILL.md")
	goal := fs.String("goal", "", "file stating what the skill should achieve")
	objective := fs.String("objective", "maximize-score", "maximize-score or shrink-holding-score")
	model := fs.String("model", "", "model for the editor and the agent under test")
	pos, err := parse(fs, args, 1, "<campaign> --target PATH --goal FILE")
	if err != nil {
		return err
	}
	if *target == "" || *goal == "" {
		return fmt.Errorf("%w: --target and --goal are required", errUsage)
	}
	if filepath.Base(*target) != "SKILL.md" {
		return fmt.Errorf("v1 improves skills only: --target must be a file named SKILL.md, got %s", filepath.Base(*target))
	}
	cfg, err := c.store.Create(pos[0], *target, *goal, *objective)
	if err != nil {
		return err
	}
	if *model != "" {
		// Not Set: its lock is host-wide, and a draft cannot be in a night.
		cfg.Model = *model
		if err := c.store.Save(cfg); err != nil {
			return err
		}
	}
	fmt.Fprintf(c.out, "created campaign %s (draft) at %s\nnext: spore research judge draft %s\n", cfg.Name, c.store.Dir(cfg.Name), cfg.Name)
	return nil
}

func (c *researchCmd) judgeApprove(args []string) error {
	fs := flag.NewFlagSet("judge approve", flag.ContinueOnError)
	from := fs.String("from", "", "drafts folder holding main/ and holdout/")
	pos, err := parse(fs, args, 1, "<campaign> --from DIR")
	if err != nil {
		return err
	}
	if *from == "" {
		return fmt.Errorf("%w: --from is required", errUsage)
	}
	bin, err := c.sporeBinary()
	if err != nil {
		return err
	}
	cfg, err := c.store.ApproveJudge(pos[0], *from, bin, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "campaign %s is ready; judge pinned at %s\nnext: spore research on %s\n", cfg.Name, cfg.JudgeArgv[0], cfg.Name)
	return nil
}

func (c *researchCmd) sporeBinary() (string, error) {
	if c.d.SporeBinary != "" {
		return c.d.SporeBinary, nil
	}
	return os.Executable()
}

func (c *researchCmd) onOff(on bool) func([]string) error {
	return func(args []string) error {
		pos, err := parse(flag.NewFlagSet("on", flag.ContinueOnError), args, 1, "<campaign>")
		if err != nil {
			return err
		}
		if err := c.store.SetOn(pos[0], on); err != nil {
			return err
		}
		cfg, err := c.store.Load(pos[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(c.out, "campaign %s is %s\n", cfg.Name, cfg.Status)
		return nil
	}
}

func (c *researchCmd) set(args []string) error {
	pos, err := parse(flag.NewFlagSet("set", flag.ContinueOnError), args, 3, "<campaign> <key> <value>")
	if err != nil {
		return err
	}
	if err := c.store.Set(pos[0], pos[1], pos[2], c.lockPath); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s: %s = %s\n", pos[0], pos[1], pos[2])
	return nil
}

func (c *researchCmd) status(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(reorderFlagsFirst(fs, args)); err != nil || fs.NArg() > 1 {
		return fmt.Errorf("%w: want [<campaign>]", errUsage)
	}
	names := fs.Args()
	if len(names) == 0 {
		entries, err := os.ReadDir(c.store.Root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, e := range entries {
			if _, err := os.Stat(filepath.Join(c.store.Dir(e.Name()), "campaign.toml")); e.IsDir() && err == nil {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			fmt.Fprintf(c.out, "no campaigns in %s\n", c.store.Root)
		}
	}
	for _, name := range names {
		cfg, err := c.store.Load(name)
		if err != nil {
			return err
		}
		best, stop := "-", "-"
		if cfg.BestCommit != "" {
			best = fmt.Sprintf("%.1f", cfg.BestScore)
		}
		if cfg.StopReason != "" {
			stop = cfg.StopReason
		}
		fmt.Fprintf(c.out, "%s  %s  nights %d/%d  best %s  stop %s\n", cfg.Name, cfg.Status, cfg.Nights, cfg.MaxNights, best, stop)
	}
	return nil
}

func (c *researchCmd) report(args []string) error {
	pos, err := parse(flag.NewFlagSet("report", flag.ContinueOnError), args, 1, "<campaign>")
	if err != nil {
		return err
	}
	if _, err := c.store.Load(pos[0]); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(c.store.Dir(pos[0]), "report.md"))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("campaign %s has no report yet; run a night first", pos[0])
	}
	if err != nil {
		return err
	}
	_, err = c.out.Write(b)
	return err
}

func (c *researchCmd) accept(args []string) error {
	fs := flag.NewFlagSet("accept", flag.ContinueOnError)
	force := fs.Bool("force", false, "accept although the holdout said overfit")
	pos, err := parse(fs, args, 1, "<campaign> [--force]")
	if err != nil {
		return err
	}
	res, err := research.Accept(c.store, pos[0], *force)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "written: %s\n", res.Written)
	if res.Backup != "" {
		fmt.Fprintf(c.out, "backup: %s\n", res.Backup)
	}
	if res.Branch != "" {
		fmt.Fprintf(c.out, "branch: %s\nopen a PR: git push -u origin %s && gh pr create --head %s\n", res.Branch, res.Branch, res.Branch)
	}
	return nil
}

func (c *researchCmd) reset(args []string) error {
	pos, err := parse(flag.NewFlagSet("reset", flag.ContinueOnError), args, 1, "<campaign>")
	if err != nil {
		return err
	}
	commit, err := research.Reset(c.store, pos[0], c.lockPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "reset %s to %s\n", pos[0], commit)
	return nil
}
