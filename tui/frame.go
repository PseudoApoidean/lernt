package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The pieces every screen is built from: a title bar across the top, rounded
// cards with their titles set in the top edge, and a key bar along the bottom.
// Every width here is in terminal cells, with any styling left out of the count.

var (
	edge     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	side     = edge.Render("│")
)

// fit cuts or pads s to exactly w cells.
func fit(s string, w int) string {
	if cells(s) > w {
		s = cut(s, max(w, 1), "…")
	}
	return s + strings.Repeat(" ", max(w-cells(s), 0))
}

// topEdge is a frame's top, w cells wide, with a label set in at either end:
// ╭─ Decks ──────── 4 decks ─╮
func topEdge(left, right string, w int) string {
	head := edge.Render("╭─")
	if left != "" {
		head += " " + left + " "
	}
	tail := edge.Render("─╮")
	if right != "" {
		tail = " " + right + " " + tail
	}
	return head + edge.Render(strings.Repeat("─", max(w-cells(head)-cells(tail), 0))) + tail
}

func bottomEdge(w int) string {
	return edge.Render("╰" + strings.Repeat("─", max(w-2, 0)) + "╯")
}

// inside is one line within a frame w cells wide.
func inside(line string, w int) string {
	return side + " " + fit(line, w-4) + " " + side
}

// card frames lines in a box w cells wide, with a blank line above and below
// them and title in the top edge. The lines get w-4 cells.
func card(title string, lines []string, w int) []string {
	return sideCard(title, "", lines, w)
}

// sideCard is a card with a second label, already styled, at the right of its
// top edge. It is dropped when the edge has no room for both.
func sideCard(title, right string, lines []string, w int) []string {
	if title != "" {
		title = bold.Render(truncate(title, w-8))
	}
	if cells(title)+cells(right)+10 > w { // the edge and its gaps, and a rule between
		right = ""
	}
	out := []string{topEdge(title, right, w), inside("", w)}
	for _, line := range lines {
		out = append(out, inside(line, w))
	}
	return append(out, inside("", w), bottomEdge(w))
}

// titleBar is the frame across the top of every screen: lernt, the context on
// the right (already styled), and on a run a line inside it with left and right
// halves.
func titleBar(context, left, right string, w int) []string {
	if context != "" {
		context = truncate(context, w-14)
	}
	out := []string{topEdge(accent.Render("lernt"), context, w)}
	if left != "" || right != "" {
		gap := max(w-4-cells(left)-cells(right), 1)
		out = append(out, inside(left+strings.Repeat(" ", gap)+right, w))
	}
	return append(out, bottomEdge(w))
}

// even pads the shorter of two cards' contents, so framed they stand as tall.
func even(a, b []string) ([]string, []string) {
	for len(a) < len(b) {
		a = append(a, "")
	}
	for len(b) < len(a) {
		b = append(b, "")
	}
	return a, b
}

// join sets two frames of one height side by side.
func join(a, b []string) []string {
	out := make([]string, len(a))
	for i := range a {
		out[i] = a[i] + " " + b[i]
	}
	return out
}

// keyBar lays out key and what-it-does pairs, wrapping to w cells:
//
//	enter run  m misses  q quit
func keyBar(keys []string, w int) []string {
	var lines []string
	line := ""
	for i := 0; i+1 < len(keys); i += 2 {
		part := accent.Render(keys[i]) + " " + faint.Render(keys[i+1])
		if line != "" && cells(line)+2+cells(part) > w {
			lines, line = append(lines, line), ""
		}
		if line == "" {
			line = " " + part
		} else {
			line += "  " + part
		}
	}
	return append(lines, line)
}

// eighths are the partial blocks a bar ends in, by eighths of a cell.
var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// bar draws n of most as up to w cells of blocks, to an eighth of a cell. Any n
// above zero gets at least a sliver.
func bar(n, most, w int) string {
	if n <= 0 {
		return ""
	}
	e := max(n*w*8/max(most, 1), 1)
	return strings.Repeat("█", e/8) + eighths[e%8]
}

// drain is the time left of a limit, as a bar w cells wide that empties while
// the card is asked: ━━━━━━━━──── - and all red once it is over.
func drain(seconds, limit, w int) string {
	if seconds > limit {
		return red.Render(strings.Repeat("─", w))
	}
	left := (limit - seconds) * w / limit
	return barStyle.Render(strings.Repeat("━", left)) + faint.Render(strings.Repeat("─", w-left))
}

// mmss is seconds as a clock: 1:05.
func mmss(seconds int) string {
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

// plural is "1 card", "3 cards".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
