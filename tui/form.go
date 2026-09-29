package main

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"lernt/tui/engine"
)

// A form is one deck or one card being written in the library: the boxes the text
// goes in, the options that are not text, and the way to save it or leave it.
// Nothing is written until it is saved, and what is written is what the engine's
// calls take, so the editor is only ever the way to say what a form cannot.
//
// The arrow keys move between the views of a form, so a view has to be entered
// before it is used: enter on a box starts typing in it, enter on an option
// selects it so that left and right change it, and esc comes back out. tab and
// shift+tab move between views as a backup. In a box enter is always a newline.

type formKind int

const (
	kindNewCard formKind = iota
	kindEditCard
	kindNewDeck
	kindEditDeck
	kindNewColour // in the settings: a colour for the custom list
)

type fieldKind int

const (
	textField   fieldKind = iota // a box of text, in the left column
	wheelField                   // a choice turned left and right
	linkField                    // cards chosen from the deck's own list
	buttonField                  // enter does it
)

// What a form is doing with the view the cursor is on.
type formMode int

const (
	navigating formMode = iota // the arrow keys move between views
	typing                     // the keys are the text's
	selecting                  // left and right change an option
)

const (
	keyTitle     = "title"
	keyAnswer    = "answer"
	keyNotes     = "notes"
	keyReference = "reference"
	keyTime      = "time"
	keyDraft     = "draft"
	keyFollows   = "follows"
	keyName      = "name"
	keyValue     = "value"
	keyAbout     = "about"
	keySave      = "save"
	keyOpen      = "open"
	keyCancel    = "cancel"
)

type formField struct {
	key   string
	label string
	kind  fieldKind
	text  []rune
	pos   int // where the cursor is in text, in runes
	rows  int // how many lines tall its box is
	wheel wheel
	links []string // card ids, for a linkField
	hint  string   // what the view is, under the form
}

type form struct {
	kind   formKind
	deck   string // the deck's id
	card   string // the card's id, when it is being edited
	title  string
	fields []formField
	at     int
	mode   formMode
	memo   [3]int // the view last on in each column, for the arrow that comes back
	picker *picker
}

// A picker is the deck's cards, to tick a card's follow-ups from. It is drawn in
// the library's list of cards, which is why the card shown does not change.
type picker struct {
	cards  []engine.CardSummary
	self   int // the card being edited, which cannot be ticked; -1 for a new one
	chosen map[string]bool
	at     int
}

// saving is a save in flight: what to say when it lands, and whether the file
// is then to be opened in the editor.
type saving struct {
	verb      string
	openAfter bool
	deck      bool
}

func textRow(key, label, hint, value string) formField {
	return formField{key: key, label: label, kind: textField, text: []rune(value),
		pos: len([]rune(value)), hint: hint}
}

func actions() []formField {
	return []formField{
		{key: keySave, label: "Save", kind: buttonField, hint: "Save it and go back."},
		{key: keyOpen, label: "Open in editor", kind: buttonField,
			hint: "Save it, then open its file in $VISUAL or $EDITOR, for what these boxes cannot say."},
		{key: keyCancel, label: "Cancel", kind: buttonField, hint: "Go back without saving."},
	}
}

func newCardForm(deck string) *form {
	f := &form{kind: kindNewCard, deck: deck, title: "New card"}
	f.fields = []formField{
		textRow(keyTitle, "Question", "The question. The card's file is named from it.", ""),
		textRow(keyAnswer, "Answer", "The answer, shown when the question is revealed.", ""),
		textRow(keyNotes, "Notes", "Anything to show beside the answer.", ""),
		textRow(keyReference, "Reference", "A link to a longer note, such as [[Some note]].", ""),
		{key: keyTime, label: "Time", kind: wheelField, wheel: limitWheel(),
			hint: "Seconds for this card. auto: the run's, else the deck's. off: untimed."},
		{key: keyDraft, label: "Draft", kind: wheelField, wheel: toggle(),
			hint: "A draft stays out of random jumps unless a run asks for drafts."},
		{key: keyFollows, label: "Leads to", kind: linkField,
			hint: "The cards this one can lead on to. Enter picks them from the card list above."},
	}
	f.fields = append(f.fields, actions()...)
	return f
}

