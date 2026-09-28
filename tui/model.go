package main

import (
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lernt/tui/engine"
)

// Engine is everything the TUI asks of the engine. *engine.Client is the real one;
// the tests drive the screens with a fake.
type Engine interface {
	Decks() (*engine.DeckList, error)
	Start(engine.StartParams) (*engine.Started, error)
	Score(engine.ScoreParams) (*engine.Scored, error)
	Menu() (*engine.Menu, error)
	Next(engine.NextParams) (*engine.Question, error)
	End() (*engine.Summary, error)
	EditCard() (*engine.CardFile, error)
	Reload() (*engine.Reloaded, error)
	Note(engine.NoteParams) (*engine.Standing, error)
	Flag(engine.FlagParams) (*engine.Standing, error)
	Done() (*engine.Standing, error)
	Misses(engine.MissParams) (*engine.MissReport, error)
	Boxes(engine.DeckParams) (*engine.BoxReport, error)
	Edits(engine.DeckParams) (*engine.EditReport, error)
	LoadOptions(engine.DeckParams) (*engine.Options, error)
	SaveOptions(engine.SaveOptionsParams) (*engine.Options, error)
	LoadSettings() (*engine.Settings, error)
	SaveSettings(engine.SaveSettingsParams) (*engine.Settings, error)
}

type screen int

const (
	decksScreen screen = iota
	optionsScreen
	askScreen
	revealScreen
	editScreen
	menuScreen
	summaryScreen
	missesScreen
	boxesScreen
	editsScreen
	sourcesScreen
)

// The run options, in the order the options screen lists them. The mode is not
// one of them: it is chosen on the deck list, for every deck at once.
const (
	countField = iota
	limitField
	autoField
	draftsField
	weakField
	editField
	filterField
	fieldCount
)

// A wheel is a choice turned left and right through a fixed run of values.
type wheel struct {
	labels []string
	values []int
	at     int
}

func (w *wheel) turn(by int)  { w.at = max(min(w.at+by, len(w.values)-1), 0) }
func (w wheel) value() int    { return w.values[w.at] }
func (w wheel) label() string { return w.labels[w.at] }

// autoLimit on the limit wheel: no limit for the run, so each card's own time.
const autoLimit = -1

var (
	counts = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 15, 20, 25, 30, 40, 50, 75, 100}
	limits = []int{5, 10, 15, 20, 25, 30, 40, 45, 60, 90, 120, 180, 240, 300}
)

// set turns the wheel to v, first adding v - in order, with its label - when it
// is not already one of the wheel's values.
func (w *wheel) set(v int, label string) {
	i, found := slices.BinarySearch(w.values, v)
	if !found {
		w.values = slices.Insert(w.values, i, v)
		w.labels = slices.Insert(w.labels, i, label)
	}
	w.at = i
}

func countLabel(v int) string {
	if v == 0 {
		return "∞"
	}
	return strconv.Itoa(v)
}

func limitLabel(v int) string {
	switch v {
	case autoLimit:
		return "auto"
	case 0:
		return "off"
	}
	return strconv.Itoa(v) + "s"
}

// countWheel starts at the deck's count, which joins the run of values if it is
// not already one of them.
func countWheel(start int) wheel {
	var w wheel
	for _, v := range counts {
		w.values, w.labels = append(w.values, v), append(w.labels, countLabel(v))
	}
	w.set(start, countLabel(start))
	return w
}

func limitWheel() wheel {
	var w wheel
	for _, v := range append([]int{autoLimit, 0}, limits...) {
		w.values, w.labels = append(w.values, v), append(w.labels, limitLabel(v))
	}
	return w
}

func toggle() wheel {
	return wheel{labels: []string{"off", "on"}, values: []int{0, 1}}
}

type options struct {
	deck   engine.DeckInfo
	wheels [filterField]wheel
	filter string
}

