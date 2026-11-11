package main

import (
	"bytes"
	"image"
	_ "image/png"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The landing page tells one matchbox story: Strike, the box, the chain,
// the night, the pocket (docs/superpowers/specs/
// 2026-11-07-landing-one-story-design.md). The install line is in the hero
// only, and the footer holds the links.

func landingPage(t *testing.T) []byte {
	t.Helper()
	page, err := os.ReadFile("../../www/index.html")
	if err != nil {
		t.Fatal(err)
	}
	return page
}

var (
	unseen = regexp.MustCompile(`(?s)<pre\b.*?</pre>|<script\b.*?</script>|<style\b.*?</style>|<svg\b.*?</svg>|<!--.*?-->`)
	markup = regexp.MustCompile(`(?s)<[^>]*>`)
)

// visibleWords counts the words a visitor reads in the body: the terminal
// replays (<pre>), scripts, styles and drawings do not count.
func visibleWords(page []byte) int {
	body := page
	if i := bytes.Index(body, []byte("<body")); i >= 0 {
		body = body[i:]
	}
	text := markup.ReplaceAll(unseen.ReplaceAll(body, []byte(" ")), []byte(" "))
	return len(strings.Fields(string(text)))
}

func TestTheLandingTellsOneStory(t *testing.T) {
	page := landingPage(t)
	if n := bytes.Count(page, []byte("<h1")); n != 1 {
		t.Errorf("the page has %d <h1>, want one", n)
	}
	var ids []string
	for _, m := range regexp.MustCompile(`<h2[^>]*\bid="([^"]+)"`).FindAllSubmatch(page, -1) {
		ids = append(ids, string(m[1]))
	}
	// install-h is the hero's visually hidden heading over the install line.
	want := "install-h box-h chain-h stoker-h pocket-h"
	if got := strings.Join(ids, " "); got != want {
		t.Errorf("the h2 ids run %q, want %q", got, want)
	}
}

func TestTheLandingIsShort(t *testing.T) {
	if n := visibleWords(landingPage(t)); n > 480 {
		t.Errorf("the page shows %d words, want at most 480", n)
	}
}

func TestTheLandingNeedsNoInstructions(t *testing.T) {
	page := landingPage(t)
	for _, s := range []string{"Tap a match to light it", "Try g and s", "Every agent you run, in one list"} {
		if bytes.Contains(page, []byte(s)) {
			t.Errorf("the page says %q: the visual must invite the action itself", s)
		}
	}
	stoker, err := os.ReadFile("../../www/stoker.js")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stoker, []byte("Try g and s")) {
		t.Error("the stoker demo's caption tells the visitor which keys to try")
	}
}

func TestTheLandingHasAPreviewCard(t *testing.T) {
	page := landingPage(t)
	for _, s := range []string{
		"<title>matchblox · a terminal console for every coding agent</title>",
		`<meta property="og:title" content="`,
		`<meta property="og:description" content="`,
		`<meta property="og:url" content="https://matchblox.com/">`,
		`<meta property="og:image" content="https://matchblox.com/assets/og.png">`,
		`<meta name="twitter:card" content="summary_large_image">`,
	} {
		if !bytes.Contains(page, []byte(s)) {
			t.Errorf("the head lacks %s", s)
		}
	}
	f, err := os.Open("../../www/assets/og.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if c.Width != 1200 || c.Height != 630 {
		t.Errorf("og.png is %dx%d, want 1200x630", c.Width, c.Height)
	}
}

func TestTheLandingIdsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\sid="([^"]+)"`).FindAllSubmatch(landingPage(t), -1) {
		id := string(m[1])
		if seen[id] {
			t.Errorf("id %q occurs twice", id)
		}
		seen[id] = true
	}
}

// The pointer meets the match under it: a match's glow and flame reach over
// its neighbour's head, so they must not catch the pointer (#341 review).
func TestTheChainHoverFindsTheMatchUnderThePointer(t *testing.T) {
	css, err := os.ReadFile("../../www/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(css, []byte(".domino .glow, .domino .flame, .domino .smoke { pointer-events: none; }")) {
		t.Error("the chain's glow, flame and smoke catch the pointer, so a hover lifts the next match")
	}
}

