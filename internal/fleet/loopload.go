package fleet

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/versality/spore"
)

// LoadLoop returns the loop called name: the project's
// bootstrap/loops/<name>.toml when present, else the bundled one.
// "role" is the pre-loop-file spelling of "dev".
func LoadLoop(projectRoot, name string) (LoopDef, error) {
	if name == "role" {
		name = "dev"
	}
	if !isLoopName(name) {
		return LoopDef{}, fmt.Errorf("%w %q", ErrUnknownLoop, name)
	}
	file := name + ".toml"
	path, err := projectLoopFile(projectRoot, file)
	if err != nil {
		return LoopDef{}, err
	}
	source := path
	var b []byte
	if path != "" {
		b, err = os.ReadFile(path)
	} else {
		source = "embedded:" + name
		b, err = fs.ReadFile(spore.BundledLoops, "bootstrap/loops/"+file)
		if errors.Is(err, fs.ErrNotExist) {
			return LoopDef{}, fmt.Errorf("%w %q: not in %s and not bundled", ErrUnknownLoop, name,
				filepath.Join(projectRoot, "bootstrap", "loops"))
		}
	}
	if err != nil {
		return LoopDef{}, fmt.Errorf("loop %s: %w", source, err)
	}
	def, err := ParseLoopDef(source, b)
	if err != nil {
		return LoopDef{}, err
	}
	if def.Name == "" {
		def.Name = name
	}
	if def.Name != name {
		return LoopDef{}, fmt.Errorf("%s: loop name %q does not match the file name %q", source, def.Name, name)
	}
	// Checked again at spawn: the mode can follow reviewer config that
	// changes after load.
	for _, s := range def.Steps {
		if s.Kind != "agent" {
			continue
		}
		if err := checkRoleBody(projectRoot, roleBodyFile(s.Role, s.Mode)); err != nil {
			return LoopDef{}, fmt.Errorf("%s: step %q: %w", source, s.Name, err)
		}
	}
	return def, nil
}

func isLoopName(name string) bool {
	return isBareTOMLKey(name)
}

// projectLoopFile applies projectRoleBody's override rules to
// bootstrap/loops/<file>: an absent component (or a non-directory
// parent, such as a ./bootstrap script) means no override, while a
// broken one is an error so a customized loop is never silently
// replaced by the bundled one.
func projectLoopFile(projectRoot, file string) (string, error) {
	p := projectRoot
	parts := []string{"bootstrap", "loops", file}
	for i, part := range parts {
		p = filepath.Join(p, part)
		_, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("loop override %s: %w", p, err)
		}
		fi, err := os.Stat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("loop override %s is a dangling symlink", p)
		case err != nil:
			return "", fmt.Errorf("loop override %s: %w", p, err)
		case i < len(parts)-1 && !fi.IsDir():
			return "", nil
		case i == len(parts)-1 && !fi.Mode().IsRegular():
			return "", fmt.Errorf("loop override %s: not a regular file", p)
		}
	}
	return p, nil
}
