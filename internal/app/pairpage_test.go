package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/replay"
	"github.com/blox-eng/matchblox/internal/setup"
)

const (
	pairHostKey  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIM4bfwfUcmyCX2tnMZ4mHkeEjSRJnkf0ehLEuB8iWLHU root@ws-1\n"
	pairPhoneKey = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFNcqPRX8M69Sf5wSAjjnCSQ7eAlW+J0FHl38LZZ/uI1dOc+R0B75EtirPaf7OBcEUynTj95ks6kamKOuSCb+mA="
	pairExe      = "/usr/local/bin/matchblox"
)

// pairRig is a console with a home of its own, a host key and a clock the
// test moves.
type pairRig struct {
	home   string
	now    time.Time
	copied []string
}

func newPairRig(t *testing.T) (*pairRig, Model) {
	t.Helper()
	r := &pairRig{home: t.TempDir(), now: time.Date(2026, 10, 21, 14, 32, 0, 0, time.UTC)}
	if err := os.MkdirAll(filepath.Join(r.home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.home, ".ssh", "authorized_keys"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	hostKey := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(hostKey, []byte(pairHostKey), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := loadedWith(t, 100, fixtureState())
	m.height = 40
	m.opt.Pair = testPairHost(r.home, pairExe, hostKey)
	m.opt.Now = func() time.Time { return r.now }
	m.opt.Clip = func(text string) { r.copied = append(r.copied, text) }
	return r, m
}

// testPairHost is main's consolePair: the console itself never imports setup.
func testPairHost(home, exe, hostKey string) *PairHost {
	return &PairHost{
		Offer: func(until time.Time) (string, string, error) {
			if exe == "" {
				return "", "", errors.New("cannot find this matchblox on the PATH")
			}
			return setup.NewPairOffer(home, exe, "ws-1.tail1234.ts.net", 22, "ws-1", hostKey, until)
		},
		Seen:    func(id string) (bool, string) { return setup.PairSeen(home, id) },
		Present: func(id string) (bool, error) { return setup.PairKeyPresent(home, id) },
		Remove: func(id string) (bool, error) {
			setup.RemovePairSeen(home, id)
			return setup.RemovePairKey(home, id)
		},
		Claim: func(id string) (PairDevice, bool) {
			d, ok := setup.TakeClaim(home, id)
			return PairDevice(d), ok
		},
	}
}

func (r *pairRig) pairLines(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.home, ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), " gate pair ")
}

func (r *pairRig) seenNotes(t *testing.T) []string {
	t.Helper()
	n, _ := filepath.Glob(filepath.Join(r.home, ".ssh", ".matchblox-pair-*.seen"))
	return n
}

func openedPage(t *testing.T) (*pairRig, Model) {
	t.Helper()
	r, m := newPairRig(t)
	m, cmd := m.openPairPage()
	m = offered(t, m, cmd)
	if m.pair == nil || m.pair.id == "" || m.pair.link == "" {
		t.Fatalf("no code: %+v", m.pair)
	}
	return r, m
}

func pairKeyMsg(k string) tea.KeyPressMsg {
	if k == "ctrl+c" {
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return keyPress(k)
}

// findOffer runs cmd and finds the answer of the offer in it.
func findOffer(cmd tea.Cmd) (pairOfferMsg, bool) {
	if cmd == nil {
		return pairOfferMsg{}, false
	}
	switch msg := cmd().(type) {
	case pairOfferMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if o, ok := findOffer(c); ok {
				return o, true
			}
		}
	}
	return pairOfferMsg{}, false
}

// offered runs the offer that cmd started and gives its answer to m.
func offered(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	msg, ok := findOffer(cmd)
	if !ok {
		t.Fatal("no offer ran")
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestPairPageWaitsForTheOfferOffUpdate(t *testing.T) {
	r, m := newPairRig(t)
	release := make(chan struct{})
	slow := *m.opt.Pair
	offer := slow.Offer
	slow.Offer = func(until time.Time) (string, string, error) {
		<-release
		return offer(until)
	}
	m.opt.Pair = &slow
	m, cmd := m.openPairPage() // returns at once: the offer has not run
	if !m.pair.waiting() || r.pairLines(t) != 0 {
		t.Fatal("the page did not wait for the offer")
	}
	for _, dark := range []bool{true, false} {
		v := m
		v.dark, v.st = dark, newStyles(dark)
		if _, err := replay.Parse(v.render(), Tokens(dark)); err != nil {
			t.Fatal(err)
		}
	}
	text := pageText(m)
	if !strings.Contains(text, "▚▞") || strings.Contains(text, "5:00") || strings.Contains(text, pairKeyLine) {
		t.Fatalf("not the shut box with no countdown:\n%s", text)
	}
	close(release)
	m = offered(t, m, cmd)
	if m.pair.link == "" || r.pairLines(t) != 1 || !strings.Contains(pageText(m), "5:00") {
		t.Fatalf("the offer did not fill the label: %+v", m.pair)
	}
}

// A page that closes while its offer runs leaves no key, whichever side
// sees the key last.
func TestPairPageCancelledOfferLeavesNoKey(t *testing.T) {
	for _, k := range []string{"esc", "ctrl+c"} {
		// The offer returns after the close.
		r, m := newPairRig(t)
		m, cmd := m.openPairPage()
		next, _ := m.Update(pairKeyMsg(k))
		m = next.(Model)
		msg, _ := findOffer(cmd)
		next, _ = m.Update(msg)
		if next.(Model).pair != nil || r.pairLines(t) != 0 {
			t.Fatalf("%s while the offer ran: %d pair lines", k, r.pairLines(t))
		}

		// The offer returned before the close; its answer comes after.
		r, m = newPairRig(t)
		m, cmd = m.openPairPage()
		msg, _ = findOffer(cmd)
		if r.pairLines(t) != 1 {
			t.Fatal("the offer wrote no key")
		}
		next, _ = m.Update(pairKeyMsg(k))
		next, _ = next.Update(msg)
		if next.(Model).pair != nil || r.pairLines(t) != 0 {
			t.Fatalf("%s before the answer: %d pair lines", k, r.pairLines(t))
		}
	}
}

func TestPairPageClosePairWaitsForTheOffer(t *testing.T) {
	r, m := newPairRig(t)
	release := make(chan struct{})
	slow := *m.opt.Pair
	offer := slow.Offer
	slow.Offer = func(until time.Time) (string, string, error) {
		<-release
		return offer(until)
	}
	m.opt.Pair = &slow
	m, cmd := m.openPairPage()
	go func() { _, _ = findOffer(cmd) }() // the program's goroutine
	next, _ := m.Update(pairKeyMsg("ctrl+c"))
	closed := make(chan struct{})
	go func() { next.(Model).ClosePair(); close(closed) }() // main, after Run
	time.Sleep(20 * time.Millisecond)
	close(release)
	<-closed
	if r.pairLines(t) != 0 {
		t.Fatal("the program ended before the offer removed its key")
	}
}

func poll(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(pairPollMsg{})
	return next.(Model)
}

func pageText(m Model) string { return ansi.Strip(m.render()) }

func TestPairPageWritesAndRemovesTheKey(t *testing.T) {
	r, m := openedPage(t)
	if n := r.pairLines(t); n != 1 {
		t.Fatalf("%d pair lines after the open, want 1", n)
	}
	if err := setup.MarkPairSeen(r.home, m.pair.id, ""); err != nil {
		t.Fatal(err)
	}
	next, _ := key(m, "esc")
	m = next.(Model)
	if m.pair != nil {
		t.Fatal("esc left the page open")
	}
	if n := r.pairLines(t); n != 0 {
		t.Fatalf("%d pair lines after esc", n)
	}
	if n := r.seenNotes(t); len(n) != 0 {
		t.Fatalf("the seen note stayed: %v", n)
	}
	if m.flash != "Stopped. The code no longer works." {
		t.Fatalf("flash = %q", m.flash)
	}
}

func TestPairPageRemovesTheKeyOnQuit(t *testing.T) {
	r, m := openedPage(t)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil || cmd() != tea.Quit() {
		t.Fatal("ctrl+c did not quit")
	}
	if next.(Model).pair != nil || r.pairLines(t) != 0 {
		t.Fatal("ctrl+c left the pair line")
	}
}

func TestPairPageRemovesTheKeyWhenTheProgramEnds(t *testing.T) {
	r, m := openedPage(t)
	// The service drops: the page stays, as it needs only this machine.
	next, _ := m.Update(lostMsg{err: errors.New("EOF"), conn: m.conn})
	m = next.(Model)
	if m.pair == nil || r.pairLines(t) != 1 {
		t.Fatal("a dropped service closed the page")
	}
	// The program ends some other way: main closes the page of the final model.
	m.ClosePair()
	if r.pairLines(t) != 0 {
		t.Fatal("the end of the program left the pair line")
	}

	r, m = openedPage(t)
	next, _ = m.Update(lostMsg{err: errors.New("EOF"), conn: m.conn})
	next, _ = next.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !next.(Model).quitting || r.pairLines(t) != 0 {
		t.Fatal("ctrl+c after a lost connection left the pair line")
	}
}

func TestPairPageShellCloseRemovesTheKey(t *testing.T) {
	r, m := openedPage(t)
	s := Shell{layer: layerConsole, cur: m}
	s.Close()
	if r.pairLines(t) != 0 {
		t.Fatal("Shell.Close left the pair line")
	}
}

func TestPairPageKnocksOnSeen(t *testing.T) {
	r, m := openedPage(t)
	m = poll(t, m)
	if m.pair.phase != boxShut {
		t.Fatalf("phase = %v before the device came", m.pair.phase)
	}
	if err := setup.MarkPairSeen(r.home, m.pair.id, ""); err != nil {
		t.Fatal(err)
	}
	m = poll(t, m)
	if m.pair.phase != boxKnock || m.pair.line != "A device is here…" {
		t.Fatalf("phase %v, line %q", m.pair.phase, m.pair.line)
	}
	if !strings.Contains(pageText(m), "A device is here…") {
		t.Fatal("the page does not show the line")
	}
	if err := setup.MarkPairSeen(r.home, m.pair.id, "iPad"); err != nil {
		t.Fatal(err)
	}
	m = poll(t, m)
	if m.pair.line != "iPad is here…" {
		t.Fatalf("line = %q", m.pair.line)
	}
}

func TestPairPageOpensOnClaim(t *testing.T) {
	r, m := openedPage(t)
	d, err := setup.ClaimPairKey(r.home, pairExe, m.pair.id, pairPhoneKey, "iPad")
	if err != nil {
		t.Fatal(err)
	}
	m = poll(t, m)
	if m.pair.phase != boxOpen || m.pair.device.Name != "iPad" {
		t.Fatalf("phase %v, device %+v", m.pair.phase, m.pair.device)
	}
	want := "Paired iPad (" + d.Fingerprint + "). Not yours? x on it in DEVICES."
	if !strings.HasPrefix(d.Fingerprint, "SHA256:") || m.pair.line != want {
		t.Fatalf("line = %q, want %q", m.pair.line, want)
	}
	if !strings.Contains(pageText(m), "iPad") {
		t.Fatal("the label does not name the device")
	}
}

func TestPairPageExpires(t *testing.T) {
	r, m := openedPage(t)
	r.now = r.now.Add(5*time.Minute - time.Second)
	m = poll(t, m)
	if !strings.Contains(pageText(m), "0:01") {
		t.Fatalf("the countdown is not 0:01:\n%s", pageText(m))
	}
	r.now = r.now.Add(time.Second)
	m = poll(t, m)
	if m.pair.phase != boxExpired || m.pair.line != "The code expired. Enter pairs again." {
		t.Fatalf("phase %v, line %q", m.pair.phase, m.pair.line)
	}
	if r.pairLines(t) != 0 {
		t.Fatal("the expired code is still in authorized_keys")
	}
	next, _ := key(m, "x")
	if next.(Model).pair == nil {
		t.Fatal("a key other than esc and Enter closed the page")
	}
	old := m.pair.id
	next, cmd := key(m, "enter")
	m = offered(t, next.(Model), cmd)
	if m.pair == nil || m.pair.phase != boxShut || m.pair.id == old || r.pairLines(t) != 1 {
		t.Fatalf("Enter made no new code: %+v", m.pair)
	}
}

func TestPairPageCopiesTheLink(t *testing.T) {
	r, m := openedPage(t)
	next, _ := key(m, "c")
	m = next.(Model)
	if len(r.copied) != 1 || r.copied[0] != m.pair.link {
		t.Fatalf("copied %q, want the link", r.copied)
	}
	if m.flash != "link copied" || !strings.Contains(pageText(m), "link copied") {
		t.Fatalf("flash = %q", m.flash)
	}
	if got := osc52("x"); got != "\x1b]52;c;eA==\x07" {
		t.Fatalf("osc52 = %q", got)
	}
}

func TestPairPageSurvivesResize(t *testing.T) {
	r, m := openedPage(t)
	if err := setup.MarkPairSeen(r.home, m.pair.id, ""); err != nil {
		t.Fatal(err)
	}
	m = poll(t, m)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = next.(Model)
	// The line wraps in 60 columns.
	if !strings.Contains(strings.Join(strings.Fields(pageText(m)), " "), "Make this terminal 61 × 33 or larger to show the code, or press c to copy the link.") {
		t.Fatalf("no make-larger line:\n%s", pageText(m))
	}
	next, _ = key(m, "c")
	m = next.(Model)
	if len(r.copied) != 1 || !strings.Contains(pageText(m), "link copied") {
		t.Fatal("c does not copy in a small terminal")
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(Model)
	if m.pair.phase != boxKnock || !strings.Contains(pageText(m), "▚▞") {
		t.Fatalf("the box did not come back in the same phase:\n%s", pageText(m))
	}
}

func TestPairPageViewIsTokens(t *testing.T) {
	r, m := openedPage(t)
	views := func(m Model) []Model {
		var out []Model
		for _, dark := range []bool{true, false} {
			for _, size := range [][2]int{{100, 40}, {80, 36}, {60, 30}} {
				v := m
				v.dark, v.st = dark, newStyles(dark)
				v.width, v.height = size[0], size[1]
				out = append(out, v)
			}
		}
		return out
	}
	check := func(m Model) {
		t.Helper()
		for _, v := range views(m) {
			if _, err := replay.Parse(v.render(), Tokens(v.dark)); err != nil {
				t.Fatalf("phase %v, dark %v, %dx%d: %v", v.pair.phase, v.dark, v.width, v.height, err)
			}
		}
	}
	next, _ := key(m, "c")
	m = next.(Model)
	check(m)
	_ = setup.MarkPairSeen(r.home, m.pair.id, "iPad")
	m = poll(t, m)
	check(m)
	if _, err := setup.ClaimPairKey(r.home, pairExe, m.pair.id, pairPhoneKey, "iPad"); err != nil {
		t.Fatal(err)
	}
	m.opt.NoMotion = false
	for _, at := range []time.Duration{0, 300 * time.Millisecond, 700 * time.Millisecond, 2 * time.Second} {
		r.now = r.now.Add(at)
		m = poll(t, m)
		check(m)
	}

	_, m = openedPage(t)
	r2 := m.opt.Now().Add(6 * time.Minute)
	m.opt.Now = func() time.Time { return r2 }
	m = poll(t, m)
	if m.pair.phase != boxExpired {
		t.Fatal("not expired")
	}
	check(m)
}

func TestPairPageOnlyForThisMachine(t *testing.T) {
	for host, want := range map[string]bool{"": true, "ws-2": false} {
		sh := NewShell(ShellOptions{
			Base:  Options{NoMotion: true, Pair: &PairHost{}},
			Open:  func(string) Options { return Options{Conn: newFake()} },
			Start: host,
		})
		if got := sh.cur.opt.Pair != nil; got != want {
			t.Errorf("host %q: the console can pair = %v, want %v", host, got, want)
		}
	}
}

func TestPairPageWithoutTheBinaryWritesNoKey(t *testing.T) {
	r, m := newPairRig(t)
	m.opt.Pair = testPairHost(r.home, "", filepath.Join(t.TempDir(), "host.pub"))
	m, cmd := m.openPairPage()
	m = offered(t, m, cmd)
	if m.pair == nil || m.pair.line != "cannot find this matchblox on the PATH" || r.pairLines(t) != 0 {
		t.Fatalf("page %+v, %d lines", m.pair, r.pairLines(t))
	}
	if !strings.Contains(pageText(m), "cannot find this matchblox on the PATH") {
		t.Fatal("the page does not say why")
	}
}
