package main

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lernt/tui/engine"
)

// The screens are driven the way a person drives them - keys in, view out -
// against a fake engine that records what it was asked. No terminal, no Python.

type fake struct {
	calls    []string
	decks    engine.DeckList
	started  engine.Started
	startErr error
	scored   engine.Scored
	menu     engine.Menu
	next     engine.Question
	summary  engine.Summary
	reloaded engine.Reloaded
	standing engine.Standing
	misses   engine.MissReport
	boxes    engine.BoxReport
	edits    engine.EditReport

	start engine.StartParams
	score engine.ScoreParams
	picks []engine.NextParams
	note  string
	miss  engine.MissParams

	saved map[string]map[string]string // deck -> key -> value, as the engine keeps it
	saves []engine.SaveOptionsParams

	settings map[string]string // key -> value, as the engine keeps them

	newDeck  engine.NewDeckParams
	newCard  engine.NewCardParams
	updCard  engine.UpdateCardParams
	refuse   error // the next CreateCard is refused with this
	updDeck  engine.UpdateDeckParams
	problems []string
	opened   []string // the paths handed to the editor
	cards    map[string][]engine.CardSummary
	deleted  []string
}

func (f *fake) record(call string) { f.calls = append(f.calls, call) }

func (f *fake) Decks() (*engine.DeckList, error) { f.record("decks"); return &f.decks, nil }
func (f *fake) Start(p engine.StartParams) (*engine.Started, error) {
	f.record("start")
	f.start = p
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.started.Auto, f.started.Edit, f.started.Weak = p.Auto, p.Edit, p.Weak
	return &f.started, nil
}
func (f *fake) Score(p engine.ScoreParams) (*engine.Scored, error) {
	f.record("score")
	f.score = p
	return &f.scored, nil
}
func (f *fake) Menu() (*engine.Menu, error) { f.record("menu"); return &f.menu, nil }
func (f *fake) Next(p engine.NextParams) (*engine.Question, error) {
	f.record("next")
	f.picks = append(f.picks, p)
	return &f.next, nil
}
func (f *fake) End() (*engine.Summary, error) { f.record("end"); return &f.summary, nil }
func (f *fake) EditCard() (*engine.CardFile, error) {
	f.record("edit")
	return &engine.CardFile{Path: "/cards/France.md"}, nil
}
func (f *fake) Reload() (*engine.Reloaded, error) { f.record("reload"); return &f.reloaded, nil }
func (f *fake) Done() (*engine.Standing, error)   { f.record("done"); return &engine.Standing{}, nil }
func (f *fake) Flag(engine.FlagParams) (*engine.Standing, error) {
	f.record("flag")
	return &f.standing, nil
}
func (f *fake) Note(p engine.NoteParams) (*engine.Standing, error) {
	f.record("note")
	f.note = p.Text
	return &f.standing, nil
}
func (f *fake) Misses(p engine.MissParams) (*engine.MissReport, error) {
	f.record("misses")
	f.miss = p
	return &f.misses, nil
}
func (f *fake) Boxes(engine.DeckParams) (*engine.BoxReport, error) {
	f.record("boxes")
	return &f.boxes, nil
}
func (f *fake) Edits(engine.DeckParams) (*engine.EditReport, error) {
	f.record("edits")
	return &f.edits, nil
}

func (f *fake) CreateDeck(p engine.NewDeckParams) (*engine.NewDeck, error) {
	f.record("create deck")
	f.newDeck = p
	id := strings.ToLower(strings.ReplaceAll(p.Name, " ", "-"))
	f.decks.Decks = append(f.decks.Decks, engine.DeckInfo{ID: id, Name: p.Name})
	return &engine.NewDeck{ID: id, Path: "/decks/" + id}, nil
}

func (f *fake) CreateCard(p engine.NewCardParams) (*engine.NewCard, error) {
	f.record("create card")
	if f.refuse != nil {
		return nil, f.refuse
	}
	f.newCard = p
	return &engine.NewCard{ID: "Q", Path: "/decks/" + p.Deck + "/Q.md"}, nil
}

func (f *fake) UpdateCard(p engine.UpdateCardParams) (*engine.NewCard, error) {
	f.record("update card")
	f.updCard = p
	return &engine.NewCard{ID: p.Card, Path: "/decks/" + p.Deck + "/" + p.Card + ".md"}, nil
}

func (f *fake) UpdateDeck(p engine.UpdateDeckParams) (*engine.NewDeck, error) {
	f.record("update deck")
	f.updDeck = p
	return &engine.NewDeck{ID: p.Deck, Path: "/decks/" + p.Deck}, nil
}

func (f *fake) CheckDeck(p engine.DeckParams) (*engine.Problems, error) {
	f.record("check")
	return &engine.Problems{Deck: p.Deck, Problems: f.problems}, nil
}

func (f *fake) ListCards(p engine.DeckParams) (*engine.CardList, error) {
	f.record("cards")
	return &engine.CardList{Deck: p.Deck, Cards: f.cards[p.Deck]}, nil
}

func (f *fake) DeleteCard(p engine.DeleteCardParams) (*engine.Trashed, error) {
	f.record("delete card")
	f.deleted = append(f.deleted, p.Deck+"/"+p.Card)
	var kept []engine.CardSummary
	for _, c := range f.cards[p.Deck] {
		if c.ID != p.Card {
			kept = append(kept, c)
		}
	}
	f.cards[p.Deck] = kept
	return &engine.Trashed{ID: p.Card, Trash: "/trash/" + p.Card + ".md"}, nil
}

func (f *fake) DeleteDeck(p engine.DeleteDeckParams) (*engine.Trashed, error) {
	f.record("delete deck")
	f.deleted = append(f.deleted, p.Deck)
	var kept []engine.DeckInfo
	for _, d := range f.decks.Decks {
		if d.ID != p.Deck {
			kept = append(kept, d)
		}
	}
	f.decks.Decks = kept
	return &engine.Trashed{ID: p.Deck, Trash: "/trash/" + p.Deck}, nil
}

func (f *fake) LoadSettings() (*engine.Settings, error) {
	f.record("settings")
	out := &engine.Settings{}
	for _, key := range slices.Sorted(maps.Keys(f.settings)) {
		out.Saved = append(out.Saved, engine.Option{Key: key, Value: f.settings[key]})
	}
	return out, nil
}

func (f *fake) SaveSettings(p engine.SaveSettingsParams) (*engine.Settings, error) {
	f.record("save settings")
	if f.settings == nil {
		f.settings = map[string]string{}
	}
	for _, o := range p.Settings {
		f.settings[o.Key] = o.Value
	}
	return f.LoadSettings()
}

func intp(n int) *int { return &n }

var (
	france = engine.Question{
		ID: "France", Title: "What is the capital of **France**?", Number: 1,
		Answer: []string{"Paris."}, Notes: []string{"", "On the Seine."}, Limit: 10,
	}
	germany = engine.Question{ID: "Germany", Title: "What is the capital of Germany?", Number: 2,
		Answer: []string{"Berlin."}, Limit: 10, FollowsFrom: "What is the capital of France?"}
)

func newFake() *fake {
	return &fake{
		decks: engine.DeckList{Home: "/decks", Decks: []engine.DeckInfo{
			{ID: "capitals", Name: "European capitals", Count: intp(2), Limit: intp(10), Boxes: []int{1, 2, 4},
				Stats: &engine.DeckStats{Size: 2, New: 2, Boxes: []int{0, 0, 0}, Due: 2}},
			{ID: "stale", Error: "No such deck"},
		}},
		started: engine.Started{Deck: "capitals", Name: "European capitals", Pool: 3,
			Count: 2, DefaultLimit: 10, Card: france},
		menu:    engine.Menu{Followups: []engine.MenuItem{{ID: "Germany", Title: "What is the capital of Germany?"}}},
		next:    germany,
		summary: engine.Summary{Passed: 1, Failed: 1, Asked: 2, Review: []engine.Review{{Score: "fail", Title: "What is the capital of **France**?"}}},
	}
}

// clock is a fake time.Now the tests move by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func start(t *testing.T, f *fake) (model, *clock) {
	t.Helper()
	c := &clock{time.Unix(1000, 0)}
	m := newModel(f)
	m.tmux = false            // whatever the tests are run in
	applyTheme(defaultColour) // the colour is global, and a test may have changed it
	m.now = c.now
	m.tick = func(int) tea.Cmd { return nil }
	m.exec = func(path string) tea.Cmd {
		f.opened = append(f.opened, path)
		return func() tea.Msg { return editorDone{} }
	}
	return drive(m, m.Init()), c
}

// drive runs cmd and every command that follows from it, as the program would.
func drive(m model, cmd tea.Cmd) model {
	for cmd != nil {
		msg := cmd()
		if _, quit := msg.(tea.QuitMsg); quit || msg == nil {
			return m
		}
		next, more := m.Update(msg)
		m, cmd = next.(model), more
	}
	return m
}

func press(m model, keys ...string) model {
	for _, k := range keys {
		next, cmd := m.Update(key(k))
		m = drive(next.(model), cmd)
	}
	return m
}

func key(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+o":
		return tea.KeyMsg{Type: tea.KeyCtrlO}
	case "ctrl+j":
		return tea.KeyMsg{Type: tea.KeyCtrlJ}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

func typeText(m model, text string) model {
	for _, r := range text {
		switch {
		case r == ' ':
			m = press(m, "space")
		case r == '\n':
			m = press(m, "enter")
		default:
			m = press(m, string(r))
		}
	}
	return m
}

func sees(t *testing.T, m model, want ...string) {
	t.Helper()
	view := m.View()
	for _, w := range want {
		if !strings.Contains(view, w) {
			t.Fatalf("view lacks %q:\n%s", w, view)
		}
	}
}

func lacks(t *testing.T, m model, unwanted string) {
	t.Helper()
	if view := m.View(); strings.Contains(view, unwanted) {
		t.Fatalf("view has %q:\n%s", unwanted, view)
	}
}

func calls(t *testing.T, f *fake, want ...string) {
	t.Helper()
	if strings.Join(f.calls, " ") != strings.Join(want, " ") {
		t.Fatalf("calls %v, want %v", f.calls, want)
	}
}

func TestTheDecksAreListedAndAnUnusableOneDoesNotOpen(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	sees(t, m, "▌ European capitals", "stale", "2 cards", "never run")
	m = press(m, "down")
	sees(t, m, "This deck will not load:", "No such deck")
	m = press(m, "enter")
	if m.screen != decksScreen {
		t.Fatal("an unusable deck opened")
	}
}

func TestAWholeRunWithTheMenu(t *testing.T) {
	f := newFake()
	m, c := start(t, f)

	m = press(m, "enter")
	sees(t, m, "European capitals", "Cards", "Seconds", "auto")
	m = press(m, "enter")
	if *f.start.Count != 2 || f.start.Limit != nil || !f.start.Log || f.start.Deck != "capitals" {
		t.Fatalf("started with %+v", f.start)
	}
	sees(t, m, "European capitals", "1/2", "What is the capital of France?")
	lacks(t, m, "Paris.") // not before the answer

	c.t = c.t.Add(14 * time.Second)
	sees(t, m, "14s - over the 10s limit")
	m = press(m, "enter")
	sees(t, m, "Paris.", "On the Seine.", "14s - over the 10s limit", "p pass")

	m = press(m, "f")
	if f.score != (engine.ScoreParams{Score: "fail", Seconds: 14}) {
		t.Fatalf("scored %+v", f.score)
	}
	sees(t, m, "Where next?", "▌ 1  What is the capital of Germany?", "r  a random card")

	m = press(m, "1")
	if f.picks[0] != (engine.NextParams{Pick: engine.PickFollowup, Card: "Germany"}) {
		t.Fatalf("picked %+v", f.picks)
	}
	sees(t, m, "2/2", "follows from: What is the capital of France?")

	f.scored = engine.Scored{Finished: true}
	m = press(m, "enter", "p")
	sees(t, m, "pass 1", "fail 1", "(of 2)", "fail  What is the capital of France?")
	calls(t, f, "settings", "decks", "options", "save", "start", "score", "menu", "next", "score", "end")

	m = press(m, "enter")
	if m.screen != decksScreen || f.calls[len(f.calls)-1] != "decks" {
		t.Fatal("enter on the summary did not go back to the decks")
	}
}

func TestAnAutoRunSkipsTheMenu(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "enter", "down", "down", "space", "enter") // Follow branches for me
	if !f.start.Auto {
		t.Fatalf("started with %+v", f.start)
	}
	m = press(m, "enter", "s")
	calls(t, f, "settings", "decks", "options", "save", "start", "score", "next")
	if f.picks[0].Pick != engine.PickAuto {
		t.Fatalf("picked %+v", f.picks)
	}
	sees(t, m, "What is the capital of Germany?")
}

