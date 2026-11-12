package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/release"
)

func TestUpdateCmdRunsTheUpdater(t *testing.T) {
	if release.IsRelease(version) {
		t.Skip("a release build would reach GitHub")
	}
	var out bytes.Buffer
	err := updateCmd(&out)
	if err == nil || !strings.Contains(err.Error(), "not a release build") {
		t.Fatalf("err = %v", err)
	}
}
