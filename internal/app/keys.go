package app

import bkey "charm.land/bubbles/v2/key"

// The console's keys, as data: one keymap per layer, one bkey.Binding per
// act. A binding's help key and text are what the footer shows today.

// bind is a binding of one act: its keys, and the help key and label.
func bind(keys []string, helpKey, desc string) bkey.Binding {
	return bkey.NewBinding(bkey.WithKeys(keys...), bkey.WithHelp(helpKey, desc))
}

func ks(k ...string) []string { return k }

// digits1to9 are the keys 1 to 9: a tab of the console, or an option of a
// question.
var digits1to9 = ks("1", "2", "3", "4", "5", "6", "7", "8", "9")

type keymaps struct {
	global   globalKeys
	main     mainKeys
	form     formKeys
	peek     peekKeys
	sheet    sheetKeys
	search   searchKeys
	pair     pairKeys
	takeOver takeOverKeys
}

type shellKeymaps struct {
	shell  shellKeys
	typing shellTypingKeys
}

// shadowsMain are the keys a modal layer shares with mainKeys on purpose:
// the layer is on top, so its key wins. A new overlap fails
// TestKeymapsHaveNoCollisions until it is added here.
var shadowsMain = map[string]bool{
	"esc": true, "enter": true, "tab": true, "shift+tab": true,
	"up": true, "down": true, "space": true, " ": true,
	"c": true, "y": true, "n": true,
	"1": true, "2": true, "3": true, "4": true, "5": true, "6": true, "7": true,
}

func defaultKeys() keymaps {
	return keymaps{
		global: globalKeys{Quit: bind(ks("ctrl+c"), "ctrl+c", "quit")},
		main: mainKeys{
			Stoker:    bind(ks("c"), "c", "stoker"),
			Quit:      bind(ks("q"), "q", "quit"),
			NextTab:   bind(ks("tab"), "tab", "next"),
			PrevTab:   bind(ks("shift+tab"), "shift+tab", "back"),
			Tabs:      bind(digits1to9[:len(tabNames)], "1-9", "tab"),
			Search:    bind(ks("/"), "/", "find"),
			Back:      bind(ks("esc"), "esc", "clear"),
			Sort:      bind(ks("s"), "s", "sort"),
			SortBack:  bind(ks("S", "shift+s"), "S", "sort back"),
			Group:     bind(ks("g"), "g", "group"),
			Park:      bind(ks("z"), "z", "park"),
			Mark:      bind(ks("space", " "), "␣", "mark"),
			Exited:    bind(ks("e"), "e", "exited"),
			MarkSafe:  bind(ks("X", "shift+x"), "X", "all safe"),
			QuitAgent: bind(ks("Q", "shift+q"), "Q", "quit it"),
			Up:        bind(ks("up", "k"), "↑↓", "move"),
			Down:      bind(ks("down", "j"), "↑↓", "move"),
			Open:      bind(ks("enter"), "⏎", "open"),
			Second:    bind(ks("x"), "x", "close"),
			New:       bind(ks("N", "shift+n"), "N", "new"),
			Switch:    bind(ks("A", "shift+a"), "A", "switch"),
			Grant:     bind(ks("y"), "y", "give the go"),
			Peek:      bind(ks("a"), "a", "look"),
			Dismiss:   bind(ks("d"), "d", "dismiss"),
			Fold:      bind(ks("h"), "h", "fold"),
			Ships:     bind(ks("p"), "p", "ships"),
			Dock:      bind(ks("u"), "u", "dock"),
			Stoke:     bind(ks("f"), "f", "stoker"),
			Night:     bind(ks("n"), "n", "stop night"),
			Order:     bind(ks("o"), "o", "order"),
			Rename:    bind(ks("r"), "r", "rename"),
		},
		form: formKeys{
			Close: bind(ks("esc"), "esc", ""),
			Enter: bind(ks("enter"), "⏎", "start"),
			Next:  bind(ks("tab", "down"), "tab", "next"),
			Prev:  bind(ks("shift+tab", "up"), "shift+tab", "back"),
			Left:  bind(ks("left"), "←→", "pick"),
			Right: bind(ks("right", "space"), "←→", "pick"),
			Erase: bind(ks("backspace"), "⌫", "erase"),
		},
		peek: peekKeys{
			Close: bind(ks("esc"), "esc", ""),
			Jump:  bind(ks("ctrl+o"), "^o", "jump"),
			Next:  bind(ks("right", "tab"), "→", "next"),
			Prev:  bind(ks("left", "shift+tab"), "←", "prev"),
			Send:  bind(ks("enter"), "⏎", "send"),
			Up:    bind(ks("up"), "↑↓", "move"),
			Down:  bind(ks("down"), "↑↓", "move"),
			Erase: bind(ks("backspace"), "⌫", "erase"),
			Pick:  bind(digits1to9, "1-9", "pick"),
			Tick:  bind(ks("space", " "), "␣", "tick"),
		},
		sheet: sheetKeys{
			Send:  bind(ks("enter"), "⏎", "send"),
			Close: bind(ks("esc"), "esc", ""),
			Chips: bind(ks("tab"), "tab", "chips"),
			Left:  bind(ks("left"), "←→", "pick"),
			Right: bind(ks("right"), "←→", "pick"),
			Tick:  bind(ks("space"), "space", "toggle"),
			Erase: bind(ks("backspace"), "⌫", "erase"),
		},
		search: searchKeys{
			Clear: bind(ks("esc"), "esc", "clear"),
			Keep:  bind(ks("enter"), "⏎", "keep"),
			Erase: bind(ks("backspace"), "⌫", "erase"),
		},
		pair: pairKeys{
			Copy:  bind(ks("c"), "c", "copies the link"),
			Close: bind(ks("esc"), "esc", "stops"),
			Again: bind(ks("enter"), "Enter", "tries again"),
		},
		takeOver: takeOverKeys{
			Rest: bind(ks("y"), "y", "rest"),
			Keep: bind(ks("n"), "n", "keep"),
		},
	}
}

