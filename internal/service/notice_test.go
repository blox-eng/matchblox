package service

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/transport"
)

func post(s *Service, pane string, texts ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, _ := s.sessionAtLocked(pane)
	for _, t := range texts {
		s.chainsLocked().Post(memberOf(x), t, s.now())
	}
}

// branchGit is newsGit with a branch on each worktree.
func branchGit(s *Service, branches ...string) {
	wts := make([]gitscan.Worktree, len(newsGit.Repos[0].Worktrees))
	for i, wt := range newsGit.Repos[0].Worktrees {
		wts[i] = gitscan.Worktree{Path: wt.Path, Branch: branches[i]}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.Git = &gitscan.Report{Repos: []gitscan.Repo{{Path: newsGit.Repos[0].Path, Worktrees: wts}}}
}

func TestNewsGivesItemsThenEmpties(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:2], "%1", "%2")
	post(s, "%2", "peer-send finished", "peer-send committed 1 file in go.mod")
	want := "[matchblox] since your last turn:\n  peer-send finished\n  peer-send committed 1 file in go.mod"
	if got := told(s, proto.News{Pane: "%2", SessionID: "n2", Event: "UserPromptSubmit"}); got != want {
		t.Fatalf("news = %q, want %q", got, want)
	}
	if got := told(s, proto.News{Pane: "%2", SessionID: "n2", Event: "PostToolUse"}); got != "" {
		t.Fatalf("second news = %q, want none", got)
	}
}

func TestNoticeOncePerPairUntilABranchChange(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:2])
	branchGit(s, "main", "peer-send", "www")
	want := "[matchblox] two also works in matchblox, on main.\n  To hear from it, run: matchblox chain two"
	if got := told(s, proto.News{Pane: "%1"}); got != want {
		t.Fatalf("first news = %q, want %q", got, want)
	}
	if got := told(s, proto.News{Pane: "%1"}); got != "" {
		t.Fatalf("second news = %q, want none", got)
	}
	branchGit(s, "feat/x", "peer-send", "www")
	want = "[matchblox] two also works in matchblox, on feat/x.\n  To hear from it, run: matchblox chain two"
	if got := told(s, proto.News{Pane: "%1"}); got != want {
		t.Fatalf("after two's branch change = %q, want %q", got, want)
	}
	if got := told(s, proto.News{Pane: "%2"}); got != "[matchblox] peer-send also works in matchblox, on peer-send.\n  To hear from it, run: matchblox chain peer-send" {
		t.Fatalf("two's own notice = %q", got)
	}
}

func TestNoticeOffInConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Chains.Notice = false
	if New(cfg, nil, nil).ChainsNotice {
		t.Fatal("[chains] notice = false did not reach the service")
	}
	if !New(config.Default(), nil, nil).ChainsNotice {
		t.Fatal("the notice is off by default")
	}
	s, _ := newsTest(t, newsSessions, "%1", "%2")
	s.ChainsNotice = false
	post(s, "%1", "two finished")
	if got := told(s, proto.News{Pane: "%1"}); got != "[matchblox] since your last turn:\n  two finished" {
		t.Fatalf("news = %q, want the item and no notice", got)
	}
}

func TestNewsBlocksRequestThenNotice(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:3], "%1", "%2")
	branchGit(s, "main", "peer-send", "www")
	if res := chainFrom(t, s, "%3", proto.ChainPrefix+"%1"); res.Err != "" {
		t.Fatalf("ask = %+v", res)
	}
	post(s, "%2", "peer-send finished")
	if got := told(s, proto.News{Pane: "%2"}); got != "[matchblox] since your last turn:\n  peer-send finished\n\n"+
		"[matchblox] three also works in matchblox, on www.\n  To hear from it, run: matchblox chain three" {
		t.Fatalf("two's news = %q", got)
	}
	post(s, "%1", "two finished")
	want := "[matchblox] since your last turn:\n  two finished\n\n" +
		"[matchblox] three asks to chain with you. To accept, run:\n  matchblox chain three"
	if got := told(s, proto.News{Pane: "%1"}); got != want {
		t.Fatalf("peer-send's news = %q, want %q", got, want)
	}
	if got := told(s, proto.News{Pane: "%1"}); got != "" {
		t.Fatalf("the request repeats: %q", got)
	}
}

// TestNewsAnswersOnlyItsOwnPane: a hook hears the news of the pane its
// process runs in, and of no other.
func TestNewsAnswersOnlyItsOwnPane(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:2], "%1", "%2")
	s.PeerPane = func(pid int) string { return map[int]string{100: "%2"}[pid] }
	post(s, "%2", "peer-send finished")
	if got := newsOver(t, s, 31337, proto.News{Pane: "%2", SessionID: "n2"}, true); got != "" {
		t.Fatalf("another process heard %q", got)
	}
	if got := newsOver(t, s, 100, proto.News{Pane: "%2", SessionID: "n2"}, true); got != "[matchblox] since your last turn:\n  peer-send finished" {
		t.Fatalf("the pane's own hook heard %q", got)
	}
}

// told is the news of n, heard.
func told(s *Service, n proto.News) string {
	d := s.news(n)
	s.heard(d)
	return d.text
}

