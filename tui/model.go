package main

import (
	"os"
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
	CreateDeck(engine.NewDeckParams) (*engine.NewDeck, error)
	CreateCard(engine.NewCardParams) (*engine.NewCard, error)
	CheckDeck(engine.DeckParams) (*engine.Problems, error)
	ListCards(engine.DeckParams) (*engine.CardList, error)
	DeleteCard(engine.DeleteCardParams) (*engine.Trashed, error)
	DeleteDeck(engine.DeleteDeckParams) (*engine.Trashed, error)
	UpdateCard(engine.UpdateCardParams) (*engine.NewCard, error)
	UpdateDeck(engine.UpdateDeckParams) (*engine.NewDeck, error)
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
	settingsScreen
	libraryScreen
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

// The settings screen's rows. The mode is the app's own; the count and the
// seconds are what a deck starts from when it says nothing of its own.
const (
	modeRow = iota
	countPref
	limitPref
	entitiesPref
	colourModePref   // where the highlight colour comes from
	colourPref       // and which: of the theme's, of the list, or the wheel's
	addColourPref    // add a colour to the list
	deleteColourPref // take the one showing off it
	prefRows
)

// The settings screen's tabs: how decks are run, how cards are edited, and how the TUI
// looks. The arrows change tab when no row is selected. Each tab's rows are rowsOf.
var settingsTabs = []string{"Decks", "Editing", "Visual"}

// rowsOf are the rows of a settings tab. The Visual tab's depend on the colour mode:
// the custom list can be added to, and taken from once it has something in it.
func (m model) rowsOf(tab int) []int {
	switch tab {
	case 0:
		return []int{modeRow, countPref, limitPref}
	case 1:
		return []int{entitiesPref}
	}
	rows := []int{colourModePref, colourPref}
	if m.prefs.colourMode == customMode {
		rows = append(rows, addColourPref)
		if len(m.prefs.custom) > 0 {
			rows = append(rows, deleteColourPref)
		}
	}
	return rows
}

// prefs are the defaults the settings screen holds for the options screen, how a
// card is opened for editing, and how the TUI looks.
type prefs struct {
	count    wheel
	limit    wheel
	entities wheel // as written, as characters: an emoji's code turned into the character

	colourMode int            // themeMode, customMode or wheelMode
	theme      wheel          // the terminal's 16 colours, for themeMode
	custom     []customColour // the colours the user added, for customMode
	customAt   int            // the one chosen
	picked     string         // the colour picked on the wheel, for wheelMode, as #rrggbb
}

// defaultPickedColour is where the wheel starts before anything has been picked: a cyan
// that is a place on the wheel exactly (full saturation, four fifths bright).
const defaultPickedColour = "#00cccc"

func defaultPrefs() prefs {
	p := prefs{count: countWheel(10), limit: limitWheel(), theme: themeWheel(),
		entities: wheel{labels: []string{"as written", "as characters"}, values: []int{0, 1}},
		picked:   defaultPickedColour}
	p.entities.at = 1 // on, as the card view already shows them
	return p
}

// decodeEntities is whether a card's character references, such as &#x1f534;, are
// turned into the characters they stand for when the card is opened for editing.
// The card is then saved with the characters.
func (p prefs) decodeEntities() bool { return p.entities.value() == 1 }

// The keys the settings screen is saved under, the same names and values the
// options screen saves, so a setting reads like an option. (The colour's are in
// colourprefs.go.)
var prefKeys = [prefRows]string{
	modeRow: modeKey, countPref: "count", limitPref: "limit", entitiesPref: "entities",
}

func (p prefs) saved(mode string) []engine.Option {
	out := []engine.Option{
		{Key: prefKeys[modeRow], Value: mode},
		{Key: prefKeys[countPref], Value: countText(p.count.value())},
		{Key: prefKeys[limitPref], Value: limitText(p.limit.value())},
		{Key: prefKeys[entitiesPref], Value: map[bool]string{true: "on", false: "off"}[p.decodeEntities()]},
	}
	for _, kv := range p.colourSaved() {
		out = append(out, engine.Option{Key: kv.Key, Value: kv.Value})
	}
	return out
}

// restore sets each default from what was saved, by key, passing over a key or
// a value it does not know as options.restore does.
func (p *prefs) restore(saved []engine.Option) {
	// The list is read before the choice from it, whatever order they were saved in.
	for _, s := range saved {
		if s.Key == keyColourCustom {
			p.restoreColours(s.Key, s.Value)
		}
	}
	for _, s := range saved {
		switch s.Key {
		case prefKeys[countPref]:
			p.count.setCount(s.Value)
		case prefKeys[limitPref]:
			p.limit.setLimit(s.Value)
		case prefKeys[entitiesPref]:
			switch strings.TrimSpace(s.Value) {
			case "on", "true":
				p.entities.at = 1
			case "off", "false":
				p.entities.at = 0
			}
		case keyColourCustom:
		default:
			p.restoreColours(s.Key, s.Value)
		}
	}
}

// defaultOptions is the options screen for a deck never run from it: the deck's
// own count if it has one, else the saved default. A deck's own limit is the
// engine's to apply, as "auto", so only a deck without one takes the default.
func defaultOptions(deck engine.DeckInfo, p prefs) options {
	o := options{deck: deck}
	o.wheels[countField] = p.count
	if deck.Count != nil {
		o.wheels[countField].set(*deck.Count, countLabel(*deck.Count))
	}
	o.wheels[limitField] = limitWheel()
	if deck.Limit == nil {
		o.wheels[limitField] = p.limit
	}
	for field := autoField; field < filterField; field++ {
		o.wheels[field] = toggle()
	}
	return o
}

// countText and limitText are a wheel's value as it is saved.
func countText(v int) string { return strconv.Itoa(v) }

func limitText(v int) string {
	switch v {
	case autoLimit:
		return "auto"
	case 0:
		return "off"
	}
	return strconv.Itoa(v)
}

// setCount and setLimit turn a wheel to a saved value; one it cannot read leaves
// the wheel where it was.
func (w *wheel) setCount(value string) {
	if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && n >= 0 {
		w.set(n, countLabel(n))
	}
}

func (w *wheel) setLimit(value string) {
	switch value = strings.TrimSpace(value); value {
	case "auto":
		w.set(autoLimit, limitLabel(autoLimit))
	case "off":
		w.set(0, limitLabel(0))
	default:
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			w.set(n, limitLabel(n))
		}
	}
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
			value = countText(v)
		case limitField:
			value = limitText(v)
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
			o.wheels[field].setCount(value)
		case limitField:
			o.wheels[field].setLimit(value)
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

// authoring is a card just written by the engine and handed to the editor.
type authoring struct {
	deck, id, path string
	verb           string // "Made" or "Edited"; id is "" when it is the deck's own file
}

// A confirm is a question the next key answers: y does it, anything else drops it.
type confirm struct {
	prompt string
	deck   string
	card   string // "": the deck itself
	colour bool   // a colour of the custom list, not a deck or a card
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
	authoring     *authoring // a card or deck file open in the editor
	confirm       *confirm
	libFocus      int // the library's pane: 0 the decks, 1 the cards
	libDeck       int // the library's deck
	libCard       int // the library's card
	library       *engine.CardList
	newDeck       string // a deck just made, for the list to put the cursor on
	form          *form
	saving        *saving // a form's save in flight
	newCard       string  // a card just made or saved, for the list to put the cursor on

	mode          string        // classic or leitner, for every deck
	tmux          bool          // running inside tmux, which draws emoji differently
	prefs         prefs         // the settings screen's defaults for the options screen
	pref          int           // the settings screen's row
	prefTab       int           // the settings screen's tab
	colourPick    *colourPicker // the colour wheel, open
	prefSelecting bool          // the row is selected: left and right change it
	decks         *engine.DeckList
	opts          options
	run           *engine.Started
	card          engine.Question
	asking        int // bumped per card, so an old card's tick stops
	asked         time.Time
	seconds       int
	scored        *engine.Scored
	moved         string // what the last score did to the card's Leitner box
	standing      []engine.EditEntry
	menu          []engine.MenuItem
	summary       *engine.Summary
	misses        *engine.MissReport
	boxes         *engine.BoxReport
	showAll       bool
	reportDeck    string
	edits         *engine.EditReport
}

func newModel(eng Engine) model {
	return model{
		eng:   eng,
		now:   time.Now,
		tick:  everySecond,
		exec:  runEditor,
		mode:  classic,
		prefs: defaultPrefs(),
		tmux:  os.Getenv("TMUX") != "",
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
		if m.authoring != nil {
			return m.authored(msg.err)
		}
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
		if m.confirm != nil {
			return m.confirming(msg.String())
		}
		if m.input != nil {
			return m.typing(msg)
		}
		if m.screen == optionsScreen {
			return m.optionsKey(msg)
		}
		if m.colourPick != nil { // the colour wheel is open: the keys are its own
			return m.colourKey(msg.String())
		}
		if m.form != nil { // a card or deck being written, in the library, in place
			return m.formKey(msg)
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
		m.prefs.restore(v.Saved)
		applyTheme(m.prefs.highlight())
		m.busy = true
		return m, call(m.eng.Decks)
	case *engine.DeckList:
		m.decks = v
		m.cursor = max(min(m.cursor, len(v.Decks)-1), 0)
		if i := slices.IndexFunc(v.Decks, func(d engine.DeckInfo) bool { return d.ID == m.newDeck }); i >= 0 {
			m.cursor, m.libDeck, m.libCard = i, i, 0
		}
		m.newDeck = ""
		if m.screen == libraryScreen {
			m.libDeck = max(min(m.libDeck, len(v.Decks)-1), 0)
			return m, m.loadCards()
		}
	case *engine.CardList:
		if deck, ok := m.libraryDeck(); ok && deck.ID == v.Deck && m.screen == libraryScreen {
			m.library = v
			m.libCard = max(min(m.libCard, len(v.Cards)-1), 0)
			if i := slices.IndexFunc(v.Cards, func(c engine.CardSummary) bool { return c.ID == m.newCard }); i >= 0 {
				m.libCard = i
			}
			m.newCard = ""
		}
	case *engine.Trashed:
		m.status = "Moved " + v.ID + " to " + tilde(v.Trash) + "."
		m.libCard, m.busy = max(m.libCard-1, 0), true
		return m, call(m.eng.Decks)
	case *engine.NewDeck:
		sv := m.saving
		m.saving, m.form, m.screen, m.newDeck = nil, nil, libraryScreen, v.ID
		m.status = sv.verb + " " + v.ID + "."
		if sv.openAfter {
			path := v.Path + "/deck.toml"
			m.authoring, m.busy = &authoring{deck: v.ID, path: path, verb: "Edited"}, true
			return m, m.exec(path)
		}
		m.busy = true
		return m, call(m.eng.Decks)
	case *engine.NewCard:
		sv := m.saving
		deck := m.form.deck
		m.saving, m.form, m.screen, m.newCard = nil, nil, libraryScreen, v.ID
		m.status = sv.verb + " " + v.ID + "."
		if sv.openAfter {
			m.authoring, m.busy = &authoring{deck: deck, id: v.ID, path: v.Path, verb: "Edited"}, true
			return m, m.exec(v.Path)
		}
		m.busy = true
		return m, call(m.eng.Decks)
	case *engine.Problems:
		m.status = m.authoring.verb + " the card " + m.authoring.id + "."
		if n := len(v.Problems); n > 0 {
			m.status += " " + plural(n, "problem") + ": " + v.Problems[0]
		}
		m.authoring, m.busy = nil, true
		return m, call(m.eng.Decks)
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
		m.opts = defaultOptions(deck, m.prefs)
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
	if to, ok := m.tabFor(k); ok {
		return m.goTab(to)
	}
	switch m.screen {
	case decksScreen:
		return m.decksKey(k)
	case libraryScreen:
		return m.libraryKey(k)
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
	case settingsScreen:
		return m.settingsKey(k)
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
		return m, m.saveSettings()
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

// The screens along the top, in order. Each is a place of its own, apart from
// the keys the screen you are on offers.
var tabs = []struct {
	screen screen
	name   string
}{{decksScreen, "Decks"}, {libraryScreen, "Library"}, {settingsScreen, "Settings"}}

func tabIndex(s screen) int {
	return slices.IndexFunc(tabs, func(t struct {
		screen screen
		name   string
	}) bool {
		return t.screen == s
	})
}

// tabFor is the screen a key switches to from the tab screen you are on: 1-3
// for each, and [ and ] for the one before and after.
func (m model) tabFor(k string) (screen, bool) {
	at := tabIndex(m.screen)
	if at < 0 {
		return 0, false
	}
	switch k {
	case "[":
		return tabs[max(at-1, 0)].screen, true
	case "]":
		return tabs[min(at+1, len(tabs)-1)].screen, true
	}
	if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(tabs) {
		return tabs[n-1].screen, true
	}
	return 0, false
}

// sel is the deck the tab screens share: the one you had highlighted when you
// left the last of them.
func (m model) sel() int {
	switch m.screen {
	case decksScreen:
		return m.cursor
	case libraryScreen:
		return m.libDeck
	}
	return m.deckCursor
}

// goTab switches to another tab screen, keeping the highlighted deck.
func (m model) goTab(to screen) (tea.Model, tea.Cmd) {
	if to == m.screen {
		return m, nil
	}
	sel := m.sel()
	m.deckCursor = sel
	m.show(to)
	m.err = ""
	switch to {
	case decksScreen:
		m.cursor, m.busy = sel, true
		return m, call(m.eng.Decks) // what the library did shows in the counts
	case libraryScreen:
		m.libDeck, m.libCard, m.libFocus, m.library = sel, 0, 0, nil
		return m, m.loadCards()
	case settingsScreen:
		m.pref, m.prefTab, m.prefSelecting = modeRow, 0, false
	}
	return m, nil
}

// -- the library --------------------------------------------------------------

// libraryDeck is the deck the library's cursor is on, and whether it loaded.
func (m model) libraryDeck() (engine.DeckInfo, bool) {
	if m.decks == nil || m.libDeck >= len(m.decks.Decks) {
		return engine.DeckInfo{}, false
	}
	deck := m.decks.Decks[m.libDeck]
	return deck, deck.Error == ""
}

// loadCards asks for the cards of the deck the library is on. A deck that will
// not load has none to list, and the page says why.
func (m model) loadCards() tea.Cmd {
	deck, ok := m.libraryDeck()
	if !ok {
		return nil
	}
	p := engine.DeckParams{Deck: deck.ID}
	return call(func() (*engine.CardList, error) { return m.eng.ListCards(p) })
}

// libraryCard is the card the library's cursor is on.
func (m model) libraryCard() (engine.CardSummary, bool) {
	if m.library == nil || m.libCard >= len(m.library.Cards) {
		return engine.CardSummary{}, false
	}
	return m.library.Cards[m.libCard], true
}

// libraryKey is the library: decks on the left, the highlighted deck's cards on
// the right, and n, e and d act on whichever pane you are in.
func (m model) libraryKey(k string) (tea.Model, tea.Cmd) {
	if m.libFocus == 0 {
		return m.libraryDecksKey(k)
	}
	return m.libraryCardsKey(k)
}

func (m model) libraryDecksKey(k string) (tea.Model, tea.Cmd) {
	_, ok := m.libraryDeck()
	switch k {
	case "esc", "q":
		return m.goTab(decksScreen)
	case "up", "k", "down", "j":
		if m.decks != nil {
			step := -1
			if k == "down" || k == "j" {
				step = 1
			}
			m.libDeck = max(min(m.libDeck+step, len(m.decks.Decks)-1), 0)
			m.libCard, m.library = 0, nil
			return m, m.loadCards()
		}
	case "right", "l", "enter":
		if ok {
			m.libFocus = 1
		}
	case "n":
		m = m.openForm(newDeckForm())
	case "e":
		if m.decks != nil && m.libDeck < len(m.decks.Decks) {
			deck := m.decks.Decks[m.libDeck]
			if deck.Error != "" {
				m.status = "That deck will not load: o opens its deck.toml in the editor."
				return m, nil
			}
			m = m.openForm(editDeckForm(deck, m.prefs.decodeEntities()))
		}
	case "o":
		if m.decks != nil && m.libDeck < len(m.decks.Decks) {
			deck := m.decks.Decks[m.libDeck]
			if deck.Path == nil {
				return m, nil
			}
			path := *deck.Path + "/deck.toml"
			m.authoring = &authoring{deck: deck.ID, path: path, verb: "Edited"}
			m.busy = true
			return m, m.exec(path)
		}
	case "d":
		if m.decks != nil && m.libDeck < len(m.decks.Decks) {
			deck := m.decks.Decks[m.libDeck]
			m.confirm = &confirm{deck: deck.ID, prompt: deckDeletion(deck)}
		}
	}
	return m, nil
}

func (m model) libraryCardsKey(k string) (tea.Model, tea.Cmd) {
	deck, _ := m.libraryDeck()
	card, have := m.libraryCard()
	switch k {
	case "esc", "q", "left", "h":
		m.libFocus = 0
	case "up", "k":
		m.libCard = max(m.libCard-1, 0)
	case "down", "j":
		if m.library != nil {
			m.libCard = max(min(m.libCard+1, len(m.library.Cards)-1), 0)
		}
	case "home", "g":
		m.libCard = 0
	case "end", "G":
		if m.library != nil {
			m.libCard = max(len(m.library.Cards)-1, 0)
		}
	case "n":
		m = m.openForm(newCardForm(deck.ID))
	case "e", "enter":
		if have {
			m = m.openForm(editCardForm(deck.ID, card, m.prefs.decodeEntities()))
		}
	case "o":
		if have {
			m.authoring = &authoring{deck: deck.ID, id: card.ID, path: card.Path, verb: "Edited"}
			m.busy = true
			return m, m.exec(card.Path)
		}
	case "d":
		if have {
			m.confirm = &confirm{deck: deck.ID, card: card.ID, prompt: "Move the card " + card.ID + " to .trash?"}
		}
	}
	return m, nil
}

// deckDeletion is the question asked before a deck is deleted, saying what
// goes with it: a deck's own folder takes the cards inside it, while the cards
// of a linked deck, or one that names a bank elsewhere, stay where they are.
func deckDeletion(deck engine.DeckInfo) string {
	q := "Move the deck " + deck.ID + " to .trash? "
	if deck.CardsInside {
		return q + "Its cards are inside it, so they go too."
	}
	return q + "Its cards are kept elsewhere and stay there."
}

// confirming is the key that answers a confirm: y does it, any other drops it.
func (m model) confirming(k string) (tea.Model, tea.Cmd) {
	c := m.confirm
	m.confirm = nil
	if k != "y" && k != "Y" {
		m.status = "Kept."
		return m, nil
	}
	if c.colour {
		m.prefs.deleteCustom()
		applyTheme(m.prefs.highlight())
		m.status = "Removed it from the list."
		if m.pref == deleteColourPref && len(m.prefs.custom) == 0 {
			m.pref = colourPref // the row it was on is gone with the last colour
		}
		return m, m.saveSettings()
	}
	m.busy = true
	if c.card != "" {
		p := engine.DeleteCardParams{Deck: c.deck, Card: c.card}
		return m, call(func() (*engine.Trashed, error) { return m.eng.DeleteCard(p) })
	}
	m.libFocus = 0
	p := engine.DeleteDeckParams{Deck: c.deck}
	return m, call(func() (*engine.Trashed, error) { return m.eng.DeleteDeck(p) })
}

// saveSettings remembers the settings. Remembering is a convenience: a failed
// save is dropped, as an option's is, and nothing waits on it.
func (m model) saveSettings() tea.Cmd {
	save := engine.SaveSettingsParams{Settings: m.prefs.saved(m.mode)}
	return func() tea.Msg {
		_, _ = m.eng.SaveSettings(save)
		return nil
	}
}

// settingsKey turns the settings screen's rows. Each change is saved as it is
// made, so leaving is just going back to the decks, on the deck you left.
func (m model) settingsKey(k string) (tea.Model, tea.Cmd) {
	// The arrow keys move between rows and tabs, so a row has to be entered to be
	// changed: enter selects it, left and right turn it, and enter or esc lets go.
	if !m.prefSelecting {
		rows := m.rowsOf(m.prefTab)
		at := slices.Index(rows, m.pref)
		if at < 0 { // the row it was on has gone: the colour list lost its last colour
			m.pref, at = rows[0], 0
		}
		switch k {
		case "esc", "q":
			return m.goTab(decksScreen)
		case "up", "k", "shift+tab":
			m.pref = rows[max(at-1, 0)]
		case "down", "j", "tab":
			m.pref = rows[min(at+1, len(rows)-1)]
		case "left", "h":
			m.prefTab = max(m.prefTab-1, 0)
			m.pref = m.rowsOf(m.prefTab)[0]
		case "right", "l":
			m.prefTab = min(m.prefTab+1, len(settingsTabs)-1)
			m.pref = m.rowsOf(m.prefTab)[0]
		case "enter":
			return m.enterRow()
		}
		return m, nil
	}
	turn := 0
	switch k {
	case "enter", "esc":
		m.prefSelecting = false
		return m, nil
	case "tab", "shift+tab":
		m.prefSelecting = false
		return m.settingsKey(k)
	case "left", "h", "-":
		turn = -1
	case "right", "l", "+":
		turn = 1
	case " ":
		turn = 1
		if (m.pref == modeRow && m.mode == leitner) || (m.pref == entitiesPref && m.prefs.decodeEntities()) {
			turn = -1 // the mode and the entities are two values, so space flips them
		}
	}
	if turn == 0 {
		return m, nil
	}
	switch m.pref {
	case modeRow:
		m.mode = map[int]string{-1: classic, 1: leitner}[turn]
	case countPref:
		m.prefs.count.turn(turn)
	case limitPref:
		m.prefs.limit.turn(turn)
	case entitiesPref:
		m.prefs.entities.turn(turn)
	case colourModePref:
		m.prefs.colourMode = max(min(m.prefs.colourMode+turn, len(colourModeNames)-1), 0)
		applyTheme(m.prefs.highlight())
	case colourPref:
		switch m.prefs.colourMode {
		case themeMode:
			m.prefs.theme.turn(turn)
		case customMode:
			m.prefs.customAt = max(min(m.prefs.customAt+turn, len(m.prefs.custom)-1), 0)
		}
		applyTheme(m.prefs.highlight())
	}
	return m, m.saveSettings()
}

// enterRow is enter on a row when none is selected: most are selected, to be turned;
// the colour wheel opens; adding a colour asks for it; removing one asks first.
func (m model) enterRow() (tea.Model, tea.Cmd) {
	switch m.pref {
	case colourPref:
		switch m.prefs.colourMode {
		case wheelMode:
			m.colourPick = newColourPicker(m.prefs.picked)
			return m, nil
		case customMode:
			if len(m.prefs.custom) == 0 {
				m.status = "No colours yet: add one with the row below."
				return m, nil
			}
		}
	case addColourPref:
		m = m.openForm(newColourForm())
		return m, nil
	case deleteColourPref:
		if len(m.prefs.custom) > 0 {
			m.confirm = &confirm{colour: true, prompt: "Remove " + m.prefs.custom[m.prefs.customAt].Name + " from your colours?"}
		}
		return m, nil
	}
	m.prefSelecting = true
	return m, nil
}

// colourKey is the colour wheel: the arrows move in the square, or along the strip of
// hues; enter takes the colour, and esc puts back the one that was there.
func (m model) colourKey(k string) (tea.Model, tea.Cmd) {
	c := m.colourPick
	switch k {
	case "esc", "q":
		m.colourPick = nil
		applyTheme(m.prefs.highlight())
		return m, nil
	case "enter":
		m.prefs.picked = c.hex()
		m.colourPick = nil
		applyTheme(m.prefs.highlight())
		m.status = "Colour " + m.prefs.picked + "."
		return m, m.saveSettings()
	case "tab", "shift+tab":
		c.strip = !c.strip
	case "left", "h":
		c.move(-1, 0)
	case "right", "l":
		c.move(1, 0)
	case "up", "k":
		c.move(0, -1)
	case "down", "j":
		c.move(0, 1)
	case "shift+left", "H":
		c.move(-4, 0)
	case "shift+right", "L":
		c.move(4, 0)
	case "shift+up", "K":
		c.move(0, -3)
	case "shift+down", "J":
		c.move(0, 3)
	case ",":
		c.turn(-1)
	case ".":
		c.turn(1)
	}
	applyTheme(c.hex()) // the highlight is the colour under the cursor, live
	return m, nil
}

// authored is the editor closing on a card the engine has just written. The
// card is there whatever happened in the editor, so it is only reported; what
// the editor left in it is the engine's to check.
func (m model) authored(err error) (tea.Model, tea.Cmd) {
	a := m.authoring
	if err != nil {
		m.authoring, m.busy = nil, true
		m.status = "The editor failed (" + err.Error() + "); the file is at " + tilde(a.path) + "."
		return m, call(m.eng.Decks)
	}
	m.busy = true
	if a.id == "" { // the deck's own file: a bad one shows on the deck list
		m.authoring = nil
		m.status = a.verb + " " + tilde(a.path) + "."
		return m, call(m.eng.Decks)
	}
	p := engine.DeckParams{Deck: a.deck}
	return m, call(func() (*engine.Problems, error) { return m.eng.CheckDeck(p) })
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
