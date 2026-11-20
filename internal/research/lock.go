package research

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

// nightScope is the set of night locks a writer takes.
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

// lockOp is one writer's request to lockLoad. what names the writer to
// whoever finds its lock held ("night", "stop", "set"). check, when set,
// refuses on the loaded config before the host-wide lock is taken, so a
// campaign that may not run never holds it, not even briefly.
type lockOp struct {
	what  string
	scope nightScope
	host  string
	check func(Config) error
}

// BusyError is a night lock held by another writer. For an op that
// spends quota, lockLoad returns it as a RefusedError instead.
type BusyError struct{ Campaign, Holder string }

func (e *BusyError) Reason() string { return "the " + e.Holder + " holds the lock" }

func (e *BusyError) Error() string {
	return fmt.Sprintf("research: campaign %s: %s; retry after it ends", e.Campaign, e.Reason())
}

// held is a campaign config loaded after its locks were taken.
type held struct {
	cfg          Config
	unlockConfig func()
	unlockNight  []func()
}

// releaseConfig lets off and the night's own saves through while the
// night locks stay held; later saves go through lockLoad with noNight.
func (h *held) releaseConfig() {
	if h.unlockConfig != nil {
		h.unlockConfig()
		h.unlockConfig = nil
	}
}

func (h *held) release() {
	h.releaseConfig()
	for i := len(h.unlockNight) - 1; i >= 0; i-- {
		h.unlockNight[i]()
	}
	h.unlockNight = nil
}

// lockLoad is the one way to read campaign.toml for a write. It takes
// the campaign's night lock (unless noNight), then the config lock, and
// only then loads; then op.check; then, for hostNight, the host-wide
// lock. Every lock is non-blocking except the config lock, which is
// always taken last among the blocking ones, so the order cannot
// deadlock.
func (s Store) lockLoad(name string, op lockOp) (_ *held, err error) {
	h := &held{}
	defer func() {
		if err != nil {
			h.release()
		}
	}()
	// A lock file cannot be created in a campaign folder that does not
	// exist; say what is missing instead.
	notFound := func(err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			if _, lerr := s.Load(name); lerr != nil {
				return lerr
			}
		}
		return err
	}
	take := func(path string) error {
		unlock, holder, err := Lock(path, op.what+" of "+name)
		if err != nil {
			if holder == "" {
				return notFound(err)
			}
			busy := &BusyError{Campaign: name, Holder: holder}
			if op.scope == hostNight {
				return &RefusedError{Reason: busy.Reason()}
			}
			return busy
		}
		h.unlockNight = append(h.unlockNight, unlock)
		return nil
	}
	if op.scope != noNight {
		beforeNightLock(name)
		if err := take(s.nightLockPath(name)); err != nil {
			return nil, err
		}
	}
	if h.unlockConfig, err = s.lockConfig(name); err != nil {
		return nil, notFound(err)
	}
	if h.cfg, err = s.Load(name); err != nil {
		return nil, err
	}
	if op.check != nil {
		if err := op.check(h.cfg); err != nil {
			return nil, err
		}
	}
	if op.scope == hostNight {
		if err := take(op.host); err != nil {
			return nil, err
		}
	}
	return h, nil
}

func DefaultLockPath() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "spore-research.lock")
	}
	return filepath.Join(os.TempDir(), "spore-research.lock")
}
