"""A deck: a directory holding a deck.toml that says where its cards are.

Decks live side by side in one directory, a subdirectory each:

    ~/.local/share/lernt/decks/
      embedded/   -> a symlink to a folder in a notes vault
        deck.toml
        Questions/*.md
      capitals/      a portable deck, the cards beside its config
        deck.toml
        *.md

    [deck]
    name  = "Embedded screens"     # optional; defaults to the directory name
    description = "Questions from embedded technical screens"  # optional
    cards = ["Questions"]          # optional; defaults to the deck directory
    count = 10                     # optional run defaults
    limit = 30
    boxes = [1, 2, 4, 8, 16]       # optional; days between reviews, per Leitner box
    log   = "~/somewhere.jsonl"    # optional; default is per deck under XDG
    edits = "~/somewhere.jsonl"    # optional; the edit log, likewise

A deck's id is its directory name in the decks directory, never its display
name. The id names the attempt log, so retitling a deck keeps its history.

`cards` takes a list: one deck can pool several banks. Relative entries resolve
against the deck directory - the real one, when the deck is a symlink - and
absolute or ~ entries point anywhere, which is how a deck keeps its cards in a
vault while its config sits in the decks directory.
"""

import os
import tomllib
from pathlib import Path

from . import leitner

DECK_FILE = "deck.toml"


class ConfigError(Exception):
    """Phrased for the user and already complete - printed, not decorated."""


def decks_home():
    if os.environ.get("LERNT_DECKS"):
        return Path(os.environ["LERNT_DECKS"]).expanduser()
    base = os.environ.get("XDG_DATA_HOME") or Path.home() / ".local/share"
    return Path(base) / "lernt" / "decks"


def state_home():
    base = os.environ.get("XDG_STATE_HOME") or Path.home() / ".local/state"
    return Path(base) / "lernt"


def settings_path():
    """What a frontend has set for lernt as a whole, not for one deck. Beside
    the logs, and it cannot be one: a deck's slug never contains a dot."""
    return state_home() / "settings.json"


class Config:
    def __init__(
        self,
        deck_id,
        name,
        cards,
        path=None,
        description="",
        count=None,
        limit=None,
        log=None,
        edits=None,
        boxes=leitner.DEFAULT_INTERVALS,
    ):
        self.id = deck_id
        self.name = name
        self.description = description  # a line or two on what the deck is
        self.cards = cards  # absolute directories, in config order
        self.path = path  # the deck directory this came from, if any
        self.count = count
        self.limit = limit
        self.boxes = tuple(boxes)  # days between reviews in each Leitner box
        self._log = log
        self._edits = edits

    @property
    def slug(self):
        """The id made filename-safe, which is what the logs are called. Each
        deck logs separately: a card id is only unique within a bank, and one
        shared log would silently merge two decks' histories."""
        safe = "".join(c if c.isalnum() or c in "-_" else "-" for c in self.id.lower())
        return "-".join(part for part in safe.split("-") if part) or "deck"

    @property
    def log_path(self):
        if self._log:
            return Path(self._log).expanduser()
        return state_home() / f"{self.slug}.jsonl"

    @property
    def edits_path(self):
        """Beside the attempt log, and per deck for the same reason. A slug
        never contains a dot, so no deck's attempt log can be named this."""
        if self._edits:
            return Path(self._edits).expanduser()
        return state_home() / f"{self.slug}.edits.jsonl"

    @property
    def options_path(self):
        """What a frontend's options screen was last set to for this deck."""
        return state_home() / f"{self.slug}.options.json"

    def describe(self):
        """The deck as JSON-ready data - what a frontend that is not this one
        needs to find the cards and write to the right logs."""
        return {
            "id": self.id,
            "name": self.name,
            "description": self.description,
            "path": str(self.path) if self.path else None,
            "cards": [str(d) for d in self.cards],
            "count": self.count,
            "limit": self.limit,
            "boxes": list(self.boxes),
            "log": str(self.log_path),
            "edits": str(self.edits_path),
        }

    def __repr__(self):
        return f"Config({self.id!r}, {self.cards!r})"


