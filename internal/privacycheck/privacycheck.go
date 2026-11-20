// Package privacycheck keeps each iOS target's privacy manifest in step with its Swift: Apple
// rejects an app that uses a required-reason API it does not declare, and a declared reason the
// code does not have is a false statement to Apple and to the person.
package privacycheck

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// apis maps each required-reason category to the Swift that uses it.
var apis = map[string]*regexp.Regexp{
	"NSPrivacyAccessedAPICategoryUserDefaults":    regexp.MustCompile(`\bUserDefaults\b|@AppStorage\b`),
	"NSPrivacyAccessedAPICategorySystemBootTime":  regexp.MustCompile(`\bsystemUptime\b|\bmach_absolute_time\b`),
	"NSPrivacyAccessedAPICategoryFileTimestamp":   regexp.MustCompile(`\.(creationDate|modificationDate|contentModificationDate)\b|\battributesOfItem\b|\bgetattrlist\b`),
	"NSPrivacyAccessedAPICategoryDiskSpace":       regexp.MustCompile(`\bvolumeAvailableCapacity\w*\b|\bsystemFreeSize\b|\bstatfs\b`),
	"NSPrivacyAccessedAPICategoryActiveKeyboards": regexp.MustCompile(`\bactiveInputModes\b`),
}

var (
	declared = regexp.MustCompile(`<key>NSPrivacyAccessedAPIType</key>\s*<string>([A-Za-z]+)</string>`)
	comments = regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`)
)

// Check fails when the Swift under dirs, or a linked library's category in linked, is not
// declared in manifest, or the manifest declares one that neither uses.
func Check(dirs []string, manifest string, linked ...string) error {
	used, err := uses(dirs)
	if err != nil {
		return err
	}
	for _, category := range linked {
		if _, ok := used[category]; !ok {
			used[category] = "a linked library"
		}
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, m := range declared.FindAllStringSubmatch(string(data), -1) {
		have[m[1]] = true
	}
	var problems []string
	for category, file := range used {
		if !have[category] {
			problems = append(problems, fmt.Sprintf("%s uses %s, and %s does not declare it", file, category, manifest))
		}
	}
	for category := range have {
		if _, ok := used[category]; !ok {
			problems = append(problems, fmt.Sprintf("%s declares %s, and no Swift uses it", manifest, category))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("privacy manifest out of step:\n%s", strings.Join(problems, "\n"))
}

// uses returns each category the Swift under dirs uses, with the first file that uses it.
func uses(dirs []string) (map[string]string, error) {
	used := map[string]string{}
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".swift" {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			code := comments.ReplaceAllString(string(data), "")
			for category, re := range apis {
				if _, seen := used[category]; !seen && re.MatchString(code) {
					used[category] = path
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return used, nil
}