// editCardForm is a form filled from a card. With decode, the character references
// in its text (&#x1f534;) are turned into the characters they stand for, so the
// card is saved with them: what the setting called Emoji codes, as characters, does.
func editCardForm(deck string, c engine.CardSummary, decode bool) *form {
	f := newCardForm(deck)
	f.kind, f.card, f.title = kindEditCard, c.ID, "Edit card"
	text := func(s string) string {
		if decode {
			return decodeReferences(s)
		}
		return s
	}
	f.set(keyTitle, text(c.Title))
	f.set(keyAnswer, text(strings.Join(c.Answer, "\n")))
	f.set(keyNotes, text(strings.Join(c.Notes, "\n")))
	f.set(keyReference, text(c.Reference))
	if c.Time != nil {
		f.field(keyTime).wheel.setLimit(limitText(*c.Time))
	}
	if c.Draft {
		f.field(keyDraft).wheel.at = 1
	}
	f.field(keyFollows).links = slices.Clone(c.Follows)
	return f
}

// newColourForm is the form that adds a colour to the settings' custom list: the colour
// in whatever form the user has it, and a name. It has no options, and no editor.
func newColourForm() *form {
	f := &form{kind: kindNewColour, title: "Add a colour"}
	f.fields = []formField{
		textRow(keyValue, "Colour", "The colour, in any form: hex like #ff8800, a number from 0 to 255, or r,g,b like 255,136,0.", ""),
		textRow(keyName, "Name", "What to call it in your list, up to 24 characters.", ""),
		{key: keySave, label: "Add", kind: buttonField, hint: "Add it to your list, and use it."},
		{key: keyCancel, label: "Cancel", kind: buttonField, hint: "Go back without adding it."},
	}
	return f
}

func newDeckForm() *form {
	f := &form{kind: kindNewDeck, title: "New deck"}
	f.fields = append([]formField{
		textRow(keyName, "Name", "The deck's name. Its id, and its folder, are made from it.", ""),
		textRow(keyAbout, "About", "A line on what the deck is.", ""),
	}, actions()...)
	return f
}

func editDeckForm(deck engine.DeckInfo, decode bool) *form {
	f := newDeckForm()
	f.kind, f.deck, f.title = kindEditDeck, deck.ID, "Edit deck"
	name, about := deck.Name, deck.Desc
	if decode {
		name, about = decodeReferences(name), decodeReferences(about)
	}
	f.set(keyName, name)
	f.set(keyAbout, about)
	return f
}

func (f *form) field(key string) *formField {
	i := slices.IndexFunc(f.fields, func(x formField) bool { return x.key == key })
	return &f.fields[i]
}

func (f *form) set(key, value string) {
	fld := f.field(key)
	fld.text = []rune(value)
	fld.pos = len(fld.text)
}

func (f *form) value(key string) string { return string(f.field(key).text) }

func (f *form) isDeck() bool { return f.kind == kindNewDeck || f.kind == kindEditDeck }

// hasOptions is whether the form has a column of options between its boxes and its
// actions: a card has, a deck and a colour have not.
func (f *form) hasOptions() bool { return f.kind == kindNewCard || f.kind == kindEditCard }

// A form has up to three columns of views, in the order they are stored: the
// boxes of text on the left, the card's options in the middle (a deck has none),
// and the actions on the right, so that however many options a card has the
// actions are no further away.
func (f *form) column(i int) int {
	switch f.fields[i].kind {
	case textField:
		return 0
	case buttonField:
		return 2
	}
	return 1
}

// bounds are the views in a column: [from, to), empty when there are none.
func (f *form) bounds(col int) (from, to int) {
	from = -1
	for i := range f.fields {
		if f.column(i) == col {
			if from < 0 {
				from = i
			}
			to = i + 1
		}
	}
	if from < 0 {
		return 0, 0
	}
	return from, to
}

// up and down move within a column; nothing wraps.
func (f *form) up()   { f.step(-1) }
func (f *form) down() { f.step(1) }

func (f *form) step(by int) {
	from, to := f.bounds(f.column(f.at))
	f.at = max(min(f.at+by, to-1), from)
}

