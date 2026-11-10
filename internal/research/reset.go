package research

import "fmt"

// Reset puts a campaign repo back on its best commit, dropping any
// half-made try, for when a killed night left it dirty. It refuses
// while any night holds the host lock.
func Reset(s Store, name, lockPath string) (string, error) {
	cfg, err := s.Load(name)
	if err != nil {
		return "", err
	}
	unlock, holder, err := Lock(lockPath, name)
	if err != nil {
		if holder != "" {
			return "", fmt.Errorf("research: reset %s: a night is running (lock held by %s)", name, holder)
		}
		return "", err
	}
	defer unlock()
	best := cfg.BestCommit
	if best == "" {
		best = cfg.StartCommit
	}
	return best, s.repo(name).ResetHard(best)
}
