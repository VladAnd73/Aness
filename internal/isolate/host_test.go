package isolate

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHostSupportedRefusesWithoutNixOS(t *testing.T) {
	t.Setenv(ForceUnsupportedEnv, "")
	missing := filepath.Join(t.TempDir(), "current-system")
	old := nixosMarker
	nixosMarker = missing
	t.Cleanup(func() { nixosMarker = old })
	err := HostSupported()
	if err == nil || !strings.Contains(err.Error(), "NixOS") || !strings.Contains(err.Error(), missing) {
		t.Errorf("HostSupported without %s = %v; want an error naming NixOS and the missing path", missing, err)
	}
}

func TestHostSupportedForcedOffNamesTheOverride(t *testing.T) {
	t.Setenv(ForceUnsupportedEnv, "1")
	err := HostSupported()
	if err == nil || !strings.Contains(err.Error(), ForceUnsupportedEnv) {
		t.Errorf("HostSupported with %s=1 = %v; want an error naming the override", ForceUnsupportedEnv, err)
	}
}
