package research

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TellCoordinator returns a Notify that runs `spore task tell
// coordinator <msg>` from projectRoot: tell picks its coordinator by
// the working directory, not by any flag.
func TellCoordinator(sporeBin, projectRoot string) func(msg string) error {
	return func(msg string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, sporeBin, "task", "tell", "coordinator", msg)
		cmd.Dir = projectRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("research: tell coordinator: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}