func TestTheOptionsChangeWhatTheRunAsksFor(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "enter", "left", "left", "left")
	sees(t, m, "  ∞    › 1    2", "Runs until you end it.")
	m = press(m, "down", "right", "right")
	m = press(m, "/")
	m = typeText(m, "Fran")
	m = press(m, "enter", "enter")
	if *f.start.Count != 0 || f.start.Limit == nil || *f.start.Limit != 5 || f.start.Filter != "Fran" {
		t.Fatalf("started with %+v", f.start)
	}
}

func TestARunThatCannotStartSaysWhyOnTheOptions(t *testing.T) {
	f := newFake()
	f.startErr = &engine.Error{Code: engine.NoMisses, Message: "Nothing is on the miss list."}
	m, _ := start(t, f)
	m = press(m, "enter", "enter")
	if m.screen != optionsScreen {
		t.Fatalf("screen %d", m.screen)
	}
	sees(t, m, "Nothing is on the miss list.")
}

func TestQEndsTheRunWhileAnswering(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "enter", "enter", "q")
	calls(t, f, "settings", "decks", "options", "save", "start", "end")
	if m.screen != summaryScreen {
		t.Fatalf("screen %d", m.screen)
	}
}

func TestEditModeNotesEditsAndGoesOn(t *testing.T) {
	f := newFake()
	f.standing = engine.Standing{Standing: []engine.EditEntry{{Kind: "note", At: "2026-09-15", Text: "clarify X"}}}
	f.reloaded = engine.Reloaded{Changed: true, Card: engine.Question{ID: "France", Title: "EDITED capital?", Number: 1}}
	m, _ := start(t, f)

	m = press(m, "enter", "down", "down", "down", "down", "down", "space", "enter") // Edit mode
	if !f.start.Edit {
		t.Fatalf("started with %+v", f.start)
	}
	m = press(m, "enter", "s")
	sees(t, m, "Nothing stands against this card.", "e edit  n note")

	m = press(m, "n")
	m = typeText(m, "clarify X")
	sees(t, m, "Note: clarify X")
	m = press(m, "enter")
	if f.note != "clarify X" {
		t.Fatalf("noted %q", f.note)
	}
	sees(t, m, "Noted.", "Standing against this card:", "clarify X", "d done")

	m = press(m, "e")
	sees(t, m, "EDITED capital?", "Edited - anything standing against it is cleared.")
	lacks(t, m, "Standing against this card:")

	m = press(m, "enter")
	calls(t, f, "settings", "decks", "options", "save", "start", "score", "note", "edit", "reload", "menu")
}

func TestAFailedEditorTakesNoEdit(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m.exec = func(string) tea.Cmd {
		return func() tea.Msg { return editorDone{err: &engine.Error{Message: "exit status 1"}} }
	}
	m = press(m, "enter", "/", "up") // the filter field, then edit mode above it
	m = press(m, "space", "enter", "enter", "p", "e")
	sees(t, m, "The editor failed (exit status 1); taking no edit.")
	calls(t, f, "settings", "decks", "options", "save", "start", "score", "edit")
}

func TestTheMissReportTogglesToEveryCard(t *testing.T) {
	f := newFake()
	f.misses = engine.MissReport{Name: "European capitals", Attempts: 3, Records: []engine.MissRecord{
		{Card: "France", Title: "What is the capital of France?", Score: "fail", Misses: 2, Attempts: 3, Seen: "2026-09-15"},
	}, Retired: []string{"Old card"}}
	m, _ := start(t, f)
	m = press(m, "m")
	sees(t, m, "European capitals", "Miss list", "fail", "2/3", "What is the capital of France?", "Old card")
	m = press(m, "a")
	if !f.miss.All || f.miss.Deck != "capitals" {
		t.Fatalf("asked for %+v", f.miss)
	}
	sees(t, m, "Every card attempted", "a the miss list")
	m = press(m, "esc")
	if m.screen != decksScreen {
		t.Fatalf("screen %d", m.screen)
	}
}

func TestTheEditReport(t *testing.T) {
	f := newFake()
	f.edits = engine.EditReport{Name: "European capitals", Log: "/state/capitals.edits.jsonl", Items: []engine.EditItem{
		{Card: "France", Title: "Capital of France?", Path: "/cards/France.md",
			Entries: []engine.EditEntry{{Kind: "remove", At: "2026-09-15", Text: "duplicate"}}},
	}}
	m, _ := start(t, f)
	m = press(m, "e")
	sees(t, m, "Edit list", "Capital of France?", "/cards/France.md", "remove", "duplicate")
}

func TestLongContentScrollsWithinTheWindow(t *testing.T) {
	f := newFake()
	for i := 0; i < 40; i++ {
		f.started.Card.Answer = append(f.started.Card.Answer, "line of the answer")
	}
	m, _ := start(t, f)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = press(next.(model), "enter", "enter", "enter")
	if lines := strings.Count(m.View(), "\n"); lines > 20 {
		t.Fatalf("%d lines in a 20-line window", lines)
	}
	sees(t, m, "j/k to scroll")
	m = press(m, "G")
	sees(t, m, "p pass")
	if m.offset == 0 || m.offset != m.maxOffset() {
		t.Fatalf("offset %d, max %d", m.offset, m.maxOffset())
	}
	m = press(m, "j")
	if m.offset != m.maxOffset() {
		t.Fatal("scrolled past the end")
	}
}

func TestCleanAndWrap(t *testing.T) {
	if got := clean("**Bold** `code` [[Note#h|alias]] [[Plain]] &amp; char *p"); got != "Bold code alias Plain & char *p" {
		t.Fatalf("clean: %q", got)
	}
	got := wrap([]string{"- one two three four", "", "plain"}, 20)
	want := []string{"- one two three four", "", "plain"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrap: %q", got)
	}
	got = wrap([]string{"- alpha beta gamma delta epsilon"}, 20)
	if len(got) != 2 || got[1] != "  delta epsilon" {
		t.Fatalf("hanging indent: %q", got)
	}
}

func TestTheLimitWheelGoesAutoOffThenSeconds(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "enter", "down")
	sees(t, m, "  auto › off  5s", "Each card's own time; 10s for a")
	m = press(m, "right")
	sees(t, m, "auto ‹ off  › 5s   10s", "No card is timed.")
	m = press(m, "right", "right")
	sees(t, m, "off   5s ‹ 10s  › 15s  20s", "Every card gets this")
	m = press(m, "left", "enter")
	if f.start.Limit == nil || *f.start.Limit != 5 {
		t.Fatalf("started with %+v", f.start)
	}
}

func TestTheWheelsShowTheirNeighboursWhenThereIsRoom(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "enter")
	sees(t, m, "∞    1 ‹ 2    › 3    4")
	for width, want := range map[int]string{100: "∞    1 ‹ 2    › 3    4", 60: "Cards   ‹ 2    ›", 40: "1 ‹ 2    › 3"} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		sees(t, next.(model), want)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	lacks(t, next.(model), "› 3")
}

func TestTheCountWheelTakesInTheDecksCount(t *testing.T) {
	w := countWheel(13)
	if w.label() != "13" || w.values[w.at-1] != 12 || w.values[w.at+1] != 15 {
		t.Fatalf("wheel %+v", w)
	}
	w.turn(-100)
	if w.label() != "∞" {
		t.Fatalf("turned past the start: %q", w.label())
	}
}

func TestACardsOwnTimeIsShownAndAnUntimedCardHasOnlyAClock(t *testing.T) {
	f := newFake()
	m, c := start(t, f)
	m = press(m, "enter", "enter")
	c.t = c.t.Add(3 * time.Second)
	sees(t, m, "0:03/0:10")

	f.started.Card.Limit = 0
	m, c = start(t, f)
	m = press(m, "enter", "enter")
	c.t = c.t.Add(99 * time.Second)
	sees(t, m, "1:39")
	lacks(t, m, "limit")
	lacks(t, m, "━") // no time to drain
}

func TestTheFilterRowTakesWhatIsTypedOnIt(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "enter", "/")
	m = typeText(m, "jq Gex")
	m = press(m, "backspace", "r")
	sees(t, m, "Filter  [jq Ger█")
	m = press(m, "up")
	sees(t, m, "Filter  [jq Ger_")
	lacks(t, m, "█")
	m = press(m, "enter")
	if f.start.Filter != "jq Ger" {
		t.Fatalf("started with filter %q", f.start.Filter)
	}
}

func TestTheWheelsLineUp(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "enter")
	lines := strings.Split(m.View(), "\n")
	column, rows := -1, 0
	for _, label := range []string{"Cards ", "Seconds ", "Follow ", "Drafts ", "Misses ", "Edit "} {
		for _, line := range lines {
			if !strings.Contains(line, label) || !strings.Contains(line, "›") {
				continue
			}
			rows++
			at := strings.Index(line, "›")
			col := len([]rune(line[:at]))
			if column == -1 {
				column = col
			} else if col != column {
				t.Fatalf("%q has its arrow at column %d, not %d:\n%s", label, col, column, m.View())
			}
		}
	}
	if rows != 6 {
		t.Fatalf("found %d wheel rows, want 6:\n%s", rows, m.View())
	}
}

func (f *fake) options(deck string) *engine.Options {
	out := &engine.Options{Deck: deck}
	for _, key := range slices.Sorted(maps.Keys(f.saved[deck])) {
		out.Saved = append(out.Saved, engine.Option{Key: key, Value: f.saved[deck][key]})
	}
	return out
}

func (f *fake) LoadOptions(p engine.DeckParams) (*engine.Options, error) {
	f.record("options")
	return f.options(p.Deck), nil
}

func (f *fake) SaveOptions(p engine.SaveOptionsParams) (*engine.Options, error) {
	f.record("save")
	f.saves = append(f.saves, p)
	if f.saved == nil {
		f.saved = map[string]map[string]string{}
	}
	if f.saved[p.Deck] == nil {
		f.saved[p.Deck] = map[string]string{}
	}
	for _, o := range p.Options {
		f.saved[p.Deck][o.Key] = o.Value
	}
	return f.options(p.Deck), nil
}

func TestTheOptionsScreenRemembersEachDeck(t *testing.T) {
	f := newFake()
	f.decks.Decks = append(f.decks.Decks, engine.DeckInfo{ID: "cv", Name: "CV defence"})
	m, _ := start(t, f)

	// Capitals: 5 cards, 45s, edit mode, filtered - then leave with esc.
	m = press(m, "enter", "right", "right", "right", "down")
	for range 9 {
		m = press(m, "right") // auto, off, 5s ... 45s
	}
	m = press(m, "down", "down", "down", "down", "space", "down")
	m = typeText(m, "Fr")
	m = press(m, "esc")
	want := map[string]string{"count": "5", "limit": "45", "auto": "off",
		"drafts": "off", "weak": "off", "edit": "on", "filter": "Fr"}
	if !maps.Equal(f.saved["capitals"], want) {
		t.Fatalf("saved %v, want %v", f.saved["capitals"], want)
	}

	// Another deck with nothing saved starts from its defaults.
	m = press(m, "down", "down", "enter", "enter")
	if *f.start.Count != 10 || f.start.Limit != nil || f.start.Edit || f.start.Filter != "" {
		t.Fatalf("cv started with %+v", f.start)
	}
	m = press(m, "q", "enter") // end that run, back to the decks

	// Capitals comes back as it was left.
	m = press(m, "up", "up", "enter", "enter")
	if *f.start.Count != 5 || f.start.Limit == nil || *f.start.Limit != 45 || !f.start.Edit || f.start.Filter != "Fr" {
		t.Fatalf("capitals started with %+v", f.start)
	}
}

func TestSavedOptionsAreReadByKeyAndABadOneKeepsItsDefault(t *testing.T) {
	f := newFake()
	f.saved = map[string]map[string]string{"capitals": {
		"filter": "Ger", "retired": "x", "count": "lots", "limit": "37", "auto": "on",
	}}
	m, _ := start(t, f)
	m = press(m, "enter")
	sees(t, m, "37s")
	m = press(m, "enter")
	if *f.start.Count != 2 || f.start.Limit == nil || *f.start.Limit != 37 || !f.start.Auto || f.start.Filter != "Ger" {
		t.Fatalf("started with %+v", f.start)
	}
	if f.saved["capitals"]["retired"] != "x" {
		t.Fatal("a key this TUI does not know was lost on save")
	}
}

