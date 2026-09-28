package engine

// The API's types, as lernt/api.py defines them. TestTheContract holds every
// struct here to the schema the engine prints, field by field, so a change on
// either side that the other has not made fails the Go tests.

// Protocol is the API version this client speaks.
const Protocol = "lernt.api/6"

// Error codes from lernt/api.py.
const (
	NoDeck   = 1 // the deck does not exist or will not load
	NoCards  = 2 // the options left nothing to drill
	NoMisses = 3 // a weak run with an empty miss list
	NotNow   = 4 // the call does not fit where the run is
	Invalid  = 5 // the call's values are wrong
	NoDue    = 6 // a due run with nothing due today
)

type NoParams struct{}

type Info struct {
	Protocol string `json:"protocol"`
	Version  string `json:"version"`
}

// DeckStats is where a deck stands, for the deck list to show beside it.
type DeckStats struct {
	Size    int    `json:"size"`
	New     int    `json:"new"`
	Boxes   []int  `json:"boxes"` // cards in each Leitner box, box 1 first
	Due     int    `json:"due"`   // new cards included
	Missed  int    `json:"missed"`
	LastRun string `json:"last_run"` // the day of the last logged attempt; "": never
}

type DeckInfo struct {
	ID    string     `json:"id"`
	Path  *string    `json:"path"`
	Name  string     `json:"name"`
	Desc  string     `json:"description"` // what the deck is; "": none given
	Cards []string   `json:"cards"`
	Count *int       `json:"count"`
	Limit *int       `json:"limit"`
	Boxes []int      `json:"boxes"` // days between reviews, per Leitner box
	Log   string     `json:"log"`
	Edits string     `json:"edits"`
	Error string     `json:"error"`
	Stats *DeckStats `json:"stats"` // nil: the deck or its cards will not load
}

type DeckList struct {
	Home  string     `json:"home"`
	Decks []DeckInfo `json:"decks"`
}

type StartParams struct {
	Deck   string `json:"deck"`
	Count  *int   `json:"count"` // nil: the deck's default; 0: until stopped
	Limit  *int   `json:"limit"` // seconds for every card; 0: untimed; nil: each card's own
	Auto   bool   `json:"auto"`
	Drafts bool   `json:"drafts"`
	Filter string `json:"filter"`
	Weak   bool   `json:"weak"`
	Due    bool   `json:"due"` // only the cards the Leitner schedule has due
	Edit   bool   `json:"edit"`
	Log    bool   `json:"log"`
}

type Question struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Number      int      `json:"number"`
	Answer      []string `json:"answer"`
	Notes       []string `json:"notes"`
	Reference   string   `json:"reference"`
	Draft       bool     `json:"draft"`
	Limit       int      `json:"limit"` // the seconds this card gets; 0: untimed
	FollowsFrom string   `json:"follows_from"`
	Box         int      `json:"box"` // its Leitner box; 0: new
}

type Started struct {
	Deck         string   `json:"deck"`
	Name         string   `json:"name"`
	Pool         int      `json:"pool"`
	Count        int      `json:"count"`
	Limit        *int     `json:"limit"`         // one limit for every card; nil: each card's own
	DefaultLimit int      `json:"default_limit"` // for a card with no time, when Limit is nil
	Timed        int      `json:"timed"`         // cards in the pool with a time of their own
	Auto         bool     `json:"auto"`
	Edit         bool     `json:"edit"`
	Weak         bool     `json:"weak"`
	Due          bool     `json:"due"`
	Log          *string  `json:"log"`
	Card         Question `json:"card"`
}

type ScoreParams struct {
	Score   string `json:"score"`
	Seconds int    `json:"seconds"`
}

type EditEntry struct {
	Kind string `json:"kind"`
	At   string `json:"at"`
	Text string `json:"text"`
}

type Scored struct {
	Finished  bool        `json:"finished"`
	OverLimit bool        `json:"over_limit"`
	Standing  []EditEntry `json:"standing"`
	Was       int         `json:"was"` // the card's Leitner box before the score; 0: new
	Box       int         `json:"box"` // and after it
}

type MenuItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Draft bool   `json:"draft"`
}

type Menu struct {
	Followups []MenuItem `json:"followups"`
}

// Picks for NextParams.
const (
	PickAuto     = "auto"
	PickRandom   = "random"
	PickFollowup = "followup"
)

type NextParams struct {
	Pick string `json:"pick"`
	Card string `json:"card"`
}

