package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lernt/tui/engine"
)

var (
	bold   = lipgloss.NewStyle().Bold(true)
	faint  = lipgloss.NewStyle().Faint(true)
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	// The cursor's row in a list: a bar across it. Bright in the pane that has
	// the keys, dull in the one that does not.
	barOn = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Bold(true)
	// The other pane's is only a tint, to say where the cursor is without
	// competing with the pane that has the keys.
	barOff = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "238"})
	// The text cursor in a form: the character it is on, reversed.
	cursor = lipgloss.NewStyle().Reverse(true)
	// A box that is being typed in, all of it: the same colours as a selected option.
	boxOn = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6"))
)

// A page is one screen laid out: the title bar, then either panes already
// framed or one card whose body scrolls, then the footer and its key bar.
type page struct {
	context     string   // the title bar's right-hand label
	tabbed      bool     // instead of the context, the tabs, this screen's lit
	left, right string   // a line inside the title bar: the run's progress
	panes       []string // framed cards, drawn as they are; none: the card below
	title       string   // the card's title
	narrow      bool     // the card is centred at reading width, not full width
	pinned      []string // inside the card, above the body, never scrolled
	body        []string // inside the card, scrolled when it does not fit
	below       []string // under the card
	keys        []string // the key bar: a key, what it does, a key, ...
}

// head is what the title bar carries on its right: the tabs on a tab screen,
// else the page's context.
func (p page) head(at screen) string {
	switch {
	case p.tabbed:
		return tabBar(at)
	case p.context != "":
		return bold.Render(p.context)
	}
	return ""
}

// tabBar is the screens along the top, the one you are on lit, each with the
// key that goes there:  1 Decks │ 2 Library │ 3 Settings
func tabBar(at screen) string {
	var parts []string
	for i, t := range tabs {
		label := strconv.Itoa(i+1) + " " + t.name
		if t.screen == at {
			parts = append(parts, accent.Render(label))
		} else {
			parts = append(parts, faint.Render(label))
		}
	}
	return strings.Join(parts, faint.Render(" │ "))
}

func (m model) View() string {
	p := m.page()
	w := m.frameWidth()
	lines := append(titleBar(p.head(m.screen), p.left, p.right, w), "")
	if p.panes != nil {
		lines = append(lines, p.panes...)
	} else {
		body, below := p.body, p.below
		if room := m.room(p); room > 0 && len(body) > room {
			offset := max(min(m.offset, len(body)-room), 0)
			body = body[offset : offset+room]
			below = append(slices.Clone(below), faint.Render(fmt.Sprintf(
				"lines %d-%d of %d · j/k to scroll", offset+1, offset+room, len(p.body))))
		}
		cw := m.cardWidth(p.narrow)
		indent := strings.Repeat(" ", (w-cw)/2)
		for _, line := range card(p.title, append(slices.Clone(p.pinned), body...), cw) {
			lines = append(lines, indent+line)
		}
		for _, line := range below {
			lines = append(lines, indent+" "+line)
		}
	}
	foot := m.footer(p.keys)
	// The footer is the last lines of the window: the page ends where it ends, and
	// what is left over is between the two, not below the footer.
	for len(lines)+len(foot) < m.height {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, foot...), "\n")
}

// room is how many body lines fit in the card, with everything else on the
// page drawn and a line kept for the scroll line. 0: the height is not known.
func (m model) room(p page) int {
	if m.height == 0 {
		return 0
	}
	used := len(titleBar(p.head(m.screen), p.left, p.right, m.frameWidth())) + 1 + 4 +
		len(p.pinned) + len(p.below) + 1 + len(m.footer(p.keys))
	return max(m.height-used, 1)
}

// frameWidth is the width everything is drawn in: the whole window. A page that
// reads better narrow (a question and its answer) sets its own width.
func (m model) frameWidth() int {
	if m.width == 0 {
		return 80
	}
	return max(m.width, 30)
}

// cardWidth is a page's card: the full frame, or reading width when narrow.
func (m model) cardWidth(narrow bool) int {
	if narrow {
		return min(m.frameWidth(), 72)
	}
	return m.frameWidth()
}

// inner is the width of the text in a page's card.
func (m model) inner(narrow bool) int { return m.cardWidth(narrow) - 4 }

func (m model) page() page {
	switch m.screen {
	case optionsScreen:
		return m.optionsPage()
	case askScreen:
		return m.askPage()
	case revealScreen:
		return m.revealPage()
	case editScreen:
		return m.editPage()
	case menuScreen:
		return m.menuPage()
	case summaryScreen:
		return m.summaryPage()
	case missesScreen:
		return m.missesPage()
	case boxesScreen:
		return m.boxesPage()
	case editsScreen:
		return m.editsPage()
	case sourcesScreen:
		return m.sourcesPage()
	case settingsScreen:
		return m.settingsPage()
	case libraryScreen:
		return m.libraryPage()
	}
	return m.decksPage()
}

// tmuxWarning is shown on the main screen under tmux. tmux does not say how it
// draws emoji (it does not answer the query for grapheme clustering, mode 2027),
// so the TUI counts them as wide as it can, and a line with one may still be a cell
// out from the border at its end.
const tmuxWarning = "In tmux, lines containing emoji may look misaligned."

// footerLines is what the footer always takes: two lines for the status and two
// for the key hints. The panes are sized around it, and it does not change with
// what is in it, so nothing moves when a status comes and goes.
const footerLines = 4

// footer is the status, any error, the line being typed, and the key bar. Where the
// height is known it is always footerLines long, whatever is in it: a status of more
// than two lines is cut, and hints that fit on one line have a blank above them, so
// the hints are always on the last line of the window and never jump.
func (m model) footer(keys []string) []string {
	w := m.frameWidth()
	var status []string
	if m.status != "" {
		status = append(status, " "+m.status)
	}
	if m.err != "" {
		for _, line := range wrap(strings.Split(m.err, "\n"), w-2) {
			status = append(status, " "+red.Render(line))
		}
	}
	if m.tmux && m.screen == decksScreen && len(status) < 2 {
		// On the main screen only, and only while there is room: it is a note, and
		// anything the screen has to say comes first.
		status = append([]string{" " + yellow.Render("!") + faint.Render(" "+cut(tmuxWarning, w-4, "…"))}, status...)
	}
	if m.input != nil {
		status = append(status, " "+bold.Render(m.input.prompt+": ")+string(m.input.text)+accent.Render("█"))
		keys = []string{"enter", "save", "esc", "cancel"}
	}
	if m.confirm != nil {
		status = append(status, " "+bold.Render(m.confirm.prompt))
		keys = []string{"y", "yes", "any other key", "keep it"}
	}
	bar := keyBar(keys, w)
	if m.height == 0 {
		return append(append([]string{""}, status...), bar...)
	}
	if len(status) > 2 {
		status = append(status[:1], truncate(status[1], w-2)+faint.Render(" …"))
	}
	for len(status) < 2 {
		status = append([]string{""}, status...)
	}
	for len(bar) < 2 {
		bar = append([]string{""}, bar...)
	}
	return append(status, bar...)
}

