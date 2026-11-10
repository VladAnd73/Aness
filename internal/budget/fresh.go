package budget

import (
	"context"
	"sync"
	"time"
)

var freshMu sync.Mutex

// EnsureFresh refreshes the login in path when its access token expires
// within minLife, writing the new tokens back. The mutex keeps parallel
// spawns from spending one rotating refresh token twice; the loser of
// that race would be logged out.
func EnsureFresh(ctx context.Context, path string, minLife time.Duration) error {
	freshMu.Lock()
	defer freshMu.Unlock()
	cf, err := loadCredentials(path)
	if err != nil {
		return err
	}
	now := time.Now()
	if time.UnixMilli(cf.oauth.ExpiresAt).Sub(now) >= minLife {
		return nil
	}
	return refreshOAuthToken(ctx, cf, now)
}