// across moves to the next column to the left (-1) or right (1) that has any
// views, to the one last on there. At the edge it stays.
func (f *form) across(by int) {
	here := f.column(f.at)
	for col := here + by; col >= 0 && col <= 2; col += by {
		from, to := f.bounds(col)
		if from == to {
			continue
		}
		f.memo[here] = f.at
		back := f.memo[col]
		if back < from || back >= to {
			back = from
		}
		f.at = back
		return
	}
}

// next and previous are tab and shift+tab: every view in turn.
func (f *form) next()     { f.at = min(f.at+1, len(f.fields)-1) }
func (f *form) previous() { f.at = max(f.at-1, 0) }

// boxRows is how many lines tall each text box is in a form d lines tall. It is
// worked out from d, which comes from the window, and never from what is typed,
// so a box does not change size as it fills. What is left after the labels and a
// few lines for the hint under the form is shared out, the answer taking the most.
func (f *form) boxRows(d int) map[int]int {
	var texts []int
	for i, fld := range f.fields {
		if fld.kind == textField {
			texts = append(texts, i)
		}
	}
	budget := max(d-len(texts)-3, len(texts))
	rows := map[int]int{}
	if f.kind == kindNewColour { // a colour and a name: one line each
		rows[texts[0]], rows[texts[1]] = 1, 1
		return rows
	}
	if f.isDeck() { // the name, then what it is about
		name := 1
		if budget >= 6 {
			name = 2
		}
		rows[texts[0]], rows[texts[1]] = name, max(budget-name, 1)
		return rows
	}
	question, reference := 1, 1 // the question, answer, notes, then the reference
	if budget >= 8 {
		question = 2
	}
	rest := max(budget-question-reference, 2)
	answer := max((rest*3+4)/5, 1)
	rows[texts[0]], rows[texts[1]] = question, answer
	rows[texts[2]], rows[texts[3]] = max(rest-answer, 1), reference
	return rows
}

// timeValue is the card's own time: nil for auto, else seconds.
func (f *form) timeValue() *int {
	if v := f.field(keyTime).wheel.value(); v != autoLimit {
		return &v
	}
	return nil
}

// invalid is what is missing before the form can be saved, and the view it is on.
func (f *form) invalid() (string, int) {
	key, what := keyTitle, "A card needs a question."
	if f.isDeck() {
		key, what = keyName, "A deck needs a name."
	}
	if strings.TrimSpace(f.value(key)) == "" {
		return what, slices.IndexFunc(f.fields, func(x formField) bool { return x.key == key })
	}
	return "", 0
}

// formKey is every key on a form, and on the picker when it is open.
func (m model) formKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	if f.picker != nil {
		return m.pickerKey(msg.String())
	}
	switch msg.String() {
	case "ctrl+s":
		return m.saveForm(false)
	case "ctrl+o":
		if f.kind == kindNewColour { // there is no file to open
			return m, nil
		}
		return m.saveForm(true)
	}
	switch f.mode {
	case typing:
		return m.typingKey(msg)
	case selecting:
		return m.selectingKey(msg.String())
	}
	return m.navigatingKey(msg.String())
}

func (m model) navigatingKey(k string) (tea.Model, tea.Cmd) {
	f := m.form
	switch k {
	case "esc":
		return m.leaveForm(), nil
	case "up", "k":
		f.up()
	case "down", "j":
		f.down()
	case "left", "h":
		f.across(-1)
	case "right", "l":
		f.across(1)
	case "tab":
		f.next()
	case "shift+tab":
		f.previous()
	case "enter":
		return m.enterView()
	}
	return m, nil
}

// enterView is enter on the view the cursor is on.
func (m model) enterView() (tea.Model, tea.Cmd) {
	f := m.form
	switch fld := &f.fields[f.at]; fld.kind {
	case textField:
		f.mode = typing
	case wheelField:
		f.mode = selecting
	case linkField:
		f.picker = m.newPicker(fld.links)
	case buttonField:
		switch fld.key {
		case keyCancel:
			return m.leaveForm(), nil
		case keyOpen:
			return m.saveForm(true)
		}
		return m.saveForm(false)
	}
	return m, nil
}