func defaultOptions(deck engine.DeckInfo) options {
	count := 10
	if deck.Count != nil {
		count = *deck.Count
	}
	o := options{deck: deck}
	o.wheels[countField] = countWheel(count)
	o.wheels[limitField] = limitWheel()
	for field := autoField; field < filterField; field++ {
		o.wheels[field] = toggle()
	}
	return o
}

func (o options) on(field int) bool { return o.wheels[field].value() == 1 }

// The run modes, as the deck list names them and as they are saved.
const (
	classic = "classic" // the whole deck, at random or down the branches
	leitner = "leitner" // only the cards the Leitner schedule has due
)

// modeKey is the mode's name in the settings.
const modeKey = "mode"

// rows are the options a run in this mode takes, in screen order. A Leitner
// run is the cards that are due, so it has no miss list to narrow it to.
func rows(mode string) []int {
	var out []int
	for field := range fieldCount {
		if field == weakField && mode == leitner {
			continue
		}
		out = append(out, field)
	}
	return out
}

// The keys the options screen is saved under. Names, never positions, so its
// rows can be reordered, added or dropped without losing a saved value.
var optionKeys = [fieldCount]string{
	countField: "count", limitField: "limit", autoField: "auto",
	draftsField: "drafts", weakField: "weak", editField: "edit", filterField: "filter",
}

// saved is the screen as key and value strings, for the engine to remember.
func (o options) saved() []engine.Option {
	var out []engine.Option
	for field := range filterField {
		v := o.wheels[field].value()
		var value string
		switch field {
		case countField:
			value = strconv.Itoa(v)
		case limitField:
			value = map[int]string{autoLimit: "auto", 0: "off"}[v]
			if value == "" {
				value = strconv.Itoa(v)
			}
		default:
			value = map[bool]string{true: "on", false: "off"}[v == 1]
		}
		out = append(out, engine.Option{Key: optionKeys[field], Value: value})
	}
	return append(out, engine.Option{Key: optionKeys[filterField], Value: o.filter})
}

func (o options) saveParams() engine.SaveOptionsParams {
	return engine.SaveOptionsParams{Deck: o.deck.ID, Options: o.saved()}
}

// restore sets each option from what was saved, by key. A key it does not know,
// or a value it cannot read, is passed over, and that option keeps its default.
func (o *options) restore(saved []engine.Option) {
	for _, option := range saved {
		field := slices.Index(optionKeys[:], option.Key)
		value := strings.TrimSpace(option.Value)
		switch field {
		case -1:
		case filterField:
			o.filter = option.Value
		case countField:
			if n, err := strconv.Atoi(value); err == nil && n >= 0 {
				o.wheels[field].set(n, countLabel(n))
			}
		case limitField:
			if value == "auto" {
				o.wheels[field].set(autoLimit, limitLabel(autoLimit))
			} else if value == "off" {
				o.wheels[field].set(0, limitLabel(0))
			} else if n, err := strconv.Atoi(value); err == nil && n > 0 {
				o.wheels[field].set(n, limitLabel(n))
			}
		default:
			switch value {
			case "on", "true":
				o.wheels[field].at = 1
			case "off", "false":
				o.wheels[field].at = 0
			}
		}
	}
}

// A line being typed: a note, a removal reason, the filter.
type input struct {
	kind   string
	prompt string
	text   []rune
}

// reply is an engine call's result arriving back in Update, typed by its value.
type reply struct {
	value any
	err   error
}

type tickMsg struct{ asking int }

type editorDone struct{ err error }

func call[T any](f func() (T, error)) tea.Cmd {
	return func() tea.Msg {
		value, err := f()
		return reply{value, err}
	}
}

