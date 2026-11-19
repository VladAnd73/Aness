package research

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Lock takes the host-wide night lock at path without blocking and
// writes holder into it. When another process holds it, heldBy is the
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

func DefaultLockPath() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "spore-research.lock")
	}
	return filepath.Join(os.TempDir(), "spore-research.lock")
}
