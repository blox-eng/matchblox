package app

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/stoker"
)

// frameHost is a host of the frame's desk and the state it sends.
type frameHost struct {
	name string
	st   proto.State
}

// frameDesk is a console at w columns whose home is ws-1, with each peer's
// state in. The clock is boardAt until the test moves it.
func frameDesk(t *testing.T, w int, home proto.State, peers ...frameHost) (Model, *time.Time, *fakeConn, map[string]*fakeConn) {
	t.Helper()
	now := boardAt
	var names []string
	for _, p := range peers {
		names = append(names, p.name)
	}
	m, homeConn, conns := hostsAt(t, &now, "ws-1", w, 40, names...)
	m = upd(t, m, stateMsg(home))
	for _, p := range peers {
		m = upd(t, m, peerMsg{host: p.name, conn: conns[p.name], msg: stateMsg(p.st)})
	}
	return m, &now, homeConn, conns
}

// mockupDesk is the state "return" of design/0022-the-desk.html: ws-1 holds
// matchblox (flint-auth asks, kindle-web and ux-pass done), ledge
// (ember-billing asks) and base; gpu-1 holds matchblox and base (spark-rates
// done). One Claude plan, the stoker off.
func mockupDesk() (home, gpu proto.State) {
	in := func(s sample.Session, cwd string) sample.Session { s.Cwd = cwd; return s }
	flint, ember := in(boardMatch("flint-auth", time.Hour), "/dev/matchblox"), in(boardMatch("ember-billing", time.Hour), "/dev/ledge")
	home = boardState(flint, in(doneMatch("kindle-web", 6*time.Minute), "/dev/matchblox/www"),
		in(doneMatch("ux-pass", 2*time.Minute), "/dev/matchblox"), in(boardMatch("base-docs", time.Hour), "/dev/base"),
		ember, in(boardMatch("wick-vat", time.Hour), "/dev/ledge"))
	home.Git = spaceRepos
	home.Queue = []queue.Item{waitFor(flint, queue.StateQuestion, time.Minute), waitFor(ember, queue.StateQuestion, time.Minute)}
	home.Machine = machineAt(34, 61)
	home.Limits = []limits.Account{plan(62)}
	home.Stoker = &stoker.View{}
	gpu = boardState(in(boardMatch("hearth-api", time.Hour), "/srv/matchblox"), in(doneMatch("spark-rates", 9*time.Minute), "/srv/base"),
		in(boardMatch("cinder-ci", time.Hour), "/srv/base"))
	gpu.Git = &proto.GitReport{Repos: []gitscan.Repo{
		{Path: "/srv/matchblox", Worktrees: []gitscan.Worktree{{Path: "/srv/matchblox"}}},
		{Path: "/srv/base", Worktrees: []gitscan.Worktree{{Path: "/srv/base"}}},
	}}
	gpu.Machine = machineAt(41, 52)
	return home, gpu
}

func machineAt(cpu, memPct float64) sample.Machine {
	return sample.Machine{CPU: cpu, MemTotal: 100 << 30, MemAvail: uint64(100-memPct) << 30}
}

// plan is a Claude plan named work: its 5 hours at pct and resetting in
// 2h48m, its week at 40% and resetting in 4 days.
func plan(pct float64) limits.Account {
	return limits.Account{Provider: "anthropic", Account: "you@example.com · work", Source: limits.SourceClaude, State: limits.Measured, At: boardAt,
		Windows: []limits.Window{
			{UsedPct: pct, Minutes: limits.BoxMinutes, ResetsAt: boardAt.Add(2*time.Hour + 48*time.Minute)},
			{UsedPct: 40, Minutes: limits.WeekMinutes, ResetsAt: boardAt.Add(4 * 24 * time.Hour)},
		}}
}

func mockupFrame(t *testing.T, w int) Model {
	t.Helper()
	home, gpu := mockupDesk()
	m, _, _, _ := frameDesk(t, w, home, frameHost{"gpu-1", gpu})
	return m
}

func busyLoop() []sample.Orphan {
	return []sample.Orphan{{PID: 4242, Comm: "node", Cmdline: "node watch.js", Cwd: "/srv/matchblox"}}
}

