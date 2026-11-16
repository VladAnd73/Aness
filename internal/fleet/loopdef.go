package fleet

import "errors"

// Compile stub for the configurable-loops acceptance tests; the loop
// definition task fills it in.

type LoopDef struct {
	Name         string
	Steps        []Step
	Summary      bool
	GitHubMirror bool
}

type Step struct {
	Name string
	Kind string
}

var ErrUnknownLoop = errors.New("unknown loop")

func LoadLoop(projectRoot, name string) (LoopDef, error) {
	return LoopDef{}, nil
}
