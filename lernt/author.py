"""Writing decks and cards: the inverse of `deck.parse_card` and `config.load`.

Nothing here knows a terminal or asks a question. A frontend collects the
words; this puts them in files in the format the README describes, so that what
is written is what `lernt-deck --check` reads back.
"""

import json
import re
import tomllib
from pathlib import Path

from . import config as config_mod

# Where a deleted card or deck goes, beside where it was. A name that starts
# with a dot is skipped by the card banks and by the decks directory alike, so
# what is in here is out of every deck and still there to move back.
TRASH = ".trash"

# What a filename cannot hold, and what would turn a card id into a link
# (`[[Some card]]`) or a tag when it is written into one.
_UNSAFE = re.compile(r'[\\/:*?"<>|#^\[\]\x00-\x1f]')
MAX_ID = 80


class AuthorError(Exception):
    """Phrased for the user; the API turns it into an invalid-call error."""


def one_line(text):
    """Text as the one line a field of the file holds: a box in a frontend can
    hold a newline, and a newline in the reference, or in a deck's name, would
    break the file's format."""
    return " ".join(text.split())


def deck_id(name):
    """A deck's directory name from its title: the slug the logs are named by,
    so a deck made here is never one whose id needs quoting."""
    safe = "".join(c if c.isalnum() else "-" for c in name.strip().lower())
    return "-".join(part for part in safe.split("-") if part)


def create_deck(home, name, description=""):
    """A new deck directory in `home`, its cards beside its deck.toml. Returns
    the directory. A deck that is already there is never overwritten."""
    name = one_line(name)
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


def render_card(title, answer, notes, reference, time, follows, titles, draft, tags=()):
    """The card as Markdown. `titles` maps a card id to its question, for the
    text of a follow-up link."""
    out = [f"# {title}", "", "> [!tip]"]
    out += [f"> {line}".rstrip() for line in answer.strip().splitlines() or [""]]
    if notes.strip():
        out += ["", *notes.strip().splitlines()]
    meta = []
    if one_line(reference):
        meta.append(f"**Reference:** {one_line(reference)}")
    if time is not None:
        meta.append(f"**Time:** {time}s" if time else "**Time:** off")
    if follows:
        meta.append("**Follow-ups:**")
        for n, target in enumerate(follows, 1):
            label = titles.get(target, target)
            link = f"[[{target}]]" if label == target else f"[[{target}|{label}]]"
            meta.append(f"{n}. {link}")
    # Tags are only read below the rule, so a tag needs one even alone.
    tagged = [t for t in tags if t != "draft"] + (["draft"] if draft else [])
    if meta or tagged:
        out += ["", "---"]
    if meta:
        out += ["", *meta]
    if tagged:
        out += ["", " ".join(f"#{t}" for t in tagged)]
    return "\n".join(out) + "\n"


def _string(text):
    """A TOML basic string. JSON's escapes are a subset of TOML's, and the only
    characters either needs escaped are the quote, the backslash and controls."""
    return json.dumps(text, ensure_ascii=False)


def trash(path):
    """Move a file or directory - or a symlink itself, never what it points at -
    into a `.trash` beside it, under a name nothing there has. Returns where it
    went. Nothing is erased: putting it back is moving it back."""
    path = Path(path)
    if not (path.exists() or path.is_symlink()):
        raise AuthorError(f"Cannot find {path}.")
    bin_ = path.parent / TRASH
    bin_.mkdir(exist_ok=True)
    target = bin_ / path.name
    n = 1
    while target.exists() or target.is_symlink():
        n += 1
        target = bin_ / f"{path.stem}-{n}{path.suffix}"
    path.rename(target)
    return target


def update_card(
    path,
    known,
    own_id,
    title,
    answer="",
    notes="",
    reference="",
    time=None,
    follows=(),
    draft=False,
    tags=(),
):
    """A card's file written again from its parts, under the id it already has:
    the id is the filename, and other cards' links and the attempt log name it,
    so a new question does not rename it. `known` is as in `create_card`.

    This writes the card format afresh, so what the format does not hold is not
    kept: an answer's callout becomes `tip`, and a line under the rule that is
    not a reference, time, follow-up or tag is dropped. The editor is the way to
    change a card without that."""
    title = " ".join(title.split())
    if not title:
        raise AuthorError("A card needs a question.")
    missing = [t for t in follows if t not in known or t == own_id]
    if missing:
        raise AuthorError("No such card to follow up with: " + ", ".join(missing))
    if time is not None and time < 0:
        raise AuthorError("A card's time is in seconds (0 = untimed).")
    path = Path(path)
    if not path.is_file():
        raise AuthorError(f"Cannot find the card at {path}.")
    path.write_text(
        render_card(title, answer, notes, reference, time, follows, known, draft, tags),
        encoding="utf-8",
    )
    return path


def update_deck(directory, name, description=""):
    """A deck's name and description changed in its deck.toml, and nothing else:
    the other keys and the file's order stay, though a comment on the two lines
    changed does not. An empty description removes the line."""
    name = one_line(name)
    if not name:
        raise AuthorError("A deck needs a name.")
    file = Path(directory) / config_mod.DECK_FILE
    lines = file.read_text(encoding="utf-8").splitlines()
    start = next((i for i, line in enumerate(lines) if line.strip() == "[deck]"), None)
    if start is None:
        raise AuthorError(f"{file.name} has no [deck] section: edit it in the editor.")
    end = next(
        (i for i in range(start + 1, len(lines)) if lines[i].lstrip().startswith("[")),
        len(lines),
    )

    def put(key, value):
        nonlocal end
        for i in range(start + 1, end):
            if re.match(rf"^\s*{key}\s*=", lines[i]):
                if '"""' in lines[i] or "'''" in lines[i]:
                    raise AuthorError(
                        f"The {key} is a multi-line string: use the editor."
                    )
                if value is None:
                    del lines[i]
                    end -= 1
                else:
                    lines[i] = f"{key} = {_string(value)}"
                return
        if value is not None:
            lines.insert(start + 1, f"{key} = {_string(value)}")
            end += 1

    put("description", " ".join(description.split()) or None)
    put("name", name)
    text = "\n".join(lines) + "\n"
    try:
        tomllib.loads(text)
    except tomllib.TOMLDecodeError as exc:
        raise AuthorError(f"{file.name} would not be valid TOML: {exc}") from exc
    file.write_text(text, encoding="utf-8")
