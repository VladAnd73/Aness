package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/versality/spore/internal/ghapp"
	"github.com/versality/spore/internal/task"
)

const ghUsage = `spore gh - act on GitHub as the project's GitHub App

Usage:
  spore gh config [--project <path>] [--set key=value ...]
  spore gh token [--project <path>]
  spore gh exec [--project <path>] -- <cmd> [args...]

The project is --project when given, else $SPORE_PROJECT_ROOT, else the
repo holding the cwd. When $SPORE_PROJECT_ROOT is set and the cwd is in
a different git repo (a worktree of the same project is fine), spore gh
refuses rather than guess; pass --project to pick one. In exec,
--project is read only before the command; after it, it is the child's.

Subcommands:
  config   Print the host-side github.toml
           ($XDG_STATE_HOME/spore/<project>/github.toml, 0600). --set
           writes a key (repeatable); an empty value deletes it. Keys:
           app_id, installation_id, private_key, api_url.
           operator_login: after exec runs gh pr create without an
           assignee, spore assigns this login to the new PR.
           pr_flow, commit_name and commit_email are accepted but
           have no effect in this build.
  token    Print an installation token (cached host-side until ~5 min
           before expiry).
  exec     Run <cmd> with GH_TOKEN set and a git credential helper
           injected, so gh and git push over https act as the app.
           Refuses when the cwd repo's origin is not https.

See docs/github-app.md for the one-time app setup.
`

func runGH(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, ghUsage)
		return 2
	}
	var err error
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(ghUsage)
		return 0
	case "config":
		err = runGHConfig(args[1:])
	case "token":
		err = runGHToken(args[1:])
	case "exec":
		var code int
		code, err = runGHExec(args[1:])
		if err == nil {
			return code
		}
	default:
		fmt.Fprintf(os.Stderr, "spore gh: unknown subcommand %q\n\n%s", args[0], ghUsage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "spore gh:", err)
		return 1
	}
	return 0
}

// takeProjectFlag removes --project <path> / --project=<path> from args.
// With stopAtCmd, scanning ends at the first other argument ("--" or
// the command), since everything from there on belongs to exec's child.
func takeProjectFlag(args []string, stopAtCmd bool) (project string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--project":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--project needs a path")
			}
			project = args[i+1]
			i++
		case strings.HasPrefix(a, "--project="):
			project = strings.TrimPrefix(a, "--project=")
		case stopAtCmd:
			return project, append(rest, args[i:]...), nil
		default:
			rest = append(rest, a)
		}
	}
	return project, rest, nil
}

func ghProjectRoot(flag string) (string, error) {
	if flag != "" {
		return task.MainRepoRoot(flag)
	}
	env := os.Getenv("SPORE_PROJECT_ROOT")
	root, err := task.MainRepoRoot(env)
	if err != nil || env == "" {
		return root, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if exec.Command("git", "-C", wd, "rev-parse", "--git-common-dir").Run() != nil {
		return root, nil
	}
	cwdRoot, err := task.MainRepoRoot(wd)
	if err != nil {
		return "", err
	}
	if !samePath(root, cwdRoot) {
		return "", fmt.Errorf("SPORE_PROJECT_ROOT is %s but the cwd is in %s; pass --project <path> to choose", root, cwdRoot)
	}
	return root, nil
}

// samePath compares after resolving symlinks: git reports a worktree's
// common dir by its real path while the env may name a symlinked one.
func samePath(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func runGHConfig(args []string) error {
	project, args, err := takeProjectFlag(args, false)
	if err != nil {
		return err
	}
	var sets []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--set" && i+1 < len(args):
			sets = append(sets, args[i+1])
			i++
		case strings.HasPrefix(a, "--set="):
			sets = append(sets, strings.TrimPrefix(a, "--set="))
		default:
			return fmt.Errorf("usage: spore gh config [--project <path>] [--set key=value ...]")
		}
	}
	root, err := ghProjectRoot(project)
	if err != nil {
		return err
	}
	for _, kv := range sets {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("--set %q: want key=value", kv)
		}
		path, err := ghapp.SetConfigKey(root, strings.TrimSpace(k), strings.TrimSpace(v))
		if err != nil {
			return err
		}
		fmt.Printf("set %s in %s\n", strings.TrimSpace(k), path)
	}
	if len(sets) > 0 {
		return nil
	}
	path, err := ghapp.ConfigPath(root)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Printf("# %s (absent: pr_flow off)\n", path)
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := ghapp.ParseConfig(string(b)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	fmt.Printf("# %s\n%s", path, b)
	return nil
}

