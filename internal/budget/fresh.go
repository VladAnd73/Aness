package budget

import (
	"fmt"
	"time"
)

// RequireFresh fails when the access token in path expires within
// minLife. It never refreshes: rotating the refresh token would log out
// every running session that holds it, so the login stays Claude Code's
// to renew.
func RequireFresh(path string, minLife time.Duration) error {
	cf, err := loadCredentials(path)
	if err != nil {
		return err
	}
	left := time.Until(time.UnixMilli(cf.oauth.ExpiresAt))
	if left < minLife {
		return fmt.Errorf("claude login expires in %s, need %s; let Claude Code refresh it and retry", left.Round(time.Second), minLife)
	}
	return nil
}