func TestADueRunShowsTheBoxAndWhereTheScoreMovedIt(t *testing.T) {
	f := newFake()
	f.started.Card.Box = 2
	f.scored = engine.Scored{Was: 2, Box: 3}
	m, _ := start(t, f)
	m = press(m, "tab", "enter")
	sees(t, m, "Leitner run")
	if hint := m.optionHint(); !strings.Contains(hint, "1, 2, 4 days") {
		t.Fatalf("the hint lacks the deck's schedule: %q", hint)
	}
	m = press(m, "enter")
	if !f.start.Due || f.start.Weak {
		t.Fatalf("started with %+v", f.start)
	}
	sees(t, m, "1/2", "box 2 · 0:00/0:10")
	m = press(m, "enter", "p")
	sees(t, m, "box 2 → box 3", "Where next?")

	f.scored = engine.Scored{Was: 3, Box: 1, Finished: true}
	f.summary.Promoted, f.summary.Demoted = 1, 1
	m = press(m, "1", "enter", "f")
	sees(t, m, "1 up a box, 1 back to box 1")
}

func TestTheModeIsChosenOnTheDeckListForEveryDeck(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	sees(t, m, "classic │ leitner")
	lacks(t, m, "due")
	if m.mode != classic {
		t.Fatalf("started in %q", m.mode)
	}

	m = press(m, "tab")
	sees(t, m, "Leitner: only the cards due today come up", "2 due", "2 due today")
	if f.settings["mode"] != leitner {
		t.Fatalf("settings %v", f.settings)
	}

	// It is the TUI's, not the deck's: a new start comes back in it.
	m, _ = start(t, f)
	if m.mode != leitner {
		t.Fatalf("came back in %q", m.mode)
	}
	m = press(m, "tab")
	lacks(t, m, "due")
	if f.settings["mode"] != classic {
		t.Fatalf("settings %v", f.settings)
	}
}

func TestAModeTheTUIDoesNotKnowIsPassedOver(t *testing.T) {
	f := newFake()
	f.settings = map[string]string{"mode": "cram"}
	m, _ := start(t, f)
	if m.mode != classic {
		t.Fatalf("started in %q", m.mode)
	}
}

func TestALeitnerRunHasNoMissListRow(t *testing.T) {
	f := newFake()
	f.saved = map[string]map[string]string{"capitals": {"weak": "on"}}
	m, _ := start(t, f)
	m = press(m, "enter")
	sees(t, m, "Classic run", "Misses")
	m = press(m, "esc", "tab", "enter")
	sees(t, m, "Leitner run")
	lacks(t, m, "Misses")
	m = press(m, "down", "down", "down", "down")
	if m.field != editField {
		t.Fatalf("four down from the count is field %d, not edit", m.field)
	}
	m = press(m, "enter")
	if !f.start.Due || f.start.Weak {
		t.Fatalf("started with %+v", f.start)
	}
	if f.saved["capitals"]["weak"] != "on" {
		t.Fatal("the saved miss list was lost in Leitner mode")
	}
}

func TestTheBoxReport(t *testing.T) {
	f := newFake()
	f.boxes = engine.BoxReport{Name: "European capitals", New: 1, Due: 2, Boxes: []engine.BoxRow{
		{Box: 1, Days: 1, Cards: 1, Due: 1}, {Box: 2, Days: 2, Cards: 1},
	}}
	m, _ := start(t, f)
	m = press(m, "b")
	sees(t, m, "European capitals", "Leitner boxes", "box 2      2d      1     0",
		"2 card(s) due today")
	f.boxes = engine.BoxReport{Name: "European capitals", Next: "2026-09-12"}
	m = press(m, "esc", "b")
	sees(t, m, "Nothing due today. The next card is due 2026-09-12.")
	calls(t, f, "settings", "decks", "boxes", "decks", "boxes")
}

func TestTheDeckCardSaysOnlyWhatBelongsToTheMode(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].Stats = &engine.DeckStats{Size: 125, New: 95, Boxes: []int{29, 1, 0, 0, 0},
		Due: 30, Missed: 29, LastRun: "2026-09-15"}
	m, c := start(t, f)
	c.t = time.Date(2026, 9, 21, 20, 0, 0, 0, time.Local)

	// Classic: the miss list drives its runs. Boxes and what is due are Leitner's.
	sees(t, m, "125 cards", "29 on the miss list", "last run 6 days ago")
	lacks(t, m, "due today")
	lacks(t, m, "3-5 ·") // the box chart
	lacks(t, m, "new ")

	// Leitner: what is due, and where the cards sit in the boxes. The miss list is not
	// what a Leitner run is made of.
	m = press(m, "tab")
	sees(t, m, "125 cards", "30 due today", "30 due", "new ", " 95", "1   ", " 29", "3-5 ·", "last run 6 days ago")
	lacks(t, m, "on the miss list")
	lacks(t, m, "miss list clear")

	f.decks.Decks[0].Stats = &engine.DeckStats{Size: 1, New: 0, Boxes: []int{0, 0, 1}, LastRun: "2026-09-21"}
	m = press(m, "r")
	sees(t, m, "1 card", "nothing due today", "2   ·", "3   ", "last run today")
	lacks(t, m, "1-2") // only empty boxes at the top end share a row
	lacks(t, m, "miss list clear")

	// Back in classic the same deck says the miss list, and not the chart.
	m = press(m, "tab")
	sees(t, m, "1 card", "miss list clear", "last run today")
	lacks(t, m, "2   ·")
	lacks(t, m, "nothing due today")
}

func TestTheDeckCardSaysWhatTheDeckIs(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].Desc = "The capital cities of European countries."
	m, _ := start(t, f)
	sees(t, m, "The capital cities of European countries.", "2 cards")
	if !strings.Contains(m.View(), "│ "+strings.Repeat("─", 10)) {
		t.Fatalf("no rule under the description:\n%s", m.View())
	}

	f.decks.Decks[0].Desc = ""
	m = press(m, "r")
	lacks(t, m, "capital cities")
	if strings.Contains(m.View(), "│ "+strings.Repeat("─", 10)) {
		t.Fatalf("a rule with no description above it:\n%s", m.View())
	}
}

func TestTheScreensFitANarrowWindow(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].Desc = "The capital cities of European countries, which runs longer than forty cells."
	m, _ := start(t, f)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	m = next.(model)
	for _, keys := range [][]string{{}, {"enter"}, {"enter"}, {"enter"}, {"p"}} {
		m = press(m, keys...)
		for _, line := range strings.Split(m.View(), "\n") {
			if w := cells(line); w > 40 {
				t.Fatalf("a line %d cells wide in a 40-cell window:\n%s", w, m.View())
			}
		}
	}
}

func TestBackFromAReportTheCursorIsOnTheSameDeck(t *testing.T) {
	f := newFake()
	f.decks.Decks = append(f.decks.Decks, engine.DeckInfo{ID: "cv", Name: "CV defence"})
	m, _ := start(t, f)
	m = press(m, "down", "down", "b", "esc")
	sees(t, m, "▌ CV defence")
	m = press(m, "enter", "enter", "q", "enter") // a run and its summary
	sees(t, m, "▌ CV defence")
}

func TestTheOptionsListTheFirstSourcesAndTheRestAreOneKeyAway(t *testing.T) {
	f := newFake()
	for i := range 12 {
		f.decks.Decks[0].Cards = append(f.decks.Decks[0].Cards, fmt.Sprintf("/srv/bank-%d", i))
	}
	m, _ := start(t, f)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(next.(model), "enter", "down")
	sees(t, m, "cards from:", " - /srv/bank-0", " - /srv/bank-2", "… and 9 more", "c sources")
	lacks(t, m, "/srv/bank-3")

	m = press(m, "c")
	sees(t, m, "Card sources", "12 sources", "- /srv/bank-0")
	m = press(m, "G")
	sees(t, m, "- /srv/bank-11")
	m = press(m, "esc")
	if m.screen != optionsScreen || m.field != limitField {
		t.Fatalf("back on screen %d, field %d", m.screen, m.field)
	}
}

func TestOneSourceIsALineAndAFewAreAListWithNothingHidden(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].Cards = []string{"/srv/a"}
	m, _ := start(t, f)
	m = press(m, "enter")
	sees(t, m, "cards from /srv/a")

	f = newFake()
	f.decks.Decks[0].Cards = []string{"/srv/a", "/srv/b"}
	m, _ = start(t, f)
	m = press(m, "enter")
	sees(t, m, "cards from:", " - /srv/a", " - /srv/b")
	lacks(t, m, "more")
	lacks(t, m, "c sources")
}

func TestTheSettingsScreenChangesTheModeAndGoesBackToTheSameDeck(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "down", "3")
	sees(t, m, "Settings", "Mode", "Cards", "Seconds", "every deck", "enter change")
	if m.screen != settingsScreen {
		t.Fatalf("on screen %d", m.screen)
	}

	// The arrows move between rows, so a row is entered before it is turned.
	m = press(m, "right") // not selected: it changes the tab, not a setting
	if m.mode != classic || len(f.settings) != 0 || m.prefTab != 1 {
		t.Fatalf("right changed a row that was not selected: %q, %v, tab %d", m.mode, f.settings, m.prefTab)
	}
	m = press(m, "left", "enter")
	sees(t, m, "←→ change", "enter done")
	m = press(m, "right")
	if m.mode != leitner || f.settings["mode"] != leitner {
		t.Fatalf("mode %q, settings %v", m.mode, f.settings)
	}
	m = press(m, "right")
	if m.mode != leitner {
		t.Fatal("turning right on the last value changed it")
	}
	m = press(m, "space")
	if m.mode != classic || f.settings["mode"] != classic {
		t.Fatalf("space did not flip it: %q, %v", m.mode, f.settings)
	}

	// esc lets go of the row first, and only then leaves the screen.
	m = press(m, "esc")
	if m.screen != settingsScreen || m.prefSelecting {
		t.Fatalf("esc left the screen with a row selected: screen %d, selecting %v", m.screen, m.prefSelecting)
	}
	m = press(m, "esc")
	if m.screen != decksScreen || m.cursor != 1 {
		t.Fatalf("back on screen %d, cursor %d", m.screen, m.cursor)
	}
}

func TestTheSettingsAreTheDefaultsOfADeckWithNoneOfItsOwn(t *testing.T) {
	f := newFake()
	f.decks.Decks = append(f.decks.Decks, engine.DeckInfo{ID: "plain", Name: "Plain"})
	m, _ := start(t, f)
	m = press(m, "3", "down", "enter", "right", "right", "enter", "down", "enter", "right", "right", "enter")
	if f.settings["count"] != "15" || f.settings["limit"] != "5" {
		t.Fatalf("settings %v", f.settings)
	}
	m = press(m, "esc", "down", "down", "enter")
	sees(t, m, "Plain")
	if got := m.opts.wheels[countField].value(); got != 15 {
		t.Fatalf("the count started at %d, not the setting", got)
	}
	if got := m.opts.wheels[limitField].value(); got != 5 {
		t.Fatalf("the seconds started at %d, not the setting", got)
	}

	// A deck that names its own count keeps it, and one that names a limit runs
	// on it: the setting is only for what a deck leaves out.
	m = press(m, "esc", "up", "up", "enter")
	if got := m.opts.wheels[countField].value(); got != 2 {
		t.Fatalf("the deck's count was %d", got)
	}
	if got := m.opts.wheels[limitField].value(); got != autoLimit {
		t.Fatalf("the deck's limit was overridden: %d", got)
	}
}

func TestTabStillMovesBetweenSettingsAndLetsGoOfTheRow(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "3", "enter", "tab")
	if m.pref != countPref || m.prefSelecting {
		t.Fatalf("tab left row %d, selecting %v", m.pref, m.prefSelecting)
	}
	m = press(m, "shift+tab")
	if m.pref != modeRow {
		t.Fatalf("shift+tab left row %d", m.pref)
	}
}

func TestSavedSettingsComeBack(t *testing.T) {
	f := newFake()
	f.settings = map[string]string{"mode": leitner, "count": "25", "limit": "auto"}
	m, _ := start(t, f)
	if m.mode != leitner || m.prefs.count.value() != 25 || m.prefs.limit.value() != autoLimit {
		t.Fatalf("mode %q, count %d, limit %d", m.mode, m.prefs.count.value(), m.prefs.limit.value())
	}
	f.settings = map[string]string{"count": "many", "limit": "-3"}
	m, _ = start(t, f)
	if m.prefs.count.value() != 10 || m.prefs.limit.value() != autoLimit {
		t.Fatalf("unreadable values were not passed over: %d, %d", m.prefs.count.value(), m.prefs.limit.value())
	}
}

