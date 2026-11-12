package app

import (
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func sessionOn(t *testing.T, st proto.State, pane string) sample.Session {
	t.Helper()
	for _, s := range st.Sessions {
		if s.Pane == pane {
			return s
		}
	}
	t.Fatalf("no session on %s", pane)
	return sample.Session{}
}
