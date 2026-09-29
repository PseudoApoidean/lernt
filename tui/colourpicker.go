package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The colour wheel is a picker in the style of a paint program: a strip of hues, and
// under it a square of that hue going from pale to full across and from bright to
// dark down. A terminal draws a picture with a half block, ▀, whose top is one colour
// and whose bottom is another, so each cell of the square is two rows of pixels.
// It needs a terminal that draws 24-bit colour to look smooth, and the colour it picks
// is a hex colour like any other. The highlight changes as the cursor moves, so what
// is picked is seen at once, and esc puts back what was there.

const (
	pickCols   = 36 // the square's width in cells, and the hue strip's
	pickRows   = 8  // the square's height in cells
	pickPixels = pickRows * 2
)

type colourPicker struct {
	hue   int  // 0 to 359
	x, y  int  // the cursor in the square: saturation across, brightness down (0 is brightest)
	strip bool // the keys move the hue, not the cursor
}

// newColourPicker starts on a colour, or on cyan if it cannot be read.
func newColourPicker(value string) *colourPicker {
	c := &colourPicker{}
	code, _, ok := resolveColour(value)
	if !ok {
		code, _, _ = resolveColour(defaultColour)
	}
	c.hue, c.x, c.y = hueOf(rgbOf(code))
	return c
}

// rgbOf is a colour as it is drawn, from a hex colour or a number from 0 to 255.
func rgbOf(code string) [3]int {
	if strings.HasPrefix(code, "#") {
		var r, g, b int
		fmt.Sscanf(code[1:], "%02x%02x%02x", &r, &g, &b)
		return [3]int{r, g, b}
	}
	var n int
	fmt.Sscanf(code, "%d", &n)
	return ansi256(n)
}

// hueOf is where a colour is on the picker: its hue, and the square's x and y.
func hueOf(rgb [3]int) (hue, x, y int) {
	r, g, b := float64(rgb[0])/255, float64(rgb[1])/255, float64(rgb[2])/255
	most, least := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	delta := most - least
	var h float64
	switch {
	case delta == 0:
		h = 0
	case most == r:
		h = math.Mod((g-b)/delta, 6)
	case most == g:
		h = (b-r)/delta + 2
	default:
		h = (r-g)/delta + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	s := 0.0
	if most > 0 {
		s = delta / most
	}
	return int(math.Round(h)) % 360, int(math.Round(s * (pickCols - 1))), int(math.Round((1 - most) * (pickPixels - 1)))
}

// hsv is a hue in degrees, and a saturation and a brightness from 0 to 1, as the hex
// colour it is.
func hsv(h, s, v float64) string {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch int(h / 60) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to := func(f float64) int { return int(math.Round((f + m) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", to(r), to(g), to(b))
}

// pixel is the colour of the square at a place in it.
func (c *colourPicker) pixel(x, y int) string {
	return hsv(float64(c.hue), float64(x)/(pickCols-1), 1-float64(y)/(pickPixels-1))
}

// cursorCell is the cell of the square the cursor is in, and which half of it: 0 for
// the top and 1 for the bottom. Each cell is two rows of pixels, and the cursor moves
// a pixel at a time.
func (c *colourPicker) cursorCell() (row, half int) { return c.y / 2, c.y % 2 }

// hex is the colour picked.
func (c *colourPicker) hex() string { return c.pixel(c.x, c.y) }

// move moves the cursor in the square, or the hue along the strip, and does not go
// past an edge.
func (c *colourPicker) move(dx, dy int) {
	if c.strip {
		c.turn(5 * dx)
		return
	}
	c.x = max(min(c.x+dx, pickCols-1), 0)
	c.y = max(min(c.y+dy, pickPixels-1), 0)
}

// turn changes the hue by degrees, round the wheel.
func (c *colourPicker) turn(degrees int) { c.hue = ((c.hue+degrees)%360 + 360) % 360 }

// lines is the picker w cells wide: the hue strip with a marker over the hue, the
// square with the cursor on it, and the colour picked.
func (c *colourPicker) lines(w int) []string {
	indent := strings.Repeat(" ", max((w-pickCols)/2, 0))
	zone := func(active bool) string {
		if active {
			return accent.Render("▌")
		}
		return " "
	}
	marker := make([]string, pickCols)
	for i := range marker {
		marker[i] = " "
	}
	marker[min(c.hue*pickCols/360, pickCols-1)] = accent.Render("▼")
	var strip strings.Builder
	for i := range pickCols {
		strip.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hsv(float64(i)*360/pickCols+360/pickCols/2, 1, 1))).Render("█"))
	}
	lines := []string{
		indent + " " + strings.Join(marker, ""),
		indent + zone(c.strip) + strip.String(),
		"",
	}
	row, half := c.cursorCell()
	_, mark, _ := resolveColour(c.hex()) // black or white, whichever stands out on the colour
	for r := range pickRows {
		var out strings.Builder
		for x := range pickCols {
			top, bottom := c.pixel(x, 2*r), c.pixel(x, 2*r+1)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(top)).Background(lipgloss.Color(bottom))
			cell := "▀"
			switch {
			case x == c.x && r == row && half == 0: // the cursor is the top pixel: it marks the top half
				style = lipgloss.NewStyle().Foreground(lipgloss.Color(mark)).Background(lipgloss.Color(bottom))
			case x == c.x && r == row: // or the bottom one: the bottom half, with the top as it is
				style = lipgloss.NewStyle().Foreground(lipgloss.Color(mark)).Background(lipgloss.Color(top))
				cell = "▄"
			}
			out.WriteString(style.Render(cell))
		}
		lines = append(lines, indent+zone(!c.strip)+out.String())
	}
	picked := c.hex()
	return append(lines, "", indent+" "+swatch(picked)+swatch(picked)+swatch(picked)+"  "+picked)
}

// trueColour is whether the terminal says it draws 24-bit colour. Under tmux it is
// what tmux was started with.
func trueColour() bool {
	v := strings.ToLower(os.Getenv("COLORTERM"))
	return strings.Contains(v, "truecolor") || strings.Contains(v, "24bit")
}