// paneRows is how many lines a page's panes have between the title bar and the
// footer, where the height is known.
func (m model) paneRows() int { return max(m.height-3-footerLines, 0) }

// fitRows is lines cut or padded to exactly n, so a pane is the size the window
// gives it and not the size of what is in it.
func fitRows(lines []string, n int) []string {
	if len(lines) > n {
		return lines[:n]
	}
	return append(lines, make([]string, n-len(lines))...)
}

// marker is the bar down the left of the row the cursor is on.
func marker(on bool) string {
	if on {
		return accent.Render("▌") + " "
	}
	return "  "
}

// -- choosing ----------------------------------------------------------------

func (m model) decksPage() page {
	p := page{title: "Decks", tabbed: true, keys: m.deckKeys()}
	switch {
	case m.decks == nil:
		p.body = []string{"Loading decks..."}
		return p
	case len(m.decks.Decks) == 0:
		p.body = []string{
			"No decks in " + tilde(m.decks.Home) + ".",
			"",
			"Press 2 for the Library, where n makes one.",
		}
		return p
	}

	w := m.frameWidth()
	// In Leitner mode each deck says what it has due, right-aligned.
	names, dues := 0, 0
	for _, deck := range m.decks.Decks {
		names = max(names, cells(deckName(deck)))
		dues = max(dues, cells(dueLabel(deck)))
	}
	if m.mode != leitner {
		dues = 0
	}
	mode := modeSwitch(m.mode)
	listW := min(max(names+6+dues+min(dues, 2), cells("Decks")+cells(mode)+10, 22), w/2)
	detailW := w - listW - 1
	if detailW < 30 { // too narrow for two: one above the other, full width
		listW, detailW = w, w
	}

	var list []string
	for i, deck := range m.decks.Decks {
		name := deckName(deck)
		room := listW - 6 - dues - min(dues, 2)
		if dues == 0 {
			room = listW - 6
		}
		name = truncate(name, room)
		switch {
		case deck.Error != "":
			name = faint.Render(name)
		case i == m.cursor:
			name = bold.Render(name)
		}
		row := marker(i == m.cursor) + name
		if dues > 0 {
			due := dueLabel(deck)
			if deck.Stats != nil && deck.Stats.Due > 0 {
				due = accent.Render(due)
			} else {
				due = faint.Render(due)
			}
			row = fit(row, listW-4-cells(due)) + due
		}
		list = append(list, row)
	}
	deck := m.decks.Decks[m.cursor]
	detail := m.deckDetail(deck, detailW-4)
	if m.height > 0 && listW != w {
		// The panes are the size the window gives them, so they do not resize as
		// the cursor moves between decks with more or less to say.
		rows := max(m.paneRows()-4, 3)
		list, detail = fitRows(scrolled(list, m.cursor, rows), rows), fitRows(detail, rows)
	}
	if listW == w {
		p.panes = append(sideCard("Decks", mode, list, w), card(deckName(deck), detail, w)...)
		return p
	}
	if m.height == 0 {
		list, detail = even(list, detail)
	}
	p.panes = join(sideCard("Decks", mode, list, listW), card(deckName(deck), detail, detailW))
	return p
}

// deckKeys are the keys the deck list offers. The reports that belong to one
// mode are listed only in it: the miss list drives classic runs, and the boxes
// are what Leitner runs are made of. The keys still work in the other mode.
// screenKeys is the hint that the tabs along the top can be reached by number: 1-3.
func screenKeys() []string {
	return []string{"1-" + strconv.Itoa(len(tabs)), "change screen"}
}

func (m model) deckKeys() []string {
	keys := []string{"enter", "run", "tab", "mode"}
	if m.mode == leitner {
		keys = append(keys, "b", "boxes")
	} else {
		keys = append(keys, "m", "misses")
	}
	keys = append(keys, "e", "edits", "r", "refresh")
	return append(append(keys, screenKeys()...), "q", "quit")
}

// modeSwitch is the mode as the deck list's edge shows it: both, the one in
// use lit.  classic │ leitner
func modeSwitch(mode string) string {
	var parts []string
	for _, each := range []string{classic, leitner} {
		if each == mode {
			parts = append(parts, accent.Render(each))
		} else {
			parts = append(parts, faint.Render(each))
		}
	}
	return strings.Join(parts, faint.Render(" │ "))
}

// modeHint says what a mode's runs are, for when it is switched to.
func modeHint(mode string) string {
	if mode == leitner {
		return "Leitner: only the cards due today come up, lowest box first."
	}
	return "Classic: every card is in play, at random or down the follow-ups."
}

// dueLabel is what a deck has due, as the deck list shows it in Leitner mode.
func dueLabel(deck engine.DeckInfo) string {
	if deck.Stats == nil {
		return ""
	}
	if deck.Stats.Due == 0 {
		return "·"
	}
	return strconv.Itoa(deck.Stats.Due) + " due"
}

func deckName(deck engine.DeckInfo) string {
	if deck.Name == "" {
		return deck.ID
	}
	return deck.Name
}

// deckDetail is what the deck is, if it says, and where it stands: its size,
// what is due (in Leitner mode only), what is missed, its cards by box, and
// when it last ran.
func (m model) deckDetail(deck engine.DeckInfo, w int) []string {
	if deck.Error != "" {
		return append([]string{red.Render("This deck will not load:"), ""},
			wrap(strings.Split(deck.Error, "\n"), w)...)
	}
	var lines []string
	if deck.Desc != "" {
		lines = append(wrap([]string{deck.Desc}, w), "", edge.Render(strings.Repeat("─", w)), "")
	}
	s := deck.Stats
	if s == nil {
		return append(lines, faint.Render("Its cards will not load."))
	}
	lines = append(lines, plural(s.Size, "card"))
	// What a deck says of itself is the mode's: in Leitner what is due and where the
	// cards sit in the boxes, in classic the miss list that drives its runs. The
	// reports for the other still work by their keys; they are not shown here.
	if m.mode == leitner {
		if s.Due > 0 {
			lines = append(lines, accent.Render(strconv.Itoa(s.Due))+" due today")
		} else {
			lines = append(lines, faint.Render("nothing due today"))
		}
		lines = append(lines, "")
		lines = append(lines, boxChart(*s, w)...)
	} else if s.Missed > 0 {
		lines = append(lines, yellow.Render(strconv.Itoa(s.Missed))+" on the miss list")
	} else {
		lines = append(lines, faint.Render("miss list clear"))
	}
	return append(lines, "", faint.Render(lastRun(s.LastRun, m.now())))
}

