package app

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// footTexts are the words of the list's foot parts.
func footTexts(m Model) []string {
	var out []string
	for _, p := range m.footParts() {
		out = append(out, p.text)
	}
	return out
}

// TestAHostsTroubleIsAFootPart: a host that is not reached, or that the
// console cannot read, is a part of the list's foot, in all hosts and on
// that host. All hosts leaves a hidden host out; s on it shows it.
func TestAHostsTroubleIsAFootPart(t *testing.T) {
	m, now, conns := hostsConsole(t)
	if got := strings.Join(footTexts(m), " · "); strings.Contains(got, "ws-2") {
		t.Fatalf("a host that answers is in the foot: %q", got)
	}
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: lostMsg{err: io.EOF, conn: conns["ws-2"]}})
	*now = now.Add(2 * time.Minute)
	if got := strings.Join(footTexts(m), " · "); !strings.Contains(got, "! ws-2 not reached") {
		t.Fatalf("all hosts: foot %q", got)
	}
	if !strings.Contains(strings.Join(frameOf(m), "\n"), "! ws-2 not reached") {
		t.Errorf("the foot does not draw ws-2:\n%s", strings.Join(frameOf(m), "\n"))
	}
	if got := strings.Join(footTexts(pressOn(m, "s", "s")), " · "); !strings.Contains(got, "! ws-2 not reached") {
		t.Errorf("on ws-2: foot %q", got)
	}

	hid := lostHidden(t)
	if got := strings.Join(footTexts(hid), " · "); strings.Contains(got, "ws-2") {
		t.Errorf("all hosts names the hidden ws-2: %q", got)
	}
	if on := pressOn(hid, "s"); on.boardHost != "ws-2" || !strings.Contains(strings.Join(footTexts(on), " · "), "! ws-2 not reached") {
		t.Errorf("s on the hidden ws-2 (%q): foot %q", on.boardHost, footTexts(on))
	}

	b, _, _ := hostsConsole(t)
	b.peers = append(b.peers[:0:0], b.peers...)
	b.peers[0].blocked = remote.ErrHostKeyUnknown
	if got := strings.Join(footTexts(b), " · "); !strings.Contains(got, "! ws-2 blocked") {
		t.Errorf("a blocked ws-2: foot %q", got)
	}
}

// TestTheHostPartOpensItsFix: ⏎ on a host's part shows its trouble and
// its fix in the chat's place; ⏎ there dials a host that is not reached
// again, and a blocked host's fix waits for its y.
func TestTheHostPartOpensItsFix(t *testing.T) {
	m, now, conns := hostsConsole(t)
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: lostMsg{err: io.EOF, conn: conns["ws-2"]}})
	*now = now.Add(2 * time.Minute)
	m.footAt = len(m.footParts())
	m = pressOn(m, "enter")
	if m.sidePlace != placeHost {
		t.Fatalf("⏎ on the part opens place %d", m.sidePlace)
	}
	frame := strings.Join(frameOf(m), "\n")
	if !strings.Contains(frame, "HOST ws-2") || !strings.Contains(frame, "not reached since") || !strings.Contains(frame, "⏎ try again") {
		t.Fatalf("the host's place:\n%s", frame)
	}
	if m = pressOn(m, "enter"); m.flash != "trying ws-2 again…" || m.sidePlace != placeChat {
		t.Errorf("⏎ in the host's place: flash %q, place %d", m.flash, m.sidePlace)
	}

	b, _, _ := hostsConsole(t)
	b.peers = append(b.peers[:0:0], b.peers...)
	b.peers[0].blocked = errors.Join(remote.ErrHostKeyUnknown)
	b.footAt = len(b.footParts())
	b = pressOn(b, "enter")
	if frame := strings.Join(frameOf(b), "\n"); !strings.Contains(frame, "HOST ws-2") {
		t.Fatalf("the blocked host's place:\n%s", frame)
	}
	if b = pressOn(b, "enter"); b.pending == nil && b.flash == "" {
		t.Error("⏎ on a blocked host's fix did nothing and said nothing")
	}
}

// TestTheSetupPlaceClosesAStepWithX: x on a setup step in the chat's
// place asks to close it, and a typed y closes it.
func TestTheSetupPlaceClosesAStepWithX(t *testing.T) {
	m, _, _ := sideShotOf(t, doorState(guideDoor), 160, 38)
	m.openPlace(placeSetup)
	if keys := keyLineOf(m); !strings.Contains(keys, "x close") {
		t.Fatalf("the setup place's key line: %q", keys)
	}
	m = pressOn(m, "x")
	if out := plainFrame(m); !strings.Contains(out, "close Open a guide session") {
		t.Fatalf("no close confirm:\n%s", out)
	}
	m = pressOn(m, "y")
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0] != (proto.Act{RecID: "door:guide", Which: "secondary", Confirm: "y"}) {
		t.Fatalf("acts %+v", acts)
	}
}