// inkOf is the ink of the first sub in line: its token, and " bold" when
// bold. Every cell of sub but a space wears it, else it says "mixed".
func inkOf(t *testing.T, line, sub string) string {
	t.Helper()
	type cell struct {
		ch, ink string
	}
	var cells []cell
	for _, sp := range parsed(t, line)[0] {
		ink := sp.FG
		if sp.Bold {
			ink += " bold"
		}
		for _, r := range sp.Text {
			cells = append(cells, cell{string(r), ink})
		}
	}
	var plain strings.Builder
	for _, c := range cells {
		plain.WriteString(c.ch)
	}
	at := strings.Index(plain.String(), sub)
	if at < 0 {
		t.Errorf("%q has no %q", plain.String(), sub)
		return "absent"
	}
	x, ink := len([]rune(plain.String()[:at])), ""
	for i, r := range []rune(sub) {
		switch c := cells[x+i]; {
		case r == ' ':
		case ink == "":
			ink = c.ink
		case c.ink != ink:
			return "mixed: " + ink + " and " + c.ink
		}
	}
	return ink
}

// wantInks checks the ink of each part of line.
func wantInks(t *testing.T, line string, inks [][2]string) {
	t.Helper()
	for _, p := range inks {
		if got := inkOf(t, line, p[0]); got != p[1] {
			t.Errorf("%q wears %s, want %s", p[0], got, p[1])
		}
	}
}

func TestHeadRow(t *testing.T) {
	line, _ := mockupFrame(t, 160).frameHead(160)
	plain := ansi.Strip(line)
	left, right := " ▰ matchblox    ALL    matchblox ●1    base    ledge ●1", "● 2 need you · ✓ 3 done    ⚙ "
	if ansi.StringWidth(plain) != 160 || !strings.HasPrefix(plain, left) || !strings.HasSuffix(plain, right) ||
		strings.TrimSpace(plain[len(left):len(plain)-len(right)]) != "" {
		t.Fatalf("the head row is\n%q\nwant %q, spaces, %q in 160 cells", plain, left, right)
	}
	wantInks(t, line, [][2]string{{"▰", "accent"}, {" matchblox ", "fg"}, {"ALL", "accent bold"}, {"matchblox ●1", "mixed: faint and muted"},
		{"base", "faint"}, {"ledge", "faint"}, {"●1", "muted"}, {"● 2 need you", "accent"}, {" · ", "faint"},
		{"✓ 3 done", "match-ember"}, {"⚙", "muted"}})

	m := mockupFrame(t, 160)
	m.space = "ledge"
	line, _ = m.frameHead(160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "all    matchblox ●1    base    LEDGE ●1") {
		t.Fatalf("with ledge open the head row is %q", plain)
	}
	wantInks(t, line, [][2]string{{"all", "faint"}, {"LEDGE", "accent bold"}, {"LEDGE ●1", "mixed: accent bold and accent"}, {"matchblox ●1", "mixed: faint and muted"}})

	for _, w := range []int{70, 60} {
		line, _ = mockupFrame(t, w).frameHead(w)
		plain := ansi.Strip(line)
		if ansi.StringWidth(plain) != w || !strings.Contains(plain, " ▾") || !strings.HasSuffix(plain, "● 2 need you · ✓ 3 done    ⚙ ") {
			t.Fatalf("at %d cells the head row is %q: want the spaces that do not fit as +N ▾, and the waits whole", w, plain)
		}
	}
	line, _ = mockupFrame(t, 70).frameHead(70)
	if plain := ansi.Strip(line); !strings.Contains(plain, "    +3 ▾") || strings.Contains(plain, "base") {
		t.Fatalf("at 70 cells the head row is %q: want ALL then +3 ▾", plain)
	}
	wantInks(t, line, [][2]string{{"+3 ▾", "faint"}})
}

func TestTheCalmHeadSaysNothing(t *testing.T) {
	home, gpu := mockupDesk()
	calm := func(s sample.Session) sample.Session { s.TurnEndedAt = time.Time{}; return s }
	for _, st := range []*proto.State{&home, &gpu} {
		for i := range st.Sessions {
			st.Sessions[i] = calm(st.Sessions[i])
		}
	}
	home.Queue = nil
	m, _, _, _ := frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	line, _ := m.frameHead(160)
	plain := ansi.Strip(line)
	if rest, ok := strings.CutSuffix(plain, "⚙ "); !ok || !strings.HasSuffix(strings.TrimRight(rest, " "), "ALL    matchblox    base    ledge") {
		t.Fatalf("with no wait and none done the head row is %q: want only ⚙ at the right", plain)
	}
	for _, word := range []string{"need", "nothing", "✓", "●"} {
		if strings.Contains(plain, word) {
			t.Errorf("the calm head row says %q: %q", word, plain)
		}
	}

	home, gpu = mockupDesk()
	home.Queue = nil
	m, _, _, _ = frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	line, _ = m.frameHead(160)
	if plain := ansi.Strip(line); !strings.HasSuffix(plain, "  ✓ 3 done    ⚙ ") || strings.Contains(plain, "need") || strings.Contains(plain, "·") {
		t.Fatalf("with none waiting and three done the head row is %q: want ✓ 3 done    ⚙", plain)
	}
}

