package main

import (
	"html"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

var (
	pipedLink = regexp.MustCompile(`\[\[[^\]|]*\|([^\]]*)\]\]`)
	plainLink = regexp.MustCompile(`\[\[([^\]]*)\]\]`)
	listItem  = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])\s+`)
)

// clean is lernt/text.py's clean: Markdown to what a terminal can print. Only
// "**" is stripped, never a lone "*" - a question about `char *p` needs its star.
func clean(s string) string {
	s = pipedLink.ReplaceAllString(s, "$1")
	s = plainLink.ReplaceAllString(s, "$1")
	s = html.UnescapeString(s)
	return strings.NewReplacer("**", "", "`", "").Replace(s)
}

// wrap cleans lines and word-wraps them to width. Blank lines are kept, and a
// list item's continuation lines hang under its text rather than its marker.
func wrap(lines []string, width int) []string {
	width = max(width, 20)
	var out []string
	for _, line := range lines {
		line = strings.TrimRight(clean(line), " \t")
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		hang := lead
		if marker := listItem.FindString(line); marker != "" {
			hang = strings.Repeat(" ", runewidth.StringWidth(marker))
		}
		current := lead
		for _, word := range strings.Fields(line) {
			if strings.TrimSpace(current) == "" {
				current += word
				continue
			}
			if runewidth.StringWidth(current)+1+runewidth.StringWidth(word) > width {
				out = append(out, current)
				current = hang + word
				continue
			}
			current += " " + word
		}
		out = append(out, current)
	}
	return out
}

// truncate cuts s to width cells, marking the cut. Styling is kept, and not
// counted.
func truncate(s string, width int) string {
	return ansi.Truncate(s, max(width, 4), "…")
}

// tilde shortens a path under the home directory, for display only.
func tilde(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(path, home+"/") {
		return "~/" + path[len(home)+1:]
	}
	return path
}
