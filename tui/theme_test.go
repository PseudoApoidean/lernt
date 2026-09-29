package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"lernt/tui/engine"
)

func TestTheThemeColoursAreTheTerminalsSixteenInOrder(t *testing.T) {
	want := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"bright black", "bright red", "bright green", "bright yellow", "bright blue",
		"bright magenta", "bright cyan", "bright white"}
	if len(themeColours) != 16 {
		t.Fatalf("%d colours", len(themeColours))
	}
	for i, c := range themeColours {
		if c.name != want[i] {
			t.Errorf("colour %d is %q, not %q", i, c.name, want[i])
		}
		if code, _, ok := resolveColour(c.name); !ok || code != c.code {
			t.Errorf("%q resolves to %q (%v)", c.name, code, ok)
		}
	}
	if code, _, _ := resolveColour("  Bright Blue "); code != "12" {
		t.Errorf("a name is read in any case and with spaces round it: %q", code)
	}
	if w := themeWheel(); w.label() != defaultColour || len(w.labels) != 16 {
		t.Fatalf("the theme wheel starts on %q with %d colours", w.label(), len(w.labels))
	}
}

func TestAColourIsReadInTheFormsPeopleWriteOne(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"#ff8800", "#ff8800"}, {"ff8800", "#ff8800"}, {"#FF8800", "#ff8800"}, {"#F80", "#ff8800"}, {"f80", "#ff8800"},
		{" #ff8800 ", "#ff8800"},
		{"255,136,0", "#ff8800"}, {"255, 136, 0", "#ff8800"}, {"255 136 0", "#ff8800"},
		{"rgb(255,136,0)", "#ff8800"}, {"rgb( 255, 136, 0 )", "#ff8800"}, {"RGB(255,136,0)", "#ff8800"},
		{"208", "208"}, {"0", "0"}, {"255", "255"}, {"016", "16"},
	} {
		got, ok := normalColour(c.in)
		if !ok || got != c.want {
			t.Errorf("%q is %q (%v), not %q", c.in, got, ok, c.want)
		}
	}
	for _, bad := range []string{"", "  ", "256", "999", "1000", "chartreuse", "#12", "#12345", "#1234567", "#gggggg",
		"-1", "6 7", "#", "256,0,0", "1,2", "1,2,3,4", "rgb(1,2)", "rgb(1,2,3", "rgb 1,2,3"} {
		if got, ok := normalColour(bad); ok {
			t.Errorf("%q was read as %q", bad, got)
		}
	}
}

func TestTheTextOnABarReadsOnIt(t *testing.T) {
	// Black or white, whichever stands out more, by the light of the colour.
	for _, c := range []struct{ colour, on string }{
		{"cyan", "0"}, {"green", "0"}, {"yellow", "0"}, {"white", "0"}, {"bright yellow", "0"}, {"bright green", "0"},
		{"bright cyan", "0"}, {"bright white", "0"}, {"bright red", "0"},
		{"black", "15"}, {"blue", "15"}, {"red", "15"}, {"magenta", "15"}, {"bright blue", "15"},
		{"bright black", "0"}, // a mid grey: black stands out a little more
		{"#000000", "15"}, {"#ffffff", "0"}, {"#ff8800", "0"}, {"#000080", "15"}, {"#800000", "15"}, {"#ffff00", "0"},
		{"0", "15"}, {"15", "0"}, {"11", "0"}, {"4", "15"},
		{"208", "0"}, {"17", "15"}, {"232", "15"}, {"255", "0"},
	} {
		if _, on, ok := resolveColour(c.colour); !ok || on != c.on {
			t.Errorf("text on %s is %q, not %q", c.colour, on, c.on)
		}
	}
}