func TestBarRowIsCalm(t *testing.T) {
	line, _ := mockupFrame(t, 160).frameBar("LIST", 160)
	plain := ansi.Strip(line)
	left := " LIST   ws-1 cpu 34% mem 61%   gpu-1 cpu 41% mem 52%  s▸   ✻ work 5h 62% ↻2h48m · wk 40% ↻4d   stoker off"
	if ansi.StringWidth(plain) != 160 || !strings.HasPrefix(plain, left) || !strings.HasSuffix(plain, "    ? keys ") ||
		strings.TrimSpace(plain[len(left):len(plain)-len("? keys ")]) != "" {
		t.Fatalf("the bar row is\n%q\nwant %q, spaces, ? keys in 160 cells", plain, left)
	}
	if strings.Contains(plain, "│") {
		t.Errorf("the bar has a │: %q", plain)
	}
	wantInks(t, line, [][2]string{{"LIST", "muted"}, {"ws-1", "muted"}, {" cpu ", "faint"}, {"34%", "muted"}, {" mem ", "faint"},
		{"61%", "muted"}, {"gpu-1", "muted"}, {"s▸", "faint"}, {"✻ work", "muted"}, {" 5h ", "faint"}, {"62%", "muted"},
		{" ↻2h48m · wk ", "faint"}, {"40%", "muted"}, {"↻4d", "faint"}, {"stoker off", "faint"}, {"?", "fg"}, {" keys", "muted"}})
	for _, sp := range parsed(t, line)[0] {
		if strings.TrimSpace(sp.Text) != "" && !slices.Contains([]string{"faint", "muted", "fg"}, sp.FG) {
			t.Errorf("a calm bar wears %s on %q", sp.FG, sp.Text)
		}
	}
}

func TestBarStatesWithoutData(t *testing.T) {
	home, gpu := mockupDesk()
	home.Limits[0].Windows = nil
	m, _, _, _ := frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	line, _ := m.frameBar("LIST", 160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "   ✻ work …   stoker off") || strings.Contains(plain, "0%") {
		t.Fatalf("with the usage not read yet the bar is %q: want the Pulse where the value goes, never 0%%", plain)
	}
	wantInks(t, line, [][2]string{{"✻ work", "muted"}, {"…", "faint"}})

	home, gpu = mockupDesk()
	home.Limits[0].State, home.Limits[0].Windows, home.Limits[0].Fix = limits.Unmeasured, nil, "run claude once"
	m, _, _, _ = frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	line, spans := m.frameBar("LIST", 160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "   ✻ work usage not measured   stoker off") || strings.Contains(plain, "…") {
		t.Fatalf("with a plan never measured the bar is %q: want usage not measured, no Pulse", plain)
	}
	wantInks(t, line, [][2]string{{"✻ work", "muted"}, {"usage not measured", "faint"}})
	if i := slices.IndexFunc(spans, func(s span) bool { return s.press == "cog:usage" }); i < 0 ||
		ansi.Strip(ansi.Cut(line, spans[i].x0, spans[i].x1)) != "✻ work usage not measured" {
		t.Fatalf("a tap on the unmeasured plan presses nothing: %+v", spans)
	}

	home, gpu = mockupDesk()
	home.Limits = nil
	m, _, _, _ = frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	if line, _ := m.frameBar("LIST", 160); strings.Contains(ansi.Strip(line), "✻") || !strings.Contains(ansi.Strip(line), "s▸   stoker off") {
		t.Fatalf("with no account the bar is %q: want no usage chip", ansi.Strip(line))
	}

	home, _ = mockupDesk()
	m, _, _, _ = frameDesk(t, 160, home)
	if plain := ansi.Strip(must(m.frameBar("LIST", 160))); !strings.HasPrefix(plain, " LIST   ws-1 cpu 34% mem 61%   ✻ work") || strings.Contains(plain, "s▸") {
		t.Fatalf("with one host the bar is %q: want its chip and no s▸", plain)
	}

	now := boardAt
	fresh, _, _ := hostsAt(t, &now, "ws-1", 160, 40)
	line, _ = fresh.frameBar("LIST", 160)
	if plain := ansi.Strip(line); !strings.HasPrefix(plain, " LIST   ws-1 cpu … mem …") || strings.Contains(plain, "0%") {
		t.Fatalf("before the first state the bar is %q: want the Pulse where each value goes, never 0%%", plain)
	}

	home, gpu = mockupDesk()
	m, _, homeConn, _ := frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	m = upd(t, m, lostMsg{err: io.EOF, conn: homeConn})
	line, _ = m.frameBar("LIST", 160)
	plain := ansi.Strip(line)
	if !strings.HasPrefix(plain, " LIST   no service · it redials") || !strings.HasSuffix(plain, "? keys ") ||
		strings.Contains(plain, "cpu") || strings.Contains(plain, "✻") || ansi.StringWidth(plain) != 160 {
		t.Fatalf("with the service lost the bar is %q: want the mode, no service · it redials and ? keys, no chip", plain)
	}
	wantInks(t, line, [][2]string{{"no service · it redials", "warn"}})
	if strings.Contains(plain, "stoker") {
		t.Fatalf("with the service lost the bar still names the stoker: %q", plain)
	}
}

