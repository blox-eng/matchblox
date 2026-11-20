package privacycheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const emptyManifest = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>NSPrivacyAccessedAPITypes</key><array></array></dict></plist>
`

func target(t *testing.T, swift, manifest string) (dir, plist string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "A.swift"), []byte(swift), 0o600); err != nil {
		t.Fatal(err)
	}
	plist = filepath.Join(dir, "PrivacyInfo.xcprivacy")
	if err := os.WriteFile(plist, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, plist
}

// Apple's email for TestFlight build 0.4.0: a required-reason API with no declaration.
func TestUndeclaredUseFails(t *testing.T) {
	dir, plist := target(t, "let d = UserDefaults.standard\n", emptyManifest)
	err := Check([]string{dir}, plist)
	if err == nil || !strings.Contains(err.Error(), "NSPrivacyAccessedAPICategoryUserDefaults") {
		t.Fatalf("got %v, want an error naming the UserDefaults category", err)
	}
}

func TestDeclaredButUnusedFails(t *testing.T) {
	manifest := strings.Replace(emptyManifest, "<array></array>", `<array><dict>
<key>NSPrivacyAccessedAPIType</key><string>NSPrivacyAccessedAPICategoryFileTimestamp</string>
<key>NSPrivacyAccessedAPITypeReasons</key><array><string>C617.1</string></array>
</dict></array>`, 1)
	dir, plist := target(t, "let x = 1\n", manifest)
	err := Check([]string{dir}, plist)
	if err == nil || !strings.Contains(err.Error(), "NSPrivacyAccessedAPICategoryFileTimestamp") {
		t.Fatalf("got %v, want an error naming the unused FileTimestamp category", err)
	}
}

func TestAMentionInACommentIsNoUse(t *testing.T) {
	dir, plist := target(t, "// UserDefaults would be wrong here\nlet x = 1\n", emptyManifest)
	if err := Check([]string{dir}, plist); err != nil {
		t.Fatal(err)
	}
}

// Each target that ships declares exactly the required-reason APIs its Swift uses. Every target
// links MatchbloxKit, so the kit's sources count for each.
func TestRepoTargetsMatchTheirManifests(t *testing.T) {
	ios := filepath.Join("..", "..", "ios")
	for _, tc := range []struct {
		name string
		dirs []string
	}{
		{"Matchblox", []string{"Matchblox", "MatchbloxKit/Sources", "Shared"}},
		{"MatchbloxIsland", []string{"MatchbloxIsland", "MatchbloxKit/Sources", "Shared"}},
		{"MatchbloxBuzz", []string{"MatchbloxBuzz", "MatchbloxKit/Sources"}},
	} {
		var dirs []string
		for _, d := range tc.dirs {
			dirs = append(dirs, filepath.Join(ios, d))
		}
		if err := Check(dirs, filepath.Join(ios, tc.name, "PrivacyInfo.xcprivacy")); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}