func TestApplyingAColourChangesEveryStyleMadeFromIt(t *testing.T) {
	defer applyTheme(defaultColour)
	applyTheme("blue")
	if accent.GetForeground() != lipgloss.Color("4") || barOn.GetBackground() != lipgloss.Color("4") ||
		boxOn.GetBackground() != lipgloss.Color("4") || barStyle.GetForeground() != lipgloss.Color("4") {
		t.Fatal("a style did not take the colour")
	}
	if barOn.GetForeground() != lipgloss.Color("15") || boxOn.GetForeground() != lipgloss.Color("15") {
		t.Fatal("the text on a blue bar is not white")
	}
	applyTheme("#ff8800")
	if accent.GetForeground() != lipgloss.Color("#ff8800") || barOn.GetForeground() != lipgloss.Color("0") {
		t.Fatal("a custom colour was not applied, with black text on it")
	}
	applyTheme("no such colour") // the default, not nothing
	if accent.GetForeground() != lipgloss.Color("6") || barOn.GetBackground() != lipgloss.Color("6") {
		t.Fatal("an unreadable colour is not the default")
	}
}

func TestASwatchIsTheColourItself(t *testing.T) {
	if got := swatch("no such colour"); got != "  " {
		t.Fatalf("a swatch of nothing is %q", got)
	}
	if cells(swatch("red")) != 2 || cells(swatch("#ff8800")) != 2 || !strings.Contains(swatch("red"), "██") {
		t.Fatal("a swatch is not two cells of block")
	}
}

func TestTheCustomListIsAddedToAndTakenFromNeverEdited(t *testing.T) {
	p := defaultPrefs()
	if err := p.addCustom("  Sunset  glow ", "255,136,0"); err != nil {
		t.Fatal(err)
	}
	if got := p.custom[0]; got.Name != "Sunset glow" || got.Value != "#ff8800" || p.customAt != 0 {
		t.Fatalf("added %+v, chosen %d", got, p.customAt)
	}
	if err := p.addCustom("Sky", "#0af"); err != nil || p.customAt != 1 || p.custom[1].Value != "#00aaff" {
		t.Fatalf("a second: %v, chosen %d, %+v", err, p.customAt, p.custom)
	}
	for _, bad := range []struct{ name, value, why string }{
		{"", "#ff8800", "A colour needs a name."},
		{"   ", "#ff8800", "A colour needs a name."},
		{strings.Repeat("x", 25), "#ff8800", "24 characters"},
		{"sunset GLOW", "#123456", "already a colour called"},
		{"New", "banana", "not a colour"},
		{"New", "256", "not a colour"},
	} {
		before := len(p.custom)
		err := p.addCustom(bad.name, bad.value)
		if err == nil || !strings.Contains(err.Error(), bad.why) || len(p.custom) != before {
			t.Errorf("%q %q: %v (list %d, was %d)", bad.name, bad.value, err, len(p.custom), before)
		}
	}
	// Taking one off chooses its neighbour; the list never changes a colour in place.
	p.customAt = 0
	p.deleteCustom()
	if len(p.custom) != 1 || p.custom[0].Name != "Sky" || p.customAt != 0 {
		t.Fatalf("after removing the first: %+v, chosen %d", p.custom, p.customAt)
	}
	p.deleteCustom()
	if len(p.custom) != 0 || p.customAt != 0 {
		t.Fatalf("after removing the last: %+v, chosen %d", p.custom, p.customAt)
	}
	p.deleteCustom() // nothing to take: no harm
	if p.highlight() != defaultColour {
		t.Fatalf("with an empty list the highlight is %q", p.highlight())
	}
}

func TestTheHighlightIsThatOfTheMode(t *testing.T) {
	p := defaultPrefs()
	if p.highlight() != "cyan" {
		t.Fatalf("theme: %q", p.highlight())
	}
	p.theme.at = 4
	if p.highlight() != "blue" {
		t.Fatalf("theme: %q", p.highlight())
	}
	p.colourMode = customMode
	if p.highlight() != defaultColour {
		t.Fatalf("custom with none: %q", p.highlight())
	}
	_ = p.addCustom("Sunset", "#ff8800")
	if p.highlight() != "#ff8800" {
		t.Fatalf("custom: %q", p.highlight())
	}
	p.colourMode = wheelMode
	p.picked = "#123456"
	if p.highlight() != "#123456" {
		t.Fatalf("wheel: %q", p.highlight())
	}
}

