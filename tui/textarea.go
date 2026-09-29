package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

// A text area is text in a box of a fixed width and height: it wraps, it has a
// cursor that moves in every direction, and it scrolls to keep the cursor in
// view. The text is a []rune and the cursor an index into it; everything else is
// worked out from those two, so there is nothing to keep in step.

// A cluster is the runes [start, end) that are one character to a person: a
// letter and the accent on it, an emoji and its skin tone, a family joined into
// one. It is the unit the cursor moves by and a line is broken at, and width is
// the cells the terminal draws it in.
type cluster struct{ start, end, width int }

func clustersOf(text []rune) []cluster {
	var out []cluster
	g := uniseg.NewGraphemes(string(text))
	i := 0
	for g.Next() {
		n := len(g.Runes())
		out = append(out, cluster{i, i + n, clusterWidth(g.Str(), g.Width())})
		i += n
	}
	return out
}

// prevBoundary is the start of the character before pos, and nextBoundary the end
// of the one after it: where the cursor goes on left and right, and how much
// backspace and delete take away.
func prevBoundary(text []rune, pos int) int {
	at := 0
	for _, c := range clustersOf(text) {
		if c.start < pos {
			at = c.start
		}
	}
	return at
}

func nextBoundary(text []rune, pos int) int {
	for _, c := range clustersOf(text) {
		if c.end > pos {
			return c.end
		}
	}
	return len(text)
}

// snap is pos moved back to the start of the character it is in; a pos that is
// between two characters, or at the end, is where it was.
func snap(text []rune, pos int) int {
	for _, c := range clustersOf(text) {
		if c.start < pos && pos < c.end {
			return c.start
		}
	}
	return min(max(pos, 0), len(text))
}

// A span is the runes [start, end) of one line as drawn. The newline that ended
// it, if one did, is not in it.
type span struct{ start, end int }

// layoutText breaks text into the lines it is drawn on in a box w cells wide:
// at a newline, or where the next character would not fit, taking back to the last
// space if there was one. A space stays on the line it ends.
func layoutText(text []rune, w int) []span {
	w = max(w, 1)
	cs := clustersOf(text)
	startOf := func(k int) int {
		if k < len(cs) {
			return cs[k].start
		}
		return len(text)
	}
	var out []span
	first, width, lastSpace := 0, 0, -1 // the first cluster on the line, its width so far, its last space
	for i, c := range cs {
		if text[c.start] == '\n' {
			out = append(out, span{startOf(first), c.start})
			first, width, lastSpace = i+1, 0, -1
			continue
		}
		if width+c.width > w && i > first { // wrapped
			brk := i
			if lastSpace >= first {
				brk = lastSpace + 1
			}
			out = append(out, span{startOf(first), startOf(brk)})
			first, width, lastSpace = brk, 0, -1
			for _, k := range cs[first:i] {
				width += k.width
			}
		}
		width += c.width
		if text[c.start] == ' ' {
			lastSpace = i
		}
	}
	return append(out, span{startOf(first), len(text)})
}

// rowOf is the line the cursor is drawn on: the last that starts at or before
// it, so the cursor at the join of a wrapped line is on the line that follows.
func rowOf(spans []span, pos int) int {
	row := 0
	for i, s := range spans {
		if s.start <= pos {
			row = i
		}
	}
	return row
}

// lineEnd is the furthest the cursor can be on a line: its end where a newline or
// the end of the text follows, and the start of its last character where the line
// wrapped, since past that it is on the next line.
func lineEnd(text []rune, spans []span, row int) int {
	if row+1 < len(spans) && spans[row+1].start == spans[row].end {
		return max(prevBoundary(text, spans[row].end), spans[row].start)
	}
	return spans[row].end
}

// moveRow is pos moved by dir lines, up (-1) or down (1), to the same column, or
// the end of a line that is shorter. At the first or last line it does not move.
func moveRow(text []rune, spans []span, pos, dir int) int {
	row := rowOf(spans, pos)
	col := cells(string(text[spans[row].start:pos]))
	to := row + dir
	if to < 0 || to >= len(spans) {
		return pos
	}
	limit := lineEnd(text, spans, to)
	at, width := spans[to].start, 0
	for _, c := range clustersOf(text) {
		if c.start < spans[to].start {
			continue
		}
		if c.start >= limit || width+c.width > col {
			break
		}
		width += c.width
		at = c.end
	}
	return at
}

// areaLines draws text as rows lines of w cells: the cursor as the character it
// is on, reversed (when there is one), and the text scrolled to keep it in view.
// Where more lies above or below, the last two cells of the first or last line say
// so. fill, if there is one, is the background of every cell, for a box that is
// being typed in.
func areaLines(text []rune, pos, w, rows int, showCursor bool, fill *lipgloss.Style) []string {
	paint := func(s string) string {
		if fill != nil {
			return fill.Render(s)
		}
		return s
	}
	spans := layoutText(text, w)
	row := 0
	if showCursor {
		row = rowOf(spans, pos)
	}
	first := 0
	if row >= rows {
		first = row - rows + 1
	}
	lines := make([]string, rows)
	for r := range rows {
		i := first + r
		tail := ""
		switch {
		case r == 0 && first > 0:
			tail = " ▴"
		case r == rows-1 && first+rows < len(spans):
			tail = " ▾"
		}
		room := w - cells(tail)
		if i >= len(spans) {
			lines[r] = paint(strings.Repeat(" ", w))
			continue
		}
		s := spans[i]
		before, at, after := string(text[s.start:s.end]), "", ""
		if showCursor && i == row {
			p := min(snap(text, pos), s.end)
			before, at = string(text[s.start:p]), " "
			if p < s.end {
				n := min(nextBoundary(text, p), s.end)
				at, after = string(text[p:n]), string(text[n:s.end])
			}
		}
		after = cut(after, max(room-cells(before)-cells(at), 0), "")
		if cells(before)+cells(at) > room {
			before = cut(before, max(room-cells(at), 0), "")
		}
		pad := max(room-cells(before)-cells(at)-cells(after), 0)
		line := paint(before)
		if at != "" {
			line += cursor.Render(at)
		}
		line += paint(after + strings.Repeat(" ", pad))
		if tail != "" {
			if fill != nil { // the arrow is part of the box, so it has the box's fill
				line += fill.Faint(true).Render(tail)
			} else {
				line += faint.Render(tail)
			}
		}
		lines[r] = line
	}
	return lines
}