func runGHToken(args []string) error {
	project, args, err := takeProjectFlag(args, false)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		return fmt.Errorf("usage: spore gh token [--project <path>]")
	}
	root, err := ghProjectRoot(project)
	if err != nil {
		return err
	}
	tok, err := ghapp.ProjectToken(context.Background(), root)
	if err != nil {
		return err
	}
	fmt.Println(tok.Token)
	return nil
}

func runGHExec(args []string) (int, error) {
	project, args, err := takeProjectFlag(args, true)
	if err != nil {
		return 0, err
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return 0, fmt.Errorf("usage: spore gh exec [--project <path>] -- <cmd> [args...]")
	}
	if out, err := exec.Command("git", "config", "--get", "remote.origin.url").Output(); err == nil {
		if err := ghapp.CheckHTTPSRemote(strings.TrimSpace(string(out))); err != nil {
			return 0, err
		}
	}
	root, err := ghProjectRoot(project)
	if err != nil {
		return 0, err
	}
	tok, err := ghapp.ProjectToken(context.Background(), root)
	if err != nil {
		return 0, err
	}
	env := ghapp.Env(os.Environ(), tok.Token)
	assignee := ""
	if needsOperatorAssignee(args) {
		cfg, err := ghapp.LoadConfig(root)
		if err != nil {
			return 0, err
		}
		assignee = cfg.OperatorLogin
	}
	var out bytes.Buffer
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if assignee != "" {
		cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	}
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return 0, err
		}
		code = exit.ExitCode()
	}
	if url := createdPRURL(out.String()); url != "" {
		assignOperator(args[0], url, assignee, env)
	}
	return code, nil
}

// assignOperator adds the operator to a PR gh just opened. It runs as a
// separate call, not as --assignee on the create: on github.com gh opens
// the PR before assigning, so a rejected assignee would fail a create
// that already happened (and on GHES fail it before the PR exists).
// Failure here only warns; the create's exit code stands.
func assignOperator(gh, url, login string, env []string) {
	cmd := exec.Command(gh, "pr", "edit", url, "--add-assignee", login)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = io.Discard, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "spore gh: warning: could not assign %s to %s: %v\n", login, url, err)
	}
}

var prURLLine = regexp.MustCompile(`^https?://\S+/pull/\d+$`)

func createdPRURL(stdout string) string {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); prURLLine.MatchString(last) {
		return last
	}
	return ""
}

// needsOperatorAssignee reports whether args run `gh pr create` (or its
// alias `new`) without any assignee flag; the caller's assignee wins.
// Flags after a bare "--" are positional to gh, so scanning stops there.
func needsOperatorAssignee(args []string) bool {
	if len(args) < 3 || filepath.Base(args[0]) != "gh" || args[1] != "pr" ||
		(args[2] != "create" && args[2] != "new") {
		return false
	}
	rest := args[3:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--":
			return true
		case a == "--assignee" || strings.HasPrefix(a, "--assignee="):
			return false
		case strings.HasPrefix(a, "--"):
			if ghPRCreateLongValue[strings.TrimPrefix(a, "--")] {
				i++
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			assignee, takesNext := scanShorthand(a[1:])
			if assignee {
				return false
			}
			if takesNext {
				i++
			}
		}
	}
	return true
}

// ghPRCreateLongValue lists gh pr create's long flags that take the next
// argument as their value (gh v2.91.0 create.go plus the inherited --repo).
var ghPRCreateLongValue = map[string]bool{
	"title": true, "body": true, "body-file": true, "base": true, "head": true,
	"reviewer": true, "label": true, "project": true, "milestone": true,
	"template": true, "repo": true,
}

// scanShorthand reads a pflag shorthand cluster like "da" or "abob". A
// value-taking letter swallows the rest of the cluster ("-ta" is a
// title), or the next argument when it ends the cluster ("-t x").
func scanShorthand(cluster string) (assignee, takesNext bool) {
	for i, c := range cluster {
		if c == 'a' {
			return true, false
		}
		if strings.ContainsRune("tbFBHrlpmTR", c) {
			return false, i == len(cluster)-1
		}
	}
	return false, false
}
