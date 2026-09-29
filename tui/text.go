package main

import (
	"html"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	pipedLink = regexp.MustCompile(`\[\[[^\]|]*\|([^\]]*)\]\]`)
	plainLink = regexp.MustCompile(`\[\[([^\]]*)\]\]`)
	listItem  = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])\s+`)
)

// characterRef is a numeric character reference: &#x1f534; or &#128308;.
var characterRef = regexp.MustCompile(`&#(?:[xX]([0-9a-fA-F]{1,6})|([0-9]{1,7}));`)

// decodeReferences turns the numeric character references in s that stand for
// characters beyond ASCII into those characters. Nothing else is touched: &amp;
// and &lt; are text a card may mean literally, and so is &#60;, so only what an
// emoji, or an accented letter, is written as elsewhere is turned back.
func decodeReferences(s string) string {
	return characterRef.ReplaceAllStringFunc(s, func(ref string) string {
		m := characterRef.FindStringSubmatch(ref)
		digits, base := m[1], 16
		if digits == "" {
			digits, base = m[2], 10
		}
		n, err := strconv.ParseInt(digits, base, 32)
		if err != nil || n < 0xA0 || !utf8.ValidRune(rune(n)) { // ASCII and the C1 controls stay as written
			return ref
		}
		return string(rune(n))
	})
}

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
			hang = strings.Repeat(" ", cells(marker))
		}
		current := lead
		for _, word := range strings.Fields(line) {
			if strings.TrimSpace(current) == "" {
				current += word
				continue
			}
			if cells(current)+1+cells(word) > width {
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
	return cut(s, max(width, 4), "…")
}

// tilde shortens a path under the home directory, for display only.
func tilde(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(path, home+"/") {
		return "~/" + path[len(home)+1:]
	}
	return path
}