func TestTheSettingsScreenIsThereWithNoDecks(t *testing.T) {
	f := newFake()
	f.decks.Decks = nil
	m, _ := start(t, f)
	m = press(m, "3")
	sees(t, m, "Settings")
	m = press(m, "esc")
	sees(t, m, "No decks in", "Press 2 for the Library")
}

func typed(m model, text string) model {
	for _, r := range text {
		switch r {
		case ' ':
			m = press(m, "space")
		case '\n':
			m = press(m, "enter")
		default:
			m = press(m, string(r))
		}
	}
	return m
}

// write types text into the view the cursor is on: enter to start, then the text,
// then esc to stop.
func write(m model, text string) model {
	return press(typed(press(m, "enter"), text), "esc")
}

func TestTheTabsAreAlongTheTopAndSwitchScreens(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	sees(t, m, "1 Decks", "2 Library", "3 Settings")
	m = press(m, "2")
	if m.screen != libraryScreen {
		t.Fatalf("2 went to screen %d", m.screen)
	}
	m = press(m, "]")
	if m.screen != settingsScreen {
		t.Fatalf("] went to screen %d", m.screen)
	}
	m = press(m, "[", "1")
	if m.screen != decksScreen {
		t.Fatalf("1 went to screen %d", m.screen)
	}
	m = press(m, "[")
	if m.screen != decksScreen {
		t.Fatal("[ on the first tab went somewhere")
	}
	// The tabs are for the tab screens: on a run the digits are not theirs.
	m = press(m, "enter", "enter")
	sees(t, m, "1/2")
	m = press(m, "2")
	if m.screen != optionsScreen && m.screen != askScreen {
		t.Fatalf("2 left the run for screen %d", m.screen)
	}
}

func TestTheHighlightedDeckFollowsYouBetweenTabs(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "down", "2")
	if m.libDeck != 1 {
		t.Fatalf("the library is on deck %d", m.libDeck)
	}
	m = press(m, "3", "1")
	if m.cursor != 1 {
		t.Fatalf("the deck list is on deck %d", m.cursor)
	}
}

func TestOnlyTheKeysOfTheModeInUseAreListed(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	sees(t, m, "m misses")
	lacks(t, m, "b boxes")
	m = press(m, "tab")
	sees(t, m, "b boxes")
	lacks(t, m, "m misses")
	// Hidden is not disabled.
	f.boxes = engine.BoxReport{Name: "European capitals"}
	m = press(m, "esc")
	m = press(m, "m")
	if m.screen != missesScreen {
		t.Fatalf("m in Leitner mode went to screen %d", m.screen)
	}
}

func TestADeckThatWillNotLoadShowsItsErrorInTheLibrary(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "down")
	sees(t, m, "No such deck")
	m = press(m, "right")
	if m.libFocus != 0 {
		t.Fatal("focus went into a deck with no cards to show")
	}
}

func TestDeletingACardAsksFirstAndOnlyYDoesIt(t *testing.T) {
	f := newFake()
	f.cards = map[string][]engine.CardSummary{"capitals": {
		{ID: "France", Title: "Capital of France?"}, {ID: "Spain", Title: "Capital of Spain?"},
	}}
	m, _ := start(t, f)
	m = press(m, "2", "right", "d")
	sees(t, m, "Move the card France to .trash?", "y yes")
	m = press(m, "n")
	sees(t, m, "Kept.")
	if len(f.deleted) != 0 {
		t.Fatalf("deleted %v without a y", f.deleted)
	}

	m = press(m, "d", "y")
	if !slices.Equal(f.deleted, []string{"capitals/France"}) {
		t.Fatalf("deleted %v", f.deleted)
	}
	sees(t, m, "Moved France to /trash/France.md", "Capital of Spain?")
	lacks(t, m, "Capital of France?")
}

func TestDeletingADeckAsksFirstAndSaysItsCardsStay(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "d")
	sees(t, m, "Move the deck capitals to .trash?", "kept elsewhere and stay")
	m = press(m, "y")
	if !slices.Equal(f.deleted, []string{"capitals"}) {
		t.Fatalf("deleted %v", f.deleted)
	}
	sees(t, m, "Moved capitals to /trash/capitals")
	if _, ok := m.libraryDeck(); ok {
		t.Fatal("the deleted deck is still on the library's cursor")
	}
}

func TestADecksDeletionSaysWhetherItsCardsGoToo(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].CardsInside = true
	m, _ := start(t, f)
	m = press(m, "2", "d")
	sees(t, m, "Its cards are inside it, so they go too")
}

func TestALongListScrollsWithTheCursor(t *testing.T) {
	f := newFake()
	var cards []engine.CardSummary
	for i := range 40 {
		id := fmt.Sprintf("card%02d", i)
		cards = append(cards, engine.CardSummary{ID: id, Title: "Question " + id})
	}
	f.cards = map[string][]engine.CardSummary{"capitals": cards}
	m, _ := start(t, f)
	m.height = 30
	m = press(m, "2", "right", "G")
	sees(t, m, "Question card39")
	lacks(t, m, "Question card00")
}

func TestTheSelectedRowIsMarkedInEachPaneAndFillsItsWidth(t *testing.T) {
	// Colour is a terminal's business, so the test holds what stays without it:
	// the marker is on the row of the pane that has the keys, and only there,
	// and a cursor row is the width of its pane whatever it says.
	on := listRow("Capital of France?", 30, true, true, false)
	off := listRow("Capital of France?", 30, true, false, false)
	rest := listRow("Capital of Spain?", 30, false, true, false)
	if !strings.Contains(on, "▌") || strings.Contains(off, "▌") || strings.Contains(rest, "▌") {
		t.Fatalf("markers: %q %q %q", on, off, rest)
	}
	if cells(on) != 30 || cells(off) != 30 {
		t.Fatalf("widths %d and %d, not 30", cells(on), cells(off))
	}

	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2")
	sees(t, m, "▌ European capitals")
	m = press(m, "right")
	lacks(t, m, "▌ European capitals")
}

func twoCards(f *fake) {
	two := 30
	f.cards = map[string][]engine.CardSummary{"capitals": {
		{ID: "France", Title: "Capital of **France**?", Path: "/decks/capitals/France.md",
			Answer: []string{"Paris.", "On the Seine."}, Notes: []string{"Since 987."},
			Reference: "[[France]]", Time: &two, Follows: []string{"Spain"}},
		{ID: "Spain", Title: "Capital of Spain?", Draft: true, Path: "/decks/capitals/Spain.md",
			Answer: []string{"Madrid."}},
	}}
}

func TestTheLibraryListsQuestionsAndShowsTheCardUnderTheCursorWholeBelow(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m.width = 100
	m = press(m, "2")
	sees(t, m, "Capital of France?", "Capital of Spain? (draft)", "→ cards")
	lacks(t, m, "Answer")
	// The first card, whole: its answer, notes, where it leads, how it is timed.
	sees(t, m, "Card", "Paris.", "On the Seine.", "notes", "Since 987.",
		"leads to", "→ Capital of Spain?", "time 30s", "see France")
	// It is below the lists, not beside them.
	view := m.View()
	if strings.Index(view, "Paris.") < strings.Index(view, "Capital of Spain?") {
		t.Fatal("the card is above the list of questions")
	}
	// The second, with the context it is asked in: the card that leads to it.
	m = press(m, "right", "down")
	sees(t, m, "Madrid.", "↳ follows Capital of France?", "draft")
	lacks(t, m, "Since 987.")
	lacks(t, m, "leads to")
}

func manyCards(f *fake, n int) {
	var cards []engine.CardSummary
	for i := range n {
		id := fmt.Sprintf("card%02d", i)
		cards = append(cards, engine.CardSummary{ID: id, Title: "Question " + id, Answer: []string{"A"}})
	}
	f.cards = map[string][]engine.CardSummary{"capitals": cards}
}

func TestAListThatRunsPastItsPaneSaysThereIsMore(t *testing.T) {
	f := newFake()
	manyCards(f, 40)
	m, _ := start(t, f)
	m.height = 40
	m = press(m, "2")
	sees(t, m, "▼")
	lacks(t, m, "▲")
	m = press(m, "right", "G")
	sees(t, m, "▲", "Question card39")
	lacks(t, m, "▼")
	m = press(m, "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up")
	sees(t, m, "▲", "▼")
}

func TestTheDecksListSaysThereIsMoreToo(t *testing.T) {
	f := newFake()
	f.decks.Decks = nil
	for i := range 30 {
		f.decks.Decks = append(f.decks.Decks, engine.DeckInfo{ID: fmt.Sprintf("deck%02d", i), Name: fmt.Sprintf("Deck %02d", i)})
	}
	m, _ := start(t, f)
	m.height = 40
	m = press(m, "2")
	sees(t, m, "▼", "more")
}

func TestAListThatFitsHasNoArrows(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m.height = 50
	m = press(m, "2")
	lacks(t, m, "▲")
	lacks(t, m, "▼")
}

func TestEditingHappensInPlaceBelowTheListsWhichStayInView(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m.width = 100
	m = press(m, "2", "right", "e")
	sees(t, m, "Edit card", "Question", "Answer", "Notes", "Reference", "Time", "Draft", "Leads to",
		"Save", "Open in editor", "Cancel", "Capital of Spain? (draft)")
	view := m.View()
	if strings.Index(view, "Edit card") < strings.Index(view, "Capital of Spain?") {
		t.Fatal("the form is above the list of questions")
	}
	if m.screen != libraryScreen {
		t.Fatalf("on screen %d", m.screen)
	}
	// Three columns side by side: the boxes, then the card's options, then the
	// actions, the last two each headed and in one column of their own.
	same := false
	for _, line := range strings.Split(view, "\n") {
		q, o, a := strings.Index(line, "Question"), strings.Index(line, "Card options"), strings.Index(line, "Actions")
		if q >= 0 && o > q && a > o {
			same = true
		}
	}
	if !same {
		t.Fatalf("the boxes, options and actions are not side by side:\n%s", view)
	}
	beside := false
	for _, line := range strings.Split(view, "\n") {
		if i, j := strings.Index(line, "Time"), strings.Index(line, "Save"); i >= 0 && j > i {
			beside = true // the first option and the first action, in two columns
		}
	}
	if !beside {
		t.Fatalf("the actions are not a column of their own, beside the options:\n%s", view)
	}
}

func TestTheArrowsMoveBetweenTheViewsOfAForm(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "e")
	at := func() string { return m.form.fields[m.form.at].key }
	if at() != keyTitle {
		t.Fatalf("starts on %s", at())
	}
	m = press(m, "down")
	if at() != keyAnswer {
		t.Fatalf("down: %s", at())
	}
	m = press(m, "down", "down", "down")
	if at() != keyReference {
		t.Fatalf("down to the last box, and no further: %s", at())
	}
	m = press(m, "right")
	if at() != keyTime {
		t.Fatalf("right goes to the options: %s", at())
	}
	m = press(m, "down", "down", "down")
	if at() != keyFollows {
		t.Fatalf("down the options, and no further: %s", at())
	}
	m = press(m, "right")
	if at() != keySave {
		t.Fatalf("right again goes to the actions: %s", at())
	}
	m = press(m, "right", "down", "down", "down")
	if at() != keyCancel {
		t.Fatalf("down the actions, and no further: %s", at())
	}
	m = press(m, "up", "up", "up", "up")
	if at() != keySave {
		t.Fatalf("up does not leave the actions: %s", at())
	}
	// left comes back to the view last on in each column.
	m = press(m, "left")
	if at() != keyFollows {
		t.Fatalf("left to the options: %s", at())
	}
	m = press(m, "left")
	if at() != keyReference {
		t.Fatalf("left to the boxes: %s", at())
	}
	m = press(m, "left", "up", "right", "left")
	if at() != keyNotes {
		t.Fatalf("and the box it was moved to: %s", at())
	}
}

func TestADeckFormHasNoOptionsColumn(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "n")
	lacks(t, m, "Card options")
	sees(t, m, "Actions", "Name", "About")
	m = press(m, "right")
	if m.form.fields[m.form.at].key != keySave {
		t.Fatalf("right from the boxes went to %s", m.form.fields[m.form.at].key)
	}
	m = press(m, "left")
	if m.form.fields[m.form.at].key != keyName {
		t.Fatalf("and back to %s", m.form.fields[m.form.at].key)
	}
}

