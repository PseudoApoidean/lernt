package main

import (
	"strings"
	"testing"
)

func TestWidthsAreTheWiderWhereTerminalsAndTablesDiffer(t *testing.T) {
	for _, c := range []struct {
		s    string
		want int
	}{
		{"a", 1}, {"é", 1}, {"é", 1}, {"日", 2}, {"日本", 4},
		{"🔴", 2}, {"👍🏽", 2}, {"👨‍👩‍👧", 2}, {"🇬🇷", 2}, {"🫠", 2}, {"🧑‍💻", 2},
		{"❤️", 2}, {"✔️", 2}, {"☺️", 2},
		{"1️⃣", 2}, // the tables say 1, and tmux draws 2
		{"🏻", 2},   // a lone skin tone: the tables say 0
		{"☝", 2}, {"☰", 2}, {"〰", 2},
		{"🏋", 2}, // an emoji the tables give 1 cell without a selector, and tmux draws 2
		{"\x1b[7m \x1b[0m", 1},
		{"\x1b[30;46mhi 🔴\x1b[0m", 5},
		{"", 0},
	} {
		if got := cells(c.s); got != c.want {
			t.Errorf("%q is %d cells, not %d", c.s, got, c.want)
		}
	}
}

func TestCutNeverGoesOverAndNeverSplitsACharacter(t *testing.T) {
	for _, s := range []string{
		"abcdefghij", "日本語日本語", "a🔴b🔴c🔴d", "x1️⃣y1️⃣z", "👨‍👩‍👧👨‍👩‍👧👨‍👩‍👧", "é́é́é́é́",
		"\x1b[1mbold 🔴 text that is long\x1b[0m", "🏋🏋🏋🏋",
	} {
		for w := 0; w <= 12; w++ {
			for _, tail := range []string{"", "…"} {
				got := cut(s, w, tail)
				if cells(got) > w && cells(s) > w {
					t.Errorf("cut(%q, %d, %q) = %q is %d cells", s, w, tail, got, cells(got))
				}
				if cells(s) <= w && got != s {
					t.Errorf("cut(%q, %d) changed text that fit: %q", s, w, got)
				}
				if strings.ContainsRune(got, 0xFFFD) {
					t.Errorf("cut(%q, %d) split a character: %q", s, w, got)
				}
			}
		}
	}
	if got := cut("abcdef", 4, "…"); got != "abc…" {
		t.Errorf("the tail takes a cell: %q", got)
	}
	if got := cut("\x1b[1mabcdef\x1b[0m", 4, "…"); !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("a cut style is not closed: %q", got)
	}
}

func TestFitIsExactlyTheWidthWhateverIsInIt(t *testing.T) {
	for _, s := range []string{"a", "🔴", "1️⃣", "a1️⃣b", "🏋 x", "日本語です", "👨‍👩‍👧 family"} {
		for _, w := range []int{1, 2, 3, 5, 10} {
			if got := cells(fit(s, w)); got != w {
				t.Errorf("fit(%q, %d) is %d cells", s, w, got)
			}
		}
	}
}

func TestEveryLineOfAnAreaIsItsWidthWithKeycapsAndModifiers(t *testing.T) {
	for _, c := range []string{"1️⃣", "🏻", "🏋", "❤️"} {
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

func TestOnlyReferencesToNonASCIICharactersAreDecoded(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"**&#x1f534; Hardware-in-the-loop**", "**🔴 Hardware-in-the-loop**"},
		{"&#x1F7E0; and &#128308;", "🟠 and 🔴"},
		{"&#233;t&#xE9;", "été"},
		{"&#60;div&#62; and &lt;b&gt; and &amp;", "&#60;div&#62; and &lt;b&gt; and &amp;"}, // not characters beyond ASCII
		{"&#x41; &#65;", "&#x41; &#65;"},
		{"&#128;&#x9f;", "&#128;&#x9f;"},                                         // the C1 controls stay as written
		{"&#xD800; &#x110000; &#xffffffff;", "&#xD800; &#x110000; &#xffffffff;"}, // not characters at all
		{"&#x1f534 no semicolon, &#xZZ;, &#;", "&#x1f534 no semicolon, &#xZZ;, &#;"},
		{"plain", "plain"},
		{"", ""},
	} {
		if got := decodeReferences(c.in); got != c.want {
			t.Errorf("%q decodes to %q, not %q", c.in, got, c.want)
		}
	}
}