type model struct {
	eng  Engine
	now  func() time.Time
	tick func(asking int) tea.Cmd
	exec func(path string) tea.Cmd

	width, height int
	screen        screen
	busy          bool   // an engine call is out; keys wait
	err           string // the last call's error
	status        string // what the last action did
	pending       string // the status to show when the call in flight returns
	cursor        int    // the decks list and the menu
	deckCursor    int    // the decks list's, kept while other screens use cursor
	field         int    // the options screen
	offset        int    // scroll, on screens that scroll
	input         *input

	mode       string // classic or leitner, for every deck
	decks      *engine.DeckList
	opts       options
	run        *engine.Started
	card       engine.Question
	asking     int // bumped per card, so an old card's tick stops
	asked      time.Time
	seconds    int
	scored     *engine.Scored
	moved      string // what the last score did to the card's Leitner box
	standing   []engine.EditEntry
	menu       []engine.MenuItem
	summary    *engine.Summary
	misses     *engine.MissReport
	boxes      *engine.BoxReport
	showAll    bool
	reportDeck string
	edits      *engine.EditReport
}

func newModel(eng Engine) model {
	return model{
		eng:  eng,
		now:  time.Now,
		tick: everySecond,
		exec: runEditor,
		mode: classic,
	}
}

func everySecond(asking int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{asking} })
}