func TestTabAndShiftTabMoveThroughEveryViewInOrder(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "e")
	var order []string
	for range len(m.form.fields) + 2 {
		order = append(order, m.form.fields[m.form.at].key)
		m = press(m, "tab")
	}
	want := []string{keyTitle, keyAnswer, keyNotes, keyReference, keyTime, keyDraft, keyFollows,
		keySave, keyOpen, keyCancel, keyCancel, keyCancel}
	if !slices.Equal(order, want) {
		t.Fatalf("tab went %v", order)
	}
	m = press(m, "shift+tab", "shift+tab", "shift+tab", "shift+tab")
	if m.form.fields[m.form.at].key != keyDraft {
		t.Fatalf("shift+tab: %s", m.form.fields[m.form.at].key)
	}
}

func TestAViewIsEnteredWithEnterAndLeftWithEsc(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	// Not entered, letters are not text.
	m = typed(m, "abc")
	if m.form.value(keyTitle) != "" {
		t.Fatalf("typed into a box that was not entered: %q", m.form.value(keyTitle))
	}
	m = press(m, "enter")
	sees(t, m, "done typing")
	m = typed(m, "Hello")
	if m.form.value(keyTitle) != "Hello" {
		t.Fatalf("typed %q", m.form.value(keyTitle))
	}
	m = press(m, "esc")
	if m.form == nil || m.form.mode != navigating {
		t.Fatal("esc in a box did not just leave it")
	}
	m = press(m, "esc")
	if m.form != nil {
		t.Fatal("esc in the form did not leave it")
	}
}

func TestEnterIsANewlineInEveryBoxAndEscEndsTheTyping(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	for _, key := range []string{keyTitle, keyAnswer, keyNotes, keyReference} {
		m.form.at = slices.IndexFunc(m.form.fields, func(x formField) bool { return x.key == key })
		m = press(m, "enter")
		m = typed(m, "one\ntwo")
		if m.form.mode != typing || m.form.value(key) != "one\ntwo" {
			t.Fatalf("%s: mode %d, text %q", key, m.form.mode, m.form.value(key))
		}
		m = press(m, "esc")
		if m.form.mode != navigating {
			t.Fatalf("%s: esc did not end the typing", key)
		}
	}
}

func TestAOneLineBoxIsMadeOneLineWhenItIsSaved(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	m = write(m, "Two\nlines?")
	m = press(m, "ctrl+s")
	// The form sends what was typed; the engine makes it one line (its tests hold that).
	if f.newCard.Title != "Two\nlines?" {
		t.Fatalf("asked for %q", f.newCard.Title)
	}
}

func TestTabLeavesTheBoxAndMovesOn(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n", "enter")
	m = typed(m, "Q?")
	m = press(m, "tab")
	if m.form.mode != navigating || m.form.fields[m.form.at].key != keyAnswer {
		t.Fatalf("mode %d on %s", m.form.mode, m.form.fields[m.form.at].key)
	}
}

func TestTheCursorMovesUpAndDownThroughABigBox(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n", "down", "enter")
	m = typed(m, "first line\nsecond\nthird line")
	m = press(m, "up", "up")
	m = typed(m, ">")
	// The column is not remembered: "second" is shorter than the end of "third line".
	if got := m.form.value(keyAnswer); got != "first >line\nsecond\nthird line" {
		t.Fatalf("up up then typing: %q", got)
	}
	m = press(m, "down", "home")
	m = typed(m, "<")
	if got := m.form.value(keyAnswer); got != "first >line\n<second\nthird line" {
		t.Fatalf("down then home: %q", got)
	}
	m = press(m, "down", "end")
	m = typed(m, "!")
	if got := m.form.value(keyAnswer); got != "first >line\n<second\nthird line!" {
		t.Fatalf("down then end: %q", got)
	}
}

func TestAnOptionIsSelectedWithEnterThenTurnedWithTheArrows(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n", "right")
	if m.form.fields[m.form.at].key != keyTime {
		t.Fatalf("on %s", m.form.fields[m.form.at].key)
	}
	m = press(m, "enter")
	sees(t, m, "←→ change")
	m = press(m, "right", "right", "right") // auto, off, 5s, 10s
	if got := m.form.field(keyTime).wheel.value(); got != 10 {
		t.Fatalf("time %d", got)
	}
	m = press(m, "enter")
	if m.form.mode != navigating {
		t.Fatal("enter did not let go of the option")
	}
	// Not selected, right does not turn it: it moves to the actions.
	m = press(m, "right")
	if got := m.form.field(keyTime).wheel.value(); got != 10 || m.form.fields[m.form.at].key != keySave {
		t.Fatalf("right on an option that was not selected: time %d, on %s", got, m.form.fields[m.form.at].key)
	}
	m = press(m, "left", "down", "enter", "space")
	if m.form.field(keyDraft).wheel.value() != 1 {
		t.Fatal("space did not flip the draft switch")
	}
	m = press(m, "esc")
	if m.form == nil || m.form.mode != navigating {
		t.Fatal("esc while selecting left the form, not the option")
	}
}

func TestANewCardIsWrittenFromTheFormWithNoEditor(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	m = write(m, "What is a delta?")
	m = press(m, "down")
	m = write(m, "Where a river meets the sea.\nOften triangular.")
	m = press(m, "down", "down")
	m = write(m, "[[Rivers]]")
	m = press(m, "right", "enter", "right", "right", "right", "enter") // time: 10s
	m = press(m, "down", "enter", "space", "enter")                    // draft
	m = press(m, "right", "enter")                                     // Save
	got := f.newCard
	if got.Deck != "capitals" || got.Title != "What is a delta?" ||
		got.Answer != "Where a river meets the sea.\nOften triangular." ||
		got.Reference != "[[Rivers]]" || got.Notes != "" || !got.Draft ||
		got.Time == nil || *got.Time != 10 || len(got.Follows) != 0 {
		t.Fatalf("asked for %+v", got)
	}
	sees(t, m, "Made the card Q.")
	if m.form != nil || len(f.opened) != 0 {
		t.Fatalf("form %v, editor %v", m.form, f.opened)
	}
}

func TestCtrlSSavesFromAnyView(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	m = write(m, "Q?")
	m = press(m, "right", "ctrl+s")
	if f.newCard.Title != "Q?" {
		t.Fatalf("asked for %+v", f.newCard)
	}
}

func TestACardNeedsAQuestionBeforeItIsSaved(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n", "ctrl+s")
	sees(t, m, "A card needs a question.")
	if f.newCard.Title != "" || m.form == nil {
		t.Fatalf("saved %+v, form %v", f.newCard, m.form)
	}
	if m.form.at != 0 {
		t.Fatalf("the cursor is on view %d, not the question", m.form.at)
	}
}

func TestAnAutoTimeIsNoTimeAtAll(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	m = write(m, "Q?")
	m = press(m, "ctrl+s")
	if f.newCard.Time != nil {
		t.Fatalf("auto sent %d", *f.newCard.Time)
	}
}

func TestTheActionsAreSaveOpenInEditorAndCancel(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	m = write(m, "Q?")
	// Cancel: the last of the actions, two columns to the right.
	m = press(m, "right", "right", "down", "down", "enter")
	if m.form != nil || f.newCard.Title != "" {
		t.Fatalf("cancel saved or stayed: form %v, %+v", m.form, f.newCard)
	}
}

func TestOpenInEditorSavesThenOpensTheFile(t *testing.T) {
	f := newFake()
	f.problems = []string{"Q: follow-up points at a missing card: Gone"}
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	m = write(m, "What is a delta?")
	m = press(m, "right", "right", "down", "enter") // Open in editor
	if f.newCard.Title != "What is a delta?" {
		t.Fatalf("the card was not saved first: %+v", f.newCard)
	}
	if !slices.Equal(f.opened, []string{"/decks/capitals/Q.md"}) {
		t.Fatalf("the editor was given %v", f.opened)
	}
	sees(t, m, "Edited the card Q.", "1 problem", "missing card: Gone")
}

func TestLeadsToIsPickedFromTheCardListAboveWithoutChangingTheCardShown(t *testing.T) {
	f := newFake()
	twoCards(f)
	f.cards["capitals"] = append(f.cards["capitals"], engine.CardSummary{ID: "Italy", Title: "Capital of Italy?", Answer: []string{"Rome."}})
	m, _ := start(t, f)
	m.width = 100
	m = press(m, "2", "right", "e") // France, which leads to Spain
	m = press(m, "right", "down", "down", "enter")
	if m.form.picker == nil {
		t.Fatal("enter on Leads to did not open the picker")
	}
	// The card list above has the boxes; the card being edited has none.
	sees(t, m, "[x] Capital of Spain? (draft)", "[ ] Capital of Italy?", "(this card)", "tick the cards it leads to",
		"space tick")
	// The form below is still the card being edited.
	sees(t, m, "Edit card", "Capital of **France**?")
	// The cursor starts on the first card that can be ticked (not France).
	if m.form.picker.cards[m.form.picker.at].ID != "Spain" {
		t.Fatalf("the picker starts on %s", m.form.picker.cards[m.form.picker.at].ID)
	}
	m = press(m, "down", "space") // Italy
	m = press(m, "up")
	if m.form.picker.cards[m.form.picker.at].ID != "Spain" {
		t.Fatalf("up went to %s", m.form.picker.cards[m.form.picker.at].ID)
	}
	m = press(m, "up") // past Spain is France, which is passed over
	if m.form.picker.cards[m.form.picker.at].ID != "Spain" {
		t.Fatalf("up onto the card being edited: %s", m.form.picker.cards[m.form.picker.at].ID)
	}
	// The card shown in the library did not move.
	if m.libCard != 0 {
		t.Fatalf("the library's card moved to %d", m.libCard)
	}
	m = press(m, "space") // untick Spain
	m = press(m, "enter")
	if !slices.Equal(m.form.field(keyFollows).links, []string{"Italy"}) {
		t.Fatalf("leads to %v", m.form.field(keyFollows).links)
	}
	sees(t, m, "1 card ▸")
	m = press(m, "ctrl+s")
	if !slices.Equal(f.updCard.Follows, []string{"Italy"}) {
		t.Fatalf("saved %v", f.updCard.Follows)
	}
}

func TestLeavingThePickerWithEscKeepsWhatWasThere(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "e", "right", "down", "down", "enter", "space", "esc")
	if !slices.Equal(m.form.field(keyFollows).links, []string{"Spain"}) || m.form.picker != nil {
		t.Fatalf("leads to %v, picker %v", m.form.field(keyFollows).links, m.form.picker)
	}
}

func TestANewCardsPickerListsEveryCard(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "n", "right", "down", "down", "enter")
	sees(t, m, "[ ] Capital of France?", "[ ] Capital of Spain? (draft)")
	lacks(t, m, "(this card)")
	m = press(m, "space", "down", "space", "enter")
	if !slices.Equal(m.form.field(keyFollows).links, []string{"France", "Spain"}) {
		t.Fatalf("leads to %v", m.form.field(keyFollows).links)
	}
}

func TestAnExistingCardIsEditedInTheFormFilledFromIt(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "enter")
	sees(t, m, "Edit card", "Capital of **France**?", "Paris.", "On the Seine.", "Since 987.", "[[France]]",
		"1 card ▸", "30s")
	m = press(m, "enter")
	m = typed(m, " Really?")
	m = press(m, "ctrl+s")
	got := f.updCard
	if got.Deck != "capitals" || got.Card != "France" || got.Title != "Capital of **France**? Really?" ||
		got.Answer != "Paris.\nOn the Seine." || got.Notes != "Since 987." || got.Reference != "[[France]]" ||
		got.Time == nil || *got.Time != 30 || !slices.Equal(got.Follows, []string{"Spain"}) || got.Draft {
		t.Fatalf("asked for %+v", got)
	}
	sees(t, m, "Saved the card France.")
	if f.newCard.Title != "" {
		t.Fatal("editing made a new card")
	}
}

func TestADraftCardComesBackAsADraft(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "down", "e", "ctrl+s")
	if !f.updCard.Draft || f.updCard.Time != nil {
		t.Fatalf("asked for %+v", f.updCard)
	}
}

func TestOOpensAnExistingCardInTheEditorAndChecksIt(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m = press(m, "2", "right", "o")
	if !slices.Equal(f.opened, []string{"/decks/capitals/France.md"}) {
		t.Fatalf("the editor was given %v", f.opened)
	}
	sees(t, m, "Edited the card France.")
	calls(t, f, "settings", "decks", "cards", "check", "decks", "cards")
}

func TestAnEditorThatFailsLeavesTheFileAndSaysWhere(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m.exec = func(path string) tea.Cmd {
		return func() tea.Msg { return editorDone{err: &engine.Error{Message: "exit status 1"}} }
	}
	m = press(m, "2", "right", "o")
	sees(t, m, "The editor failed", "/decks/capitals/France.md")
}

