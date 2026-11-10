package research

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func appendReport(s Store, name, section string) error {
	f, err := os.OpenFile(filepath.Join(s.Dir(name), "report.md"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(section); err != nil {
		return err
	}
	return f.Close()
}

func finalReport(s Store, cfg Config, h holdoutResult) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Final\n\n- stop reason: %s\n- nights used: %d\n- start score: %.1f\n- best score: %.1f\n",
		cfg.StopReason, cfg.Nights, cfg.StartScore, cfg.BestScore)
	if cfg.hasKeptChange() {
		fmt.Fprintf(&b, "- holdout: %s (original %.1f, winner %.1f)\n", cfg.HoldoutVerdict, h.original, h.winner)
	} else {
		b.WriteString("- holdout: no change\n")
	}
	delta := 0.0
	if cfg.StartBytes > 0 {
		delta = float64(cfg.BestBytes-cfg.StartBytes) * 100 / float64(cfg.StartBytes)
	}
	fmt.Fprintf(&b, "- length: %d -> %d bytes (%+.1f%%)\n", cfg.StartBytes, cfg.BestBytes, delta)
	rows, err := ReadRows(filepath.Join(s.Dir(cfg.Name), "results.tsv"))
	if err != nil {
		return "", err
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Status]++
	}
	fmt.Fprintf(&b, "- tries: kept %d, reset %d, rejected-length %d, crash %d\n",
		counts["kept"], counts["reset"], counts["rejected-length"], counts["crash"])
	fmt.Fprintf(&b, "- quota used: %.1f points\n- judge cost: $%.2f\n\n", cfg.QuotaUsed, cfg.JudgeCostUSD)
	if cfg.HoldoutVerdict == "overfit" {
		b.WriteString("Recommendation: do not accept (overfit on holdout).\n\n")
	}
	if cfg.hasKeptChange() {
		diff, err := s.repo(cfg.Name).git("diff", cfg.StartCommit, cfg.BestCommit, "--", cfg.targetRel())
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "```diff\n%s\n```\n\n", diff)
	}
	return b.String(), nil
}
