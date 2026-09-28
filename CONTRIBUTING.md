# Working on lernt

## Layout

```
lernt/
  config.py   a deck directory's deck.toml -> Config (id, name, banks, logs, defaults)
  deck.py     the engine: a deck's card banks -> one JSON deck
  session.py  pure selection and scoring over a deck - no I/O, no clock
  misses.py   the append-only attempt log, and what counts as a miss
  leitner.py  the Leitner schedule, replayed from the attempt log: boxes and what is due
  edits.py    the append-only edit log, and what still stands against a card
  options.py  a frontend's saved options per deck: keys to values, merged on save
  text.py     Markdown -> something a terminal can print
  api.py      the API: every operation a frontend can ask of lernt, as typed calls
  rpc.py      the API as JSON-RPC over stdio (lernt-deck --serve), and its JSON Schema
  cli.py      the terminal frontend over api: prompts, timing, layout; bare `lernt` execs the TUI
docs/
  tui-flows.md  every TUI screen, how you get to it, and where its keys go
completions/
  _lernt      zsh completion for lernt and lernt-deck
tui/          the TUI: a Go module (bubbletea), built into lernt/_bin/lernt-tui
  engine/     the TUI's only door into lernt: a JSON-RPC client for the API, and its types
Makefile      builds the TUI; install and uninstall; runs both test suites
```

A flag lives in three places: the parser (`build_parser` in `cli.py` or
`deck.py`), `lernt.1`, and `completions/_lernt`. `tests/test_completion.py`
fails when the completion file and a parser disagree; the man page has no such
check, so update it by hand.

`config.py`, `deck.py`, `session.py`, `misses.py`, `leitner.py` and `edits.py` know nothing about a terminal, and
`cli.py` parses no Markdown. That seam is the point of the project — a
frontend is written against `api.py`, and should need no change here. Keep
new behaviour on the correct side of it: how a run moves belongs in `session.py` or `api.py`
even when only the CLI can currently show it.

The TUI is such a frontend. It ships in `tui/` and is built into the package,
but it is coupled to Python only through the API: `lernt` with no arguments
execs `lernt/_bin/lernt-tui` with the engine command in `LERNT_ENGINE`, and
`tui/engine` is the only Go code that runs it (`--serve`). Keep it that way -
screen code that shells out, or reads a card file, is code that has to change
when the other side does.

A new screen, or a key that moves between screens, is also a change to
`docs/tui-flows.md`.

Changing the API is a change on both sides. A method or field added to `api.py`
needs its twin in `tui/engine/api.go`, and `TestTheContract` fails in `go test`
until it has one. A change an older TUI would misread also bumps `api.PROTOCOL`
and `engine.Protocol` together.

## Style

`uvx ruff format . && uvx ruff check .` — config in `pyproject.toml`, which is the vault's
`general.ruff.toml` plus this project's ignores. The reasoning lives in the vault's
*Python Code Style* note; the short version is that formatting is never discussed, and a
suppression carries a comment saying why the rule is wrong there.

## Tests

```sh
make test         # uv run --with pytest pytest -q, then go test ./... in tui/
```

Tests build their own small card bank in a `tmp_path`, and anything touching
decks or the logs points `XDG_DATA_HOME` (or `LERNT_DECKS`) and `XDG_STATE_HOME` there
too - a test must never read or write the real ones. Never assert against the
real decks — it is edited constantly, and such a test fails for reasons
that have nothing to do with the code.