// The console replay is hidden from screen readers; its label is their only
// account of what it shows.
func TestTheConsoleSaysWhatItShows(t *testing.T) {
	m := regexp.MustCompile(`class="stage" aria-label="([^"]*)"`).FindSubmatch(landingPage(t))
	if m == nil || !bytes.Contains(m[1], []byte("who waits for you")) {
		t.Errorf("the console's label does not say what the replay shows: %q", m)
	}
}

// "Open the box" lands on the section that holds its heading, so the heading
// is in view.
func TestOpenTheBoxShowsItsHeading(t *testing.T) {
	page := landingPage(t)
	if !bytes.Contains(page, []byte(`href="#console"`)) ||
		!bytes.Contains(page, []byte(`<section class="agents box" id="console" aria-labelledby="box-h">`)) {
		t.Error(`"Open the box" must point at the section that holds "Every agent, one box"`)
	}
}

// The chain strikes once and rests: no timer lights a match again after the
// run, so the row is still unless the visitor strikes it or scrolls back.
func TestTheChainRestsAfterItsRun(t *testing.T) {
	js, err := os.ReadFile("../../www/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(js, []byte("later(flicker")) || bytes.Contains(js, []byte("const flicker")) {
		t.Error("app.js relights the chain at random after its run")
	}
}

// The top bar holds the brand and the light switch; GitHub and Discord sit in
// the footer, once.
func TestTheProjectLinksAreInTheFooter(t *testing.T) {
	page := string(landingPage(t))
	header := page[strings.Index(page, "<header"):strings.Index(page, "</header>")]
	footer := page[strings.Index(page, "<footer"):strings.Index(page, "</footer>")]
	for _, link := range []string{"https://github.com/blox-eng/matchblox\"", "https://discord.gg/tYxBUpGfX3\""} {
		if strings.Contains(header, link) {
			t.Errorf("the top bar links %s", link)
		}
		if n := strings.Count(footer, link); n != 1 {
			t.Errorf("the footer links %s %d times, want once", link, n)
		}
	}
	if n := strings.Count(page, "curl -fsSL https://matchblox.sh | sh"); n != 2 { // the tab's data-cmd and the shown line
		t.Errorf("the install command shows %d times, want 2 (the hero only)", n)
	}
}

// One-way chains (#359): one command joins a team, and a send waits in the
// mate's inbox. The chain section must not promise the old two-sided join.
func TestTheChainSectionSaysOneCommandJoins(t *testing.T) {
	page := string(landingPage(t))
	for _, gone := range []string{"Nothing is shared until both sides chain", "Each agent chooses to join"} {
		if strings.Contains(page, gone) {
			t.Errorf("the chain section still says %q", gone)
		}
	}
	for _, want := range []string{"Chain your agents into a team with one command.", "<h3>One command joins</h3>", "<h3>Nothing bounces</h3>", "<h3>Any agent, any repo</h3>"} {
		if !strings.Contains(page, want) {
			t.Errorf("the chain section lacks %q", want)
		}
	}
}

// The chain room (#326): ⌥s in an agent opens its team's room, and ⌥s again
// goes back. The room shows only what the board (#372) keeps in a narrowed
// chain: the stoker's card, the matches and the talk line.
func TestTheChainSectionShowsTheRoom(t *testing.T) {
	page := string(landingPage(t))
	for _, want := range []string{"<h3>Every team has a room</h3>", "<kbd>⌥s</kbd>", "tell the stoker"} {
		if !strings.Contains(page, want) {
			t.Errorf("the chain section lacks %q", want)
		}
	}
	for _, gone := range []string{"SHIPS</span>", "GIT</span>"} {
		if strings.Contains(page, gone) {
			t.Errorf("the room drawing shows %q, which the board moves onto the cards", gone)
		}
	}
}
