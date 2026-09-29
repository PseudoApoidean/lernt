package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The highlight colour is the one that says where you are: the lit titles and labels,
// the bar on the row the cursor is on, the box being typed in, the bars of a chart.
// It is a setting with three modes. Terminal theme is one of the terminal's own 16
// colours, so it follows the terminal's theme. Custom is one from a list of the user's
// own, each a colour in whatever format they like and a name they gave it. Colour
// wheel is one picked on a square of colours (colourpicker.go).

// A choice is one of the terminal's 16 colours, by name.
type choice struct {
	name string
	code string // the ANSI colour number, as lipgloss takes it
}

// themeColours are the terminal's 16 colours in their order, named as everyone names
// them. What each looks like is the terminal theme's business.
var themeColours = func() []choice {
	base := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	var out []choice
	for i, name := range base {
		out = append(out, choice{name, strconv.Itoa(i)})
	}
	for i, name := range base {
		out = append(out, choice{"bright " + name, strconv.Itoa(8 + i)})
	}
	return out
}()

const defaultColour = "cyan"

var (
	hexColour    = regexp.MustCompile(`^#?([0-9a-f]{3}|[0-9a-f]{6})$`)
	numberColour = regexp.MustCompile(`^[0-9]{1,3}$`)
	rgbColour    = regexp.MustCompile(`^(?:rgb\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*\)|([0-9]{1,3})\s*[, ]\s*([0-9]{1,3})\s*[, ]\s*([0-9]{1,3}))$`)

	xtermStandard = [16][3]int{ // what a terminal draws colours 0 to 15 as, near enough
		{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
		{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
)

// normalColour reads a colour the way people write one, and gives it in the one form
// the settings keep: "#rrggbb" for a hex colour (#rgb, rrggbb and #RRGGBB too) and for
// rgb(255, 136, 0), 255,136,0 and 255 136 0; and a number from 0 to 255 as it is.
// False for anything else.
func normalColour(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case numberColour.MatchString(value):
		if n, _ := strconv.Atoi(value); n <= 255 {
			return strconv.Itoa(n), true
		}
	case hexColour.MatchString(value):
		hex := strings.TrimPrefix(value, "#")
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		return "#" + hex, true
	}
	if m := rgbColour.FindStringSubmatch(value); m != nil {
		var c [3]int
		for i := range c {
			part := m[i+1] // written rgb(1, 2, 3), or bare
			if part == "" {
				part = m[i+4]
			}
			c[i], _ = strconv.Atoi(part)
			if c[i] > 255 {
				return "", false
			}
		}
		return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]), true
	}
	return "", false
}

// resolveColour reads a colour setting: the name of one of the terminal's colours, or
// a colour in any form normalColour reads. It gives what to draw it as, and the colour
// of text that reads on a bar of it; false for anything else.
func resolveColour(value string) (code, on string, ok bool) {
	name := strings.ToLower(strings.TrimSpace(value))
	for _, c := range themeColours {
		if c.name == name {
			n, _ := strconv.Atoi(c.code)
			return c.code, textOn(ansi256(n)), true
		}
	}
	normal, ok := normalColour(value)
	if !ok {
		return "", "", false
	}
	if strings.HasPrefix(normal, "#") {
		n, _ := strconv.ParseUint(normal[1:], 16, 32)
		return normal, textOn([3]int{int(n >> 16), int(n >> 8 & 0xff), int(n & 0xff)}), true
	}
	n, _ := strconv.Atoi(normal)
	return normal, textOn(ansi256(n)), true
}

// ansi256 is the red, green and blue of one of the 256 colours.
func ansi256(n int) [3]int {
	switch {
	case n < 16:
		return xtermStandard[n]
	case n < 232: // the 6x6x6 cube
		level := func(i int) int { return []int{0, 95, 135, 175, 215, 255}[i] }
		n -= 16
		return [3]int{level(n / 36), level(n / 6 % 6), level(n % 6)}
	}
	grey := 8 + 10*(n-232)
	return [3]int{grey, grey, grey}
}

// luminance is how bright a colour looks, from 0 to 1, as the web accessibility
// rules work it out: from the light of each part, not the number it is written as.
func luminance(rgb [3]int) float64 {
	part := func(v int) float64 {
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*part(rgb[0]) + 0.7152*part(rgb[1]) + 0.0722*part(rgb[2])
}

// textOn is the text that reads on a bar of a colour: black or white, whichever
// stands out more from it.
func textOn(rgb [3]int) string {
	l := luminance(rgb)
	if (l+0.05)/0.05 >= 1.05/(l+0.05) { // the contrast with black, and with white
		return "0"
	}
	return "15"
}

// applyTheme sets the highlight colour, and every style made from it. An unreadable
// colour is the default. The styles are the ones in view.go and frame.go.
func applyTheme(value string) {
	code, on, ok := resolveColour(value)
	if !ok {
		code, on, _ = resolveColour(defaultColour)
	}
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color(code)).Bold(true)
	barOn = lipgloss.NewStyle().Foreground(lipgloss.Color(on)).Background(lipgloss.Color(code)).Bold(true)
	boxOn = lipgloss.NewStyle().Foreground(lipgloss.Color(on)).Background(lipgloss.Color(code))
	barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(code))
}

// swatch is a block of a colour, to show what it looks like: for a colour of the
// terminal's, in that terminal.
func swatch(value string) string {
	code, _, ok := resolveColour(value)
	if !ok {
		return "  "
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(code)).Render("██")
}