// Init reads the settings, then the decks. The settings are a convenience: if
// they will not load, the TUI starts from its defaults.
func (m model) Init() tea.Cmd {
	return call(func() (*engine.Settings, error) {
		if s, err := m.eng.LoadSettings(); err == nil {
			return s, nil
		}
		return &engine.Settings{}, nil
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.offset = min(m.offset, m.maxOffset())
		// A resize only repaints the lines drawn, and whatever the terminal
		// left in the rest of the window stays; clearing it all takes that too.
		return m, tea.ClearScreen
	case tickMsg:
		if m.screen == askScreen && msg.asking == m.asking {
			return m, m.tick(m.asking)
		}
	case editorDone:
		if msg.err != nil {
			m.busy = false
			m.status = "The editor failed (" + msg.err.Error() + "); taking no edit."
			return m, nil
		}
		return m, call(m.eng.Reload)
	case reply:
		return m.receive(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		if m.input != nil {
			return m.typing(msg)
		}
		if m.screen == optionsScreen {
			return m.optionsKey(msg)
		}
		return m.key(msg.String())
	}
	return m, nil
}

func (m *model) show(s screen) {
	if m.screen == decksScreen {
		m.deckCursor = m.cursor
	}
	m.screen, m.offset, m.status, m.cursor = s, 0, "", 0
}

func (m model) receive(r reply) (tea.Model, tea.Cmd) {
	m.busy = false
	if r.err != nil {
		m.err = r.err.Error()
		return m, nil
	}
	m.err = ""
	switch v := r.value.(type) {
	case *engine.Settings:
		for _, s := range v.Saved {
			if s.Key == modeKey && (s.Value == classic || s.Value == leitner) {
				m.mode = s.Value
			}
		}
		m.busy = true
		return m, call(m.eng.Decks)
	case *engine.DeckList:
		m.decks = v
		m.cursor = max(min(m.cursor, len(v.Decks)-1), 0)
	case *engine.Started:
		m.run, m.summary = v, nil
		return m.ask(v.Card)
	case *engine.Question:
		return m.ask(*v)
	case *engine.Scored:
		m.scored, m.standing, m.moved = v, v.Standing, boxMove(v.Was, v.Box)
		if m.run.Edit {
			m.show(editScreen)
			return m, nil
		}
		return m.proceed()
	case *engine.Menu:
		m.show(menuScreen)
		m.menu = v.Followups
	case *engine.Summary:
		m.show(summaryScreen)
		m.summary = v
	case *engine.CardFile:
		m.busy = true
		return m, m.exec(v.Path)
	case *engine.Reloaded:
		m.card, m.standing = v.Card, v.Standing
		m.status = "Unchanged."
		if v.Changed {
			m.status = "Edited - anything standing against it is cleared."
		}
	case *engine.Standing:
		m.standing, m.status = v.Standing, m.pending
	case *engine.MissReport:
		m.show(missesScreen)
		m.misses = v
	case *engine.BoxReport:
		m.show(boxesScreen)
		m.boxes = v
	case *engine.Options:
		deck, ok := m.selected()
		if m.screen != decksScreen || !ok || deck.ID != v.Deck {
			return m, nil
		}
		cursor := m.cursor
		m.show(optionsScreen)
		m.cursor, m.field = cursor, rows(m.mode)[0]
		m.opts = defaultOptions(deck)
		m.opts.restore(v.Saved)
	case *engine.EditReport:
		m.show(editsScreen)
		m.edits = v
	}
	return m, nil
}

// -- the run ---------------------------------------------------------------

func (m model) ask(card engine.Question) (tea.Model, tea.Cmd) {
	m.show(askScreen)
	m.card, m.asked = card, m.now()
	m.asking++
	return m, m.tick(m.asking)
}

func (m model) elapsed() int {
	return int(m.now().Sub(m.asked).Seconds())
}

// proceed moves the run on after a card is scored and any editing is done.
func (m model) proceed() (tea.Model, tea.Cmd) {
	m.busy = true
	switch {
	case m.scored.Finished:
		return m, call(m.eng.End)
	case m.run.Auto:
		return m.next(engine.NextParams{Pick: engine.PickAuto})
	}
	return m, call(m.eng.Menu)
}

func (m model) next(p engine.NextParams) (tea.Model, tea.Cmd) {
	m.busy = true
	return m, call(func() (*engine.Question, error) { return m.eng.Next(p) })
}

func (m model) end() (tea.Model, tea.Cmd) {
	m.busy = true
	return m, call(m.eng.End)
}

// -- keys --------------------------------------------------------------------

func (m model) key(k string) (tea.Model, tea.Cmd) {
	switch m.screen {
	case decksScreen:
		return m.decksKey(k)
	case askScreen:
		switch k {
		case "enter", " ":
			m.seconds = m.elapsed()
			m.show(revealScreen)
		case "q", "esc":
			return m.end()
		}
	case revealScreen:
		scores := map[string]string{"p": "pass", "s": "shaky", "f": "fail"}
		if score, ok := scores[k]; ok {
			m.busy = true
			p := engine.ScoreParams{Score: score, Seconds: m.seconds}
			return m, call(func() (*engine.Scored, error) { return m.eng.Score(p) })
		}
		if k == "q" || k == "esc" {
			return m.end()
		}
		m.scroll(k)
	case editScreen:
		return m.editKey(k)
	case menuScreen:
		return m.menuKey(k)
	case summaryScreen:
		switch k {
		case "q":
			return m, tea.Quit
		case "enter", "esc":
			return m.backToDecks()
		}
		m.scroll(k)
	case missesScreen:
		switch k {
		case "a":
			m.showAll = !m.showAll
			return m.loadMisses(m.reportDeck)
		case "q", "esc":
			return m.backToDecks()
		}
		m.scroll(k)
	case boxesScreen, editsScreen:
		if k == "q" || k == "esc" {
			return m.backToDecks()
		}
		m.scroll(k)
	case sourcesScreen:
		if k == "q" || k == "esc" {
			m.screen, m.offset = optionsScreen, 0
			return m, nil
		}
		m.scroll(k)
	}
	return m, nil
}

func (m model) backToDecks() (tea.Model, tea.Cmd) {
	cursor := m.deckCursor
	m.show(decksScreen)
	m.cursor = cursor // back on the deck you left
	m.run, m.summary, m.misses, m.boxes, m.edits = nil, nil, nil, nil, nil
	m.busy = true
	return m, call(m.eng.Decks)
}

func (m model) selected() (engine.DeckInfo, bool) {
	if m.decks == nil || m.cursor >= len(m.decks.Decks) {
		return engine.DeckInfo{}, false
	}
	deck := m.decks.Decks[m.cursor]
	return deck, deck.Error == ""
}

func (m model) decksKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "q":
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		if m.decks != nil {
			m.cursor = max(min(m.cursor+1, len(m.decks.Decks)-1), 0)
		}
	case "r":
		m.busy = true
		return m, call(m.eng.Decks)
	case "tab", "shift+tab":
		m.mode = map[string]string{classic: leitner, leitner: classic}[m.mode]
		m.status = modeHint(m.mode)
		save := engine.SaveSettingsParams{Settings: []engine.Option{{Key: modeKey, Value: m.mode}}}
		// Remembering is a convenience: a failed save is dropped, as an
		// option's is, and nothing waits on it.
		return m, func() tea.Msg {
			_, _ = m.eng.SaveSettings(save)
			return nil
		}
	case "enter", "m", "b", "e":
		deck, ok := m.selected()
		if !ok {
			return m, nil
		}
		switch k {
		case "enter":
			m.busy = true
			p := engine.DeckParams{Deck: deck.ID}
			return m, call(func() (*engine.Options, error) { return m.eng.LoadOptions(p) })
		case "m":
			m.showAll = false
			return m.loadMisses(deck.ID)
		case "b":
			m.busy = true
			p := engine.DeckParams{Deck: deck.ID}
			return m, call(func() (*engine.BoxReport, error) { return m.eng.Boxes(p) })
		case "e":
			m.busy = true
			p := engine.DeckParams{Deck: deck.ID}
			return m, call(func() (*engine.EditReport, error) { return m.eng.Edits(p) })
		}
	}
	return m, nil
}

