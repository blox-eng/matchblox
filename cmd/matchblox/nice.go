package main

// lowersPriority tells if a command runs at the lowest CPU priority, so
// matchblox adds no load. The console does not: a person types into it,
// and at the lowest priority a busy host showed each key half a second
// late. The service it starts lowers its own.
func lowersPriority(cmd string) bool { return cmd != "console" }
