package app

// stackRows splits the body rows of the stacked board. NEEDS YOU takes its
// cards and its head first, but leaves 2 rows to each other section shown.
// A blank row sits between sections and is the first to go when rows are
// short. WORKING and SHIPPING split the rest 3 to 2, WORKING taking the odd
// row; a lone section takes all of it.
func stackRows(need, rows int, work, ship bool) (needH, workH, shipH, gaps int) {
	shown := 0
	if work {
		shown++
	}
	if ship {
		shown++
	}
	if need > 0 {
		needH = min(need+1, rows-2*shown)
	}
	sections := shown
	if needH > 0 {
		sections++
	}
	gaps = max(0, min(min(sections-1, 2), rows-needH-2*shown))
	rest := max(0, rows-needH-gaps)
	switch {
	case work && ship:
		shipH = max(2, rest-(rest*3+4)/5)
		workH = rest - shipH
	case work:
		workH = rest
	case ship:
		shipH = rest
	}
	return needH, workH, shipH, gaps
}
