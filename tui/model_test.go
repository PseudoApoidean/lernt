package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	problems []string
	opened   []string // the paths handed to the editor
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
	f.newCard = p
	return &engine.NewCard{ID: "Q", Path: "/decks/" + p.Deck + "/Q.md"}, nil
}

func (f *fake) CheckDeck(p engine.DeckParams) (*engine.Problems, error) {
	f.record("check")
	return &engine.Problems{Deck: p.Deck, Problems: f.problems}, nil
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
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

func typeText(m model, text string) model {
	for _, r := range text {
		if r == ' ' {
			m = press(m, "space")
		} else {
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

func TestTheDeckCardSaysWhereTheDeckStands(t *testing.T) {
	f := newFake()
	f.decks.Decks[0].Stats = &engine.DeckStats{Size: 125, New: 95, Boxes: []int{29, 1, 0, 0, 0},
		Due: 30, Missed: 29, LastRun: "2026-09-15"}
	m, c := start(t, f)
	c.t = time.Date(2026, 9, 21, 20, 0, 0, 0, time.Local)
	sees(t, m, "125 cards", "29 on the miss list", "new ", " 95", "1   ", " 29",
		"3-5 ·", "last run 6 days ago")
	lacks(t, m, "due today") // a Leitner idea: classic mode does not say
	m = press(m, "tab")
	sees(t, m, "30 due today", "30 due")

	f.decks.Decks[0].Stats = &engine.DeckStats{Size: 1, New: 0, Boxes: []int{0, 0, 1}, LastRun: "2026-09-21"}
	m = press(m, "r")
	sees(t, m, "1 card", "nothing due today", "miss list clear", "2   ·", "3   ", "last run today")
	lacks(t, m, "1-2") // only empty boxes at the top end share a row
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
	m = press(m, "down", "s")
	sees(t, m, "Settings", "Mode", "Cards", "Seconds", "every deck")
	if m.screen != settingsScreen {
		t.Fatalf("on screen %d", m.screen)
	}

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

	m = press(m, "esc")
	if m.screen != decksScreen || m.cursor != 1 {
		t.Fatalf("back on screen %d, cursor %d", m.screen, m.cursor)
	}
}

func TestTheSettingsAreTheDefaultsOfADeckWithNoneOfItsOwn(t *testing.T) {
	f := newFake()
	f.decks.Decks = append(f.decks.Decks, engine.DeckInfo{ID: "plain", Name: "Plain"})
	m, _ := start(t, f)
	m = press(m, "s", "down", "right", "right", "down", "right", "right")
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
	m = press(m, "s")
	sees(t, m, "Settings")
	m = press(m, "esc")
	sees(t, m, "No decks in")
}

func typed(m model, text string) model {
	for _, r := range text {
		if r == ' ' {
			m = press(m, "space")
		} else {
			m = press(m, string(r))
		}
	}
	return m
}

func TestANewDeckIsNamedDescribedAndSelected(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "N")
	sees(t, m, "New deck's name")
	m = press(m, "enter")
	sees(t, m, "New deck's name") // a deck needs a name
	m = typed(m, "Rivers")
	m = press(m, "enter")
	sees(t, m, "What it is about")
	m = typed(m, "Long ones")
	m = press(m, "enter")
	if f.newDeck.Name != "Rivers" || f.newDeck.Description != "Long ones" {
		t.Fatalf("asked for %+v", f.newDeck)
	}
	sees(t, m, "Made the deck rivers", "n adds a card")
	if d, _ := m.selected(); d.ID != "rivers" {
		t.Fatalf("the cursor is on %q, not the new deck", d.ID)
	}
	calls(t, f, "settings", "decks", "create deck", "decks")
}

func TestANewDeckCanBeCancelled(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "N")
	m = typed(m, "Rivers")
	m = press(m, "esc")
	calls(t, f, "settings", "decks")
	lacks(t, m, "New deck's name")
}

func TestANewCardIsWrittenThenOpenedInTheEditorThenChecked(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "n")
	sees(t, m, "New card's question")
	m = typed(m, "What is a delta?")
	m = press(m, "enter")
	if f.newCard.Deck != "capitals" || f.newCard.Title != "What is a delta?" {
		t.Fatalf("asked for %+v", f.newCard)
	}
	if !slices.Equal(f.opened, []string{"/decks/capitals/Q.md"}) {
		t.Fatalf("the editor was given %v", f.opened)
	}
	sees(t, m, "Made the card Q.")
	lacks(t, m, "problem")
	calls(t, f, "settings", "decks", "create card", "check", "decks")
	if m.authoring != nil {
		t.Fatal("still authoring")
	}
}

func TestWhatTheEditorBrokeIsSaid(t *testing.T) {
	f := newFake()
	f.problems = []string{"Q: follow-up points at a missing card: Gone"}
	m, _ := start(t, f)
	m = press(m, "n")
	m = typed(m, "A question")
	m = press(m, "enter")
	sees(t, m, "1 problem", "missing card: Gone")
}

func TestAnEditorThatFailsLeavesTheCardAndSaysWhere(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m.exec = func(path string) tea.Cmd {
		return func() tea.Msg { return editorDone{err: &engine.Error{Message: "exit status 1"}} }
	}
	m = press(m, "n")
	m = typed(m, "A question")
	m = press(m, "enter")
	sees(t, m, "The editor failed", "/decks/capitals/Q.md")
	calls(t, f, "settings", "decks", "create card", "decks")
}

func TestANewCardNeedsADeckAndAQuestion(t *testing.T) {
	f := newFake()
	m, _ := start(t, f)
	m = press(m, "n", "enter")
	sees(t, m, "New card's question")
	calls(t, f, "settings", "decks")
	m = press(m, "esc", "down", "n")
	sees(t, m, "Choose a deck")
	calls(t, f, "settings", "decks")
}