// typingKey: the keys are the text's, and esc, or tab, comes back out.
func (m model) typingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.form
	row := &f.fields[f.at]
	spans := layoutText(row.text, m.boxWidth())
	switch msg.Type {
	case tea.KeyEsc:
		f.mode = navigating
	case tea.KeyTab:
		f.mode = navigating
		f.next()
	case tea.KeyShiftTab:
		f.mode = navigating
		f.previous()
	case tea.KeyEnter:
		row.insert('\n') // in every box: esc, or tab, is what ends the typing
	case tea.KeyRunes:
		row.insert(msg.Runes...)
	case tea.KeySpace:
		row.insert(' ')
	case tea.KeyBackspace: // a character, which may be several runes of an emoji
		if row.pos > 0 {
			from := prevBoundary(row.text, row.pos)
			row.text = slices.Delete(row.text, from, row.pos)
			row.pos = from
		}
	case tea.KeyDelete:
		if row.pos < len(row.text) {
			row.text = slices.Delete(row.text, row.pos, nextBoundary(row.text, row.pos))
		}
	case tea.KeyLeft:
		row.pos = prevBoundary(row.text, row.pos)
	case tea.KeyRight:
		row.pos = nextBoundary(row.text, row.pos)
	case tea.KeyUp:
		row.pos = moveRow(row.text, spans, row.pos, -1)
	case tea.KeyDown:
		row.pos = moveRow(row.text, spans, row.pos, 1)
	case tea.KeyHome, tea.KeyCtrlA:
		row.pos = spans[rowOf(spans, row.pos)].start
	case tea.KeyEnd, tea.KeyCtrlE:
		row.pos = lineEnd(row.text, spans, rowOf(spans, row.pos))
	case tea.KeyCtrlU:
		row.text, row.pos = nil, 0
	}
	return m, nil
}

// boxWidth is the cells a text box has for its text: the left column of the form,
// less the edge and the space beside it on each side.
func (m model) boxWidth() int {
	left, _, _ := formColumns(m.formWidth(), m.form.hasOptions())
	return left - 4
}

// formWidth is the cells a form has: the whole window in the library, and the narrow
// card of the settings.
func (m model) formWidth() int {
	if m.screen == settingsScreen {
		return m.inner(true)
	}
	return m.frameWidth() - 4
}

// selectingKey: left and right change the option, and enter or esc lets go of it.
func (m model) selectingKey(k string) (tea.Model, tea.Cmd) {
	f := m.form
	row := &f.fields[f.at]
	switch k {
	case "left", "h", "-":
		row.wheel.turn(-1)
	case "right", "l", "+":
		row.wheel.turn(1)
	case " ":
		if len(row.wheel.values) == 2 {
			row.wheel.at = 1 - row.wheel.at
		} else {
			row.wheel.turn(1)
		}
	case "enter", "esc":
		f.mode = navigating
	case "tab":
		f.mode = navigating
		f.next()
	case "shift+tab":
		f.mode = navigating
		f.previous()
	}
	return m, nil
}

// insert types runes at the cursor. A box for what is one line in the file, such
// as the question, can hold a newline as any other can: the engine makes it one
// line when it is saved.
func (f *formField) insert(r ...rune) {
	f.text = slices.Insert(f.text, f.pos, r...)
	f.pos += len(r)
}

// openForm puts a form on the screen, with nothing left over from what came before.
func (m model) openForm(f *form) model {
	m.form, m.status, m.err = f, "", ""
	return m
}

// leaveForm drops the form, unsaved, and goes back to the library.
func (m model) leaveForm() model {
	m.form, m.status, m.err = nil, "", ""
	return m
}

