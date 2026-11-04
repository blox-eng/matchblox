package demo

import "github.com/blox-eng/matchblox/internal/app"

// WithConsole lets a test change the console's options before it starts.
func WithConsole(o Options, change func(*app.Options)) Options {
	o.console = change
	return o
}

// Ran is every argv the service gave the world.
func (d *Demo) Ran() [][]string { return d.script.world.Ran() }
