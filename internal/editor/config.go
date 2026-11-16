package editor

import (
	"fmt"

	"github.com/versality/spore/internal/sporetoml"
)

// AutoConnectIDEEnv makes a Claude Code session connect to the IDE
// window whose workspace folders contain its cwd. Only the coordinator
// may carry it: one Claude Code instance can hold a VS Code window at a
// time, and N bypass-mode workers would fight over it.
const AutoConnectIDEEnv = "CLAUDE_CODE_AUTO_CONNECT_IDE"

// Config captures the [editor] section of a project's spore.toml.
type Config struct {
	AutoConnectIDE bool
}

// LoadConfig reads `[editor]` from <projectRoot>/spore.toml. A missing
// file or section returns the zero Config (everything off).
func LoadConfig(projectRoot string) (Config, error) {
	entries, err := sporetoml.ReadSection(projectRoot, "editor")
	if err != nil {
		return Config{}, fmt.Errorf("editor: %w", err)
	}
	var cfg Config
	for _, e := range entries {
		switch e.Key {
		case "auto_connect_ide":
			v, err := e.Bool()
			if err != nil {
				return Config{}, fmt.Errorf("editor: %w", err)
			}
			cfg.AutoConnectIDE = v
		default:
			return Config{}, fmt.Errorf("editor: line %d: unknown key %q in [editor]", e.Line, e.Key)
		}
	}
	return cfg, nil
}
