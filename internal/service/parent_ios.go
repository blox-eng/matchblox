package service

// parentOf finds no parent on iOS: the app cannot read the process table, and
// the demo's panes come from its own world, not from tmux.
func parentOf(int) (int, bool) { return 0, false }

func ttyOf(int) (major, minor uint32, ok bool) { return 0, 0, false }

func devOf(string) (major, minor uint32, ok bool) { return 0, 0, false }