// boxChart is a deck's cards by Leitner box as bars, w cells wide. The empty
// boxes at the top end share one row:  3-5 ·
func boxChart(s engine.DeckStats, w int) []string {
	type row struct {
		label string
		n     int
	}
	rows := []row{{"new", s.New}}
	last := len(s.Boxes)
	for last > 0 && s.Boxes[last-1] == 0 {
		last--
	}
	for i := range last {
		rows = append(rows, row{strconv.Itoa(i + 1), s.Boxes[i]})
	}
	switch {
	case last == len(s.Boxes)-1:
		rows = append(rows, row{strconv.Itoa(last + 1), 0})
	case last < len(s.Boxes):
		rows = append(rows, row{fmt.Sprintf("%d-%d", last+1, len(s.Boxes)), 0})
	}
	most, labelW, countW := 1, 0, 0
	for _, r := range rows {
		most = max(most, r.n)
		labelW = max(labelW, len(r.label))
		countW = max(countW, len(strconv.Itoa(r.n)))
	}
	room := max(w-labelW-countW-2, 1)
	var out []string
	for _, r := range rows {
		line := fmt.Sprintf("%-*s ", labelW, r.label)
		if r.n == 0 {
			line += faint.Render("·")
		} else {
			line += barStyle.Render(bar(r.n, most, room)) + " " + strconv.Itoa(r.n)
		}
		out = append(out, line)
	}
	return out
}