func TestTheDigitsAreTheTextsInABoxNotTabs(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n", "enter")
	m = typed(m, "2 plus 3?")
	if m.screen != libraryScreen || m.form == nil || m.form.value(keyTitle) != "2 plus 3?" {
		t.Fatalf("screen %d, question %q", m.screen, m.form.value(keyTitle))
	}
	// And in a form that is not being typed in, a digit is not a tab either.
	m = press(m, "esc", "3")
	if m.screen != libraryScreen || m.form == nil {
		t.Fatalf("a digit on a form went to screen %d", m.screen)
	}
}

func TestAnEngineRefusalKeepsTheFormAndWhatWasTyped(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "right", "n")
	m = write(m, "Duplicate?")
	f.refuse = &engine.Error{Message: "There is already a card called 'Duplicate'."}
	m = press(m, "ctrl+s")
	sees(t, m, "There is already a card called")
	if m.form == nil || m.form.value(keyTitle) != "Duplicate?" {
		t.Fatalf("form %v", m.form)
	}
}

func TestANewDeckIsMadeFromAForm(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "2", "n")
	sees(t, m, "New deck", "Name", "About", "Save", "Open in editor", "Cancel")
	m = press(m, "ctrl+s")
	sees(t, m, "A deck needs a name.")
	if f.newDeck.Name != "" {
		t.Fatal("a nameless deck was made")
	}
	m = write(m, "Rivers")
	m = press(m, "down")
	m = write(m, "Long ones")
	m = press(m, "ctrl+s")
	if f.newDeck.Name != "Rivers" || f.newDeck.Description != "Long ones" {
		t.Fatalf("asked for %+v", f.newDeck)
	}
	sees(t, m, "Made the deck rivers.")
	if m.form != nil {
		t.Fatal("the form is still open")
	}
	if deck, _ := m.libraryDeck(); deck.ID != "rivers" {
		t.Fatalf("the cursor is on %q, not the new deck", deck.ID)
	}
	if len(f.opened) != 0 {
		t.Fatalf("the editor was opened: %v", f.opened)
	}
}

func TestADeckIsEditedInAFormAndCanBeCancelled(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].Desc = "Capital cities."
	m, _ := start(t, f)
	m = press(m, "2", "e")
	sees(t, m, "Edit deck", "European capitals", "Capital cities.")
	m = press(m, "enter")
	m = typed(m, " of Europe")
	m = press(m, "ctrl+s")
	if f.updDeck.Deck != "capitals" || f.updDeck.Name != "European capitals of Europe" ||
		f.updDeck.Description != "Capital cities." {
		t.Fatalf("asked for %+v", f.updDeck)
	}
	sees(t, m, "Saved the deck capitals.")

	m = press(m, "e", "esc")
	if m.form != nil {
		t.Fatal("esc did not leave the deck form")
	}
	if f.updDeck.Name != "European capitals of Europe" {
		t.Fatal("cancel saved")
	}
}

func TestADeckThatWillNotLoadIsOpenedInTheEditorNotAForm(t *testing.T) {
	f := newFake()
	p := "/decks/stale"
	f.decks.Decks[1].Path = &p
	m, _ := start(t, f)
	m = press(m, "2", "down", "e")
	if m.form != nil {
		t.Fatal("a deck that will not load has a form")
	}
	sees(t, m, "will not load", "o opens")
	m = press(m, "o")
	if !slices.Equal(f.opened, []string{"/decks/stale/deck.toml"}) {
		t.Fatalf("the editor was given %v", f.opened)
	}
}

func TestADecksFileIsOpenedInTheEditorWithO(t *testing.T) {
	f := newFake()
	p := "/decks/capitals"
	f.decks.Decks[0].Path = &p
	m, _ := start(t, f)
	m = press(m, "2", "o")
	if !slices.Equal(f.opened, []string{"/decks/capitals/deck.toml"}) {
		t.Fatalf("the editor was given %v", f.opened)
	}
	sees(t, m, "Edited /decks/capitals/deck.toml")
}

func viewLines(m model) []string { return strings.Split(m.View(), "\n") }

// keyLine is the line of the view the key hints start on, from the top.
func keyLine(t *testing.T, m model, hint string) int {
	t.Helper()
	for i, l := range viewLines(m) {
		if strings.Contains(l, hint) {
			return i
		}
	}
	t.Fatalf("no line has %q:\n%s", hint, m.View())
	return -1
}

func TestTheKeyHintsAreTheLastLinesOfTheWindowOnEveryPage(t *testing.T) {
	for _, h := range []int{30, 40, 60} {
		f := newFake()
		twoCards(f)
		m, _ := start(t, f)
		m.width, m.height = 100, h
		pages := map[string]model{"decks": m}
		lib := press(m, "2")
		pages["library"] = lib
		pages["library, on a card"] = press(lib, "right")
		pages["editing"] = press(lib, "right", "e")
		pages["picking"] = press(lib, "right", "e", "right", "down", "down", "enter")
		pages["settings"] = press(m, "3")
		pages["options"] = press(m, "enter")
		for name, page := range pages {
			lines := viewLines(page)
			if len(lines) != h {
				t.Errorf("height %d, %s: %d lines, not the window's", h, name, len(lines))
				continue
			}
			if !strings.Contains(lines[h-1], "  ") || strings.TrimSpace(lines[h-1]) == "" {
				t.Errorf("height %d, %s: the last line is not the key hints: %q", h, name, lines[h-1])
			}
		}
	}
}

func TestTheKeyHintsDoNotMoveWhenAStatusComesAndGoes(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m.width, m.height = 100, 40
	m = press(m, "2", "right")
	before := keyLine(t, m, "choose")
	m = press(m, "d", "n") // a confirmation, then "Kept."
	sees(t, m, "Kept.")
	if got := keyLine(t, m, "choose"); got != before {
		t.Fatalf("the key hints moved from line %d to %d when a status appeared", before, got)
	}
	m.err = "There is already a card called 'X'.\nAnd a second line of it."
	if got := keyLine(t, m, "choose"); got != before {
		t.Fatalf("the key hints moved to line %d for an error", got)
	}
}

func TestTheKeyHintsDoNotMoveWhenTheKeysAreLongerOrShorter(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m.width, m.height = 80, 40
	m = press(m, "2", "right", "e")
	last := len(viewLines(m)) - 1
	for _, k := range []string{"enter", "esc", "enter", "esc"} { // selecting, typing: longer hints
		m = press(m, k)
		if got := len(viewLines(m)) - 1; got != last {
			t.Fatalf("the window is %d lines after %s, not %d", got+1, k, last+1)
		}
		if !strings.Contains(viewLines(m)[last], " ") || strings.TrimSpace(viewLines(m)[last]) == "" {
			t.Fatalf("the last line is not the hints after %s", k)
		}
	}
}

func TestThePanesAreTheSizeOfTheWindowAndNotOfWhatIsInThem(t *testing.T) {
	f := newFake()
	twoCards(f)
	f.decks.Decks = append(f.decks.Decks, engine.DeckInfo{ID: "more", Name: "More", Desc: "A deck with a longer description that wraps over more than one line of the detail."})
	m, _ := start(t, f)
	m.width, m.height = 100, 45
	m = press(m, "2")
	top := func(m model, mark string) int { return keyLine(t, m, mark) }
	// The card below the lists starts on the same line whichever card is shown.
	first := top(m, "Card")
	m = press(m, "right", "down")
	if got := top(m, "Card"); got != first {
		t.Fatalf("the card moved from line %d to %d as the cursor moved", first, got)
	}
	// And whether it is being read or written.
	m = press(m, "e")
	if got := top(m, "Edit card"); got != first {
		t.Fatalf("the form is on line %d, the card was on %d", got, first)
	}
	// The boxes do not grow as they fill.
	before := len(viewLines(m))
	m = press(m, "enter")
	m = typed(m, strings.Repeat("a long line of text ", 30))
	if got := len(viewLines(m)); got != before {
		t.Fatalf("the window grew from %d to %d lines as a box filled", before, got)
	}
	// The decks tab too: its panes do not resize between decks.
	d := press(start2(t, f, 100, 45), "down")
	for range 2 {
		if got := len(viewLines(d)); got != 45 {
			t.Fatalf("the deck list is %d lines", got)
		}
		d = press(d, "down")
	}
}

func start2(t *testing.T, f *fake, w, h int) model {
	t.Helper()
	m, _ := start(t, f)
	m.width, m.height = w, h
	return m
}

func TestTheBoxesGrowWithTheWindowNotWithWhatIsTypedInThem(t *testing.T) {
	small := newCardForm("d").boxRows(10)
	large := newCardForm("d").boxRows(30)
	if large[1] <= small[1] || large[2] <= small[2] {
		t.Fatalf("the answer and notes boxes did not grow with the window: %v then %v", small, large)
	}
	if large[1] < large[2] {
		t.Fatalf("the answer box is smaller than the notes: %v", large)
	}
	for _, rows := range []map[int]int{small, large, newDeckForm().boxRows(10), newDeckForm().boxRows(20)} {
		for i, n := range rows {
			if n < 1 {
				t.Fatalf("box %d has %d lines: %v", i, n, rows)
			}
		}
	}
	// A form fits the lines it is given.
	f := newFake()
	m, _ := start(t, f)
	m.width, m.height = 100, 46
	m = press(m, "2", "right", "n")
	_, want := m.librarySplit()
	if got := len(m.formRegion(96, want)); got != want {
		t.Fatalf("the form is %d lines, not the %d it was given", got, want)
	}
}

func TestTheWidthIsTheWholeWindow(t *testing.T) {
	f := newFake()
	twoCards(f)
	for _, w := range []int{60, 100, 140, 200, 300} {
		m, _ := start(t, f)
		m.width, m.height = w, 40
		for name, page := range map[string]model{
			"decks": m, "library": press(m, "2"), "editing": press(m, "2", "right", "e"), "settings": press(m, "3"),
		} {
			widest := 0
			for _, l := range viewLines(page) {
				widest = max(widest, cells(l))
			}
			if name == "settings" { // narrow by choice: it is a card of a few rows
				continue
			}
			if widest != w {
				t.Errorf("%s at %d columns is %d wide", name, w, widest)
			}
		}
	}
}

func redCard(f *fake) {
	f.cards = map[string][]engine.CardSummary{"capitals": {{
		ID: "Red", Path: "/decks/capitals/Red.md",
		Title:  "Which is **&#x1f534; red**?",
		Answer: []string{"&#x1F7E0; not orange, &#128308; not amber", "&lt;b&gt; stays"},
		Notes:  []string{"A &#60; B"},
	}}}
}

func TestCharacterReferencesInACardAreDecodedWhenItIsOpenedToBeEdited(t *testing.T) {
	f := newFake()
	redCard(f)
	m, _ := start(t, f)
	// The card view shows the characters, as it always has.
	m = press(m, "2")
	sees(t, m, "Which is 🔴 red?", "🟠 not orange, 🔴 not amber")
	// Opened for editing, the boxes hold the characters, so that it is saved with them.
	m = press(m, "right", "e")
	if got := m.form.value(keyTitle); got != "Which is **🔴 red**?" {
		t.Fatalf("the question is %q", got)
	}
	if got := m.form.value(keyAnswer); got != "🟠 not orange, 🔴 not amber\n&lt;b&gt; stays" {
		t.Fatalf("the answer is %q", got)
	}
	if got := m.form.value(keyNotes); got != "A &#60; B" {
		t.Fatalf("a reference to an ASCII character was decoded: %q", got)
	}
	m = press(m, "ctrl+s")
	if got := f.updCard.Title; got != "Which is **🔴 red**?" || strings.Contains(f.updCard.Answer, "&#") {
		t.Fatalf("saved with references: %q, %q", got, f.updCard.Answer)
	}
}

func TestTheEntitiesSettingCanLeaveThemAsWritten(t *testing.T) {
	f := newFake()
	redCard(f)
	f.settings = map[string]string{"entities": "off"}
	m, _ := start(t, f)
	m = press(m, "2", "right", "e")
	if got := m.form.value(keyTitle); got != "Which is **&#x1f534; red**?" {
		t.Fatalf("with the setting off the question is %q", got)
	}
	m = press(m, "ctrl+s")
	if f.updCard.Title != "Which is **&#x1f534; red**?" {
		t.Fatalf("saved %q", f.updCard.Title)
	}
	// The card view is the same either way.
	m = press(m, "esc")
	sees(t, m, "Which is 🔴 red?")
}

