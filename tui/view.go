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
)

// A page is one screen laid out: the title bar, then either panes already
// framed or one card whose body scrolls, then the footer and its key bar.
type page struct {
	context     string   // the title bar's right-hand label
	left, right string   // a line inside the title bar: the run's progress
	panes       []string // framed cards, drawn as they are; none: the card below
	title       string   // the card's title
	narrow      bool     // the card is centred at reading width, not full width
	pinned      []string // inside the card, above the body, never scrolled
	body        []string // inside the card, scrolled when it does not fit
	below       []string // under the card
	keys        []string // the key bar: a key, what it does, a key, ...
}

func (m model) View() string {
	p := m.page()
	w := m.frameWidth()
	lines := append(titleBar(p.context, p.left, p.right, w), "")
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
	return strings.Join(append(lines, m.footer(p.keys)...), "\n")
}

// room is how many body lines fit in the card, with everything else on the
// page drawn and a line kept for the scroll line. 0: the height is not known.
func (m model) room(p page) int {
	if m.height == 0 {
		return 0
	}
	used := len(titleBar(p.context, p.left, p.right, m.frameWidth())) + 1 + 4 +
		len(p.pinned) + len(p.below) + 1 + len(m.footer(p.keys))
	return max(m.height-used, 1)
}

// frameWidth is the width everything is drawn in: the window, up to a limit.
func (m model) frameWidth() int {
	if m.width == 0 {
		return 80
	}
	return max(min(m.width, 100), 30)
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
	}
	return m.decksPage()
}

// footer is the status, any error, the line being typed, and the key bar.
func (m model) footer(keys []string) []string {
	w := m.frameWidth()
	lines := []string{""}
	if m.status != "" {
		lines = append(lines, " "+m.status)
	}
	if m.err != "" {
		for _, line := range wrap(strings.Split(m.err, "\n"), w-2) {
			lines = append(lines, " "+red.Render(line))
		}
	}
	if m.input != nil {
		lines = append(lines, " "+bold.Render(m.input.prompt+": ")+string(m.input.text)+accent.Render("█"))
		keys = []string{"enter", "save", "esc", "cancel"}
	}
	return append(lines, keyBar(keys, w)...)
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
	p := page{title: "Decks", keys: []string{
		"enter", "run", "tab", "mode", "n", "new card", "N", "new deck", "m", "misses", "b", "boxes", "e", "edits", "s", "settings",
		"r", "refresh", "q", "quit"}}
	switch {
	case m.decks == nil:
		p.body = []string{"Loading decks..."}
		return p
	case len(m.decks.Decks) == 0:
		p.body = []string{
			"No decks in " + tilde(m.decks.Home) + ".",
			"",
			"`lernt --list` says how to make one.",
		}
		return p
	}
	p.context = plural(len(m.decks.Decks), "deck")

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
	if listW == w {
		p.panes = append(sideCard("Decks", mode, list, w), card(deckName(deck), detail, w)...)
		return p
	}
	list, detail = even(list, detail)
	p.panes = join(sideCard("Decks", mode, list, listW), card(deckName(deck), detail, detailW))
	return p
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
	if m.mode == leitner {
		if s.Due > 0 {
			lines = append(lines, accent.Render(strconv.Itoa(s.Due))+" due today")
		} else {
			lines = append(lines, faint.Render("nothing due today"))
		}
	}
	if s.Missed > 0 {
		lines = append(lines, yellow.Render(strconv.Itoa(s.Missed))+" on the miss list")
	} else {
		lines = append(lines, faint.Render("miss list clear"))
	}
	lines = append(lines, "")
	lines = append(lines, boxChart(*s, w)...)
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

var prefLabels = [prefRows]string{"Mode", "Cards", "Seconds"}

// settingsPage is the settings for lernt as a whole: one card of wheels, and
// what the focused one does under it.
func (m model) settingsPage() page {
	p := page{context: "settings", narrow: true, title: "Settings", keys: []string{
		"↑↓", "choose", "←→", "change", "esc", "back"}}
	mode := wheel{labels: []string{classic, leitner}, values: []int{0, 1}}
	mode.at = slices.Index(mode.labels, m.mode)
	wheels := [prefRows]wheel{modeRow: mode, countPref: m.prefs.count, limitPref: m.prefs.limit}

	slot := 0
	for _, w := range wheels {
		for _, label := range w.labels {
			slot = max(slot, cells(label))
		}
	}
	for i, w := range wheels {
		label := fit(prefLabels[i], labelWidth)
		if i == m.pref {
			label = bold.Render(label)
		}
		p.pinned = append(p.pinned, marker(i == m.pref)+label+w.render(i == m.pref, slot, 1))
	}
	p.pinned = append(p.pinned, "")
	p.body = wrap([]string{prefHint(m.pref, m.mode)}, m.inner(true))
	return p
}

// prefHint says what a setting does, at its current value where that matters.
func prefHint(row int, mode string) string {
	switch row {
	case countPref:
		return "How many cards a run asks for, in a deck that does not set its own count. " +
			"∞ runs until you stop it."
	case limitPref:
		return "Seconds for each answer, in a deck that does not set its own limit. " +
			"auto gives each card its own time; off leaves the clock out."
	}
	return modeHint(mode) + " This applies to every deck. On the deck list, tab switches it."
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
