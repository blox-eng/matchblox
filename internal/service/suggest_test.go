package service

import "testing"

// TestASuggestionIsNoDraft: #82, the guard of a compact, a send and a wake
// reads the pane with its styles, so Claude Code's grey suggestion does not
// stop them, and a line a person typed still does.
func TestASuggestionIsNoDraft(t *testing.T) {
	s, _ := orderTest(t)
	frame := "\x1b[2m────────────────────\x1b[22m\n"
	suggests := "⏺ Done.\n\n" + frame + "\x1b[39m❯\u00a0\x1b[38;5;244madd the ci label to 4611\x1b[39m\n" + frame
	typed := "⏺ Done.\n\n" + frame + "\x1b[39m❯\u00a0add the ci label to 4611\x1b[0m\n" + frame
	for screen, ok := range map[string]bool{suggests: true, typed: false} {
		s.Capture = func(panes []string) map[string]string { return map[string]string{"%1": screen} }
		if err := s.stokable("%1", false); (err == nil) != ok {
			t.Fatalf("%q: stokable %v", screen, err)
		}
	}
}
