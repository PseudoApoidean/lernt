package main

import (
	"slices"
	"strings"
	"testing"
)

func spansOf(text string, w int) []string {
	r := []rune(text)
	var out []string
	for _, s := range layoutText(r, w) {
		out = append(out, string(r[s.start:s.end]))
	}
	return out
}

func TestTextWrapsAtSpacesAndNewlines(t *testing.T) {
	cases := []struct {
		text string
		w    int
		want []string
	}{
		{"", 10, []string{""}},
		{"short", 10, []string{"short"}},
		{"one two three", 8, []string{"one two ", "three"}},
		{"a\nb", 10, []string{"a", "b"}},
		{"a\n", 10, []string{"a", ""}},
		{"\n\n", 10, []string{"", "", ""}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"日本語です", 4, []string{"日本", "語で", "す"}},
	}
	for _, c := range cases {
		if got := spansOf(c.text, c.w); !slices.Equal(got, c.want) {
			t.Errorf("%q at %d: %q, want %q", c.text, c.w, got, c.want)
		}
	}
}

func TestTheCursorIsOnTheLineThatFollowsAWrap(t *testing.T) {
	text := []rune("one two three")
	spans := layoutText(text, 8) // "one two " and "three"
	if rowOf(spans, 8) != 1 {
		t.Fatal("the cursor at the join is on the first line, not the second")
	}
	if rowOf(spans, 7) != 0 {
		t.Fatal("the cursor before the join is not on the first line")
	}
	if lineEnd(text, spans, 0) != 7 {
		t.Fatalf("a wrapped line's end is %d, not one short of the join", lineEnd(text, spans, 0))
	}
	if lineEnd(text, spans, 1) != len(text) {
		t.Fatal("the last line ends at the end of the text")
	}
}

func TestTheCursorMovesUpAndDownKeepingItsColumn(t *testing.T) {
	text := []rune("abcdef\nab\nabcdef")
	spans := layoutText(text, 20)
	pos := 5 // "abcde|f"
	pos = moveRow(text, spans, pos, 1)
	if pos != 9 { // the short line ends at 9
		t.Fatalf("down onto a short line: %d", pos)
	}
	pos = moveRow(text, spans, pos, 1)
	if pos != 12 { // column 2 of the last line
		t.Fatalf("down again: %d", pos)
	}
	if moveRow(text, spans, pos, 1) != pos {
		t.Fatal("down from the last line moved")
	}
	pos = moveRow(text, spans, 2, -1)
	if pos != 2 {
		t.Fatalf("up from the first line moved: %d", pos)
	}
}

func TestAnAreaScrollsToKeepTheCursorInViewAndSaysThereIsMore(t *testing.T) {
	text := []rune(strings.Join([]string{"one", "two", "three", "four", "five"}, "\n"))
	top := areaLines(text, 0, 12, 3, true, nil)
	if !strings.Contains(top[0], "one") || strings.Contains(top[0], "▴") || !strings.Contains(top[2], "▾") {
		t.Fatalf("top: %q", top)
	}
	end := areaLines(text, len(text), 12, 3, true, nil)
	if !strings.Contains(end[2], "five") || !strings.Contains(end[0], "▴") || strings.Contains(end[2], "▾") {
		t.Fatalf("end: %q", end)
	}
	for _, l := range append(top, end...) {
		if cells(l) != 12 {
			t.Fatalf("a line is %d cells, not 12: %q", cells(l), l)
		}
	}
}

// Characters of more than one rune: a skin tone, a joined family, an emoji made
// by a variation selector, and an accent made by a combining mark.
const (
	thumb  = "👍🏽"
	family = "👨‍👩‍👧"
	heart  = "❤️"
	acute  = "é"
)

func TestACharacterOfSeveralRunesIsOneUnit(t *testing.T) {
	for _, c := range []string{thumb, family, heart, acute, "😀", "日"} {
		text := []rune("a" + c + "b")
		mid := len([]rune("a"))
		end := mid + len([]rune(c))
		if got := nextBoundary(text, mid); got != end {
			t.Errorf("%q: right from before it lands at %d, not %d", c, got, end)
		}
		if got := prevBoundary(text, end); got != mid {
			t.Errorf("%q: left from after it lands at %d, not %d", c, got, mid)
		}
		if got := snap(text, end-1); len([]rune(c)) > 1 && got != mid {
			t.Errorf("%q: a cursor inside it snaps to %d, not %d", c, got, mid)
		}
	}
}

func TestEmojiAreMeasuredAsTheTerminalDrawsThem(t *testing.T) {
	for _, c := range []string{thumb, family, heart, "😀"} {
		if got := cells(c); got != 2 {
			t.Fatalf("%q is %d cells", c, got)
		}
		// Three cells of room: "a" and the emoji fit on a line, "b" wraps.
		if got := spansOf("a"+c+"b", 3); !slices.Equal(got, []string{"a" + c, "b"}) {
			t.Errorf("%q wraps as %q", c, got)
		}
		// Two cells: the emoji is not split from what it is joined to.
		if got := spansOf(c+c, 2); !slices.Equal(got, []string{c, c}) {
			t.Errorf("%q twice wraps as %q", c, got)
		}
	}
}

func TestEveryLineOfAnAreaIsItsWidthWhateverIsInIt(t *testing.T) {
	for _, c := range []string{thumb, family, heart, acute, "日本"} {
		text := []rune("say " + c + " and " + c + c + "\n" + c + " again " + c)
		for _, pos := range []int{0, 5, len(text) / 2, len(text)} {
			for _, l := range areaLines(text, pos, 12, 4, true, nil) {
				if cells(l) != 12 {
					t.Errorf("%q at %d: a line is %d cells, not 12: %q", c, pos, cells(l), l)
				}
			}
		}
	}
}

func TestTheCursorNeverLandsInsideACharacter(t *testing.T) {
	text := []rune("a" + family + "b" + thumb)
	spans := layoutText(text, 20)
	pos := 0
	for range 6 {
		pos = nextBoundary(text, pos)
		if snap(text, pos) != pos {
			t.Fatalf("right landed inside a character at %d", pos)
		}
	}
	if pos != len(text) {
		t.Fatalf("right stopped at %d of %d", pos, len(text))
	}
	if got := lineEnd(text, spans, 0); got != len(text) {
		t.Fatalf("the line ends at %d", got)
	}
}