// TestThePulseMoves: with motion on, the Pulse is faint, then muted 0.8 s
// later, then faint again.
func TestThePulseMoves(t *testing.T) {
	now := time.UnixMilli(1600 * 1_100_000_000)
	m, _, _ := hostsAt(t, &now, "ws-1", 160, 40)
	m.opt.NoMotion = false
	for i, want := range []string{"faint", "muted", "faint"} {
		now = time.UnixMilli(1600*1_100_000_000 + int64(i)*800)
		line, _ := m.frameBar("LIST", 160)
		if got := inkOf(t, line, "…"); got != want {
			t.Errorf("at +%dms the Pulse is %s, want %s", i*800, got, want)
		}
	}
}

// TestTheBarGivesWayWhenNarrow: short of room the bar folds the hosts,
// then drops the resets, the week, the usage; the mode, the way out, a
// trouble chip and ? keys stay whole.
func TestTheBarGivesWayWhenNarrow(t *testing.T) {
	home, gpu := mockupDesk()
	gpu.Orphans = busyLoop()
	for _, w := range []int{60, 104, 132} {
		m, _, _, _ := frameDesk(t, w, home, frameHost{"gpu-1", gpu})
		for _, mode := range []string{"LIST", "SHELL"} {
			line, _ := m.frameBar(mode, w)
			plain := ansi.Strip(line)
			if ansi.StringWidth(plain) != w || !strings.HasPrefix(plain, " "+mode+"   ") || !strings.HasSuffix(plain, "   ? keys ") ||
				!strings.Contains(plain, "gpu-1 cpu 41% ⚠ busy loop") || strings.Contains(plain, "…") {
				t.Errorf("at %d cells in %s the bar is %q: want the mode, the trouble chip and ? keys whole, in %d cells", w, mode, plain, w)
			}
			if mode == "SHELL" && !strings.Contains(plain, "ctrl+q list   ? keys ") {
				t.Errorf("at %d cells in SHELL the bar is %q: want ctrl+q list", w, plain)
			}
		}
	}
	m, _, _, _ := frameDesk(t, 104, home, frameHost{"gpu-1", gpu})
	if plain := ansi.Strip(must(m.frameBar("LIST", 104))); !strings.Contains(plain, "2 hosts s▸   ✻ work 5h 62% · wk 40%   stoker off") {
		t.Errorf("at 104 cells the bar is %q: want the resets gone first", plain)
	}
	m, _, _, _ = frameDesk(t, 90, home, frameHost{"gpu-1", gpu})
	if plain := ansi.Strip(must(m.frameBar("LIST", 90))); !strings.Contains(plain, "2 hosts s▸   ✻ work 5h 62%   stoker off") {
		t.Errorf("at 90 cells the bar is %q: want the week gone next", plain)
	}
}

func must(line string, _ []span) string { return line }

func TestBarRowColoursOnlyTrouble(t *testing.T) {
	home, gpu := mockupDesk()
	gpu.Orphans = busyLoop()
	home.Limits = []limits.Account{plan(91)}
	m, now, _, conns := frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	line, _ := m.frameBar("LIST", 160)
	plain := ansi.Strip(line)
	if !strings.Contains(plain, "   gpu-1 cpu 41% ⚠ busy loop  s▸") {
		t.Fatalf("with a busy loop on gpu-1 the bar is %q", plain)
	}
	wantInks(t, line, [][2]string{{"gpu-1 cpu 41% ⚠ busy loop", "neg"}, {"ws-1", "muted"}, {"34%", "muted"},
		{"✻ work 5h 91% ↻2h48m · wk 40% ↻4d", "warn"}, {"stoker off", "faint"}})
	head, _ := m.frameHead(160)
	if !strings.HasSuffix(ansi.Strip(head), "⚙ ! ") {
		t.Fatalf("with a busy loop the head row is %q: want ⚙ !", ansi.Strip(head))
	}
	wantInks(t, head, [][2]string{{"⚙", "muted"}, {"!", "neg"}})

	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: lostMsg{err: io.EOF, conn: conns["gpu-1"]}})
	*now = boardAt.Add(2 * time.Minute)
	line, _ = m.frameBar("LIST", 160)
	if plain := ansi.Strip(line); !strings.Contains(plain, "   gpu-1 not reached · since 2m  s▸") {
		t.Fatalf("with gpu-1 not reached the bar is %q", plain)
	}
	wantInks(t, line, [][2]string{{"gpu-1 not reached · since 2m", "warn"}, {"ws-1", "muted"}})
}

