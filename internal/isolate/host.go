package isolate

import (
	"fmt"
	"os"
)

// ForceUnsupportedEnv set to 1 makes HostSupported fail, to rehearse a
// non-NixOS host (such as the CI runner) from a NixOS one. It can only
// refuse the sandbox, never allow it.
const ForceUnsupportedEnv = "SPORE_SANDBOX_FORCE_UNSUPPORTED"

var nixosMarker = "/run/current-system"

// HostSupported reports whether the sandbox Args builds can run an
// agent here. It binds only NixOS's system paths, so on another
// distribution bwrap starts but finds no /usr/lib to load binaries
// with, and no host temp dir an agent outside /nix may live in.
func HostSupported() error {
	if os.Getenv(ForceUnsupportedEnv) == "1" {
		return fmt.Errorf("isolate: the bwrap sandbox is off because %s=1", ForceUnsupportedEnv)
	}
	if _, err := os.Stat(nixosMarker); err != nil {
		return fmt.Errorf("isolate: the bwrap sandbox is NixOS-only for now and %s is missing: it binds /nix, /etc and %s, not /usr/lib", nixosMarker, nixosMarker)
	}
	return Available()
}
