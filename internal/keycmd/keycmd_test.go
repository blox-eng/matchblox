package keycmd

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

var agents = map[string]bool{"claude": true, "codex": true, "opencode": true, "cursor": true}

func TestTableReads(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("empty table")
	}
	for _, c := range all {
		if c.ID == "" || c.Label == "" {
			t.Errorf("%+v: id and label are required", c)
		}
		if !agents[c.Agent] {
			t.Errorf("%s: unknown agent %q", c.ID, c.Agent)
		}
		if _, err := os.Stat(filepath.Join("..", "logos", "png", "dark", c.Mark+".png")); err != nil {
			t.Errorf("%s: mark %q: %v", c.ID, c.Mark, err)
		}
		switch c.Run {
		case "compact":
		case "type":
			if c.Text == "" {
				t.Errorf("%s: run type needs a text", c.ID)
			}
		default:
			t.Errorf("%s: unknown run %q", c.ID, c.Run)
		}
	}
}

func TestSlotsAreUniqueFrom901(t *testing.T) {
	seen := map[int]string{}
	ids := map[string]bool{}
	for _, c := range All() {
		if c.Slot < 901 || c.Slot >= 1000 {
			t.Errorf("%s: slot %d out of range", c.ID, c.Slot)
		}
		if o, dup := seen[c.Slot]; dup {
			t.Errorf("%s and %s share slot %d", c.ID, o, c.Slot)
		}
		seen[c.Slot] = c.ID
		if ids[c.ID] {
			t.Errorf("duplicate id %s", c.ID)
		}
		ids[c.ID] = true
	}
}

func TestIDsAreSafe(t *testing.T) {
	safe := regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
	for _, c := range All() {
		if !safe.MatchString(c.ID) {
			t.Errorf("id %q breaks the APC", c.ID)
		}
	}
}

func TestForAgent(t *testing.T) {
	have := map[string]bool{}
	for _, c := range ForAgent("claude") {
		have[c.ID] = true
		if c.Agent != "claude" {
			t.Errorf("%s is %s's", c.ID, c.Agent)
		}
	}
	for _, id := range []string{"compact", "clear", "model", "review"} {
		if !have[id] {
			t.Errorf("claude lacks %s", id)
		}
	}
	if got := ForAgent(""); len(got) != 0 {
		t.Errorf("ForAgent(\"\") = %v", got)
	}
}

func TestSequences(t *testing.T) {
	if got, want := Sequence("compact"), "\x1b_matchblox;cmd=compact\x1b\\"; got != want {
		t.Errorf("Sequence = %q, want %q", got, want)
	}
	if got, want := AgentSignal("codex"), "\x1b_matchblox;agent=codex\x1b\\"; got != want {
		t.Errorf("AgentSignal = %q, want %q", got, want)
	}
}

func TestCompactTypesItsText(t *testing.T) {
	i := slices.IndexFunc(ForAgent("claude"), func(c Command) bool { return c.ID == "compact" })
	if i < 0 || ForAgent("claude")[i].Text != "/compact" {
		t.Errorf("claude's commands %+v have no compact that types /compact", ForAgent("claude"))
	}
}

func TestTheSignalsNameThePaneAndTheMarks(t *testing.T) {
	if got := PaneSignal("claude", "%12"); got != "\x1b_matchblox;agent=claude;pane=%12\x1b\\" {
		t.Errorf("PaneSignal = %q", got)
	}
	if got := MarksSignal(); got != "\x1b_matchblox;agent=;marks\x1b\\" {
		t.Errorf("MarksSignal = %q", got)
	}
}

func TestTheBoardSignalCarriesTheBeltKeys(t *testing.T) {
	if got := BoardSignal("1,2*;g"); got != "\x1b_matchblox;board=1,2*;g\x1b\\" {
		t.Errorf("BoardSignal = %q", got)
	}
	if got := BoardSignal(""); got != "\x1b_matchblox;board=\x1b\\" {
		t.Errorf("BoardSignal(\"\") = %q", got)
	}
}

func TestTheAppAnswersTheBoardSignal(t *testing.T) {
	if BeltKey != "\x1b_matchblox;belt\x1b\\" {
		t.Errorf("BeltKey = %q", BeltKey)
	}
	if BeltFromPrefix != "\x1b_matchblox;belt=" {
		t.Errorf("BeltFromPrefix = %q", BeltFromPrefix)
	}
	if BeltSlot == MarkSlot {
		t.Errorf("BeltKey shares the mark's slot %d", MarkSlot)
	}
}

// A command's text goes unquoted into a tmux binding (panes.CommandKeysArgv):
// one word, no tmux syntax.
func TestACommandsTextIsOneSlashWord(t *testing.T) {
	word := regexp.MustCompile(`^/[a-z-]+$`)
	for _, c := range table {
		if c.Text != "" && !word.MatchString(c.Text) {
			t.Errorf("%s types %q", c.ID, c.Text)
		}
	}
}
