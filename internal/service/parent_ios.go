package service

// parentOf finds no parent on iOS: the app cannot read the process table, and
// the demo's panes come from its own world, not from tmux.
func parentOf(int) (int, bool) { return 0, false }