func (m model) loadMisses(deck string) (tea.Model, tea.Cmd) {
	m.busy, m.reportDeck = true, deck
	p := engine.MissParams{Deck: deck, All: m.showAll}
	return m, call(func() (*engine.MissReport, error) { return m.eng.Misses(p) })
}

func (m model) optionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	o := &m.opts
	// The filter row is a text field: what is typed on it is the filter.
	if m.field == filterField {
		switch msg.Type {
		case tea.KeyRunes:
			o.filter += string(msg.Runes)
			return m, nil
		case tea.KeySpace:
			o.filter += " "
			return m, nil
		case tea.KeyBackspace:
			if r := []rune(o.filter); len(r) > 0 {
				o.filter = string(r[:len(r)-1])
			}
			return m, nil
		case tea.KeyCtrlU:
			o.filter = ""
			return m, nil
		}
	}
	switch k := msg.String(); k {
	case "esc", "q":
		cursor, save := m.cursor, o.saveParams()
		m.show(decksScreen)
		m.cursor, m.err, m.busy = cursor, "", true
		return m, call(func() (*engine.DeckList, error) {
			// Remembering is a convenience: a failed save is dropped rather
			// than keeping you here, and the same goes for starting a run.
			_, _ = m.eng.SaveOptions(save)
			return m.eng.Decks()
		})
	case "up", "k", "shift+tab":
		shown := rows(m.mode)
		m.field = shown[max(slices.Index(shown, m.field)-1, 0)]
	case "down", "j", "tab":
		shown := rows(m.mode)
		m.field = shown[min(slices.Index(shown, m.field)+1, len(shown)-1)]
	case "/":
		m.field = filterField
	case "c":
		if len(o.deck.Cards) > 0 {
			// Not show: back on the options, the field and the rest stay put.
			m.screen, m.offset = sourcesScreen, 0
		}
	case "left", "h", "-":
		o.wheels[m.field].turn(-1)
	case "right", "l", "+":
		o.wheels[m.field].turn(1)
	case " ":
		if w := &o.wheels[m.field]; len(w.values) == 2 {
			w.at = 1 - w.at
		} else {
			w.turn(1)
		}
	case "enter", "s":
		m.busy = true
		count := o.wheels[countField].value()
		var limit *int
		if v := o.wheels[limitField].value(); v != autoLimit {
			limit = &v
		}
		p := engine.StartParams{
			Deck: o.deck.ID, Count: &count, Limit: limit, Auto: o.on(autoField),
			Drafts: o.on(draftsField), Filter: strings.TrimSpace(o.filter),
			// A saved miss list stays saved in Leitner mode, but is not its to use.
			Weak: o.on(weakField) && m.mode != leitner, Due: m.mode == leitner,
			Edit: o.on(editField), Log: true,
		}
		save := o.saveParams()
		return m, call(func() (*engine.Started, error) {
			_, _ = m.eng.SaveOptions(save)
			return m.eng.Start(p)
		})
	}
	return m, nil
}