// lastRun is the day of the last attempt, counted back from now.
func lastRun(day string, now time.Time) string {
	if day == "" {
		return "never run"
	}
	then, err := time.ParseInLocation(time.DateOnly, day, now.Location())
	if err != nil {
		return "last run " + day
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch days := int(today.Sub(then).Hours()/24 + 0.5); { // +0.5: a DST day is 23h or 25h
	case days <= 0:
		return "last run today"
	case days == 1:
		return "last run yesterday"
	case days < 60:
		return fmt.Sprintf("last run %d days ago", days)
	}
	return "last run " + day
}

// The options screen's rows: a short label beside the value, and the name the
// help card gives the row.
var (
	optionLabels = [fieldCount]string{"Cards", "Seconds", "Follow", "Drafts", "Misses", "Edit", "Filter"}
	optionNames  = [fieldCount]string{"Cards", "Seconds per answer", "Follow branches",
		"Drafts", "Only the miss list", "Edit mode", "Card id filter"}
)

const labelWidth = 8

func (m model) optionsPage() page {
	o := m.opts
	p := page{context: deckName(o.deck), keys: []string{
		"enter", "start", "↑↓", "choose", "←→", "change", "/", "filter", "esc", "back"}}
	if m.field == filterField {
		p.keys = []string{"enter", "start", "↑↓", "choose", "type", "filter", "backspace", "delete", "esc", "back"}
	}

	// One slot as wide as the widest value of any wheel, so a value never moves
	// its arrows. The column is wider, for room to see a filter being typed.
	slot := 0
	for _, w := range o.wheels {
		for _, label := range w.labels {
			slot = max(slot, cells(label))
		}
	}
	valueW := func(side int) int { return max(wheelWidth(slot, side), 17) }
	settingsW := func(side int) int { return 2 + labelWidth + valueW(side) + 4 }

	// Each wheel shows as many of its neighbouring values as leave the help card
	// beside it room; with no room for the card even bare, the two stack, and the
	// wheels show what the full width has room for.
	w := m.frameWidth()
	const helpMin = 28
	side, stacked := 0, true
	for n := maxSide; n >= 0; n-- {
		if w-settingsW(n)-1 >= helpMin {
			side, stacked = n, false
			break
		}
	}
	if stacked {
		for side < maxSide && settingsW(side+1) <= w {
			side++
		}
	}

	var settings []string
	for _, i := range rows(m.mode) {
		var value string
		if i < filterField {
			value = o.wheels[i].render(i == m.field, slot, side)
		} else {
			value = textBox(o.filter, valueW(side), i == m.field)
		}
		label := fit(optionLabels[i], labelWidth)
		if i == m.field {
			label = bold.Render(label)
		}
		settings = append(settings, marker(i == m.field)+label+value)
	}

	run := map[string]string{classic: "Classic run", leitner: "Leitner run"}[m.mode]
	if stacked {
		help := wrap([]string{m.optionHint()}, w-4)
		p.panes = append(card(run, settings, w), card(optionNames[m.field], help, w)...)
	} else {
		helpW := w - settingsW(side) - 1
		settings, help := even(settings, wrap([]string{m.optionHint()}, helpW-4))
		p.panes = join(card(run, settings, settingsW(side)), card(optionNames[m.field], help, helpW))
	}
	below, cut := sourceLines(o.deck.Cards, w)
	p.panes = append(p.panes, below...)
	if cut && m.field != filterField {
		p.keys = slices.Insert(p.keys, len(p.keys)-2, "c", "sources")
	}
	return p
}

// prefLabelWidth is the label column of the settings: the longest label and two cells
// of space before the value.
const prefLabelWidth = 15

var prefLabels = [prefRows]string{
	modeRow: "Mode", countPref: "Cards", limitPref: "Seconds", entitiesPref: "Emoji codes",
	colourModePref: "Colour mode",
}

// colourLabel is what the colour row is called: it is a different setting in each mode.
func (m model) colourLabel() string {
	return []string{"Theme colour", "Custom colour", "Wheel colour"}[m.prefs.colourMode]
}

// arrowed is a value with an arrow to each side that it can turn, when it is selected.
func arrowed(text string, left, right, selecting bool) string {
	if !selecting {
		return text
	}
	l, r := " ", " "
	if left {
		l = "‹"
	}
	if right {
		r = "›"
	}
	return l + " " + text + " " + r
}

// colourValue is the colour row's value in the mode it is in, with a swatch of the
// colour: for one of the terminal's, drawn by the terminal, as it looks in its theme.
// Once the row is selected it is a bar of that very colour, so there is no swatch, and
// nothing in it is styled: a styled part would end the bar where it ends.
func (m model) colourValue(selecting bool) string {
	p := m.prefs
	swatchOf := func(value string) string {
		if selecting {
			return "  " // the room it takes, so the row does not move
		}
		return swatch(value)
	}
	switch p.colourMode {
	case themeMode:
		name := p.theme.label()
		return arrowed(swatchOf(name)+" "+name, p.theme.at > 0, p.theme.at < len(p.theme.labels)-1, selecting)
	case customMode:
		if len(p.custom) == 0 {
			return faint.Render("none yet")
		}
		c := p.custom[p.customAt]
		value := "  " + c.Value
		if !selecting {
			value = faint.Render(value)
		}
		return arrowed(swatchOf(c.Value)+" "+c.Name+value, p.customAt > 0, p.customAt < len(p.custom)-1, selecting)
	}
	return swatchOf(p.picked) + " " + p.picked
}

// settingText is a settings row as text: its name and its value, or for an action just
// what it does.
func (m model) settingText(i int, selecting bool) string {
	label := func(s string) string { return fit(s, prefLabelWidth) }
	switch i {
	case modeRow:
		mode := wheel{labels: []string{classic, leitner}, values: []int{0, 1}}
		mode.at = slices.Index(mode.labels, m.mode)
		return label(prefLabels[i]) + wheelValue(mode, selecting)
	case countPref:
		return label(prefLabels[i]) + wheelValue(m.prefs.count, selecting)
	case limitPref:
		return label(prefLabels[i]) + wheelValue(m.prefs.limit, selecting)
	case entitiesPref:
		return label(prefLabels[i]) + wheelValue(m.prefs.entities, selecting)
	case colourModePref:
		modes := wheel{labels: colourModeNames, values: []int{0, 1, 2}, at: m.prefs.colourMode}
		return label(prefLabels[i]) + wheelValue(modes, selecting)
	case colourPref:
		return label(m.colourLabel()) + m.colourValue(selecting)
	case addColourPref:
		return "Add a colour…"
	}
	return "Remove this colour"
}

// settingsPage is the settings for lernt as a whole, in tabs: how decks are run, how
// cards are edited, how the TUI looks. One card of rows, the focused one explained
// underneath. With the colour wheel open the card is the wheel.
func (m model) settingsPage() page {
	p := page{tabbed: true, narrow: true, title: "Settings", keys: []string{
		"↑↓", "choose", "←→", "settings tab", "enter", "change", "esc", "back"}}
	p.keys = append(p.keys, screenKeys()...)
	if m.prefSelecting {
		p.keys = []string{"←→", "change", "enter", "done"}
	}

	var names []string
	for i, name := range settingsTabs {
		if i == m.prefTab {
			names = append(names, accent.Render(name))
		} else {
			names = append(names, faint.Render(name))
		}
	}
	p.pinned = append(p.pinned, " "+strings.Join(names, faint.Render(" │ ")), "")

	if m.form != nil { // adding a colour: a form in the card, like the library's
		p.title = m.form.title
		p.keys = formKeys(m.form)
		p.pinned = m.formRegion(m.inner(true), 9) // in place of the tabs, which it is not one of
		return p
	}
	if m.colourPick != nil {
		p.keys = []string{"←→↑↓", "move", "tab", "hue or colour", ",.", "fine hue", "enter", "use it", "esc", "cancel"}
		p.pinned = append(p.pinned, m.colourPick.lines(m.inner(true))...)
		if !trueColour() {
			p.body = wrap([]string{"This terminal does not say it draws 24-bit colour, so the colours here will be approximate."}, m.inner(true))
		}
		return p
	}

	for _, i := range m.rowsOf(m.prefTab) {
		focus := i == m.pref
		selected := focus && m.prefSelecting
		p.pinned = append(p.pinned, choiceRow(m.settingText(i, selected), m.inner(true), focus, selected))
	}
	p.pinned = append(p.pinned, "")
	p.body = wrap([]string{m.prefHint()}, m.inner(true))
	return p
}

// prefHint says what the setting under the cursor does, at its current value where
// that matters.
func (m model) prefHint() string {
	switch m.pref {
	case countPref:
		return "How many cards a run asks for, in a deck that does not set its own count. " +
			"\"∞\" runs until you stop it."
	case limitPref:
		return "Seconds for each answer, in a deck that does not set its own limit. " +
			"\"auto\" gives each card its own time; \"off\" leaves the clock out."
	case entitiesPref:
		return "An emoji in a card's file can be written as a code, such as \"&amp;#x1f534;\", which the " +
			"card view shows as \"🔴\". When a card is opened to be edited, \"as characters\" turns each " +
			"code into the character it stands for, so the card is saved with the characters. " +
			"\"As written\" leaves the codes as they are in the file."
	case colourModePref:
		return "Where the highlight colour comes from: one of the terminal's own 16 colours, so " +
			"that it follows the terminal's theme; one from a list of your own; or one picked on " +
			"a colour wheel. The highlight is the lit titles, the bar on the row you are on, and " +
			"the box being typed in."
	case colourPref:
		switch m.prefs.colourMode {
		case themeMode:
			return "One of the terminal's 16 colours. What each looks like is up to the terminal's " +
				"theme, so the swatch is the real thing: pick by what you see, not by the name."
		case customMode:
			if len(m.prefs.custom) == 0 {
				return "You have not added a colour yet, so the highlight is cyan until you do. " +
					"The next row adds one."
			}
			return "One of the colours you added. To change one, remove it and add it again."
		}
		return "Enter opens the colour wheel. ↑↓←→ move in the square, tab moves along the hues, " +
			", and . fine-tune the hue, shift with an arrow takes bigger steps. It looks best in a " +
			"terminal that draws 24-bit colour."
	case addColourPref:
		return "Add a colour to your list: its value, in any form (hex like #ff8800, a number from " +
			"0 to 255, or r,g,b like 255,136,0), and then a name for it."
	case deleteColourPref:
		return "Remove the colour showing from your list. A list is only added to and taken from."
	}
	return modeHint(m.mode) + " This applies to every deck. On the deck list, tab switches it."
}

// formKeys is the key bar of a form, by what the cursor is doing, or of the
// picker when it is open.
func formKeys(f *form) []string {
	var keys []string
	switch {
	case f.picker != nil:
		return []string{"↑↓", "choose", "space", "tick", "enter", "done", "esc", "cancel"}
	case f.mode == typing:
		keys = []string{"↑↓←→", "cursor", "enter", "new line", "esc", "done typing"}
	case f.mode == selecting:
		keys = []string{"←→", "change", "enter", "done"}
	default:
		keys = []string{"↑↓←→", "move", "enter", "select", "esc", "cancel"}
	}
	return append(keys, "ctrl+s", "save")
}

// formColumns splits a form w cells wide into its boxes, its options (a deck has
// none) and its actions, with a rule between each.
func formColumns(w int, options bool) (boxes, opts, acts int) {
	acts = 17
	if !options {
		return w - acts - 3, 0, acts
	}
	opts = 22
	return w - opts - acts - 6, opts, acts
}

// choiceRow is an option or an action as a row w cells wide. The three states of
// it look alike and differ in how much of the row the cursor covers: none of it; a
// block at each end when the cursor is on it; all of it once it is selected.
func choiceRow(text string, w int, focused, selected bool) string {
	inner := fit(" "+text, w-2)
	switch {
	case selected:
		return barOn.Render(" " + inner + " ")
	case focused:
		return accent.Render("▌") + inner + accent.Render("▐")
	}
	return " " + inner + " "
}

// wheelValue is an option's value, with an arrow to each side that it can turn
// when it is selected.
func wheelValue(w wheel, selecting bool) string {
	if !selecting {
		return w.label()
	}
	l, r := " ", " "
	if w.at > 0 {
		l = "‹"
	}
	if w.at < len(w.labels)-1 {
		r = "›"
	}
	return l + " " + w.label() + " " + r
}

// formRegion is a deck or a card being written, drawn where its detail is drawn:
// the boxes on the left, one above the next; in the middle the card's options, the
// things that are not text; and on the right the actions. The focused view is
// explained underneath.
func (m model) formRegion(w, d int) []string {
	f := m.form
	options := f.hasOptions()
	lw, mw, rw := formColumns(w, options)
	boxRows := f.boxRows(d)

	var left []string
	for i, fld := range f.fields {
		if fld.kind != textField {
			continue
		}
		focus, writing := i == f.at, i == f.at && f.mode == typing
		label := faint.Render(fld.label)
		if focus {
			label = accent.Render(fld.label)
		}
		if fld.key == keyValue { // what the colour looks like, as it is typed
			if normal, ok := normalColour(string(fld.text)); ok {
				label += "  " + swatch(normal) + swatch(normal) + swatch(normal) + faint.Render(" "+normal)
			}
		}
		left = append(left, label)
		var fill *lipgloss.Style
		if writing {
			fill = &boxOn
		}
		for _, line := range areaLines(fld.text, fld.pos, lw-4, boxRows[i], writing, fill) {
			switch {
			case writing:
				left = append(left, boxOn.Render("  ")+line+boxOn.Render("  "))
			case focus:
				left = append(left, accent.Render("▌")+" "+line+" "+accent.Render("▐"))
			default:
				left = append(left, faint.Render("│")+" "+line+"  ")
			}
		}
	}

	const labelW = 9
	var mid, right []string
	for i, fld := range f.fields {
		focus := i == f.at
		selected := focus && f.mode == selecting
		switch fld.kind {
		case wheelField:
			mid = append(mid, choiceRow(fit(fld.label, labelW)+wheelValue(fld.wheel, selected), mw, focus, selected))
		case linkField:
			mid = append(mid, choiceRow(fit(fld.label, labelW)+linksLabel(len(fld.links), focus), mw, focus, false))
		case buttonField:
			right = append(right, choiceRow(fld.label, rw, focus, false))
		}
	}
	if options {
		mid = append([]string{faint.Render("Card options"), ""}, mid...)
	}
	right = append([]string{faint.Render("Actions"), ""}, right...)

	rule := faint.Render(" │ ")
	columns := [][]string{left}
	widths := []int{lw}
	if options {
		columns, widths = append(columns, mid), append(widths, mw)
	}
	columns, widths = append(columns, right), append(widths, rw)
	height := 0
	for _, c := range columns {
		height = max(height, len(c))
	}
	lines := make([]string, height)
	for i := range height {
		var parts []string
		for c, col := range columns {
			cell := ""
			if i < len(col) {
				cell = col[i]
			}
			parts = append(parts, fit(cell, widths[c]))
		}
		lines[i] = strings.Join(parts, rule)
	}
	lines = append(lines, "")
	for _, line := range wrap([]string{f.fields[f.at].hint}, w) {
		lines = append(lines, faint.Render(line))
	}
	return fitRows(lines, d)
}

// linksLabel is how many cards a card leads to, as the option shows it.
func linksLabel(n int, focus bool) string {
	text := "none ▸"
	if n > 0 {
		text = plural(n, "card") + " ▸"
	}
	return text
}

// linkTitles are the questions of the cards with these ids; one that is not in
// the deck is shown by its id.
func (m model) linkTitles(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id
		if m.library != nil {
			if j := slices.IndexFunc(m.library.Cards, func(c engine.CardSummary) bool { return c.ID == id }); j >= 0 {
				out[i] = oneLine([]string{m.library.Cards[j].Title})
			}
		}
	}
	return out
}

