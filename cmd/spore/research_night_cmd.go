package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/versality/spore/internal/budget"
	"github.com/versality/spore/internal/evalharness"
	"github.com/versality/spore/internal/isolate"
	"github.com/versality/spore/internal/research"
	"github.com/versality/spore/internal/research/fakemcp"
)

// signalContext ends on Ctrl-C, SIGTERM, or SIGHUP (the tmux window
// the night runs in was closed), so the night can reset its repo and
// release the lock before it exits.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}

// engineDeps fills what the test did not inject with the real budget
// readers, the campaign's claude editor and judge command, and a
// coordinator notice sent from this project.
func (c *researchCmd) engineDeps(cfg research.Config) (research.Deps, error) {
	d := research.Deps{
		Editor:     c.d.Editor,
		Judge:      research.CommandJudge{Argv: cfg.JudgeArgv},
		Room:       c.d.Room,
		WeeklyUsed: c.d.WeeklyUsed,
		Notify:     c.d.Notify,
		LockPath:   c.lockPath,
	}
	if d.Editor == nil {
		d.Editor = research.ClaudeEditor{Binary: cfg.AgentBinary, Model: cfg.Model, Isolation: cfg.Isolation}
	}
	if d.Room == nil {
		d.Room = func() (int, string) { return budget.RoomCode(time.Now()) }
	}
	if d.WeeklyUsed == nil {
		d.WeeklyUsed = budget.WeeklyUsed
	}
	if d.Notify == nil {
		bin, err := c.sporeBinary()
		if err != nil {
			return d, err
		}
		root := os.Getenv("SPORE_PROJECT_ROOT")
		if root == "" {
			if root, err = os.Getwd(); err != nil {
				return d, err
			}
		}
		d.Notify = research.TellCoordinator(bin, root)
	}
	return d, nil
}

func (c *researchCmd) night(args []string) error {
	pos, err := parse(flag.NewFlagSet("night", flag.ContinueOnError), args, 1, "<campaign>")
	if err != nil {
		return err
	}
	cfg, err := c.store.Load(pos[0])
	if err != nil {
		return err
	}
	switch cfg.Isolation {
	case "none":
		if os.Getenv(evalharness.AllowUnsandboxedEnv) != "1" {
			return &research.RefusedError{Reason: fmt.Sprintf("isolation is none; set %s=1 to run agents unsandboxed", evalharness.AllowUnsandboxedEnv)}
		}
	default:
		if err := isolate.Available(); err != nil {
			return &research.RefusedError{Reason: fmt.Sprintf("bubblewrap is not usable: %v", err)}
		}
	}
	d, err := c.engineDeps(cfg)
	if err != nil {
		return err
	}
	start := time.Now()
	d.Progress = func(r research.Row) {
		fmt.Fprintf(c.out, "try %d  %-15s  score %.1f  bytes %d  elapsed %s\n",
			r.Try, r.Status, r.Score, r.Bytes, time.Since(start).Round(time.Second))
	}
	ctx, stop := signalContext()
	defer stop()
	sum, err := research.RunNight(ctx, c.store, cfg.Name, d)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "night %d of %s ended after %s; stop reason: %s; tries %d, kept %d; score %.1f -> %.1f\n",
		sum.Night, cfg.Name, time.Since(start).Round(time.Second), sum.StopReason, sum.Tries, sum.Kept, sum.StartScore, sum.EndScore)
	if after, err := c.store.Load(cfg.Name); err == nil && after.Status != research.StatusOn {
		fmt.Fprintf(c.out, "campaign %s is now %s (%s)\n", after.Name, after.Status, after.StopReason)
	}
	return nil
}

func (c *researchCmd) stop(args []string) error {
	pos, err := parse(flag.NewFlagSet("stop", flag.ContinueOnError), args, 1, "<campaign>")
	if err != nil {
		return err
	}
	cfg, err := c.store.Load(pos[0])
	if err != nil {
		return err
	}
	d, err := c.engineDeps(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := signalContext()
	defer cancel()
	if err := research.Stop(ctx, c.store, cfg.Name, "manual", d); err != nil {
		return err
	}
	after, err := c.store.Load(cfg.Name)
	if err != nil {
		return err
	}
	verdict := after.HoldoutVerdict
	if verdict == "" {
		verdict = "no change"
	}
	fmt.Fprintf(c.out, "campaign %s stopped; holdout: %s\nreport: spore research report %s\n", after.Name, verdict, after.Name)
	return nil
}

func (c *researchCmd) fakeMCP(args []string) error {
	fs := flag.NewFlagSet("fake-mcp", flag.ContinueOnError)
	services := fs.String("services", "", "JSON array of tools to serve")
	callsLog := fs.String("calls-log", "", "JSON lines file every call is appended to")
	listen := fs.String("listen", "127.0.0.1:0", "address to listen on")
	if _, err := parse(fs, args, 0, "--services FILE --calls-log FILE"); err != nil {
		return err
	}
	if *services == "" || *callsLog == "" {
		return fmt.Errorf("%w: --services and --calls-log are required", errUsage)
	}
	ctx, stop := signalContext()
	defer stop()
	return fakemcp.Serve(ctx, *services, *callsLog, *listen, func(urls map[string]string) {
		names := make([]string, 0, len(urls))
		for n := range urls {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(c.out, "%s %s\n", n, urls[n])
		}
	})
}
