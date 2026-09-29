package engine

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

// realEngine points LERNT_ENGINE at the Python engine in this repository, the
// way `lernt` hands it over, with the example decks and a throwaway state dir.
func realEngine(t *testing.T) []string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	argv := []string{python, "-m", "lernt.deck"}
	raw, _ := json.Marshal(argv)
	t.Setenv("LERNT_ENGINE", string(raw))
	t.Setenv("PYTHONPATH", repo)
	t.Setenv("LERNT_DECKS", filepath.Join(repo, "examples", "decks"))
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return argv
}

// TestTheContract holds every type in api.go to the schema the engine derives
// from lernt/api.py: the same methods, and for each, the same fields with the
// same JSON types and the same nullability, all the way down.
func TestTheContract(t *testing.T) {
	argv := realEngine(t)
	out, err := exec.Command(argv[0], append(argv[1:], "--schema")...).Output()
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Protocol string `json:"protocol"`
		Methods  map[string]struct {
			Params map[string]any `json:"params"`
			Result map[string]any `json:"result"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Protocol != Protocol {
		t.Errorf("the engine speaks %s, this client %s", schema.Protocol, Protocol)
	}

	var engineSide, goSide []string
	for name := range schema.Methods {
		engineSide = append(engineSide, name)
	}
	for name := range Methods {
		goSide = append(goSide, name)
	}
	sort.Strings(engineSide)
	sort.Strings(goSide)
	if !slices.Equal(engineSide, goSide) {
		t.Fatalf("methods differ:\n engine %v\n     go %v", engineSide, goSide)
	}

	for name, method := range schema.Methods {
		match(t, name+" params", method.Params, reflect.TypeOf(Methods[name].Params))
		match(t, name+" result", method.Result, reflect.TypeOf(Methods[name].Result))
	}
}

func match(t *testing.T, where string, node map[string]any, typ reflect.Type) {
	t.Helper()
	if anyOf, ok := node["anyOf"].([]any); ok {
		if typ.Kind() != reflect.Pointer {
			t.Errorf("%s: the engine allows null; Go has %s, not a pointer", where, typ)
			return
		}
		for _, option := range anyOf {
			if option := option.(map[string]any); option["type"] != "null" {
				match(t, where, option, typ.Elem())
			}
		}
		return
	}
	if typ.Kind() == reflect.Pointer {
		t.Errorf("%s: Go has %s, but the engine never sends null", where, typ)
		return
	}

	want := map[string]reflect.Kind{
		"string": reflect.String, "integer": reflect.Int, "boolean": reflect.Bool,
		"array": reflect.Slice, "object": reflect.Struct,
	}[fmt.Sprint(node["type"])]
	if typ.Kind() != want {
		t.Errorf("%s: the engine has %v, Go has %s", where, node["type"], typ)
		return
	}
	switch want {
	case reflect.Slice:
		match(t, where+"[]", node["items"].(map[string]any), typ.Elem())
	case reflect.Struct:
		properties := node["properties"].(map[string]any)
		fields := map[string]reflect.Type{}
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			fields[tag] = typ.Field(i).Type
		}
		for name, property := range properties {
			field, ok := fields[name]
			if !ok {
				t.Errorf("%s: %s.%s is missing in Go", where, typ.Name(), name)
				continue
			}
			match(t, where+"."+name, property.(map[string]any), field)
		}
		for name := range fields {
			if _, ok := properties[name]; !ok {
				t.Errorf("%s: %s.%s is not in the engine's API", where, typ.Name(), name)
			}
		}
	}
}

// TestARunAgainstTheRealEngine is the seam as it really is, Python and all.
func TestARunAgainstTheRealEngine(t *testing.T) {
	realEngine(t)
	c, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	decks, err := c.Decks()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(decks.Decks, func(d DeckInfo) bool {
		return d.ID == "capitals" && d.Name == "European capitals"
	}) {
		t.Fatalf("capitals not listed: %+v", decks)
	}

	count := 2
	started, err := c.Start(StartParams{Deck: "capitals", Count: &count, Log: true})
	if err != nil {
		t.Fatal(err)
	}
	if started.Card.Number != 1 || len(started.Card.Answer) == 0 {
		t.Fatalf("started %+v", started)
	}
	if _, err := c.Score(ScoreParams{Score: "fail", Seconds: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Next(NextParams{Pick: PickAuto}); err != nil {
		t.Fatal(err)
	}
	scored, err := c.Score(ScoreParams{Score: "pass", Seconds: 2})
	if err != nil || !scored.Finished {
		t.Fatalf("scored %+v, err %v", scored, err)
	}
	summary, err := c.End()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Asked != 2 || summary.Logged != 2 || summary.EndedEarly {
		t.Fatalf("summary %+v", summary)
	}

	misses, err := c.Misses(MissParams{Deck: "capitals"})
	if err != nil || len(misses.Records) != 1 {
		t.Fatalf("misses %+v, err %v", misses, err)
	}
	if _, err := c.Score(ScoreParams{Score: "pass"}); Code(err) != NotNow {
		t.Fatalf("scoring with no run open: %v", err)
	}
}

// TestAuthoringAgainstTheRealEngine is a deck and a card made, listed, checked
// and deleted the way the Library does it. The fake the screens are tested with
// cannot say what the wire will accept: an empty Go slice, say, is JSON null.
func TestAuthoringAgainstTheRealEngine(t *testing.T) {
	realEngine(t)
	decks := t.TempDir()
	t.Setenv("LERNT_DECKS", decks)
	c, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	deck, err := c.CreateDeck(NewDeckParams{Name: "Rivers"})
	if err != nil || deck.ID != "rivers" {
		t.Fatalf("deck %+v, err %v", deck, err)
	}
	card, err := c.CreateCard(NewCardParams{Deck: "rivers", Title: "What is a delta?"})
	if err != nil {
		t.Fatalf("a card with no follow-ups: %v", err)
	}
	listed, err := c.ListCards(DeckParams{Deck: "rivers"})
	if err != nil || len(listed.Cards) != 1 || listed.Cards[0].Path != card.Path {
		t.Fatalf("listed %+v, err %v", listed, err)
	}
	ten := 10
	if _, err := c.UpdateCard(UpdateCardParams{Deck: "rivers", Card: card.ID, Title: "What is a delta?",
		Answer: "Where a river meets the sea.", Time: &ten}); err != nil {
		t.Fatalf("updating a card with no follow-ups: %v", err)
	}
	listed, err = c.ListCards(DeckParams{Deck: "rivers"})
	if err != nil || len(listed.Cards) != 1 || listed.Cards[0].Time == nil || *listed.Cards[0].Time != 10 ||
		!slices.Equal(listed.Cards[0].Answer, []string{"Where a river meets the sea."}) {
		t.Fatalf("after the update %+v, err %v", listed, err)
	}
	if _, err := c.UpdateDeck(UpdateDeckParams{Deck: "rivers", Name: "Rivers of the world"}); err != nil {
		t.Fatal(err)
	}
	if renamed, err := c.Decks(); err != nil || renamed.Decks[0].Name != "Rivers of the world" {
		t.Fatalf("decks after the rename %+v, err %v", renamed, err)
	}
	if _, err := c.CreateCard(NewCardParams{Deck: "rivers", Title: "Which?", Follows: []string{"Nope"}}); Code(err) != Invalid {
		t.Fatalf("a follow-up that is not a card: %v", err)
	}
	problems, err := c.CheckDeck(DeckParams{Deck: "rivers"})
	if err != nil || len(problems.Problems) != 0 {
		t.Fatalf("problems %+v, err %v", problems, err)
	}
	if _, err := c.DeleteCard(DeleteCardParams{Deck: "rivers", Card: card.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteDeck(DeleteDeckParams{Deck: "rivers"}); err != nil {
		t.Fatal(err)
	}
	after, err := c.Decks()
	if err != nil || len(after.Decks) != 0 {
		t.Fatalf("decks after %+v, err %v", after, err)
	}
}