// deckByID is a deck from the list, by id.
func (m model) deckByID(id string) (engine.DeckInfo, bool) {
	if m.decks == nil {
		return engine.DeckInfo{}, false
	}
	i := slices.IndexFunc(m.decks.Decks, func(d engine.DeckInfo) bool { return d.ID == id })
	if i < 0 {
		return engine.DeckInfo{}, false
	}
	return m.decks.Decks[i], true
}

// libraryPage is the library: the decks and the highlighted deck's questions side
// by side, and below them the card under the cursor whole. That card is where it
// is read and written: e turns it into a form in the same place, and the lists
// above stay. A list that runs past its pane says so with an arrow.
func (m model) libraryPage() page {
	p := page{title: "Library", tabbed: true}
	switch {
	case m.form != nil:
		p.keys = formKeys(m.form)
	case m.libFocus == 0:
		p.keys = []string{"↑↓", "choose", "→", "cards", "n", "new", "e", "edit", "o", "editor", "d", "delete"}
	default:
		p.keys = []string{"↑↓", "choose", "←", "decks", "n", "new", "e", "edit", "o", "editor", "d", "delete"}
	}
	if m.form == nil { // in a form the digits are text, not screens
		p.keys = append(p.keys, screenKeys()...)
	}
	if m.decks == nil {
		p.body = []string{"Loading decks..."}
		return p
	}
	w := m.frameWidth()
	rows, detailRows := m.librarySplit()
	picking := m.form != nil && m.form.picker != nil
	writing := m.form != nil && !picking

	names := 0
	for _, deck := range m.decks.Decks {
		names = max(names, cells(deckName(deck)))
	}
	decksW := min(max(names+6, 20), 30)
	cardsW := w - decksW - 1
	stacked := w < 60
	if stacked {
		decksW, cardsW = w, w
	}

	var all []string
	for i, deck := range m.decks.Decks {
		all = append(all, listRow(truncate(deckName(deck), decksW-6), decksW-4,
			i == m.libDeck, m.libFocus == 0 && m.form == nil, deck.Error != ""))
	}
	list := scrolled(all, m.libDeck, rows)
	if len(list) == 0 {
		list = []string{faint.Render("No decks. n makes one.")}
	}

	title, questions := "Cards", []string{}
	if deck, ok := m.libraryDeck(); ok {
		title = truncate(deckName(deck), cardsW-8)
		questions = m.questionLines(cardsW-4, rows)
	} else if m.libDeck < len(m.decks.Decks) {
		questions = wrap([]string{m.decks.Decks[m.libDeck].Error}, cardsW-4)
	}

	// The pane that has the keys has its title lit.
	decksTitle, cardsTitle, detailTitle := "Decks", title, "Card"
	if m.form != nil {
		detailTitle = m.form.title // the card being edited stays where it was
	}
	switch {
	case picking:
		cardsTitle = accent.Render(title + " · tick the cards it leads to")
	case writing:
		detailTitle = accent.Render(detailTitle)
	case m.libFocus == 0:
		decksTitle = accent.Render(decksTitle)
	default:
		cardsTitle = accent.Render(cardsTitle)
	}

	var detail []string
	if m.form != nil {
		detail = m.formRegion(w-4, detailRows)
	} else {
		detail = fitRows(m.cardDetail(w-4, detailRows), detailRows)
	}

	// Each pane is the size the window gives it, whatever is in it.
	list, questions = fitRows(list, rows), fitRows(questions, rows)
	if stacked {
		p.panes = append(card(decksTitle, list, w), card(cardsTitle, questions, w)...)
	} else {
		p.panes = join(card(decksTitle, list, decksW), card(cardsTitle, questions, cardsW))
	}
	p.panes = append(p.panes, card(detailTitle, detail, w)...)
	return p
}

// questionLines are the questions of the deck the library is on, one to a line.
// A draft says so. With the picker open each has a box to tick, and the card
// being edited, which cannot lead to itself, is marked instead.
func (m model) questionLines(w, rows int) []string {
	switch {
	case m.library == nil:
		return []string{faint.Render("Loading...")}
	case len(m.library.Cards) == 0:
		return []string{faint.Render("No cards yet. n writes one.")}
	}
	pk := (*picker)(nil)
	if m.form != nil {
		pk = m.form.picker
	}
	var all []string
	cursorAt := m.libCard
	for i, c := range m.library.Cards {
		text := oneLine([]string{c.Title})
		if c.Draft {
			text = truncate(text, w-2-8) + " (draft)"
		}
		switch {
		case pk != nil && i == pk.self:
			all = append(all, "  "+faint.Render(truncate("    "+text+" (this card)", w-2)))
		case pk != nil:
			box := "[ ] "
			if pk.chosen[c.ID] {
				box = "[x] "
			}
			all = append(all, listRow(truncate(box+text, w-2), w, i == pk.at, true, false))
		default:
			all = append(all, listRow(truncate(text, w-2), w, i == m.libCard, m.libFocus == 1 && m.form == nil, false))
		}
	}
	if pk != nil {
		cursorAt = pk.at
	}
	return scrolled(all, cursorAt, rows)
}