// saveForm makes the call that writes the form. It stays where it is, with its
// values, until the engine has taken it: a refusal (a card that is already
// there) is shown under the form and nothing is lost.
func (m model) saveForm(openAfter bool) (tea.Model, tea.Cmd) {
	f := m.form
	if f.kind == kindNewColour {
		return m.saveColour()
	}
	if what, at := f.invalid(); what != "" {
		f.at, f.mode, m.status = at, navigating, what
		return m, nil
	}
	f.mode = navigating
	m.busy, m.status = true, ""
	switch f.kind {
	case kindNewDeck:
		m.saving = &saving{verb: "Made the deck", openAfter: openAfter, deck: true}
		p := engine.NewDeckParams{Name: f.value(keyName), Description: f.value(keyAbout)}
		return m, call(func() (*engine.NewDeck, error) { return m.eng.CreateDeck(p) })
	case kindEditDeck:
		m.saving = &saving{verb: "Saved the deck", openAfter: openAfter, deck: true}
		p := engine.UpdateDeckParams{Deck: f.deck, Name: f.value(keyName), Description: f.value(keyAbout)}
		return m, call(func() (*engine.NewDeck, error) { return m.eng.UpdateDeck(p) })
	case kindNewCard:
		m.saving = &saving{verb: "Made the card", openAfter: openAfter}
		p := engine.NewCardParams{
			Deck: f.deck, Title: f.value(keyTitle), Answer: f.value(keyAnswer),
			Notes: f.value(keyNotes), Reference: f.value(keyReference),
			Time: f.timeValue(), Follows: slices.Clone(f.field(keyFollows).links),
			Draft: f.field(keyDraft).wheel.value() == 1,
		}
		return m, call(func() (*engine.NewCard, error) { return m.eng.CreateCard(p) })
	}
	m.saving = &saving{verb: "Saved the card", openAfter: openAfter}
	p := engine.UpdateCardParams{
		Deck: f.deck, Card: f.card, Title: f.value(keyTitle), Answer: f.value(keyAnswer),
		Notes: f.value(keyNotes), Reference: f.value(keyReference),
		Time: f.timeValue(), Follows: slices.Clone(f.field(keyFollows).links),
		Draft: f.field(keyDraft).wheel.value() == 1,
	}
	return m, call(func() (*engine.NewCard, error) { return m.eng.UpdateCard(p) })
}

// saveColour adds the colour to the custom list, as the settings keep it, and uses it.
// There is no engine call to wait for: a refusal (no name, a name that is taken, a
// colour that cannot be read) is said, and the form stays as it is.
func (m model) saveColour() (tea.Model, tea.Cmd) {
	f := m.form
	f.mode = navigating
	if err := m.prefs.addCustom(f.value(keyName), f.value(keyValue)); err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.form, m.err = nil, ""
	m.status = "Added " + m.prefs.custom[m.prefs.customAt].Name + "."
	applyTheme(m.prefs.highlight())
	return m, m.saveSettings()
}

// newPicker lists the deck's cards with the ones already chosen ticked, and the
// cursor on the first that can be.
func (m model) newPicker(chosen []string) *picker {
	p := &picker{chosen: map[string]bool{}, self: -1}
	for _, id := range chosen {
		p.chosen[id] = true
	}
	if m.library != nil {
		p.cards = m.library.Cards
		p.self = slices.IndexFunc(p.cards, func(c engine.CardSummary) bool { return c.ID == m.form.card })
	}
	p.at = p.step(-1, 1)
	return p
}

// step is the card a move by by from at lands on, passing over the one being
// edited, which cannot be ticked. At the ends it stays where it was.
func (p *picker) step(at, by int) int {
	for i := at + by; i >= 0 && i < len(p.cards); i += by {
		if i != p.self {
			return i
		}
	}
	if at >= 0 && at < len(p.cards) && at != p.self {
		return at
	}
	return 0
}

// pickerKey: space ticks the card under the cursor, enter takes the ticks back
// to the form, and esc leaves them as they were. The card list is the picker's,
// so the card shown does not change as it moves.
func (m model) pickerKey(k string) (tea.Model, tea.Cmd) {
	f := m.form
	p := f.picker
	switch k {
	case "esc", "q":
		f.picker = nil
	case "enter":
		row := f.field(keyFollows)
		row.links = row.links[:0]
		for _, c := range p.cards { // in the deck's order, not the order ticked
			if p.chosen[c.ID] {
				row.links = append(row.links, c.ID)
			}
		}
		f.picker = nil
	case "up", "k":
		p.at = p.step(p.at, -1)
	case "down", "j":
		p.at = p.step(p.at, 1)
	case "home", "g":
		p.at = p.step(-1, 1)
	case "end", "G":
		p.at = p.step(len(p.cards), -1)
	case " ", "x":
		if p.at < len(p.cards) && p.at != p.self {
			id := p.cards[p.at].ID
			p.chosen[id] = !p.chosen[id]
		}
	}
	return m, nil
}
