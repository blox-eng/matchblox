//go:build linux

package app

import "testing"

func TestCommaTypesInTheAgent(t *testing.T) {
	d := newScreenDesk(t, dragScreen, 40, 8)
	d.do(keyPress(flipKey))
	d.do(keyPress(","))
	if got := d.agentRead(); got != "," {
		t.Errorf("with the agent's keys , reached the agent as %q, want ,", got)
	}
	if d.m.tab != tabSessions {
		t.Errorf("with the agent's keys , opened tab %d", d.m.tab)
	}
}