func TestTheEntitiesSettingIsOnTheSettingsScreenAndIsSaved(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	if !m.prefs.decodeEntities() {
		t.Fatal("the setting is not on by default")
	}
	m = press(m, "3")
	lacks(t, m, "Emoji codes") // on the Editing tab, not the first
	m = press(m, "right")
	sees(t, m, "Emoji codes", "as characters")
	if m.pref != entitiesPref {
		t.Fatalf("on row %d", m.pref)
	}
	// The hint shows the code itself, and what the card view makes of it.
	sees(t, m, `"&#x1f534;"`, `"🔴"`, `"as`, `characters"`, `"As`, `written"`) // in speech marks; it wraps, so by word
	m = press(m, "enter", "left")
	sees(t, m, "as written")
	m = press(m, "enter")
	if m.prefs.decodeEntities() || f.settings["entities"] != "off" {
		t.Fatalf("turned off: %v, saved %v", m.prefs.decodeEntities(), f.settings)
	}
	m = press(m, "enter", "space", "enter")
	if !m.prefs.decodeEntities() || f.settings["entities"] != "on" {
		t.Fatalf("space did not flip it back: %v, saved %v", m.prefs.decodeEntities(), f.settings)
	}
	// It comes back on the next start.
	f.settings = map[string]string{"entities": "off"}
	m, _ = start(t, f)
	if m.prefs.decodeEntities() {
		t.Fatal("a saved off did not come back")
	}
	f.settings = map[string]string{"entities": "maybe"}
	m, _ = start(t, f)
	if !m.prefs.decodeEntities() {
		t.Fatal("a value it does not know changed the default")
	}
}

func TestADecksReferencesAreDecodedToo(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].Name = "Red &#x1f534; deck"
	m, _ := start(t, f)
	m = press(m, "2", "e")
	if got := m.form.value(keyName); got != "Red 🔴 deck" {
		t.Fatalf("the deck's name is %q", got)
	}
}

func TestAKeycapAndEveryOtherEmojiKeepTheWindowTheSize(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m.width, m.height = 100, 40
	m = press(m, "2", "right", "n", "enter")
	for _, e := range []string{"1️⃣", "🏻", "🏋", "❤️", "🧑‍💻", "🫠"} {
		m = typed(m, "x"+e+"y")
		lines := viewLines(m)
		if len(lines) != 40 {
			t.Fatalf("%q: %d lines", e, len(lines))
		}
		for i, l := range lines {
			if cells(l) > 100 {
				t.Errorf("%q: line %d is %d cells, over the window: %q", e, i, cells(l), l)
			}
		}
	}
}

func TestTheKeyHintsSayHowToChangeScreen(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	m.width = 120
	sees(t, m, "1-3 change screen")
	lib := press(m, "2")
	sees(t, lib, "1-3 change screen")
	sees(t, press(lib, "right"), "1-3 change screen")
	set := press(m, "3")
	sees(t, set, "1-3 change screen")
	// Where the digits are text, or a row is selected, they are not offered.
	editing := press(lib, "right", "e")
	lacks(t, editing, "change screen")
	lacks(t, press(set, "enter"), "change screen")
	// And the number is how many screens there are.
	if got := screenKeys(); got[0] != "1-"+strconv.Itoa(len(tabs)) {
		t.Fatalf("hint %v for %d screens", got, len(tabs))
	}
}

func TestTmuxGetsASmallWarningOnTheMainScreenOnly(t *testing.T) {
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	lacks(t, m, "In tmux")
	m.tmux = true
	m.width, m.height = 100, 40
	sees(t, m, "In tmux, lines containing emoji may look misaligned.")
	// It does not move the key hints, or take the status's place.
	before := keyLine(t, m, "change screen")
	m.status = "Kept."
	sees(t, m, "In tmux", "Kept.")
	if got := keyLine(t, m, "change screen"); got != before {
		t.Fatalf("the key hints moved from %d to %d", before, got)
	}
	m.err = "one\ntwo\nthree"
	lacks(t, m, "In tmux") // a screen with something to say says it
	m.err = ""
	m.status = ""
	// Not on the other screens.
	lacks(t, press(m, "2"), "In tmux")
	lacks(t, press(m, "3"), "In tmux")
}

func TestTheTmuxWarningFitsANarrowWindow(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m.tmux = true
	for _, w := range []int{30, 40, 60, 100} {
		m.width, m.height = w, 40
		sees(t, m, "In tmux")
		for _, line := range viewLines(m) {
			if got := cells(line); got > w {
				t.Fatalf("a line is %d cells in a %d-cell window: %q", got, w, line)
			}
		}
	}
}

// visual is the settings screen on its Visual tab, on the colour mode.
func visual(t *testing.T, f *fake) model {
	t.Helper()
	m, _ := start(t, f)
	return press(m, "3", "right", "right")
}

func TestTheSettingsAreInTabs(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "3")
	sees(t, m, "Decks", "Editing", "Visual", "settings tab", "Mode", "Cards", "Seconds")
	lacks(t, m, "Emoji codes")
	lacks(t, m, "Colour mode")

	m = press(m, "right")
	sees(t, m, "Emoji codes")
	lacks(t, m, "Mode")
	if m.pref != entitiesPref || m.prefTab != 1 {
		t.Fatalf("row %d, tab %d", m.pref, m.prefTab)
	}
	m = press(m, "right")
	sees(t, m, "Colour mode", "Terminal theme")
	lacks(t, m, "Emoji codes")
	m = press(m, "right") // no further
	if m.prefTab != 2 {
		t.Fatalf("right went past the last tab to %d", m.prefTab)
	}
	m = press(m, "left", "left", "left")
	if m.prefTab != 0 || m.pref != modeRow {
		t.Fatalf("left went to tab %d, row %d", m.prefTab, m.pref)
	}

	// Up and down stay in the tab, and do not wrap into the next.
	m = press(m, "down", "down", "down", "down")
	if m.pref != limitPref {
		t.Fatalf("down the first tab went to row %d", m.pref)
	}
	m = press(m, "up", "up", "up", "up")
	if m.pref != modeRow {
		t.Fatalf("up went to row %d", m.pref)
	}

	// Once a row is selected the arrows change it, and do not change the tab.
	m = press(m, "enter", "right")
	if m.prefTab != 0 || m.mode != leitner {
		t.Fatalf("with a row selected right went to tab %d, mode %q", m.prefTab, m.mode)
	}
	m = press(m, "enter", "right")
	if m.prefTab != 1 {
		t.Fatalf("with nothing selected right stayed on tab %d", m.prefTab)
	}
}

func TestEverySettingHasSpaceBetweenItsNameAndItsValueOnEveryTab(t *testing.T) {
	f := newFake()
	m := visual(t, f)
	m = press(m, "left", "left")
	checked := 0
	for range settingsTabs {
		for _, line := range viewLines(m) {
			for _, label := range append([]string{"Theme colour"}, prefLabels[:]...) {
				if label == "" {
					continue
				}
				if i := strings.Index(line, label+" "); i >= 0 {
					checked++
					if !strings.HasPrefix(line[i+len(label):], "  ") {
						t.Errorf("no space after %q: %q", label, line)
					}
				}
			}
		}
		m = press(m, "right")
	}
	if checked < 6 {
		t.Fatalf("checked only %d settings", checked)
	}
}

func TestTheColourIsTwoSettingsAModeAndWhatTheModeNeeds(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	sees(t, m, "Colour mode", "Terminal theme", "Theme colour", "cyan")
	lacks(t, m, "Add a colour")

	m = press(m, "enter", "right")
	if m.prefs.colourMode != customMode || f.settings["colour_mode"] != "custom" {
		t.Fatalf("mode %d, saved %v", m.prefs.colourMode, f.settings)
	}
	sees(t, m, "‹ Custom ›")
	m = press(m, "enter")
	sees(t, m, "Custom colour", "none yet", "Add a colour")
	lacks(t, m, "Theme colour")
	lacks(t, m, "Remove this colour") // nothing on the list to take off

	m = press(m, "up", "enter", "right")
	m = press(m, "enter")
	sees(t, m, "Wheel colour", defaultPickedColour)
	lacks(t, m, "Add a colour")
	m = press(m, "up", "enter", "left", "left", "enter")
	if m.prefs.colourMode != themeMode {
		t.Fatalf("back to theme: %d", m.prefs.colourMode)
	}
	sees(t, m, "Theme colour")
}

func TestTheThemeColoursAreAllSixteenWithASwatch(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "down")
	sees(t, m, "Theme colour", "cyan")
	m = press(m, "enter", "right")
	if f.settings["colour_theme"] != "white" || accent.GetForeground() != lipgloss.Color("7") {
		t.Fatalf("saved %v, accent %v", f.settings, accent.GetForeground())
	}
	lacks(t, m, "██ white") // selected: the bar is the colour, so it has no swatch
	sees(t, m, "‹    white ›")
	seen := []string{"cyan", "white"}
	for range 20 {
		m = press(m, "right")
		if last := seen[len(seen)-1]; m.prefs.theme.label() != last {
			seen = append(seen, m.prefs.theme.label())
		}
	}
	if len(seen) != 10 || seen[len(seen)-1] != "bright white" {
		t.Fatalf("turning right went through %v", seen)
	}
	if accent.GetForeground() != lipgloss.Color("15") || f.settings["colour_theme"] != "bright white" {
		t.Fatalf("the last: accent %v, saved %v", accent.GetForeground(), f.settings)
	}
	m = press(m, "left", "left", "left", "left", "left", "left", "left", "left", "left", "left",
		"left", "left", "left", "left", "left", "left", "left", "left", "left", "left")
	if m.prefs.theme.label() != "black" || accent.GetForeground() != lipgloss.Color("0") {
		t.Fatalf("the first: %q, accent %v", m.prefs.theme.label(), accent.GetForeground())
	}
}

// addColour adds a colour from the settings' add row: its form, the colour box first,
// then the name, then saved.
func addColour(m model, value, name string) model {
	m = press(m, "enter")
	m = write(m, value)
	m = press(m, "down")
	m = write(m, name)
	return press(m, "ctrl+s")
}

func TestACustomColourIsAddedInAFormInTheCardNotOnTheBottomLine(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "enter", "down", "down") // custom mode, the add row
	m = press(m, "enter")
	if m.form == nil || m.form.kind != kindNewColour || m.input != nil {
		t.Fatalf("form %v, input %v", m.form, m.input)
	}
	sees(t, m, "Add a colour", "Colour", "Name", "Actions", "Add", "Cancel", "hex like #ff8800")
	lacks(t, m, "Colour (hex, 0-255 or r,g,b)") // the old prompt on the bottom line
	// It is in the card, above the key hints, in the window.
	m.width, m.height = 100, 40
	if keyLine(t, m, "Add a colour") >= keyLine(t, m, "ctrl+s save") {
		t.Fatal("the form is not above the key hints")
	}
	lacks(t, m, "Card options")

	// A colour that cannot be read is refused, and the form is still there.
	m = write(m, "banana")
	m = press(m, "ctrl+s")
	sees(t, m, "That is not a colour")
	if m.form == nil || m.form.value(keyValue) != "banana" || len(m.prefs.custom) != 0 {
		t.Fatalf("form %v, list %+v", m.form, m.prefs.custom)
	}
	m = press(m, "up", "enter", "ctrl+u")
	m = typed(m, "255, 136, 0")
	sees(t, m, "#ff8800") // what it is, as it is typed
	m = press(m, "esc")

	m = press(m, "down")
	m = press(m, "ctrl+s")
	sees(t, m, "A colour needs a name.") // no name yet
	m = write(m, "Sunset glow")
	m = press(m, "ctrl+s")
	sees(t, m, "Added Sunset glow.")
	if m.form != nil || len(m.prefs.custom) != 1 || m.prefs.custom[0].Value != "#ff8800" {
		t.Fatalf("form %v, list %+v", m.form, m.prefs.custom)
	}
	if accent.GetForeground() != lipgloss.Color("#ff8800") || f.settings["colour_custom_choice"] != "Sunset glow" ||
		!strings.Contains(f.settings["colour_custom"], `"value":"#ff8800"`) {
		t.Fatalf("accent %v, saved %v", accent.GetForeground(), f.settings)
	}
	sees(t, m, "Remove this colour")

	// A second, in another form of colour; a name already used is refused, in any case.
	m = press(m, "enter")
	m = write(m, "#0af")
	m = press(m, "down")
	m = write(m, "SUNSET GLOW")
	m = press(m, "ctrl+s")
	sees(t, m, "There is already a colour called SUNSET GLOW.")
	m = press(m, "enter", "ctrl+u")
	m = typed(m, "Sky")
	m = press(m, "esc", "ctrl+s")
	if len(m.prefs.custom) != 2 || m.prefs.custom[1].Value != "#00aaff" || m.prefs.customAt != 1 {
		t.Fatalf("list %+v, chosen %d", m.prefs.custom, m.prefs.customAt)
	}

	// Esc leaves the form and adds nothing; so does Cancel.
	m = press(m, "enter")
	m = write(m, "#123456")
	m = press(m, "esc")
	if m.form != nil || len(m.prefs.custom) != 2 {
		t.Fatalf("esc: form %v, list %+v", m.form, m.prefs.custom)
	}
	m = press(m, "enter")
	m = write(m, "#123456")
	m = press(m, "right", "down", "enter") // the Cancel action
	if m.form != nil || len(m.prefs.custom) != 2 {
		t.Fatalf("cancel: form %v, list %+v", m.form, m.prefs.custom)
	}
}

