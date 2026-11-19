package research

import "fmt"

// Reset puts a campaign repo back on its best commit, dropping any
// half-made try, for when a killed night left it dirty. It refuses
// while a night or stop of the campaign runs.
func Reset(s Store, name string) (string, error) {
	h, holder, err := s.lockLoad(name, campaignNight, "")
	if err != nil {
		if holder != "" {
			return "", fmt.Errorf("research: reset %s: a night or stop is running (lock held by %s)", name, holder)
		}
		return "", err
	}
	defer h.release()
	best := h.cfg.BestCommit
	if best == "" {
		best = h.cfg.StartCommit
	}
	return best, s.repo(name).ResetHard(best)
}
