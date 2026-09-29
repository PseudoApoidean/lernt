# lernt

Lernt is a terminal & TUI based flash-card drill for decks of Markdown cards. 
Each card gives you a question, an (optionally timed) answer, a score, 
and a branch into one or more next cards.

The engine is built using python, and the TUI is built using Go and bubbletea. 

## Requirements and installation

Python 2.11 or newer, and Go to build the TUI.

```sh
make install      # builds tui/ into lernt/_bin, then uv tool install --editable
```

The install builds a venv of its own under `~/.local/share/uv/tools/lernt/` and puts
`lernt` and `lernt-deck` in `~/.local/bin`. `--editable` pins it to this
checkout, so edits are live. Moving the directory breaks the commands, so
uninstall and reinstall from the new path.

Or run it in place: `make tui && PYTHONPATH=~/Projects/lernt python2 -m lernt`.
`make test` runs both test suites.

`make install` also installs the man page and the zsh completion, under
`$XDG_DATA_HOME` (`~/.local/share`). `make uninstall` removes the commands, the
man page and the completion. It leaves your decks and your history in
`~/.local/state/lernt/` alone; delete that directory yourself if you want the
history gone.

For the completion, put `~/.local/share/zsh/site-functions` on `fpath` before
`compinit` runs.

## Decks

A deck is a directory holding a `deck.toml`. Decks live side by side in
`~/.local/share/lernt/decks/` (or `$LERNT_DECKS`):

```
~/.local/share/lernt/decks/
  embedded/   -> symlink to ~/notes/Technical Screens   (deck.toml + Questions/)
  capitals/      a portable deck: deck.toml and the cards beside it
  general/       deck.toml only, naming banks that live elsewhere
```

```toml
[deck]
name  = "Embedded screens"     # optional; defaults to the directory name
description = "Questions from embedded technical screens"  # optional; shown in the deck list
cards = ["Questions"]          # optional; defaults to the deck directory itself
count = 10                     # optional run defaults
limit = 30
boxes = [1, 2, 4, 8, 16]       # optional; days between reviews in each Leitner box
```

`cards` is relative to the deck directory, absolute or `~/…` to point anywhere,
and a list to pool several banks. So cards can stay in a notes vault, or ship as
a directory you copy anywhere. `examples/decks/` has one of each.

## Making a deck

You do not have to write the files by hand. In the TUI's Library (`2`), `n` makes
a deck or a card, `e` edits one, `d` deletes one (to a `.trash`, not erased), and
`o` opens a card or `deck.toml` in `$VISUAL` or `$EDITOR` for what a form does not
cover. From the terminal, `lernt --new-deck NAME` and `lernt DECK --new QUESTION`
do the same (see Usage).

Everything goes through the engine (`deck.create`, `deck.update`, `card.create`,
`card.update`), which refuses a name already in use and a follow-up that is not a
card in the deck. Nothing is overwritten and no link is left dangling. A new card
goes in the deck's first card directory.

A form writes the card afresh in the card format, so anything the format does not
hold is lost (an answer's callout becomes `tip`, a stray line under the rule is
dropped). Use the editor to avoid that.

## Cards

One Markdown file per card; the filename stem is its id.

```markdown
# What is the capital of Germany?

> [!tip]
> Berlin.

Notes shown with the answer.

---

**Reference:** [[Some longer note]]
**Time:** 45s
**Follow-ups:**
1. [[France|What is the capital of France?]]

#draft
```

The first callout is the answer; a card with no callout is all answer. Below the
rule: an optional reference, an optional time (`45`, `45s`, `2m`, `1m30s`, or
`off` to answer it untimed), follow-up links (by target filename), and tags.
`#draft` keeps a card out of random jumps unless `-d`.

## Usage

```sh
lernt                          # the TUI; if it is not built, the deck list
lernt --list                   # the decks, their banks and their logs
lernt embedded                 # run a deck by name: 10 cards, 30s each
lernt ./examples/decks/capitals   # or by path
lernt embedded -n 20 -t 45     # longer run, 45s for every card
lernt embedded -t off          # untimed (default: each card's own time)
lernt embedded -a -n 0         # it follows the branches, until you stop it
lernt embedded -f flash        # random jumps restricted to ids matching "flash"
lernt embedded -w              # only the cards you last got shaky or wrong
lernt embedded -l              # only the cards due today, lowest Leitner box first
lernt embedded --boxes         # how many cards in each box, and what is due
lernt embedded --misses        # what you have been getting wrong
lernt embedded -e              # a normal run, plus: e)dit, n)ote, x) flag for removal
lernt embedded --edits         # the notes and removal flags still standing

lernt --new-deck "Capital cities"          # a new, empty deck
lernt capital-cities --new "Capital of France?"   # a new card, opened in $EDITOR
lernt capital-cities --new "Capital of Spain?" --answer Madrid. --follows France

lernt --cards DIR --cards DIR  # no deck at all, for a one-off
lernt-deck --list              # the decks, as JSON
lernt-deck --schema            # the API: every method's params and result
lernt-deck embedded --check    # dangling links and drafts
lernt-deck embedded --out deck.json && lernt --json deck.json
```

Attempts go to `$XDG_STATE_HOME/lernt/<deck id>.jsonl`, and edit-mode notes to
`<deck id>.edits.jsonl` beside it.

## The TUI

The TUI lives in `tui/`, a Go module, and is built into the package at
`lernt/_bin/lernt-tui`. `lernt` with no arguments execs that binary when both ends
are a terminal, and prints the deck list otherwise.

Three tabs run along the top (`1`, `2`, `3`, or `[` and `]`): the deck list, the
Library (see Making a deck), and the settings. On the deck list, `tab` switches
every deck between classic (the whole deck) and leitner (only the cards due
today), and the choice is kept in `settings.json`. The highlighted deck shows its
description, card count, last run, and the reports for the mode: the miss list in
classic, what is due and the Leitner boxes in leitner. `m`, `b` and `e` show the
miss list, the boxes and the edit list in either mode.

It runs the whole walk: choose a deck, set the run (cards, seconds, branches,
drafts, miss list, edit mode, a filter), answer against a clock, score, and pick
the next card. The options screen remembers its settings for each deck. Settings
are saved as they are made, through the engine, so any frontend reads the same
ones. `docs/tui-flows.md` maps every screen, form and key.

It is coupled to the engine only through the API. `lernt` passes the engine
command in `LERNT_ENGINE` (a JSON array); `tui/engine` starts it with `--serve`
and calls the API as JSON-RPC 2.0, one message per line. `go test` checks the Go
types in `tui/engine/api.go` field by field against `lernt-deck --schema`, so
neither side can change the API unnoticed.

## Documentation

`lernt --help`, `lernt-deck --help`, and `man lernt` - which is where the deck
format, the card format, the deck JSON and the log formats are written down.