// scrolled is rows of the lines of a list with the cursor in view, the first line
// or the last given over to an arrow and a count where more lies beyond it.
func scrolled(all []string, cursor, rows int) []string {
	rows = max(rows, 3)
	if len(all) <= rows {
		return all
	}
	from := max(min(cursor-rows/2, len(all)-rows), 0)
	out := slices.Clone(all[from : from+rows])
	if from > 0 {
		out[0] = faint.Render("  ▲ " + strconv.Itoa(from) + " more")
	}
	if below := len(all) - from - rows; below > 0 {
		out[rows-1] = faint.Render("  ▼ " + strconv.Itoa(below) + " more")
	}
	return out
}

// oneLine is text of several lines on one, cleaned.
func oneLine(lines []string) string {
	return strings.Join(strings.Fields(clean(strings.Join(lines, " "))), " ")
}

// cardDetail is the card under the cursor whole: its question and the cards it
// follows from, its answer, notes and reference, then where it leads and how it
// is timed. Cut to most lines, with the cut marked.
func (m model) cardDetail(w, most int) []string {
	c, ok := m.libraryCard()
	if !ok {
		return []string{faint.Render("n writes a card.")}
	}
	lines := wrap([]string{"**" + c.Title + "**"}, w)
	for i := range lines {
		lines[i] = bold.Render(strings.ReplaceAll(lines[i], "**", ""))
	}
	for _, parent := range m.parentsOf(c) {
		lines = append(lines, faint.Render(truncate("↳ follows "+oneLine([]string{parent.Title}), w)))
	}
	lines = append(lines, "")
	if len(c.Answer) == 0 {
		lines = append(lines, faint.Render("no answer yet"))
	}
	lines = append(lines, wrap(c.Answer, w)...)
	if len(c.Notes) > 0 {
		lines = append(lines, "", faint.Render("notes"))
		lines = append(lines, wrap(c.Notes, w)...)
	}
	if len(c.Follows) > 0 {
		lines = append(lines, "", faint.Render("leads to"))
		for _, title := range m.linkTitles(c.Follows) {
			lines = append(lines, truncate("→ "+title, w))
		}
	}
	var facts []string
	if c.Time != nil {
		facts = append(facts, "time "+limitLabel(*c.Time))
	}
	if c.Draft {
		facts = append(facts, "draft")
	}
	if c.Reference != "" {
		facts = append(facts, "see "+clean(c.Reference))
	}
	if len(facts) > 0 {
		lines = append(lines, "", faint.Render(truncate(strings.Join(facts, " · "), w)))
	}
	if len(lines) > most {
		lines = append(lines[:most-1], faint.Render("…"))
	}
	return lines
}

// parentsOf are the cards of the deck that lead on to this one, whose questions
// give it the context it is asked in.
func (m model) parentsOf(c engine.CardSummary) []engine.CardSummary {
	var out []engine.CardSummary
	if m.library == nil {
		return out
	}
	for _, other := range m.library.Cards {
		if slices.Contains(other.Follows, c.ID) {
			out = append(out, other)
		}
	}
	return out
}

// listRow is one line of a list, w cells wide. The cursor's row is a bar; the
// marker stays on it for a terminal with no colour to draw the bar in.
func listRow(text string, w int, at, focused, dim bool) string {
	if !at {
		if dim {
			return "  " + faint.Render(text)
		}
		return "  " + text
	}
	marker := "  "
	style := barOff
	if focused {
		marker, style = "▌ ", barOn
	}
	return style.Render(fit(marker+text, w))
}

// librarySplit is how the window is shared between the lists across the top and
// the card below them, each as a number of lines inside its frame. The card gets a
// little over half, so a form has room for its boxes, and both grow with the
// window. Where the height is not known, enough for a small deck.
func (m model) librarySplit() (lists, card int) {
	if m.height == 0 {
		return 15, 17
	}
	avail := m.paneRows() - 8 // two frames
	card = min(max(avail*11/20, 8), 40)
	lists = avail - card
	if lists < 4 {
		lists, card = 4, max(avail-4, 5)
	}
	return lists, card
}

// libraryRows is how many lines the lists across the top have.
func (m model) libraryRows() int {
	lists, _ := m.librarySplit()
	return lists
}

// shownSources is how many of a deck's card sources the options screen lists;
// the rest are counted, and the sources screen has them all.
const shownSources = 3

// sourceLines are where a deck's cards come from, under the options: one source
// on a line of its own, more as a list. cut: a source is hidden or cut short.
func sourceLines(dirs []string, w int) (lines []string, cut bool) {
	line := func(s string) string {
		short := truncate(s, w-2)
		cut = cut || short != s
		return " " + faint.Render(short)
	}
	switch len(dirs) {
	case 0:
		return nil, false
	case 1:
		return []string{line("cards from " + tilde(dirs[0]))}, cut
	}
	lines = []string{line("cards from:")}
	for _, dir := range dirs[:min(len(dirs), shownSources)] {
		lines = append(lines, line(" - "+tilde(dir)))
	}
	if hidden := len(dirs) - shownSources; hidden > 0 {
		lines = append(lines, line(fmt.Sprintf(" … and %d more - c to see them all", hidden)))
		cut = true
	}
	return lines, cut
}

// sourcesPage lists every place a deck's cards come from, each path whole.
func (m model) sourcesPage() page {
	dirs := m.opts.deck.Cards
	p := page{context: deckName(m.opts.deck), title: "Card sources",
		keys: []string{"j/k", "scroll", "esc", "back"}}
	p.pinned = []string{faint.Render(plural(len(dirs), "source")), ""}
	for _, dir := range dirs {
		// A path has no spaces to wrap at, so it breaks where the card ends.
		for i, part := range strings.Split(ansi.Hardwrap(tilde(dir), m.inner(false)-2, false), "\n") {
			lead := "- "
			if i > 0 {
				lead = "  "
			}
			p.body = append(p.body, lead+part)
		}
	}
	return p
}

// textBox is a typed value in a box w cells wide: [flash_____]. What does not
// fit scrolls off the left, so the end being typed stays in view.
func textBox(text string, w int, focused bool) string {
	room := w - 2
	if focused {
		room--
	}
	r := []rune(text)
	for cells(string(r)) > room {
		r = r[1:]
	}
	shown := string(r)
	if focused {
		shown += accent.Render("█")
	}
	return faint.Render("[") + shown + faint.Render(strings.Repeat("_", max(w-2-cells(shown), 0))+"]")
}

