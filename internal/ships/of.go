package ships

import "slices"

// Drives says whether the session in pane with session id drives sh: by
// its session id when both have one, so an agent that ran earlier in the
// same pane keeps its own ships, else by its pane.
func Drives(sh Ship, pane, id string) bool {
	if sh.Session.ID != "" && id != "" {
		return sh.Session.ID == id
	}
	return sh.Session.Pane != "" && sh.Session.Pane == pane
}

// Of are the ships the session in pane with session id drives, the one that
// most needs you first (see Rank). Ships of a rank keep the report's order.
// The console's row leads with the first; the service, the CLI and the
// stoker name it.
func Of(all []Ship, pane, id string) []Ship {
	var out []Ship
	for _, sh := range all {
		if Drives(sh, pane, id) {
			out = append(out, sh)
		}
	}
	slices.SortStableFunc(out, func(a, b Ship) int { return Rank(a) - Rank(b) })
	return out
}

// Rank is where a ship stands among a session's: stuck, waiting for you,
// in flight, merged, then live. A merged ship still deploying runs, so the
// stage tells it from one in flight.
func Rank(sh Ship) int {
	switch {
	case sh.Kind == Stuck:
		return 0
	case sh.Kind == WaitsForYou:
		return 1
	case sh.Stage < StageMerged:
		return 2
	case sh.Stage < StageLive:
		return 3
	}
	return 4
}
