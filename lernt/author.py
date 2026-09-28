"""Writing decks and cards: the inverse of `deck.parse_card` and `config.load`.

Nothing here knows a terminal or asks a question. A frontend collects the
words; this puts them in files in the format the README describes, so that what
is written is what `lernt-deck --check` reads back.
"""

import json
import re
from pathlib import Path

from . import config as config_mod

# What a filename cannot hold, and what would turn a card id into a link
# (`[[Some card]]`) or a tag when it is written into one.
_UNSAFE = re.compile(r'[\\/:*?"<>|#^\[\]\x00-\x1f]')
MAX_ID = 80


class AuthorError(Exception):
    """Phrased for the user; the API turns it into an invalid-call error."""


def deck_id(name):
    """A deck's directory name from its title: the slug the logs are named by,
    so a deck made here is never one whose id needs quoting."""
    safe = "".join(c if c.isalnum() else "-" for c in name.strip().lower())
    return "-".join(part for part in safe.split("-") if part)


def create_deck(home, name, description=""):
    """A new deck directory in `home`, its cards beside its deck.toml. Returns
    the directory. A deck that is already there is never overwritten."""
    name = name.strip()
    slug = deck_id(name)
    if not slug:
        raise AuthorError("A deck needs a name.")
    directory = Path(home) / slug
    if directory.exists() or directory.is_symlink():
        raise AuthorError(f"There is already a deck called {slug!r}.")
    lines = ["[deck]", f"name = {_string(name)}"]
    if description.strip():
        lines.append(f"description = {_string(' '.join(description.split()))}")
    directory.mkdir(parents=True)
    (directory / config_mod.DECK_FILE).write_text(
        "\n".join(lines) + "\n", encoding="utf-8"
    )
    return directory


def card_id(title):
    """A card's filename stem from its question. The question stays as it was
    written in the card; only the filename loses what a filename cannot hold."""
    stem = " ".join(_UNSAFE.sub(" ", title).split())
    return stem[:MAX_ID].rstrip(" .")


def create_card(
    directory,
    known,
    title,
    answer="",
    notes="",
    reference="",
    time=None,
    follows=(),
    draft=False,
):
    """A new card file in `directory`, in the card format. `known` maps every card
    id the deck already has to its question: the new id must not be one, and a
    follow-up must be. Returns (id, path)."""
    title = " ".join(title.split())
    if not title:
        raise AuthorError("A card needs a question.")
    stem = card_id(title)
    if not stem:
        raise AuthorError("A card's question has to contain some letters.")
    if stem in known:
        raise AuthorError(f"There is already a card called {stem!r}.")
    missing = [target for target in follows if target not in known]
    if missing:
        raise AuthorError("No such card to follow up with: " + ", ".join(missing))
    if time is not None and time < 0:
        raise AuthorError("A card's time is in seconds (0 = untimed).")
    directory = Path(directory)
    if not directory.is_dir():
        raise AuthorError(f"Cannot find the card bank at {directory}")
    path = directory / f"{stem}.md"
    if path.exists():
        raise AuthorError(f"There is already a file {path.name!r} in {directory}.")
    path.write_text(
        render_card(title, answer, notes, reference, time, follows, known, draft),
        encoding="utf-8",
    )
    return stem, path


def render_card(title, answer, notes, reference, time, follows, titles, draft):
    """The card as Markdown. `titles` maps a card id to its question, for the
    text of a follow-up link."""
    out = [f"# {title}", "", "> [!tip]"]
    out += [f"> {line}".rstrip() for line in answer.strip().splitlines() or [""]]
    if notes.strip():
        out += ["", *notes.strip().splitlines()]
    meta = []
    if reference.strip():
        meta.append(f"**Reference:** {reference.strip()}")
    if time is not None:
        meta.append(f"**Time:** {time}s" if time else "**Time:** off")
    if follows:
        meta.append("**Follow-ups:**")
        for n, target in enumerate(follows, 1):
            label = titles.get(target, target)
            link = f"[[{target}]]" if label == target else f"[[{target}|{label}]]"
            meta.append(f"{n}. {link}")
    # Tags are only read below the rule, so a draft needs one even alone.
    if meta or draft:
        out += ["", "---"]
    if meta:
        out += ["", *meta]
    if draft:
        out += ["", "#draft"]
    return "\n".join(out) + "\n"


def _string(text):
    """A TOML basic string. JSON's escapes are a subset of TOML's, and the only
    characters either needs escaped are the quote, the backslash and controls."""
    return json.dumps(text, ensure_ascii=False)