// optionHint says what the focused option does at its current value.
func (m model) optionHint() string {
	o := m.opts
	switch m.field {
	case countField:
		if o.wheels[countField].value() == 0 {
			return "Runs until you end it."
		}
		if m.mode == leitner {
			return "At most this many of the cards due today. A pass moves a card up a " +
				"box, and each box waits longer before the card comes back" +
				boxDays(o.deck.Boxes) + ". A fail sends it back to box 1."
		}
		return "Cards asked before the run ends."
	case limitField:
		fallback := "30s"
		if l := o.deck.Limit; l != nil {
			fallback = fmt.Sprintf("%ds", *l)
			if *l == 0 {
				fallback = "untimed"
			}
		}
		switch o.wheels[limitField].value() {
		case autoLimit:
			return "Each card's own time; " + fallback + " for a card without one."
		case 0:
			return "No card is timed."
		}
		return "Every card gets this, whatever its own time."
	case autoField:
		return "Follow a card's branches without the menu."
	case draftsField:
		return "Cards tagged #draft may come up at random."
	case weakField:
		return "Only the cards you last got shaky or wrong."
	case editField:
		return "After each score: edit the card, note it, or flag it."
	}
	return "Only cards whose ids contain this - type it here. Empty: any card."
}

// boxDays is a deck's schedule as the mode hint says it: " - 1, 2, 4 days".
func boxDays(days []int) string {
	if len(days) == 0 {
		return ""
	}
	var each []string
	for _, d := range days {
		each = append(each, fmt.Sprint(d))
	}
	return " - " + strings.Join(each, ", ") + " days"
}

// maxSide is the most neighbouring values a wheel shows on each side.
const maxSide = 2

// wheelWidth is the cells a wheel takes with side neighbours shown each way.
func wheelWidth(slot, side int) int { return slot + 4 + 2*side*(slot+1) }

// render draws the wheel's value left-aligned in a slot as wide as its widest
// value, between arrows that show which ways it turns, with up to side of its
// neighbouring values faint on either side:  5s  off ‹ 10s  › 15s  20s
func (w wheel) render(focused bool, slot, side int) string {
	left, right := "‹", "›"
	if w.at == 0 {
		left = " "
	}
	if w.at == len(w.labels)-1 {
		right = " "
	}
	style, arrows := bold, faint
	if focused {
		style, arrows = accent, accent
	}
	var before, after strings.Builder
	for i := w.at - side; i < w.at; i++ {
		label := ""
		if i >= 0 {
			label = w.labels[i]
		}
		before.WriteString(strings.Repeat(" ", slot-cells(label)) + label + " ")
	}
	for i := w.at + 1; i <= w.at+side; i++ {
		label := ""
		if i < len(w.labels) {
			label = w.labels[i]
		}
		after.WriteString(" " + fit(label, slot))
	}
	return faint.Render(before.String()) + arrows.Render(left) + " " + style.Render(fit(w.label(), slot)) +
		" " + arrows.Render(right) + faint.Render(after.String())
}

// -- the run -------------------------------------------------------------------

// progress is how far into the run: a dot per card, or a count when there are
// too many for dots or no end.
func (m model) progress() string {
	n, total := m.card.Number, m.run.Count
	switch {
	case total == 0:
		return fmt.Sprintf("card %d", n)
	case total > 30:
		return fmt.Sprintf("%d/%d", n, total)
	}
	done := min(n, total)
	return accent.Render(strings.Repeat("●", done)) + faint.Render(strings.Repeat("○", total-done)) +
		fmt.Sprintf("  %d/%d", n, total)
}

// runPage is a page of the run: its progress and the card's box and time in
// the title bar, and a centred card.
func (m model) runPage(seconds int) page {
	right := boxLabel(m.card.Box)
	if m.card.Draft {
		right += " · " + yellow.Render("draft")
	}
	t := mmss(seconds)
	if limit := m.card.Limit; limit > 0 {
		t += "/" + mmss(limit)
		if seconds > limit {
			t = red.Render(t)
		}
	}
	return page{context: m.run.Name, left: m.progress(), right: right + " · " + t, narrow: true}
}

func (m model) title() []string {
	var lines []string
	for _, line := range wrap([]string{m.card.Title}, m.inner(true)) {
		lines = append(lines, bold.Render(line))
	}
	return lines
}

// timer is the card's time as a draining bar, and a word once it is over.
func (m model) timer(seconds int) []string {
	limit := m.card.Limit
	if limit == 0 {
		return nil
	}
	lines := []string{"", drain(seconds, limit, m.inner(true))}
	if seconds > limit {
		lines = append(lines, red.Render(fmt.Sprintf("%ds - over the %ds limit", seconds, limit)))
	}
	return lines
}

// question is the card's question above a rule, pinned over the answer as it scrolls.
func (m model) question() []string {
	return append(m.title(), edge.Render(strings.Repeat("┄", m.inner(true))))
}

func (m model) answer() []string {
	body := wrap(m.card.Answer, m.inner(true))
	var notes []string
	for _, line := range m.card.Notes {
		if strings.TrimSpace(line) != "" {
			notes = append(notes, line)
		}
	}
	if len(notes) > 0 {
		body = append(body, "")
		for _, line := range wrap(notes, m.inner(true)) {
			body = append(body, faint.Render(line))
		}
	}
	return body
}

func (m model) askPage() page {
	seconds := m.elapsed()
	p := m.runPage(seconds)
	p.body = append(m.title(), m.timer(seconds)...)
	if m.card.FollowsFrom != "" {
		p.below = []string{faint.Render(truncate("follows from: "+clean(m.card.FollowsFrom), m.inner(true)))}
	}
	p.keys = []string{"enter", "reveal", "q", "end the run"}
	return p
}

func (m model) revealPage() page {
	p := m.runPage(m.seconds)
	p.pinned = m.question()
	p.body = append(m.answer(), m.timer(m.seconds)...)
	p.keys = []string{"p", "pass", "s", "shaky", "f", "fail", "q", "end the run"}
	return p
}

func (m model) editPage() page {
	p := m.runPage(m.seconds)
	p.pinned = m.question()
	p.body = append(m.answer(), "", faint.Render(m.moved), "")
	if len(m.standing) == 0 {
		p.body = append(p.body, faint.Render("Nothing stands against this card."))
	} else {
		p.body = append(p.body, "Standing against this card:")
		for _, entry := range m.standing {
			p.body = append(p.body, "  "+entryLine(entry))
		}
	}
	p.keys = []string{"e", "edit", "n", "note", "x", "flag for removal"}
	if len(m.standing) > 0 {
		p.keys = append(p.keys, "d", "done")
	}
	p.keys = append(p.keys, "enter", "go on", "q", "end the run")
	return p
}

func entryLine(entry engine.EditEntry) string {
	kind := entry.Kind
	if kind == "remove" {
		kind = red.Render("remove")
	}
	return strings.TrimRight(fmt.Sprintf("%-6s  %s  %s", kind, entry.At, entry.Text), " ")
}

