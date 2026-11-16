// Package editor opens a file location in the operator's connected
// editor. Backends sit behind one interface so a Neovim backend can
// join the VS Code one later.
package editor

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNoEditor means no backend could reach a connected editor. Callers
// report it and exit 0: a missing editor never fails an agent turn.
var ErrNoEditor = errors.New("no editor connected")

type Location struct {
	Path string
	Line int
	Col  int
}

func (l Location) String() string {
	s := l.Path
	if l.Line > 0 {
		s += ":" + strconv.Itoa(l.Line)
		if l.Col > 0 {
			s += ":" + strconv.Itoa(l.Col)
		}
	}
	return s
}

type Backend interface {
	Name() string
	Open(Location) error
}

// ParseLocation splits <path>[:<line>[:<col>]] from the right, so a
// path that itself holds a colon still parses when no line follows a
// non-numeric segment. Relative paths resolve against cwd.
func ParseLocation(arg, cwd string) (Location, error) {
	if arg == "" {
		return Location{}, errors.New("empty location")
	}
	path, nums := arg, []int{}
	for len(nums) < 2 {
		i := strings.LastIndex(path, ":")
		if i < 0 {
			break
		}
		n, err := strconv.Atoi(path[i+1:])
		if err != nil || n < 1 {
			break
		}
		nums = append([]int{n}, nums...)
		path = path[:i]
	}
	if path == "" {
		return Location{}, fmt.Errorf("no path in %q", arg)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	loc := Location{Path: filepath.Clean(path)}
	if len(nums) > 0 {
		loc.Line = nums[0]
	}
	if len(nums) > 1 {
		loc.Col = nums[1]
	}
	return loc, nil
}