def load(path, deck_id=None):
    """A deck directory, or its deck.toml, into a Config. `deck_id` defaults to
    the directory's name as given - a symlink's name, not its target's."""
    given = Path(path).expanduser()
    if given.name == DECK_FILE:
        given = given.parent
    deck_id = deck_id or given.absolute().name
    if not given.is_dir():
        raise ConfigError(f"No such deck: {given}")
    # resolve(), not absolute(): a deck reached through a symlink must resolve
    # its relative card paths against where it really is, not against the link.
    directory = given.resolve()
    file = directory / DECK_FILE
    if not file.is_file():
        raise ConfigError(f"{given} is not a deck: it has no {DECK_FILE}.")
    try:
        with file.open("rb") as fh:
            data = tomllib.load(fh)
    except tomllib.TOMLDecodeError as exc:
        raise ConfigError(f"{file} is not valid TOML: {exc}") from exc

    section = data.get("deck", {} if not data else None)
    if not isinstance(section, dict):
        raise ConfigError(f"{file} has no [deck] section.")

    cards = section.get("cards", ["."])
    if isinstance(cards, str):
        cards = [cards]
    if not cards or not isinstance(cards, list):
        raise ConfigError(f"{file}: cards must be a directory, or a list of them.")

    resolved = []
    for entry in cards:
        if not isinstance(entry, str):
            raise ConfigError(f"{file}: every entry in cards must be a path.")
        # `/` on a Path is absolute-aware: an absolute entry replaces the base
        # rather than appending to it, so the two cases need no branch.
        full = Path(os.path.normpath(directory / Path(entry).expanduser()))
        if not full.is_dir():
            raise ConfigError(
                f"{file} names a card directory that does not exist: {full}"
            )
        if full not in resolved:
            resolved.append(full)

    description = section.get("description", "")
    if not isinstance(description, str):
        raise ConfigError(f"{file}: description must be a string.")

    boxes = leitner.DEFAULT_INTERVALS
    if "boxes" in section:
        boxes, problem = leitner.check_intervals(section["boxes"])
        if problem:
            raise ConfigError(f"{file}: {problem}.")

    return Config(
        deck_id=deck_id,
        name=section.get("name") or deck_id,
        description=" ".join(description.split()),
        cards=resolved,
        path=directory,
        count=section.get("count"),
        limit=section.get("limit"),
        log=section.get("log"),
        edits=section.get("edits"),
        boxes=boxes,
    )


def available():
    """id -> path for every entry in the decks directory. Symlinks count, and so
    does a link or directory that turns out not to be a deck: `listing` reports
    those rather than hiding them."""
    home = decks_home()
    if not home.is_dir():
        return {}
    return {
        entry.name: entry
        for entry in sorted(home.iterdir())
        if not entry.name.startswith(".") and (entry.is_dir() or entry.is_symlink())
    }


def listing():
    """(id, Config or None, error or None) for every deck, in name order."""
    rows = []
    for deck_id, path in available().items():
        try:
            rows.append((deck_id, load(path, deck_id=deck_id), None))
        except ConfigError as exc:
            rows.append((deck_id, None, str(exc)))
    return rows


def resolve(wanted):
    """A Config from what the user typed: a bare name from the decks directory,
    else a path. Nothing is ever picked for you - no deck named is an error that
    lists the decks, because a run logs against the deck it ran."""
    known = available()
    if not wanted:
        raise ConfigError("Name a deck." + _known_note(known))
    wanted = str(wanted)
    if os.sep not in wanted and wanted in known:
        return load(known[wanted], deck_id=wanted)
    if os.sep in wanted or wanted.startswith(".") or Path(wanted).expanduser().exists():
        return load(wanted)
    raise ConfigError(f"No deck called {wanted!r}." + _known_note(known))


def how_to():
    home = decks_home()
    return (
        f"A deck is a directory in {home} holding a {DECK_FILE}:\n\n"
        "    [deck]\n"
        '    name  = "Embedded screens"\n'
        '    cards = ["~/notes/Questions"]   # leave out if the cards sit beside it\n\n'
        "A folder that already holds one can be linked in:\n\n"
        f"    ln -s ~/notes/Screens {home}/embedded\n\n"
        "`--cards DIR` runs without a deck at all."
    )


def _known_note(known):
    if not known:
        return "\n\n" + how_to()
    return "\n\nDecks:\n" + "\n".join(f"    {name}" for name in sorted(known))


def from_dirs(dirs, name="ad hoc"):
    """A deck that was never a directory of its own - what `--cards` builds."""
    resolved = []
    for entry in dirs:
        full = Path(entry).expanduser().absolute()
        if not full.is_dir():
            raise ConfigError(f"Cannot find the card bank at {full}")
        if full not in resolved:
            resolved.append(full)
    return Config(deck_id="ad-hoc", name=name, cards=resolved)