// newsOver asks for n over the socket as the process pid, and reads the
// reply; heard: then says it heard it. It returns once the service is done.
func newsOver(t *testing.T, s *Service, pid int, n proto.News, heard bool) string {
	t.Helper()
	x, y := net.Pipe()
	done := make(chan struct{})
	go func() { _ = s.HandlePeer(t.Context(), transport.NewConn(y), pid); close(done) }()
	c := transport.NewConn(x)
	next(t, c, proto.KindHello)
	go func() { _ = c.Send(proto.KindNews, "", n) }()
	env, err := recv(t, c, "news")
	var r proto.NewsReply
	if err != nil || env.Kind != proto.KindNews || json.Unmarshal(env.Body, &r) != nil {
		r.Text = ""
	}
	if heard {
		_ = c.Send(proto.KindNews, "", struct{}{})
	}
	c.Close()
	<-done
	return r.Text
}

// TestNewsAHookDidNotHearComesBack: a hook that reads the reply and goes
// without saying it heard it leaves the news, the requests and the notices
// for the next ask.
func TestNewsAHookDidNotHearComesBack(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:2], "%1", "%2")
	s.PeerPane = func(pid int) string { return map[int]string{100: "%2"}[pid] }
	post(s, "%2", "peer-send finished")
	want := "[matchblox] since your last turn:\n  peer-send finished"
	if got := newsOver(t, s, 100, proto.News{Pane: "%2", SessionID: "n2"}, false); got != want {
		t.Fatalf("first ask = %q", got)
	}
	if got := newsOver(t, s, 100, proto.News{Pane: "%2", SessionID: "n2"}, true); got != want {
		t.Fatalf("after a hook that did not hear = %q, want %q", got, want)
	}
	if got := newsOver(t, s, 100, proto.News{Pane: "%2", SessionID: "n2"}, true); got != "" {
		t.Fatalf("heard news came back: %q", got)
	}
}

func TestANoticeAHookDidNotHearIsToldAgain(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:2])
	s.PeerPane = func(pid int) string { return map[int]string{100: "%1"}[pid] }
	branchGit(s, "main", "peer-send", "www")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	first := newsOver(t, s, 100, proto.News{Pane: "%1"}, false)
	if !strings.Contains(first, "two asks to chain with you") {
		t.Fatalf("first ask = %q", first)
	}
	if got := newsOver(t, s, 100, proto.News{Pane: "%1"}, true); got != first {
		t.Fatalf("after a hook that did not hear = %q, want %q", got, first)
	}
	if got := newsOver(t, s, 100, proto.News{Pane: "%1"}, true); got != "" {
		t.Fatalf("a heard request came back: %q", got)
	}
}

// TestNewsNotHeardInTimeComesBack: a hook that holds the line and says
// nothing gives the news back after newsHeardWait.
func TestNewsNotHeardInTimeComesBack(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:2], "%1", "%2")
	s.PeerPane = func(pid int) string { return map[int]string{100: "%2"}[pid] }
	post(s, "%2", "peer-send finished")
	x, y := net.Pipe()
	done := make(chan struct{})
	go func() { _ = s.HandlePeer(t.Context(), transport.NewConn(y), 100); close(done) }()
	c := transport.NewConn(x)
	defer c.Close()
	next(t, c, proto.KindHello)
	go func() { _ = c.Send(proto.KindNews, "", proto.News{Pane: "%2", SessionID: "n2"}) }()
	start := time.Now()
	next(t, c, proto.KindNews)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the service waits for the hook forever")
	}
	if d := time.Since(start); d < newsHeardWait/2 {
		t.Fatalf("gave up after %v", d)
	}
	if got := told(s, proto.News{Pane: "%2", SessionID: "n2"}); got != "[matchblox] since your last turn:\n  peer-send finished" {
		t.Fatalf("after the wait = %q", got)
	}
}

// TestNewsWithNoPeerCredentialsIsNone: with no pid the service cannot tell
// whose hook asks, so it answers nothing and takes nothing.
func TestNewsWithNoPeerCredentialsIsNone(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:2], "%1", "%2")
	post(s, "%2", "peer-send finished")
	if got := newsOver(t, s, 0, proto.News{Pane: "%2", SessionID: "n2"}, true); got != "" {
		t.Fatalf("an unknown process heard %q", got)
	}
	if got := told(s, proto.News{Pane: "%2", SessionID: "n2"}); got != "[matchblox] since your last turn:\n  peer-send finished" {
		t.Fatalf("the news was taken: %q", got)
	}
}

// TestNewsHeldForOneHookIsNotToldToAnother: while one hook holds a
// request and a notice it has not said it heard, a second hook of the
// match is not told them too; once the first gives them back, they are.
func TestNewsHeldForOneHookIsNotToldToAnother(t *testing.T) {
	s, _ := newsTest(t, newsSessions[:3])
	branchGit(s, "main", "peer-send", "www")
	chainFrom(t, s, "%2", proto.ChainPrefix+"%1")
	first := s.news(proto.News{Pane: "%1"})
	if !strings.Contains(first.text, "two asks to chain with you") || !strings.Contains(first.text, "three also works in") {
		t.Fatalf("first = %q", first.text)
	}
	second := s.news(proto.News{Pane: "%1"})
	if second.text != "" {
		t.Fatalf("a second hook was told the held news: %q", second.text)
	}
	s.unheard(second)
	s.unheard(first)
	if got := told(s, proto.News{Pane: "%1"}); got != first.text {
		t.Fatalf("after the give-back = %q, want %q", got, first.text)
	}
	if got := told(s, proto.News{Pane: "%1"}); got != "" {
		t.Fatalf("heard news came back: %q", got)
	}
}