func (m model) menuPage() page {
	p := m.runPage(m.seconds)
	p.title = "Where next?"
	if m.moved != "" {
		p.pinned = []string{faint.Render(m.moved), ""}
	}
	for i, item := range m.menu {
		line := fmt.Sprintf("%s%d  %s", marker(i == m.cursor), i+1, clean(item.Title))
		if item.Draft {
			line += yellow.Render("   draft")
		}
		p.body = append(p.body, line)
	}
	if len(m.menu) == 0 {
		p.body = append(p.body, faint.Render("  nothing follows this one"))
	}
	p.body = append(p.body, marker(m.cursor == len(m.menu))+"r  a random card")
	p.keys = []string{"enter", "choose", "1-9", "follow-up", "r", "random", "q", "end the run"}
	return p
}

func (m model) summaryPage() page {
	s := m.summary
	p := page{title: "Run over", narrow: true, keys: []string{"enter", "back to the decks", "q", "quit"}}
	if m.run != nil {
		p.context = m.run.Name
	}
	p.body = []string{fmt.Sprintf("%s   %s   %s   (of %d)",
		green.Render(fmt.Sprintf("pass %d", s.Passed)),
		yellow.Render(fmt.Sprintf("shaky %d", s.Shaky)),
		red.Render(fmt.Sprintf("fail %d", s.Failed)), s.Asked)}
	if s.EndedEarly {
		p.body = append(p.body, faint.Render("Ended early."))
	}
	if s.Promoted > 0 || s.Demoted > 0 {
		p.body = append(p.body, fmt.Sprintf("%d up a box, %d back to box 1", s.Promoted, s.Demoted))
	}
	p.body = append(p.body, "")
	if len(s.Review) == 0 {
		p.body = append(p.body, "Clean run. Stop here rather than going again.")
	} else {
		p.body = append(p.body, "Worth another pass:")
		for _, item := range s.Review {
			label := yellow.Render("shaky ")
			if item.Score == "fail" {
				label = red.Render("fail  ")
			}
			p.body = append(p.body, "  "+label+clean(item.Title))
		}
		p.body = append(p.body, "")
		for _, line := range wrap([]string{"Re-run the fails once - Misses on the options is exactly that list."}, m.inner(true)) {
			p.body = append(p.body, faint.Render(line))
		}
	}
	if s.Log != nil && s.Logged > 0 {
		p.body = append(p.body, "", faint.Render(fmt.Sprintf("%s logged to %s", plural(s.Logged, "attempt"), tilde(*s.Log))))
	}
	var edited []string
	for _, part := range []struct {
		n     int
		label string
	}{{s.Edited, "edited"}, {s.Noted, "noted"}, {s.Flagged, "flagged for removal"}, {s.Cleared, "cleared"}} {
		if part.n > 0 {
			edited = append(edited, fmt.Sprintf("%d %s", part.n, part.label))
		}
	}
	if len(edited) > 0 {
		p.body = append(p.body, faint.Render(strings.Join(edited, ", ")+" - in "+tilde(s.Edits)))
	}
	return p
}

// -- reports -------------------------------------------------------------------

func (m model) missesPage() page {
	r := m.misses
	p := page{context: r.Name, title: "Miss list", keys: []string{"a", "every card attempted", "j/k", "scroll", "esc", "back"}}
	if m.showAll {
		p.title, p.keys[1] = "Every card attempted", "the miss list"
	}
	if r.Log != nil {
		p.pinned = []string{faint.Render(fmt.Sprintf("%s logged in %s", plural(r.Attempts, "attempt"), tilde(*r.Log))), ""}
	}

	switch {
	case r.Attempts == 0:
		p.body = []string{"Nothing logged yet."}
	case len(r.Records) == 0:
		p.body = []string{"No misses standing - every card you have attempted, you last got right."}
	default:
		p.pinned = append(p.pinned, faint.Render("last    misses  last seen   question"))
		for _, rec := range r.Records {
			score := fmt.Sprintf("%-6s", rec.Score)
			switch rec.Score {
			case "fail":
				score = red.Render(score)
			case "shaky":
				score = yellow.Render(score)
			case "pass":
				score = green.Render(score)
			}
			title := clean(rec.Title)
			if rec.Retired {
				title += faint.Render("   retired")
			}
			p.body = append(p.body, fmt.Sprintf("%s  %2d/%-3d  %-10s  %s", score, rec.Misses, rec.Attempts, rec.Seen, title))
		}
	}
	if len(r.Retired) > 0 {
		p.body = append(p.body, "", fmt.Sprintf("%s no longer in the deck - renamed or deleted:", plural(len(r.Retired), "logged card")))
		for _, card := range r.Retired {
			p.body = append(p.body, "  "+faint.Render(card))
		}
	}
	return p
}

func (m model) boxesPage() page {
	r := m.boxes
	p := page{context: r.Name, title: "Leitner boxes", keys: []string{"j/k", "scroll", "esc", "back"}}
	if r.Log == nil {
		p.pinned = []string{faint.Render("logging is off, so every card counts as new"), ""}
	}
	p.pinned = append(p.pinned, faint.Render(fmt.Sprintf("%-6s  %5s  %5s  %4s", "", "every", "cards", "due")))
	most := r.New
	for _, row := range r.Boxes {
		most = max(most, row.Cards)
	}
	room := max(m.inner(false)-28, 0) // what the columns leave for the bars
	p.body = []string{fmt.Sprintf("%-6s  %5s  %5d  %4d  %s", "new", "", r.New, r.New, bar(r.New, most, room))}
	for _, row := range r.Boxes {
		p.body = append(p.body, fmt.Sprintf("%-6s  %5s  %5d  %4d  %s", boxLabel(row.Box),
			fmt.Sprintf("%dd", row.Days), row.Cards, row.Due, barStyle.Render(bar(row.Cards, most, room))))
	}
	var said string
	switch {
	case r.Due > 0:
		said = fmt.Sprintf("%d card(s) due today - Leitner mode runs exactly those.", r.Due)
	case r.Next != "":
		said = "Nothing due today. The next card is due " + r.Next + "."
	default:
		said = "The deck has no cards."
	}
	p.body = append(append(p.body, ""), wrap([]string{said}, m.inner(false))...)
	return p
}

func (m model) editsPage() page {
	r := m.edits
	p := page{context: r.Name, title: "Edit list", keys: []string{"j/k", "scroll", "esc", "back"}}
	p.pinned = []string{faint.Render(fmt.Sprintf("%s, from %s", plural(len(r.Items), "card"), tilde(r.Log))), ""}
	if len(r.Items) == 0 {
		p.body = []string{"Nothing stands against any card."}
	}
	for i, item := range r.Items {
		if i > 0 {
			p.body = append(p.body, "")
		}
		p.body = append(p.body, bold.Render(clean(item.Title)), faint.Render("  "+tilde(item.Path)))
		for _, entry := range item.Entries {
			p.body = append(p.body, "  "+entryLine(entry))
		}
	}
	if len(r.Orphaned) > 0 {
		p.body = append(p.body, "", fmt.Sprintf("%s with notes no longer in the deck:", plural(len(r.Orphaned), "card")))
		for _, card := range r.Orphaned {
			p.body = append(p.body, "  "+faint.Render(card))
		}
	}
	p.below = []string{faint.Render("Editing a card in an edit-mode run, or d there, clears it.")}
	return p
}
