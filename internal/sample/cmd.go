package sample

import (
	"context"
	"os/exec"
	"time"
)

// cmdTimeout bounds each command the sampler runs: a wedged daemon must
// not freeze the only sample loop.
var cmdTimeout = 10 * time.Second

// output runs one command with the bound, and does not wait for a child
// that keeps its pipe open.
func output(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	c.WaitDelay = time.Second
	return c.Output()
}