func defaultShellKeys() shellKeymaps {
	return shellKeymaps{
		shell: shellKeys{
			Quit:      bind(ks("q"), "q", "quit"),
			Up:        bind(ks("up", "k"), "↑↓", "select"),
			Down:      bind(ks("down", "j"), "↑↓", "select"),
			Back:      bind(ks("esc"), "esc", "back"),
			Remove:    bind(ks("x"), "x", "remove"),
			Open:      bind(ks("enter"), "⏎", "open"),
			Yes:       bind(ks("y"), "y", "run"),
			Interrupt: bind(ks("ctrl+c"), "ctrl+c", "quit"),
		},
		typing: shellTypingKeys{
			Cancel:  bind(ks("esc"), "esc", "cancel"),
			Connect: bind(ks("enter"), "⏎", "connect"),
			Erase:   bind(ks("backspace"), "⌫", "erase"),
		},
	}
}

// globalKeys work in every layer.
type globalKeys struct{ Quit bkey.Binding }

func (k globalKeys) bindings() []bkey.Binding { return []bkey.Binding{k.Quit} }

// mainKeys are the console's own keys: the tabs, with nothing on top.
type mainKeys struct {
	Stoker, Quit, NextTab, PrevTab, Tabs, Search, Back, Sort, SortBack bkey.Binding
	Group, Park, Mark, Exited, MarkSafe, QuitAgent, Up, Down, Open     bkey.Binding
	Second, New, Switch, Grant, Peek, Dismiss, Fold, Ships, Dock       bkey.Binding
	Stoke, Night, Order, Rename                                        bkey.Binding
}

func (k mainKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Stoker, k.Quit, k.NextTab, k.PrevTab, k.Tabs, k.Search, k.Back, k.Sort, k.SortBack,
		k.Group, k.Park, k.Mark, k.Exited, k.MarkSafe, k.QuitAgent, k.Up, k.Down, k.Open,
		k.Second, k.New, k.Switch, k.Grant, k.Peek, k.Dismiss, k.Fold, k.Ships, k.Dock,
		k.Stoke, k.Night, k.Order, k.Rename}
}

// formKeys are the new-session, switch and rename form.
type formKeys struct{ Close, Enter, Next, Prev, Left, Right, Erase bkey.Binding }

func (k formKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Close, k.Enter, k.Next, k.Prev, k.Left, k.Right, k.Erase}
}

// peekKeys are the peek box. Every other key is text for its typing line.
type peekKeys struct{ Close, Jump, Next, Prev, Send, Up, Down, Erase, Pick, Tick bkey.Binding }

func (k peekKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Close, k.Jump, k.Next, k.Prev, k.Send, k.Up, k.Down, k.Erase, k.Pick, k.Tick}
}

// sheetKeys are the stoker's order sheet.
type sheetKeys struct{ Send, Close, Chips, Left, Right, Tick, Erase bkey.Binding }

func (k sheetKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Send, k.Close, k.Chips, k.Left, k.Right, k.Tick, k.Erase}
}

// searchKeys are the search line. Every printable key is text.
type searchKeys struct{ Clear, Keep, Erase bkey.Binding }

func (k searchKeys) bindings() []bkey.Binding { return []bkey.Binding{k.Clear, k.Keep, k.Erase} }

// pairKeys are the pair page.
type pairKeys struct{ Copy, Close, Again bkey.Binding }

func (k pairKeys) bindings() []bkey.Binding { return []bkey.Binding{k.Copy, k.Close, k.Again} }

// takeOverKeys answer "take over?": y rests the stoker, n keeps it on.
type takeOverKeys struct{ Rest, Keep bkey.Binding }

func (k takeOverKeys) bindings() []bkey.Binding { return []bkey.Binding{k.Rest, k.Keep} }

// shellKeys are the Hosts list.
type shellKeys struct{ Quit, Up, Down, Back, Remove, Open, Yes, Interrupt bkey.Binding }

func (k shellKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Quit, k.Up, k.Down, k.Back, k.Remove, k.Open, k.Yes, k.Interrupt}
}

// shellTypingKeys are the host name line.
type shellTypingKeys struct{ Cancel, Connect, Erase bkey.Binding }

func (k shellTypingKeys) bindings() []bkey.Binding {
	return []bkey.Binding{k.Cancel, k.Connect, k.Erase}
}
