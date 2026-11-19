package research

// Reset puts a campaign repo back on its best commit, dropping any
// half-made try, for when a killed night left it dirty. It refuses
// while a night or stop of the campaign runs.
func Reset(s Store, name string) (string, error) {
	h, err := s.lockLoad(name, lockOp{what: "reset", scope: campaignNight})
	if err != nil {
		return "", err
	}
	defer h.release()
	best := h.cfg.BestCommit
	if best == "" {
		best = h.cfg.StartCommit
	}
	return best, s.repo(name).ResetHard(best)
}
