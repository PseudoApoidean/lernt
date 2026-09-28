"""The attempt log: what you were asked, and whether you got it.

Append-only JSONL, one object per attempt, keyed by the card's **id** - its
filename stem in the bank, which is the same identity the vault's wikilinks
already use. That choice is the whole design:

  - Adding cards changes nothing. A new card simply has no history yet.
  - Reordering cards changes nothing, because nothing here is positional. A
    log of "question 7 failed" would have been silently re-pointed by the next
    insertion; there is no question 7 to re-point.
  - Renaming a card orphans exactly that card's history, and the log keeps the
    title it was asked under, so the orphan is still readable rather than a
    bare dead id. `retired()` names them instead of dropping them.
  - Nothing is ever rewritten, so a run that dies mid-question loses only that
    question, and two runs at once cannot corrupt each other.

A "miss" is a card whose *most recent* attempt was shaky or fail. That is
self-clearing: answer it well once and it leaves the list, which is what you
want from a re-run list and what a cumulative failure count never gives.
"""

import json
import os
import time
from pathlib import Path

VERSION = 1
MISS_SCORES = ("shaky", "fail")


def default_path(slug="attempts"):
    """XDG state, never the notes directory: this is a record of runs, not a
    note, and writing it beside the cards would turn every run into a dirty
    git tree. One file per deck - `Config.log_path` picks the name."""
    base = os.environ.get("XDG_STATE_HOME") or Path.home() / ".local/state"
    return Path(base) / "lernt" / f"{slug}.jsonl"


def append(path, card, title, score, seconds, run=None, reached=None):
    """One attempt, flushed immediately. The whole point of the log is to
    survive a Ctrl-C, so nothing is buffered until the end of a run."""
    entry = {
        "v": VERSION,
        "at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "run": run,
        "card": card,
        "title": title,
        "score": score,
        "seconds": seconds,
        "reached": reached,
    }
    path = Path(path).expanduser()
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as fh:
        fh.write(json.dumps(entry, ensure_ascii=False) + "\n")
        fh.flush()
    return entry


def load(path):
    """Every attempt, oldest first. A line that will not parse is skipped, not
    fatal: a truncated final line from a killed run must not cost you the
    history above it."""
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
            if isinstance(entry, dict) and entry.get("card"):
                entries.append(entry)
    return entries


def history(entries):
    """card id -> {attempts, misses, last, title, streak}. `streak` counts the
    consecutive passes at the end, so "got it twice running" is answerable."""
    out = {}
    for entry in entries:
        card = entry["card"]
        rec = out.setdefault(
            card,
            {
                "card": card,
                "attempts": 0,
                "misses": 0,
                "last": None,
                "title": card,
                "streak": 0,
            },
        )
        rec["attempts"] += 1
        rec["last"] = entry
        rec["title"] = entry.get("title") or rec["title"]
        if entry.get("score") in MISS_SCORES:
            rec["misses"] += 1
            rec["streak"] = 0
        else:
            rec["streak"] += 1
    return out


def missed(entries, known=None):
    """Cards whose most recent attempt was a miss, worst first (fail before
    shaky, then most-missed, then oldest attempt - the one you have avoided
    longest sorts up). `known` filters to cards still in the deck."""
    recs = [
        r for r in history(entries).values() if r["last"].get("score") in MISS_SCORES
    ]
    if known is not None:
        recs = [r for r in recs if r["card"] in known]
    recs.sort(
        key=lambda r: (
            r["last"]["score"] != "fail",
            -r["misses"],
            r["last"].get("at") or "",
        )
    )
    return recs


def retired(entries, known):
    """Logged cards that are no longer in the deck - renamed or deleted. Named
    rather than silently dropped, because a renamed card's history going quiet
    should be visible."""
    return sorted({e["card"] for e in entries if e["card"] not in known})
