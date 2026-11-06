package service

import (
	"math"
	"testing"
)

func TestTTYFromNrRejectsOutOfRange(t *testing.T) {
	for _, nr := range []int{0, -1, math.MinInt32} {
		if _, ok := ttyFromNr(nr); ok {
			t.Errorf("ttyFromNr(%d) ok; want no terminal", nr)
		}
	}
	if math.MaxInt == math.MaxInt64 {
		if _, ok := ttyFromNr(int(math.MaxUint32) + 1); ok {
			t.Error("ttyFromNr(MaxUint32+1) ok; want no terminal")
		}
	}
	if got, ok := ttyFromNr(34817); !ok || got != ttyDev(34817) {
		t.Errorf("ttyFromNr(34817) = %+v, %v", got, ok)
	}
}
