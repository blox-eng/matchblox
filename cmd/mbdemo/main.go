// mbdemo runs the demo on a terminal. SPIKE.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/demo"
)

func main() {
	c := demo.New()
	defer c.Close()
	if _, err := tea.NewProgram(app.New(demo.Options(c)), tea.WithFPS(15)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
