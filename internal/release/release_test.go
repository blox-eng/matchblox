package release

import "testing"

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "v0.1.0", 0},
		{"v0.1.0", "v0.2.0", -1},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0", "v0.99.99", 1},
		{"v0.2.0-rc.1", "v0.2.0", -1},
		{"v0.2.0", "v0.2.0-rc.1", 1},
		{"v0.2.1-0.20261022-abc", "v0.2.0", 1},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsRelease(t *testing.T) {
	for tag, want := range map[string]bool{
		"v0.1.0": true, "v10.20.30": true, "v0.2.0-rc.1": true,
		"dev": false, "0.1.0": false, "v0.1": false, "v0.1.0+x": false, "": false,
		"v0.2.1-0.20261022-abc": true,
	} {
		if got := IsRelease(tag); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", tag, got, want)
		}
	}
}
