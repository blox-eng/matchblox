package said

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// The question tool's pages, as Claude Code draws them: chips for each
// question, the question, its numbered options with descriptions, the
// typing row and the hints.
const (
	page1 = `
⏺ Two ways on from here.

────────────────────────────────────────────────────────
 ←  ☐ Cache  ☐ Scope  ✔ Submit  →

Which cache should the service use for the session
tokens it reads on every request?

❯ 1. Redis
     Fast, shared by every instance
  2. In memory
     Simple, lost on a restart
  3. Type something.

Enter to select · ↑/↓ to navigate · Esc to cancel
`
	page2 = `
────────────────────────────────────────────────────────
 ←  ☒ Cache  ☐ Scope  ✔ Submit  →

Which parts should it cover?

❯ 1. [ ] Sessions
  2. [✔] Tokens
  3. [ ] Type something
     Next

Enter to select · ↑/↓ to navigate · Esc to cancel
`
	review = `
────────────────────────────────────────────────────────
 ←  ☒ Cache  ☒ Scope  ✔ Submit  →

Review your answers

 ● Which cache should the service use for the session tokens it reads on every request?
   → Redis

Ready to submit your answers?

❯ 1. Submit answers
  2. Cancel
`
)

var (
	qCache = Question{Question: "Which cache should the service use for the session tokens it reads on every request?",
		Options: []Option{{Label: "Redis"}, {Label: "In memory"}}}
	qScope = Question{Question: "Which parts should it cover?", Multi: true,
		Options: []Option{{Label: "Sessions"}, {Label: "Tokens"}}}
)

func TestShowsQuestion(t *testing.T) {
	for _, c := range []struct {
		name   string
		screen string
		q      Question
		want   bool
	}{
		{"its page", page1, qCache, true},
		{"its page, several choices", page2, qScope, true},
		{"the next page shows: answered in the pane", page2, qCache, false},
		{"the page before shows", page1, qScope, false},
		{"the review page", review, qCache, false},
		{"no menu", "⏺ Done. The tests pass.\n\n> \n", qCache, false},
		{"the same options under another question", page1, Question{Question: "Which queue?", Options: qCache.Options}, false},
		{"an option it lacks", page1, Question{Question: qCache.Question, Options: []Option{{Label: "Redis"}, {Label: "Memcached"}}}, false},
		{"a cut row on a narrow pane", "Which one?\n❯ 1. A very long option label that the pa…\n  2. Short\n", Question{Question: "Which one?",
			Options: []Option{{Label: "A very long option label that the pane cuts"}, {Label: "Short"}}}, true},
	} {
		if got := ShowsQuestion(c.screen, c.q); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestShowsReview(t *testing.T) {
	if !ShowsReview(review) {
		t.Fatal("the review page is not read")
	}
	for _, s := range []string{page1, page2, "Submit answers when you are ready.\n"} {
		if ShowsReview(s) {
			t.Fatalf("read as the review page: %q", s)
		}
	}
}

// TestMenusFromALivePane reads screens captured from Claude Code 2.1.296
// in a tmux pane during the live check of #68: each page is read as its
// own question and no other, the review page as the review.
func TestMenusFromALivePane(t *testing.T) {
	fruit := Question{Question: "Which fruit do you want?", Options: []Option{{Label: "Apple"}, {Label: "Banana"}, {Label: "Cherry"}}}
	toppings := Question{Question: "Which toppings do you want?", Multi: true,
		Options: []Option{{Label: "Cheese"}, {Label: "Olives"}, {Label: "Basil"}, {Label: "Garlic"}}}
	days := Question{Question: "Which days can you work?", Multi: true, Options: []Option{{Label: "Monday"}, {Label: "Tuesday"}, {Label: "Wednesday"}}}
	shift := Question{Question: "Which shift do you prefer?", Options: []Option{{Label: "Morning"}, {Label: "Evening"}}}
	colour := Question{Question: "Which colour should the door be?", Options: []Option{{Label: "Red"}, {Label: "Blue"}}}
	twoFruit := Question{Question: "Which fruit do you want?", Options: []Option{{Label: "Apple"}, {Label: "Banana"}}}
	all := []Question{fruit, toppings, days, shift, colour}
	for _, c := range []struct {
		file   string
		shows  []Question // the questions it shows; every other one it does not
		review bool
	}{
		{"fruit", []Question{fruit}, false},
		{"toppings", []Question{toppings}, false},
		{"days-shift-1", []Question{days}, false},
		{"days-shift-2", []Question{shift}, false},
		{"days-shift-review", nil, true},
		{"colour", []Question{colour}, false},
		{"fruit-default-mode", []Question{twoFruit}, false}, // fruit's three options are not all there
		{"answered", nil, false},
	} {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "menus", c.file+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		screen := string(b)
		for _, q := range append(all, c.shows...) {
			want := slices.ContainsFunc(c.shows, func(s Question) bool { return reflect.DeepEqual(s, q) })
			if got := ShowsQuestion(screen, q); got != want {
				t.Errorf("%s: ShowsQuestion(%q, %d options) = %v, want %v", c.file, q.Question, len(q.Options), got, want)
			}
		}
		if got := ShowsReview(screen); got != c.review {
			t.Errorf("%s: ShowsReview = %v, want %v", c.file, got, c.review)
		}
	}
}