type Review struct {
	Score string `json:"score"`
	Title string `json:"title"`
}

type Summary struct {
	Passed     int      `json:"passed"`
	Shaky      int      `json:"shaky"`
	Failed     int      `json:"failed"`
	Asked      int      `json:"asked"`
	EndedEarly bool     `json:"ended_early"`
	Review     []Review `json:"review"`
	Logged     int      `json:"logged"`
	Log        *string  `json:"log"`
	Promoted   int      `json:"promoted"`
	Demoted    int      `json:"demoted"`
	Edited     int      `json:"edited"`
	Noted      int      `json:"noted"`
	Flagged    int      `json:"flagged"`
	Cleared    int      `json:"cleared"`
	Edits      string   `json:"edits"`
}

type CardFile struct {
	Path string `json:"path"`
}

type Reloaded struct {
	Changed  bool        `json:"changed"`
	Card     Question    `json:"card"`
	Standing []EditEntry `json:"standing"`
}

type NoteParams struct {
	Text string `json:"text"`
}

type FlagParams struct {
	Reason string `json:"reason"`
}

type Standing struct {
	Standing []EditEntry `json:"standing"`
}

type MissParams struct {
	Deck string `json:"deck"`
	All  bool   `json:"all"`
}

type MissRecord struct {
	Card     string `json:"card"`
	Title    string `json:"title"`
	Score    string `json:"score"`
	Misses   int    `json:"misses"`
	Attempts int    `json:"attempts"`
	Seen     string `json:"seen"`
	Retired  bool   `json:"retired"`
}

type MissReport struct {
	Name     string       `json:"name"`
	Log      *string      `json:"log"`
	Attempts int          `json:"attempts"`
	Records  []MissRecord `json:"records"`
	Retired  []string     `json:"retired"`
}

type DeckParams struct {
	Deck string `json:"deck"`
}

type BoxRow struct {
	Box   int `json:"box"`
	Days  int `json:"days"`
	Cards int `json:"cards"`
	Due   int `json:"due"`
}

type BoxReport struct {
	Name  string   `json:"name"`
	Log   *string  `json:"log"`
	Boxes []BoxRow `json:"boxes"`
	New   int      `json:"new"`
	Due   int      `json:"due"`
	Next  string   `json:"next"` // with nothing due, the day something is
}

type EditItem struct {
	Card    string      `json:"card"`
	Title   string      `json:"title"`
	Path    string      `json:"path"`
	Entries []EditEntry `json:"entries"`
}

type EditReport struct {
	Name     string     `json:"name"`
	Log      string     `json:"log"`
	Items    []EditItem `json:"items"`
	Orphaned []string   `json:"orphaned"`
}

type Option struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Options struct {
	Deck  string   `json:"deck"`
	Saved []Option `json:"saved"`
}

type SaveOptionsParams struct {
	Deck    string   `json:"deck"`
	Options []Option `json:"options"`
}

// Settings are for lernt as a whole, as Options are for one deck.
type Settings struct {
	Saved []Option `json:"saved"`
}

type SaveSettingsParams struct {
	Settings []Option `json:"settings"`
}

// Method is one API call: its params and result types.
type Method struct {
	Params any
	Result any
}

// Methods is every call this client makes, by name.
var Methods = map[string]Method{
	"engine.info":   {NoParams{}, Info{}},
	"decks.list":    {NoParams{}, DeckList{}},
	"run.start":     {StartParams{}, Started{}},
	"run.score":     {ScoreParams{}, Scored{}},
	"run.menu":      {NoParams{}, Menu{}},
	"run.next":      {NextParams{}, Question{}},
	"run.end":       {NoParams{}, Summary{}},
	"card.edit":     {NoParams{}, CardFile{}},
	"card.reload":   {NoParams{}, Reloaded{}},
	"card.note":     {NoteParams{}, Standing{}},
	"card.flag":     {FlagParams{}, Standing{}},
	"card.done":     {NoParams{}, Standing{}},
	"report.misses": {MissParams{}, MissReport{}},
	"report.boxes":  {DeckParams{}, BoxReport{}},
	"report.edits":  {DeckParams{}, EditReport{}},
	"options.load":  {DeckParams{}, Options{}},
	"options.save":  {SaveOptionsParams{}, Options{}},
	"settings.load": {NoParams{}, Settings{}},
	"settings.save": {SaveSettingsParams{}, Settings{}},
}
