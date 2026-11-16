package fleet

// Compile stub for the configurable-loops acceptance tests; the loop
// driver task fills it in.

type LoopState struct {
	Slug     string
	Loop     string
	Step     string
	Phase    string
	Terminal string
	Round    int
}

func DriveLoop(projectRoot, tasksDir, slug string) (LoopState, error) {
	return LoopState{}, nil
}

func DeriveLoopSnapshot(projectRoot, slug string, def LoopDef) (Snapshot, error) {
	return Snapshot{}, nil
}
