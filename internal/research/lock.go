package research

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Lock takes the night lock at path without blocking and writes holder
// into it. When another process holds it, heldBy is the
// name that process wrote.
func Lock(path, holder string) (unlock func(), heldBy string, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("research: lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		b, _ := io.ReadAll(f)
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			heldBy = strings.TrimSpace(string(b))
			if heldBy == "" {
				heldBy = "(unknown)"
			}
			return nil, heldBy, fmt.Errorf("research: lock %s held by %s", path, heldBy)
		}
		return nil, "", fmt.Errorf("research: lock: %w", err)
	}
	if err := f.Truncate(0); err == nil {
		f.WriteAt([]byte(holder+"\n"), 0)
	}
	return func() {
		f.Truncate(0)
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, "", nil
}

// beforeNightLock runs just before a writer takes its night lock; tests
// use it to force a competing write into that window.
var beforeNightLock = func(name string) {}

func (s Store) nightLockPath(name string) string { return filepath.Join(s.Dir(name), "night.lock") }

// nightScope is the set of night locks a writer takes before the config
// lock.
type nightScope int

const (
	// noNight is for a caller that already holds its night locks, and for
	// off, which must reach a running night.
	noNight nightScope = iota
	// campaignNight excludes a night or stop of the same campaign.
	campaignNight
	// hostNight adds the host-wide lock: every night and holdout spends
	// the one shared weekly quota.
	hostNight
)

// held is a campaign config loaded after its locks were taken.
type held struct {
	cfg                       Config
	unlockConfig, unlockNight func()
}

// releaseConfig lets off and the night's own saves through while the
// night locks stay held; later saves go through lockLoad(noNight).
func (h *held) releaseConfig() {
	if h.unlockConfig != nil {
		h.unlockConfig()
		h.unlockConfig = nil
	}
}

func (h *held) release() {
	h.releaseConfig()
	h.unlockNight()
}

// lockLoad is the one way to read campaign.toml for a write: it takes
// the night locks scope names (host-wide first, then the campaign's),
// then the config lock, and only then loads. A night lock held by
// someone else returns that holder's name with the error.
func (s Store) lockLoad(name string, scope nightScope, hostPath string) (_ *held, holder string, err error) {
	var unlocks []func()
	unlockAll := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	defer func() {
		if err != nil {
			unlockAll()
		}
	}()
	if scope != noNight {
		beforeNightLock(name)
		paths := []string{s.nightLockPath(name)}
		if scope == hostNight {
			paths = []string{hostPath, s.nightLockPath(name)}
		}
		for _, p := range paths {
			unlock, holder, err := Lock(p, name)
			if err != nil {
				return nil, holder, err
			}
			unlocks = append(unlocks, unlock)
		}
	}
	night := slices.Clone(unlocks)
	unlockCfg, err := s.lockConfig(name)
	if err != nil {
		return nil, "", err
	}
	unlocks = append(unlocks, unlockCfg)
	cfg, err := s.Load(name)
	if err != nil {
		return nil, "", err
	}
	return &held{cfg: cfg, unlockConfig: unlockCfg, unlockNight: func() {
		for i := len(night) - 1; i >= 0; i-- {
			night[i]()
		}
	}}, "", nil
}

func DefaultLockPath() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "spore-research.lock")
	}
	return filepath.Join(os.TempDir(), "spore-research.lock")
}
