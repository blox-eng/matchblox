package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestErrorInTheHomeSessionWaits: the console is the only command of the
// tmux session matchblox, so on an error its pane closes; the error must
// stay on the screen until the person reads it.
func TestErrorInTheHomeSessionWaits(t *testing.T) {
	env := func(k string) string {
		if k == "MATCHBLOX_HOME" {
			return "1"
		}
		return ""
	}
	in := strings.NewReader("\n")
	var out bytes.Buffer
	holdOnError(errors.New("the service does not answer"), env, in, &out)
	if !strings.Contains(out.String(), "press Enter") {
		t.Fatalf("printed %q", out.String())
	}
	if in.Len() != 0 {
		t.Fatal("it did not wait for Enter")
	}

	in, out = strings.NewReader("\n"), bytes.Buffer{}
	holdOnError(errors.New("x"), func(string) string { return "" }, in, &out)
	if in.Len() == 0 || out.Len() != 0 {
		t.Fatal("outside the home session it waited")
	}
}
