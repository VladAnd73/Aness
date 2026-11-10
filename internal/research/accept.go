package research

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/versality/spore/internal/statefile"
)

type AcceptResult struct{ Written, Backup, Branch string }

const sporeModule = "module github.com/versality/spore"

// Accept lands a stopped campaign's winner. Outside the spore repo it
// overwrites the live target, keeping a backup beside it. Inside the
// spore repo it commits the winner on a new research/<campaign> branch
// through a temporary worktree, so the operator's checkout never moves.
func Accept(s Store, name string, force bool) (AcceptResult, error) {
	cfg, err := s.Load(name)
	if err != nil {
		return AcceptResult{}, err
	}
	switch {
	case cfg.Status == StatusAccepted:
		return AcceptResult{}, fmt.Errorf("research: campaign %s is already accepted", name)
	case cfg.Status != StatusStopped:
		return AcceptResult{}, fmt.Errorf("research: campaign %s is %s, not stopped; run stop first", name, cfg.Status)
	case !cfg.hasKeptChange():
		return AcceptResult{}, fmt.Errorf("research: campaign %s has no kept change to accept", name)
	case cfg.HoldoutVerdict == "overfit" && !force:
		return AcceptResult{}, fmt.Errorf("research: campaign %s: the holdout said overfit; pass --force to accept anyway", name)
	}
	top := sporeCheckout(filepath.Dir(cfg.Target))
	if top == "" {
		if _, err := os.Lstat(cfg.backupPath()); err == nil {
			return AcceptResult{}, fmt.Errorf("research: backup %s already exists; move it away first", cfg.backupPath())
		}
	}
	r := s.repo(name)
	if changed, err := firstLiveChange(r, cfg); err != nil {
		return AcceptResult{}, err
	} else if changed != "" {
		return AcceptResult{}, fmt.Errorf("research: the live skill folder changed since the campaign started: %s", changed)
	}
	winner, err := r.show(cfg.BestCommit, cfg.targetRel())
	if err != nil {
		return AcceptResult{}, err
	}
	var res AcceptResult
	if top != "" {
		res, err = acceptOnBranch(s, cfg, top, winner)
	} else {
		res, err = acceptInPlace(cfg, winner)
	}
	if err != nil {
		return res, err
	}
	cfg.Status = StatusAccepted
	if err := s.Save(cfg); err != nil {
		return res, err
	}
	return res, pruneTries(s.Dir(name))
}

// firstLiveChange compares the live folder with the start commit, which
// holds an exact copy of it, and names the first path that differs.
func firstLiveChange(r repo, cfg Config) (string, error) {
	liveDir := filepath.Dir(cfg.Target)
	if h, err := HashTree(liveDir); err == nil && h == cfg.LiveFolderHash {
		return "", nil
	}
	out, err := r.gitRaw("ls-tree", "-r", "-z", cfg.StartCommit)
	if err != nil {
		return "", err
	}
	start := map[string]string{}
	for _, entry := range strings.Split(string(out), "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		if ok {
			start[path] = strings.Fields(meta)[0]
		}
	}
	var diffs []string
	seen := map[string]bool{}
	err = filepath.WalkDir(liveDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(liveDir, path)
		if d.IsDir() {
			return nil
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		mode, ok := start[rel]
		if !ok || !d.Type().IsRegular() {
			diffs = append(diffs, rel)
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		live, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		orig, err := r.show(cfg.StartCommit, rel)
		if err != nil {
			return err
		}
		if !bytes.Equal(live, orig) || (fi.Mode().Perm()&0o111 != 0) != (mode == "100755") {
			diffs = append(diffs, rel)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return filepath.Base(liveDir) + " (folder missing)", nil
	}
	if err != nil {
		return "", err
	}
	for p := range start {
		if !seen[p] {
			diffs = append(diffs, p+" (removed)")
		}
	}
	if len(diffs) == 0 {
		return "(folder hash changed)", nil
	}
	slices.Sort(diffs)
	return diffs[0], nil
}

// sporeCheckout returns the toplevel of the git checkout holding dir
// when its go.mod declares the spore module, else "".
func sporeCheckout(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	top := strings.TrimSpace(string(out))
	mod, err := os.ReadFile(filepath.Join(top, "go.mod"))
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(mod), "\n")
	if strings.TrimSpace(first) != sporeModule {
		return ""
	}
	return top
}

func (cfg Config) backupPath() string { return cfg.Target + ".research-" + cfg.Name + ".bak" }

// acceptInPlace creates the backup with O_EXCL, so a backup that
// appeared since the check above is never overwritten.
func acceptInPlace(cfg Config, winner []byte) (AcceptResult, error) {
	backup := cfg.backupPath()
	fi, err := os.Stat(cfg.Target)
	if err != nil {
		return AcceptResult{}, err
	}
	orig, err := os.ReadFile(cfg.Target)
	if err != nil {
		return AcceptResult{}, err
	}
	f, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return AcceptResult{}, fmt.Errorf("research: backup: %w", err)
	}
	if _, err := f.Write(orig); err != nil {
		f.Close()
		return AcceptResult{}, fmt.Errorf("research: backup: %w", err)
	}
	if err := f.Close(); err != nil {
		return AcceptResult{}, fmt.Errorf("research: backup: %w", err)
	}
	if err := statefile.WriteFileAtomic(cfg.Target, ".research-accept-*", winner); err != nil {
		return AcceptResult{}, err
	}
	if err := os.Chmod(cfg.Target, fi.Mode().Perm()); err != nil {
		return AcceptResult{}, err
	}
	return AcceptResult{Written: cfg.Target, Backup: backup}, nil
}

func acceptOnBranch(s Store, cfg Config, top string, winner []byte) (AcceptResult, error) {
	rel, err := filepath.Rel(top, cfg.Target)
	if err != nil {
		return AcceptResult{}, err
	}
	branch := "research/" + cfg.Name
	git := func(dir string, args ...string) error {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("research: accept: git %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	wt := filepath.Join(s.Dir(cfg.Name), "accept-worktree")
	os.RemoveAll(wt)
	if err := git(top, "worktree", "add", "--quiet", "-b", branch, wt, "HEAD"); err != nil {
		return AcceptResult{}, err
	}
	defer git(top, "worktree", "remove", "--force", wt)
	if err := os.WriteFile(filepath.Join(wt, rel), winner, 0o644); err != nil {
		return AcceptResult{}, err
	}
	if err := git(wt, "add", "--", rel); err != nil {
		return AcceptResult{}, err
	}
	if err := git(wt, "commit", "--quiet", "-m", fmt.Sprintf("research: %s winner for %s", cfg.Name, filepath.ToSlash(rel))); err != nil {
		return AcceptResult{}, err
	}
	return AcceptResult{Written: filepath.Join(wt, rel), Branch: branch}, nil
}

// pruneTries drops editor scratch folders and transcripts, keeping the
// judge result files.
func pruneTries(dir string) error {
	tries, err := filepath.Glob(filepath.Join(dir, "nights", "*", "try-*"))
	if err != nil {
		return err
	}
	for _, t := range tries {
		entries, err := os.ReadDir(t)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if err := os.RemoveAll(filepath.Join(t, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