func TestTheColourSettingsAreSavedAndReadBack(t *testing.T) {
	p := defaultPrefs()
	p.colourMode = customMode
	p.theme.at = 9
	_ = p.addCustom("Sunset", "#ff8800")
	_ = p.addCustom("Sky", "0af")
	p.customAt = 0
	p.picked = "#123456"
	saved := p.saved(classic)

	q := defaultPrefs()
	q.restore(saved)
	if q.colourMode != customMode || q.theme.label() != "bright red" || len(q.custom) != 2 ||
		q.custom[q.customAt].Name != "Sunset" || q.picked != "#123456" || q.custom[1].Value != "#00aaff" {
		t.Fatalf("read back %+v", q)
	}
	// The choice is read after the list, whatever order they come in.
	q = defaultPrefs()
	rev := append([]engine.Option(nil), saved...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	q.restore(rev)
	if q.custom[q.customAt].Name != "Sunset" {
		t.Fatalf("the choice was read before the list: %d of %+v", q.customAt, q.custom)
	}
	// What cannot be read is passed over.
	q = defaultPrefs()
	q.restore([]engine.Option{
		{Key: keyColourMode, Value: "sparkly"}, {Key: keyColourTheme, Value: "chartreuse"},
		{Key: keyColourCustom, Value: "not json"}, {Key: keyColourWheel, Value: "banana"},
		{Key: keyColourChoice, Value: "Nobody"},
	})
	if q.colourMode != themeMode || q.theme.label() != "cyan" || len(q.custom) != 0 || q.picked != defaultPickedColour {
		t.Fatalf("an unreadable value changed %+v", q)
	}
	// A list with bad entries keeps the good ones: no name, a colour that is not, a name twice.
	q = defaultPrefs()
	q.restore([]engine.Option{{Key: keyColourCustom, Value: `[{"name":"A","value":"#ff0000"},{"name":"","value":"#00ff00"},` +
		`{"name":"B","value":"nonsense"},{"name":"a","value":"#0000ff"},{"name":"C","value":"12,34,56"}]`}})
	if len(q.custom) != 2 || q.custom[0].Name != "A" || q.custom[1].Name != "C" || q.custom[1].Value != "#0c2238" {
		t.Fatalf("read %+v", q.custom)
	}
}

func TestTheWheelPicksFromHueSaturationAndBrightness(t *testing.T) {
	for _, c := range []struct {
		h, s, v float64
		want    string
	}{
		{0, 1, 1, "#ff0000"}, {120, 1, 1, "#00ff00"}, {240, 1, 1, "#0000ff"}, {60, 1, 1, "#ffff00"},
		{180, 1, 1, "#00ffff"}, {300, 1, 1, "#ff00ff"}, {0, 0, 1, "#ffffff"}, {0, 0, 0, "#000000"},
		{360, 1, 1, "#ff0000"}, {-60, 1, 1, "#ff00ff"}, {0, 0, 0.5, "#808080"},
	} {
		if got := hsv(c.h, c.s, c.v); got != c.want {
			t.Errorf("hsv(%v, %v, %v) = %s, not %s", c.h, c.s, c.v, got, c.want)
		}
	}
}

func TestThePickerStartsWhereTheColourIs(t *testing.T) {
	near := func(a, b string) bool {
		x, y := rgbOf(a), rgbOf(b)
		for i := range x {
			if d := x[i] - y[i]; d > 24 || d < -24 {
				return false
			}
		}
		return true
	}
	for _, colour := range []string{"#ff0000", "#00ff00", "#0000ff", "#ffffff", "#000000", "#00cdcd", "#ff8800", "cyan", "208", "bright red"} {
		code, _, _ := resolveColour(colour)
		want := code
		if !strings.HasPrefix(code, "#") { // a terminal colour, as the colour it is drawn as
			want = "#" + hexOf(rgbOf(code))
		}
		if got := newColourPicker(colour).hex(); !near(got, want) {
			t.Errorf("%s: the picker starts on %s, not near %s", colour, got, want)
		}
	}
	if got := newColourPicker("nonsense").hex(); !near(got, "#00cdcd") {
		t.Errorf("an unreadable colour starts on %s", got)
	}
	// Pure colours are exact.
	if got := newColourPicker("#ff0000").hex(); got != "#ff0000" {
		t.Errorf("red is %s", got)
	}
}

func hexOf(rgb [3]int) string {
	const digits = "0123456789abcdef"
	var b strings.Builder
	for _, v := range rgb {
		b.WriteByte(digits[v>>4])
		b.WriteByte(digits[v&15])
	}
	return b.String()
}

func TestThePickerMovesAndStopsAtItsEdges(t *testing.T) {
	c := newColourPicker("#ff0000") // the top right corner of red
	if c.x != pickCols-1 || c.y != 0 {
		t.Fatalf("red is at %d,%d", c.x, c.y)
	}
	c.move(1, 0)
	c.move(0, -1)
	if c.x != pickCols-1 || c.y != 0 {
		t.Fatalf("it went past the corner: %d,%d", c.x, c.y)
	}
	c.move(-2, 3)
	if c.x != pickCols-3 || c.y != 3 {
		t.Fatalf("moved to %d,%d", c.x, c.y)
	}
	c.move(-100, 100)
	if c.x != 0 || c.y != pickPixels-1 || c.hex() != "#000000" {
		t.Fatalf("the bottom left is %d,%d %s, not black", c.x, c.y, c.hex())
	}
	c.strip = true
	c.hue = 0
	c.move(1, 0) // along the strip: the hue, not the cursor
	if c.hue != 5 || c.x != 0 {
		t.Fatalf("on the strip: hue %d, x %d", c.hue, c.x)
	}
	c.move(-2, 0)
	if c.hue != 355 {
		t.Fatalf("the hue goes round the wheel: %d", c.hue)
	}
	c.turn(10)
	c.turn(-1)
	if c.hue != 4 {
		t.Fatalf("turned to %d", c.hue)
	}
}

func TestThePickerDrawsTheStripTheSquareAndTheColour(t *testing.T) {
	c := newColourPicker("#ff8800")
	for _, w := range []int{40, 60, 68} {
		lines := c.lines(w)
		if len(lines) != 3+pickRows+2 {
			t.Fatalf("%d lines", len(lines))
		}
		text := strings.Join(lines, "\n")
		if strings.Count(text, "▼") != 1 || !strings.Contains(text, "▀") || !strings.Contains(text, c.hex()) {
			t.Fatalf("width %d:\n%s", w, text)
		}
		for _, l := range lines {
			if cells(l) > w {
				t.Errorf("a line is %d cells in %d: %q", cells(l), w, l)
			}
		}
	}
}

func TestTheCursorMovesAPixelAtATimeAndShowsWhichHalfOfTheCell(t *testing.T) {
	c := newColourPicker("#ff0000")
	c.x, c.y = 10, 0
	half := func() string { // the cursor is the bottom half, ▄, or the top, which is drawn as ▀
		text := strings.Join(c.lines(60), "\n")
		if n := strings.Count(text, "▄"); n > 1 {
			t.Fatalf("%d bottom-half marks", n)
		} else if n == 1 {
			return "bottom"
		}
		return "top"
	}
	var seen []string
	for range 6 {
		row, h := c.cursorCell()
		seen = append(seen, half()+":"+string(rune('0'+row)))
		if (h == 1) != (half() == "bottom") {
			t.Fatalf("y %d is half %d but the square draws the %s", c.y, h, half())
		}
		c.move(0, 1)
	}
	want := []string{"top:0", "bottom:0", "top:1", "bottom:1", "top:2", "bottom:2"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("moving down a pixel at a time went %v, not %v", seen, want)
		}
	}
	// Up the same way: every press moves the mark, half a cell.
	seen = nil
	for range 4 {
		row, _ := c.cursorCell()
		seen = append(seen, half()+":"+string(rune('0'+row)))
		c.move(0, -1)
	}
	if strings.Join(seen, " ") != "top:3 bottom:2 top:2 bottom:1" && strings.Join(seen, " ") != "bottom:2 top:2 bottom:1 top:1" {
		t.Fatalf("moving up went %v", seen)
	}
	// And the mark is in the right column, in the square, not the strip.
	c.x, c.y = 3, 5
	for _, line := range c.lines(60)[3:] {
		if strings.Contains(line, "▄") {
			if col := strings.Index(line, "▄"); cells(line[:col]) != (60-pickCols)/2+1+3 {
				t.Fatalf("the mark is %d cells in", cells(line[:col]))
			}
		}
	}
}