func TestTheColourFormTakesDigitsAndPastesAsText(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "enter", "down", "down", "enter", "enter")
	m = typed(m, "208")
	if m.screen != settingsScreen || m.form == nil || m.form.value(keyValue) != "208" {
		t.Fatalf("screen %d, colour %q", m.screen, m.form.value(keyValue))
	}
	// The colour form has no file to open in an editor: ctrl+o does nothing.
	m = press(m, "esc", "ctrl+o")
	if m.form == nil || len(f.opened) != 0 || m.busy {
		t.Fatal("ctrl+o did something in the colour form")
	}
	// Its two boxes and two actions, and no column of options.
	if got := len(m.form.fields); got != 4 || m.form.hasOptions() {
		t.Fatalf("%d fields, options %v", got, m.form.hasOptions())
	}
}

func TestACustomColourIsChosenFromTheListAndOnlyRemovedNeverEdited(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "enter", "down", "enter")
	sees(t, m, "No colours yet")
	m = press(m, "down")
	m = addColour(m, "#ff0000", "Red")
	m = addColour(m, "#00ff00", "Green")

	// Turn through the list.
	m = press(m, "up", "enter")
	sees(t, m, "Green")
	m = press(m, "left")
	if m.prefs.custom[m.prefs.customAt].Name != "Red" || accent.GetForeground() != lipgloss.Color("#ff0000") {
		t.Fatalf("chosen %d, accent %v", m.prefs.customAt, accent.GetForeground())
	}
	m = press(m, "left") // no further
	if m.prefs.customAt != 0 {
		t.Fatalf("went past the first: %d", m.prefs.customAt)
	}
	m = press(m, "enter")

	// Removing one asks first, and only y does it.
	m = press(m, "down", "down", "enter")
	sees(t, m, "Remove Red from your colours?", "y yes")
	m = press(m, "n")
	sees(t, m, "Kept.")
	if len(m.prefs.custom) != 2 {
		t.Fatal("a colour was removed without a y")
	}
	m = press(m, "enter", "y")
	sees(t, m, "Removed it from the list.")
	if len(m.prefs.custom) != 1 || m.prefs.custom[0].Name != "Green" || accent.GetForeground() != lipgloss.Color("#00ff00") {
		t.Fatalf("list %+v, accent %v", m.prefs.custom, accent.GetForeground())
	}
	if !strings.Contains(f.settings["colour_custom"], "Green") || strings.Contains(f.settings["colour_custom"], "Red") {
		t.Fatalf("saved %v", f.settings)
	}
	// The last one: the row for removing goes with it, and the highlight is the default.
	m = press(m, "enter", "y")
	if len(m.prefs.custom) != 0 || accent.GetForeground() != lipgloss.Color("6") || m.pref != colourPref {
		t.Fatalf("list %+v, accent %v, row %d", m.prefs.custom, accent.GetForeground(), m.pref)
	}
	lacks(t, m, "Remove this colour")
	sees(t, m, "none yet", "Add a colour")

	// The only ways in are to add and to remove: nothing on the list is edited.
	m = press(m, "down")
	m = addColour(m, "#ff0000", "Red")
	for _, k := range []string{"e", "r", "n", "d", "x"} {
		before := m.prefs.custom[0]
		m = press(m, "up", k)
		if m.prefs.custom[0] != before || m.input != nil || m.confirm != nil || m.form != nil {
			t.Fatalf("%s did something to the list: %+v, input %v", k, m.prefs.custom, m.input)
		}
	}
}

func TestTheColourWheelPicksLiveAndCancelsBackToWhatWasThere(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "right", "enter", "down") // wheel mode, its row
	sees(t, m, "Wheel colour", defaultPickedColour, "colour wheel")
	m = press(m, "enter")
	if m.colourPick == nil {
		t.Fatal("enter on the wheel did not open it")
	}
	sees(t, m, "▼", defaultPickedColour, "tab", "hue or colour", "enter use it")
	before := accent.GetForeground()

	m = press(m, "left", "left", "down")
	picked := m.colourPick.hex()
	if picked == defaultPickedColour || accent.GetForeground() != lipgloss.Color(picked) {
		t.Fatalf("picked %s, accent %v: the highlight is not the colour under the cursor", picked, accent.GetForeground())
	}
	sees(t, m, picked)
	if m.prefs.picked != defaultPickedColour {
		t.Fatal("what is picked was kept before enter")
	}
	m = press(m, "esc")
	if m.colourPick != nil || accent.GetForeground() != before || m.prefs.picked != defaultPickedColour {
		t.Fatalf("esc: open %v, accent %v, picked %s", m.colourPick != nil, accent.GetForeground(), m.prefs.picked)
	}
	if v := f.settings["colour_wheel"]; v != "" && v != defaultPickedColour {
		t.Fatalf("esc saved %v", f.settings)
	}

	m = press(m, "enter", "left", "left", "down", "down")
	picked = m.colourPick.hex()
	m = press(m, "enter")
	if m.colourPick != nil || m.prefs.picked != picked || f.settings["colour_wheel"] != picked ||
		accent.GetForeground() != lipgloss.Color(picked) {
		t.Fatalf("enter: picked %s, saved %v, accent %v", m.prefs.picked, f.settings, accent.GetForeground())
	}
	sees(t, m, picked)
	// The wheel opens on the colour that is there.
	m = press(m, "enter")
	if got := m.colourPick.hex(); got != picked {
		near := rgbOf(got)
		want := rgbOf(picked)
		for i := range near {
			if d := near[i] - want[i]; d > 30 || d < -30 {
				t.Fatalf("it opened on %s, not %s", got, picked)
			}
		}
	}
}

func TestTheWheelsKeys(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "right", "enter", "down", "enter")
	c := m.colourPick
	hue := c.hue
	x, y := c.x, c.y
	m = press(m, ".")
	if c.hue != (hue+1)%360 {
		t.Fatalf("`.` did not turn the hue: %d to %d", hue, c.hue)
	}
	m = press(m, ",", ",")
	if c.hue != (hue+359)%360 {
		t.Fatalf("`,` did not turn it back: %d", c.hue)
	}
	m = press(m, "up")
	if c.y != y-1 && y > 0 {
		t.Fatalf("up: %d to %d", y, c.y)
	}
	m = press(m, "tab") // to the strip: the arrows turn the hue
	before := c.hue
	m = press(m, "right")
	if !c.strip || c.hue != (before+5)%360 || c.x != x {
		t.Fatalf("on the strip: strip %v, hue %d to %d, x %d to %d", c.strip, before, c.hue, x, c.x)
	}
	m = press(m, "tab")
	if c.strip {
		t.Fatal("tab did not go back to the square")
	}
	// The screens' numbers are not the wheel's: it takes no key that leaves it.
	m = press(m, "1", "2", "[", "]")
	if m.colourPick == nil || m.screen != settingsScreen {
		t.Fatalf("a key left the wheel: open %v, screen %d", m.colourPick != nil, m.screen)
	}
}

func TestTheWheelSaysWhenTheTerminalIsNotTrueColour(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "right", "enter", "down", "enter")
	t.Setenv("COLORTERM", "")
	sees(t, m, "does not say it draws 24-bit colour")
	t.Setenv("COLORTERM", "truecolor")
	lacks(t, m, "24-bit colour, so")
}

func TestASavedColourModeComesBackAndIsAppliedAtStart(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	f.settings = map[string]string{"colour_mode": "theme", "colour_theme": "magenta"}
	m, _ := start(t, f)
	if m.prefs.highlight() != "magenta" || accent.GetForeground() != lipgloss.Color("5") {
		t.Fatalf("theme: %q, accent %v", m.prefs.highlight(), accent.GetForeground())
	}
	f.settings = map[string]string{
		"colour_mode":          "custom",
		"colour_custom":        `[{"name":"Sunset","value":"#FF8800"},{"name":"Sky","value":"0af"}]`,
		"colour_custom_choice": "Sky",
	}
	m, _ = start(t, f)
	if m.prefs.highlight() != "#00aaff" || accent.GetForeground() != lipgloss.Color("#00aaff") {
		t.Fatalf("custom: %q, accent %v", m.prefs.highlight(), accent.GetForeground())
	}
	f.settings = map[string]string{"colour_mode": "wheel", "colour_wheel": "#123456"}
	m, _ = start(t, f)
	if m.prefs.highlight() != "#123456" || accent.GetForeground() != lipgloss.Color("#123456") {
		t.Fatalf("wheel: %q, accent %v", m.prefs.highlight(), accent.GetForeground())
	}
	// Nothing readable is the default.
	f.settings = map[string]string{"colour_mode": "sparkly", "colour_theme": "chartreuse"}
	m, _ = start(t, f)
	if m.prefs.highlight() != "cyan" || accent.GetForeground() != lipgloss.Color("6") {
		t.Fatalf("unreadable: %q, accent %v", m.prefs.highlight(), accent.GetForeground())
	}
}

func TestTheColourReachesTheLibraryAndItsForms(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	twoCards(f)
	m, _ := start(t, f)
	applyTheme("green")
	m = press(m, "2", "right", "e", "enter")
	if barOn.GetBackground() != lipgloss.Color("2") || boxOn.GetBackground() != lipgloss.Color("2") {
		t.Fatal("the bar and the box being typed in are not green")
	}
	sees(t, m, "Edit card")
}

func TestTheColourFormTakesTheCardsPlaceNotTheTabsAndTheKeysAreItsOwn(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "enter", "down", "down", "enter")
	lacks(t, m, "Editing") // the settings tabs are not shown over it
	m = press(m, "left", "right", "1", "2", "3", "[", "]")
	if m.form == nil || m.screen != settingsScreen || m.prefTab != 2 {
		t.Fatalf("a key left the form: form %v, screen %d, tab %d", m.form != nil, m.screen, m.prefTab)
	}
	m = press(m, "esc")
	sees(t, m, "Editing", "Add a colour…")
}

func TestASelectedColourRowIsPlainSoTheBarIsUnbrokenAndAnUnselectedOneHasASwatch(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "enter") // custom mode
	m = press(m, "down", "down")
	m = addColour(m, "#ff8800", "Sunset")
	m = press(m, "up", "up") // the colour row
	for name, mode := range map[string]int{"theme": themeMode, "custom": customMode, "wheel": wheelMode} {
		m.prefs.colourMode = mode
		plain, styled := m.colourValue(true), m.colourValue(false)
		if strings.Contains(plain, "██") || strings.ContainsRune(plain, '\x1b') {
			t.Errorf("%s, selected: %q has a swatch or a style in it", name, plain)
		}
		if !strings.Contains(styled, "██") {
			t.Errorf("%s, not selected: %q has no swatch", name, styled)
		}
		// Selected it has an arrow either side (four cells) where it can be turned, and
		// otherwise it takes the room it did: the swatch is blank, not gone.
		arrows := map[string]int{"theme": 4, "custom": 4, "wheel": 0}[name]
		if cells(plain) != cells(styled)+arrows {
			t.Errorf("%s: %d cells selected and %d not, so it moves", name, cells(plain), cells(styled))
		}
	}
}

func TestTheWheelsCursorMovesWithEveryKeyAndNotEveryOther(t *testing.T) {
	defer applyTheme(defaultColour)
	f := newFake()
	m := visual(t, f)
	m = press(m, "enter", "right", "right", "enter", "down", "enter")
	c := m.colourPick
	seen := map[[2]int]int{}
	for range 6 {
		row, half := c.cursorCell()
		seen[[2]int{row, half}]++
		m = press(m, "down")
	}
	if len(seen) != 6 {
		t.Fatalf("six presses down visited %d places, not six: %v", len(seen), seen)
	}
}

func TestTheHintsPutTheValuesTheyNameInSpeechMarks(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "3", "down")
	sees(t, m, `"∞"`)
	m = press(m, "down")
	sees(t, m, `"auto"`, `"off"`)
}
