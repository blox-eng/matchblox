// Package logos draws the providers' logos in a terminal that has kitty's
// graphics protocol (kitty, Ghostty), through tmux too: each logo is sent
// once as an image, then takes two text cells of Unicode placeholders,
// which tmux redraws like any other character (#183).
//
// The logos are lobe-icons' (MIT, github.com/lobehub/lobe-icons,
// @lobehub/icons-static-png 2.2.0), each centred at 80% of a 96px square.
package logos

import (
	"embed"
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
)

//go:embed png
var files embed.FS

// placeholder is kitty's Unicode placeholder: a cell its image is drawn in.
const placeholder = '\U0010EEEE'

// diacritics number the row and column of a placeholder cell (the first
// of kitty's rowcolumn-diacritics table).
var diacritics = []rune{'̅', '̍'}

// logo is one provider's image: its kitty image id (also the 256-colour
// index its cells carry) and its file.
type logo struct {
	id   int
	file string
}

// byProvider names the providers whose accounts USAGE shows. The ids are
// high to keep clear of other programs' images in the same terminal.
var byProvider = map[string]logo{
	"anthropic": {201, "claude-color.png"},
	"openai":    {202, "openai.png"},
	"cursor":    {203, "cursor.png"},
	"opencode":  {204, "opencode.png"},
	"google":    {205, "google-color.png"},
	"gemini":    {206, "gemini-color.png"},
}

func idOf(provider string) string { return strconv.Itoa(byProvider[provider].id) }

// Cells is provider's logo as two placeholder cells, or "" without one.
func Cells(provider string) string {
	l, ok := byProvider[provider]
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("\x1b[38;5;" + strconv.Itoa(l.id) + "m")
	for col := range 2 {
		b.WriteRune(placeholder)
		b.WriteRune(diacritics[0]) // row 0
		b.WriteRune(diacritics[col])
	}
	b.WriteString("\x1b[39m")
	return b.String()
}

// Transmit sends every logo, for a dark or a light background, as virtual
// placements of 2x1 cells. In tmux each chunk is wrapped for passthrough,
// which the pane must allow. Sending again replaces the images.
func Transmit(dark, tmux bool) string {
	theme := "light"
	if dark {
		theme = "dark"
	}
	var b strings.Builder
	for _, l := range byProvider {
		png, err := files.ReadFile("png/" + theme + "/" + l.file)
		if err != nil {
			continue
		}
		data := base64.StdEncoding.EncodeToString(png)
		for i := 0; i < len(data); i += 4096 {
			chunk := data[i:min(i+4096, len(data))]
			more := "0"
			if i+4096 < len(data) {
				more = "1"
			}
			head := "m=" + more
			if i == 0 {
				head = "a=T,U=1,f=100,i=" + strconv.Itoa(l.id) + ",c=2,r=1,q=2," + head
			}
			apc := "\x1b_G" + head + ";" + chunk + "\x1b\\"
			if tmux {
				apc = "\x1bPtmux;" + strings.ReplaceAll(apc, "\x1b", "\x1b\x1b") + "\x1b\\"
			}
			b.WriteString(apc)
		}
	}
	return b.String()
}

// drawing are the terminals with kitty's Unicode placeholders.
var drawing = map[string]bool{"xterm-kitty": true, "xterm-ghostty": true}

// Viewers says whether every terminal that shows this console can draw the
// logos, and a key that changes with them: a client that attaches or
// switches to the console's tmux session needs the images again. env is
// os.Getenv; tmux runs tmux with args.
func Viewers(env func(string) string, tmux func(args ...string) ([]byte, error)) (bool, string) {
	if env("TMUX") == "" {
		return drawing[env("TERM")] || env("TERM_PROGRAM") == "ghostty", "direct"
	}
	session, err := tmux("display", "-p", "-t", env("TMUX_PANE"), "#{session_name}")
	if err != nil {
		return false, ""
	}
	out, err := tmux("list-clients", "-F", "#{client_tty} #{client_termname} #{client_session}")
	if err != nil {
		return false, ""
	}
	name := strings.TrimSpace(string(session))
	var here []string
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) == 3 && f[2] == name {
			if !drawing[f[1]] {
				return false, ""
			}
			here = append(here, f[0])
		}
	}
	slices.Sort(here)
	return len(here) > 0, strings.Join(here, ",")
}
