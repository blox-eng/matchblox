package service

import (
	"testing"

	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/proto"
)

const markedScreen = `Which colour for the front door?
  1. Red
❯ 2. Blue
  3. Green
`

func menuAct(pane string) proto.Act {
	return proto.Act{RecID: proto.MenuPrefix + pane, Console: true}
}

// TestMenuActReadsLiveScreen: a menu: act answers the live screen's menu,
// the option it marks and its sign, and types nothing.
func TestMenuActReadsLiveScreen(t *testing.T) {
	for _, tc := range []struct {
		name, screen string
		read         bool
	}{{"menu", markedScreen, true}, {"no menu", "● Done.\n\n❯ \n", true}, {"unreadable", "", false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, ran := chooseTest(t, tc.screen)
			s.ScreenOf = func(pane string) (string, bool, bool) { return tc.screen, false, tc.read && pane == "%3" }
			res := s.act(t.Context(), menuAct("%3"))
			if len(*ran) != 0 || len(res.Ran) != 0 {
				t.Fatalf("ran %q", *ran)
			}
			if !tc.read {
				if res.Err == "" || res.Menu != nil {
					t.Fatalf("%+v, want an error", res)
				}
				return
			}
			if res.Err != "" || res.Menu == nil {
				t.Fatalf("%+v", res)
			}
			want := proto.Menu{Chosen: menu.Chosen(tc.screen), Sign: menu.Sign(tc.screen)}
			for n, text := range menu.Read(tc.screen) {
				if res.Menu.Options[n-1] != (proto.Option{N: n, Text: text}) {
					t.Errorf("option %d: %+v, want %q", n, res.Menu.Options[n-1], text)
				}
			}
			if len(res.Menu.Options) != len(menu.Read(tc.screen)) || res.Menu.Chosen != want.Chosen || res.Menu.Sign != want.Sign {
				t.Fatalf("%+v, want %+v", *res.Menu, want)
			}
			if tc.screen == markedScreen && (len(res.Menu.Options) != 3 || res.Menu.Chosen != 2 || res.Menu.Sign == "") {
				t.Fatalf("%+v, want 3 options, 2 chosen, a sign", *res.Menu)
			}
		})
	}
}