// boxLabel is a Leitner box as the run shows it.
func boxLabel(box int) string {
	if box == 0 {
		return "new"
	}
	return "box " + strconv.Itoa(box)
}

// boxMove is what a score did to the card's box, in a few words.
func boxMove(was, box int) string {
	switch {
	case box > max(was, 1):
		return boxLabel(was) + " → box " + strconv.Itoa(box)
	case box < was:
		return boxLabel(was) + " → box " + strconv.Itoa(box) + ", back to the start"
	case was == 0:
		return "new → box " + strconv.Itoa(box)
	}
	return "stays in box " + strconv.Itoa(box)
}

func (m model) editKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "enter":
		return m.proceed()
	case "q", "esc":
		return m.end()
	case "e":
		m.busy = true
		return m, call(m.eng.EditCard)
	case "n":
		m.input = &input{kind: "note", prompt: "Note"}
	case "x":
		m.input = &input{kind: "flag", prompt: "Why remove it? (enter for no reason)"}
	case "d":
		if len(m.standing) > 0 {
			m.busy, m.pending = true, "Cleared."
			return m, call(m.eng.Done)
		}
	default:
		m.scroll(k)
	}
	return m, nil
}

func (m model) menuKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "q", "esc":
		return m.end()
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, len(m.menu)) // the last row is a random card
	case "r":
		return m.next(engine.NextParams{Pick: engine.PickRandom})
	case "enter":
		if m.cursor < len(m.menu) {
			return m.next(engine.NextParams{Pick: engine.PickFollowup, Card: m.menu[m.cursor].ID})
		}
		return m.next(engine.NextParams{Pick: engine.PickRandom})
	default:
		if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(m.menu) {
			return m.next(engine.NextParams{Pick: engine.PickFollowup, Card: m.menu[n-1].ID})
		}
	}
	return m, nil
}

func (m model) typing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	in := m.input
	switch msg.Type {
	case tea.KeyEsc:
		m.input = nil
	case tea.KeyEnter:
		m.input = nil
		text := string(in.text)
		switch in.kind {
		case "note":
			m.busy, m.pending = true, "Noted."
			return m, call(func() (*engine.Standing, error) {
				return m.eng.Note(engine.NoteParams{Text: text})
			})
		case "flag":
			m.busy, m.pending = true, "Flagged for removal."
			return m, call(func() (*engine.Standing, error) {
				return m.eng.Flag(engine.FlagParams{Reason: text})
			})
		}
	case tea.KeyBackspace:
		if len(in.text) > 0 {
			in.text = in.text[:len(in.text)-1]
		}
	case tea.KeyCtrlU:
		in.text = nil
	case tea.KeySpace:
		in.text = append(in.text, ' ')
	case tea.KeyRunes:
		in.text = append(in.text, msg.Runes...)
	}
	return m, nil
}

// -- scrolling ---------------------------------------------------------------

func (m *model) scroll(k string) {
	page := max(m.room(m.page())/2, 1)
	switch k {
	case "down", "j":
		m.offset++
	case "up", "k":
		m.offset--
	case "pgdown", "ctrl+d":
		m.offset += page
	case "pgup", "ctrl+u":
		m.offset -= page
	case "home", "g":
		m.offset = 0
	case "end", "G":
		m.offset = m.maxOffset()
	}
	m.offset = max(min(m.offset, m.maxOffset()), 0)
}

func (m model) maxOffset() int {
	p := m.page()
	room := m.room(p)
	if room <= 0 || p.panes != nil {
		return 0
	}
	return max(len(p.body)-room, 0)
}
