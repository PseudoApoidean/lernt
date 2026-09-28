"""The edit log: what should change in the bank, noticed while drilling it.

The attempt log's sibling, and built the same way - append-only JSONL, one
object per entry, keyed by card id, one file per deck. A run is where a bad
question shows itself ("that is two questions", "the answer never says why"),
and it is the wrong moment to go and fix it; this is where it is written down
instead, so the fix can happen later, at a desk, from a list.

Four kinds of entry:

  - `note`    free text against a card - "the question needs to clarify X".
  - `remove`  the card should leave the bank, with an optional reason.
  - `edit`    the card's file was changed from inside a run.
  - `done`    what stood against the card has been dealt with some other way -
              fixed in another editor, or a removal flag withdrawn.

A note or a removal flag *stands* until an `edit` or `done` follows it for the
same card, or until the card is no longer in the bank. That is self-clearing in
the way the miss list is: act on it once and it leaves the list, and nothing is
ever rewritten to make that happen. A removal flag on a card that has left the
bank is simply satisfied; a note on one is named, because a rename carries the
card off with its notes still unread.
"""

import json
import time
from pathlib import Path

VERSION = 1
REQUESTS = ("note", "remove")
RESOLVES = ("edit", "done")
KINDS = REQUESTS + RESOLVES


def append(path, card, title, kind, text="", run=None):
    """One entry, flushed immediately - a note typed just before a Ctrl-C is
    exactly the note that should survive it."""
    if kind not in KINDS:
        raise ValueError(f"Unknown edit kind {kind!r}")
    entry = {
        "v": VERSION,
        "at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "run": run,
        "card": card,
        "title": title,
        "kind": kind,
        "text": text,
    }
    path = Path(path).expanduser()
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as fh:
        fh.write(json.dumps(entry, ensure_ascii=False) + "\n")
        fh.flush()
    return entry


def load(path):
    """Every entry, oldest first. Unparseable lines are skipped, as in the
    attempt log, so a torn final line costs only itself."""
    path = Path(path).expanduser()
    if not path.exists():
        return []
    entries = []
    with path.open(encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                entry = json.loads(line)
            except ValueError:
                continue
            if (
                isinstance(entry, dict)
                and entry.get("card")
                and entry.get("kind") in KINDS
            ):
                entries.append(entry)
    return entries


def standing(entries):
    """card id -> the notes and removal flags still standing against it, oldest
    first. Cards with nothing standing are absent."""
    out = {}
    for entry in entries:
        card = entry["card"]
        if entry["kind"] in RESOLVES:
            out.pop(card, None)
        else:
            out.setdefault(card, []).append(entry)
    return out


def outstanding(entries, known):
    """The edit list: (card id, standing entries) for cards still in the bank.
    Cards flagged for removal sort first - they are the cheapest to act on and
    the most wrong to keep drilling - then the longest-standing."""
    items = [(card, rows) for card, rows in standing(entries).items() if card in known]
    items.sort(
        key=lambda item: (
            not any(r["kind"] == "remove" for r in item[1]),
            item[1][0].get("at") or "",
        )
    )
    return items


def orphaned(entries, known):
    """Cards no longer in the bank that still have notes standing. Removal flags
    are left out: a flagged card that has gone is a flag that was acted on."""
    return sorted(
        card
        for card, rows in standing(entries).items()
        if card not in known and any(r["kind"] == "note" for r in rows)
    )