func TestBarFoldsHosts(t *testing.T) {
	home, gpu := mockupDesk()
	gpu.Orphans = busyLoop()
	peer := func(name string) frameHost {
		st := boardState()
		st.Machine = machineAt(20, 30)
		return frameHost{name, st}
	}
	m, _, _, _ := frameDesk(t, 120, home, frameHost{"gpu-1", gpu}, peer("gpu-2"), peer("mac-1"))
	line, _ := m.frameBar("LIST", 120)
	plain := ansi.Strip(line)
	if !strings.HasPrefix(plain, " LIST   gpu-1 cpu 41% ⚠ busy loop   4 hosts s▸   ✻ work") || ansi.StringWidth(plain) != 120 {
		t.Fatalf("at 120 cells with 4 hosts the bar is %q: want gpu-1's chip, then 4 hosts s▸", plain)
	}
	for _, host := range []string{"ws-1", "gpu-2", "mac-1"} {
		if strings.Contains(plain, host) {
			t.Errorf("the folded bar keeps %s: %q", host, plain)
		}
	}
	wantInks(t, line, [][2]string{{"gpu-1 cpu 41% ⚠ busy loop", "neg"}, {"4 hosts", "muted"}, {"s▸", "faint"}})
}

func TestBarSaysTheWayOut(t *testing.T) {
	m := mockupFrame(t, 160)
	for mode, lead := range map[string]string{"SHELL": " SHELL   ws-1", "AGENT": " AGENT  flint-auth   ws-1"} {
		line, _ := m.frameBar(mode, 160)
		plain := ansi.Strip(line)
		if !strings.HasPrefix(plain, lead) || !strings.HasSuffix(plain, "   ctrl+q list   ? keys ") {
			t.Fatalf("in %s the bar is %q: want ctrl+q list before ? keys", mode, plain)
		}
		wantInks(t, line, [][2]string{{mode, "accent bold"}, {"ctrl+q", "fg"}, {" list", "muted"}})
	}
	for _, mode := range []string{"LIST", "REVIEW", "MACHINE"} {
		if line, _ := m.frameBar(mode, 160); strings.Contains(ansi.Strip(line), "ctrl+q") {
			t.Errorf("in %s the bar says ctrl+q: %q", mode, ansi.Strip(line))
		}
	}
}

func TestTapsOnTheFrame(t *testing.T) {
	home, gpu := mockupDesk()
	gpu.Orphans = busyLoop()
	m, _, _, _ := frameDesk(t, 160, home, frameHost{"gpu-1", gpu})
	hit := func(line string, spans []span, press string) string {
		t.Helper()
		i := slices.IndexFunc(spans, func(s span) bool { return s.press == press })
		if i < 0 {
			t.Fatalf("no span presses %s: %+v", press, spans)
		}
		return ansi.Cut(line, spans[i].x0, spans[i].x1)
	}
	head, hs := m.frameHead(160)
	bar, bs := m.frameBar("LIST", 160)
	for _, c := range []struct {
		line  string
		spans []span
		press string
		want  string
	}{
		{head, hs, "space:base", "base"},
		{head, hs, "space:all", "ALL"},
		{head, hs, "waits", "● 2 need you"},
		{head, hs, "cog:usage", "⚙"},
		{bar, bs, "cog:loops:gpu-1", "gpu-1 cpu 41% ⚠ busy loop"},
		{bar, bs, "cog:health", "ws-1"},
		{bar, bs, "cog:stoker", "stoker off"},
		{bar, bs, "keys", "? keys"},
		{bar, bs, "s", "s▸"},
		{head, hs, "cog:loops", "!"},
	} {
		if got := ansi.Strip(hit(c.line, c.spans, c.press)); got != c.want {
			t.Errorf("the span of %s holds %q, want %q", c.press, got, c.want)
		}
	}
}
