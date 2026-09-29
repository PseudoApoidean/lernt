package main

import (
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

// How many cells of a terminal a piece of text takes is a question the terminal
// answers, and terminals disagree with the Unicode tables and with each other:
// tmux draws a keycap (1️⃣) in two cells where the tables say one, and any
// character with a variation selector, or in the emoji planes, in two. Every
// width in the TUI comes from here so that it is one answer, and it is biased: where
// the tables and a terminal can differ it says the wider, because a line that is a
// cell short only leaves a border a cell out on that line, while a line that is a
// cell long wraps and pushes the whole page down.

// A control sequence, such as the styling lipgloss puts round text: no width.
var sequence = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]`)

// clusterWidth is the cells one character (a grapheme cluster: an emoji and its skin
// tone, a letter and its accent) takes; tables is what the Unicode tables say.
func clusterWidth(cluster string, tables int) int {
	if strings.ContainsAny(cluster, "️⃣") { // emoji presentation, and keycaps
		return max(tables, 2)
	}
	for _, r := range cluster { // the first rune is enough to say what it is
		if (r >= 0x1F000 && r <= 0x1FAFF) || wideSymbol(r) {
			return max(tables, 2)
		}
		break
	}
	return tables
}

// wideSymbol is the few symbols outside the emoji planes that tmux draws in two cells
// and the tables give one: a hand, the trigrams, the wavy dash and the circled
// ideographs (found by asking tmux for every one of them).
func wideSymbol(r rune) bool {
	switch {
	case r == 0x261D, r == 0x26F9, r == 0x270C, r == 0x270D,
		r == 0x3030, r == 0x303D, r == 0x3297, r == 0x3299,
		r >= 0x2630 && r <= 0x2637, r >= 0x268A && r <= 0x268F:
		return true
	}
	return false
}

// cells is the width of s in cells, control sequences not counted.
func cells(s string) int {
	w := 0
	g := uniseg.NewGraphemes(sequence.ReplaceAllString(s, ""))
	for g.Next() {
		w += clusterWidth(g.Str(), g.Width())
	}
	return w
}

// cut is s cut to w cells, with tail (such as an ellipsis) in the last of them when
// it had to be. Styling is kept and closed, and not counted.
func cut(s string, w int, tail string) string {
	if cells(s) <= w {
		return s
	}
	if cells(tail) > w { // no room to mark the cut
		tail = ""
	}
	room := max(w-cells(tail), 0)
	var out strings.Builder
	used, styled := 0, false
	for len(s) > 0 {
		loc := sequence.FindStringIndex(s)
		if loc != nil && loc[0] == 0 {
			out.WriteString(s[:loc[1]])
			s, styled = s[loc[1]:], true
			continue
		}
		end := len(s)
		if loc != nil {
			end = loc[0]
		}
		g := uniseg.NewGraphemes(s[:end])
		for g.Next() {
			cw := clusterWidth(g.Str(), g.Width())
			if used+cw > room {
				out.WriteString(tail)
				if styled {
					out.WriteString("\x1b[0m")
				}
				return out.String()
			}
			out.WriteString(g.Str())
			used += cw
		}
		s = s[end:]
	}
	return out.String()
}
