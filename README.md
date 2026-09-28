# lernt

Lernt is a flash-card drill for decks of Markdown cards: a question, a timed answer, the
card's answer, a score, and a branch into the next card.

It is an **engine with an API, and frontends over it**. The engine (Python)
reads a deck's cards and owns every rule of a run - what comes next, what is
logged, what a miss is - behind a typed API (`lernt/api.py`). `lernt DECK` drives
that API in-process as a walk in the terminal. The TUI, which ships in this
repository (`tui/`, Go and bubbletea), drives the same API over JSON-RPC, and
`lernt` on its own hands straight over to it.

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

## Leitner boxes

Every card sits in a Leitner box, and box *n* comes due `boxes[n-1]` days after
the card last moved. `lernt DECK -l` drills only what is due today, lowest box
first, and `lernt DECK --boxes` shows where the deck stands.

- A new card is due. A pass on a card that is due moves it up a box.
- A pass on a card that is *not* due changes nothing, so re-running the miss list
  all afternoon cannot rush a card up the boxes.
- A fail sends it back to box 1, due or not - from any run, not only `-l`.
- A shaky keeps it in its box and starts its wait again.

Boxes are never stored. They are replayed from the attempt log, like the miss
list, so a history logged before the schedule existed already has boxes, and
changing `boxes` re-reads every card under the new schedule.

## Design principles

- **The engine knows no paths and no subject.** Nothing is built in: a deck says
  where its cards are, and a card is question, answer, notes and links.
- **Nothing is picked for you.** `lernt` with a flag but no deck asks for one,
  because a run logs against the deck it ran.
- **Each deck logs separately, under its id** - the directory name, not the
  title - so retitling a deck keeps its history.
- **The engine never touches a terminal, and the frontend never parses
  Markdown.** Cards come out as the Markdown they were written in, links
  resolved to card ids.
- **Frontends call the API; they never re-implement a run.** The terminal
  walk and the TUI make the same calls, so they cannot disagree about one.
- **The walk is data, not a shuffle.** A card names where to go next, and `-a`
  follows one line until it is exhausted before starting a fresh one.
- **The attempt log is append-only and keyed by card id**, never by position.
  Adding or reordering cards cannot corrupt a history; renaming one orphans
  exactly that card, and the log still holds the title it was asked under.
- **A miss is the *last* score, not a running tally.** Answer a card well once
  and it leaves the re-run list, which is what `-w` drills.
- **A run is where a bad card shows itself, so that is where it gets noted.**
  `-e` adds a step after each score: open the card in `$EDITOR`, leave a note,
  or flag it for removal.

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
`lernt/_bin/lernt-tui`. `lernt` with no arguments execs that binary - one path,
no search - when both ends are a terminal, and prints the deck list otherwise.

The deck list is where the mode is chosen, for every deck at once: `tab`
switches between classic (the whole deck) and leitner (only the cards due
today), and the TUI remembers it in `settings.json`. The highlighted deck shows
beside the list: its description, its card count, what is on the miss list, its
cards by Leitner box, and when it last ran. In leitner mode the list shows what
each deck has due, and the highlighted deck says what is due today.

It runs the whole walk: choose a deck, set the run (cards, seconds, follow
branches for me, drafts, only the miss list - classic only - edit mode, a
filter), answer against a clock, score, pick the next card, and in edit mode
open the card in `$EDITOR`, note it or flag it. `m`, `b` and `e` on the deck
list show the miss list, the Leitner boxes and the edit list. The options screen
remembers what it was last set to for each deck, by option name, and a deck
never run from it starts from its `deck.toml` defaults. Beside the options, a
help card explains the one the cursor is on, at its current value.
`docs/tui-flows.md` maps every screen and the keys between them.

It is a view, coupled to the engine only through the API. `lernt` passes the
engine command in `LERNT_ENGINE` (a JSON array); `tui/engine` starts it with
`--serve` and calls the API as JSON-RPC 2.0, one message per line. The Go types
in `tui/engine/api.go` are checked field by field against `lernt-deck --schema`
in `go test`, so neither side can change the API without the other noticing.

## Requirements and installation

Python 3.11 or newer, and Go to build the TUI.

```sh
make install      # builds tui/ into lernt/_bin, then uv tool install --editable
```

The install builds a venv of its own under `~/.local/share/uv/tools/lernt/` and puts
`lernt` and `lernt-deck` in `~/.local/bin`. `--editable` pins it to this
checkout, so edits are live - but moving the directory breaks the commands;
uninstall and reinstall from the new path. `uv tool uninstall lernt` removes
both.

Or run it in place: `make tui && PYTHONPATH=~/Projects/lernt python3 -m lernt`.
`make test` runs both test suites.

The man page and zsh completion are installed by hand:

```sh
install -Dm644 lernt.1 ~/.local/share/man/man1/lernt.1
install -Dm644 completions/_lernt ~/.local/share/zsh/site-functions/_lernt
```

The completion directory must be on `fpath` before `compinit` runs:
`fpath=(~/.local/share/zsh/site-functions $fpath)`, then
`rm -f ~/.zcompdump*; exec zsh`.

## Documentation

`lernt --help`, `lernt-deck --help`, and `man lernt` - which is where the deck
format, the card format, the deck JSON and the log formats are written down.
