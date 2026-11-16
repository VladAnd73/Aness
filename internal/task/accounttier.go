package task

import "github.com/versality/spore/internal/sporetoml"

// LoadAccountTier reads `[fleet] account_tier = "<tier>"` from
// <projectRoot>/spore.toml. The spawn path injects the value as
// SPORE_ACCOUNT_TIER into worker sessions so `spore worker
// token-monitor` resolves the intended per-tier caps. Returns "" when
// the file or key is missing or the section is malformed (workers then
// default to the sub-max caps). Lives here rather than internal/fleet
// because ensureSession is the injection point and fleet imports task.
func LoadAccountTier(projectRoot string) string {
	entries, err := sporetoml.ReadSection(projectRoot, "fleet")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.Key == "account_tier" {
			return e.Value
		}
	}
	return ""
}
