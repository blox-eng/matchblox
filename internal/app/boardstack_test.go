package app

import "testing"

func TestStackRowsSplitsTheRestThreeToTwo(t *testing.T) {
	tests := []struct {
		name                          string
		need, rows                    int
		work, ship                    bool
		needH, workH, shipH, wantGaps int
	}{
		{"rest splits three to two", 9, 21, true, true, 10, 6, 3, 2},
		{"needs you that does not fit leaves two rows each", 30, 21, true, true, 17, 2, 2, 0},
		{"one shown section takes the rest", 9, 21, true, false, 10, 10, 0, 1},
		{"only shipping shown takes the rest", 9, 21, false, true, 10, 0, 10, 1},
		{"hidden needs you gives its rows away", 0, 21, true, true, 0, 12, 8, 1},
		{"neither section shown leaves needs you the rows", 30, 21, false, false, 21, 0, 0, 0},
		{"one shown section keeps two rows when needs you is long", 30, 21, true, false, 19, 2, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needH, workH, shipH, gaps := stackRows(tt.need, tt.rows, tt.work, tt.ship)
			if needH != tt.needH || workH != tt.workH || shipH != tt.shipH || gaps != tt.wantGaps {
				t.Fatalf("stackRows(%d, %d, %v, %v) = %d, %d, %d, %d; want %d, %d, %d, %d",
					tt.need, tt.rows, tt.work, tt.ship, needH, workH, shipH, gaps,
					tt.needH, tt.workH, tt.shipH, tt.wantGaps)
			}
		})
	}
}

func TestStackRowsIgnoresHowManyMatchesTheSectionsHold(t *testing.T) {
	aN, aW, aS, aG := stackRows(9, 21, true, true)
	bN, bW, bS, bG := stackRows(9, 21, true, true)
	if aN != bN || aW != bW || aS != bS || aG != bG {
		t.Fatal("the split changed between two calls with the same inputs")
	}
}
