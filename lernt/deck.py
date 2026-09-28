"""The engine: a deck's Markdown cards in, one JSON document out.

USAGE: lernt-deck DECK [--out FILE] [--check]
       lernt-deck --cards DIR... [--out FILE] [--check]
       lernt-deck --list
       lernt-deck --serve | --schema

A card is a Markdown file whose stem is its id:

    # The question

    > [!tip]
    > The answer. Any callout type; the first callout is the answer.

    Notes: anything else to show alongside the answer.

    ---

    **Reference:** [[Some longer note]]
    **Time:** 45s
    **Follow-ups:**
    1. [[Another card|Its question]]

    #draft #any-other-tag

A card with no callout is all answer. Everything below the first rule is
metadata: an optional reference and time, follow-up links (the target's filename stem
is the id; any folder path, heading or alias in the link is ignored) and tags.
A card tagged #draft stays out of random jumps unless a run asks for drafts.

Everything is emitted as the Markdown it was written in, links resolved to
card ids but otherwise untouched. Rendering is a frontend's business: the
terminal wants "**" stripped and a lone "*" kept (a C question about `char *p`
is ruined by a cleaner that is too clever), and a GUI wants neither.
"""

import argparse
import datetime
import json
import re
import sys
from pathlib import Path

from . import config as config_mod

SCHEMA = "lernt.deck/4"
# /4 only added each card's "time", so a /3 deck still reads; a deck written
# before the rename carries the old "drill." prefix, and reads too.
READABLE = ("drill.deck/3", "drill.deck/4", SCHEMA)
LIST_SCHEMA = "lernt.decks/1"

_CALLOUT = re.compile(r"^>\s?\[![\w-]+\][+-]?\s?")
_QUOTE = re.compile(r"^>\s?")
_FOLLOWUP = re.compile(r"^\d+\.\s*\[\[(?:[^\]|#]*/)?([^/\]|#]+)[^\]]*\]\]")
_REFERENCE = re.compile(r"^\*\*Reference:\*\*\s*(.*)$")
_TIME = re.compile(r"^\*\*Time:\*\*\s*(.*)$", re.IGNORECASE)
_DURATION = re.compile(r"^(?:(\d+)\s*m(?:in)?)?\s*(?:(\d+)\s*s?)?$")
# Not after a word character, "&" (an HTML entity), "#" (a heading) or "/".
_TAG = re.compile(r"(?<![\w&#/])#([A-Za-z][\w/-]*)")


class DeckError(Exception):
    """A bank that cannot be read at all - not a card that parses oddly."""


def parse_card(path):
    """One card file into its dict. Missing parts come back empty rather than
    raising: the bank is hand-written prose and a card mid-edit should drop a
    field, not take the whole deck down with it."""
    path = Path(path)
    lines = path.read_text(encoding="utf-8").splitlines()

    card = {
        "id": path.stem,
        # str, not Path: this dict is about to become JSON, and the boundary
        # is where a Path stops being one.
        "source": str(path.absolute().parent),
        "title": "",
        "answer": [],
        "notes": [],
        "followups": [],
        "reference": "",
        # Seconds of its own. 0: answered untimed. None: the run decides.
        "time": None,
        "tags": [],
        "draft": False,
    }

    start = 0
    for i, line in enumerate(lines):
        if line.startswith("# "):
            card["title"] = line[2:].strip()
            start = i + 1
            break

    rest = lines[start:]
    rule = next((i for i, line in enumerate(rest) if line.strip() == "---"), len(rest))
    body, meta = rest[:rule], rest[rule + 1 :]

    found = in_callout = False
    for line in body:
        if not found and _CALLOUT.match(line):
            found = in_callout = True
            card["answer"].append(_CALLOUT.sub("", line))
        elif in_callout and line.startswith(">"):
            card["answer"].append(_QUOTE.sub("", line))
        else:
            in_callout = False
            card["notes"].append(line)
    if not found:
        card["answer"], card["notes"] = card["notes"], []

    for line in meta:
        m = _FOLLOWUP.match(line)
        if m:
            card["followups"].append(m.group(1).strip())
            continue
        m = _REFERENCE.match(line)
        if m:
            card["reference"] = m.group(1).strip()
            continue
        m = _TIME.match(line)
        if m:
            card["time"] = duration(m.group(1))
            if card["time"] is None:
                card["_problems"] = [
                    f"{card['id']}: **Time:** is not a duration: {m.group(1).strip()!r}"
                ]
            continue
        for tag in _TAG.findall(line):
            if tag not in card["tags"]:
                card["tags"].append(tag)

    card["draft"] = "draft" in card["tags"]
    # A callout with no label ("> [!tip]" alone) opens on a blank line.
    card["answer"] = _trim_blanks(card["answer"])
    card["notes"] = _trim_blanks(card["notes"])
    return card


def duration(text):
    """Seconds from a card's **Time:** - "45", "45s", "2m", "1m30s" - or 0 for
    "off", a card to answer untimed. None when it is not a duration."""
    text = text.strip().lower()
    if text in ("off", "none", "untimed"):
        return 0
    m = _DURATION.match(text)
    if not m or not any(m.groups()):
        return None
    return int(m.group(1) or 0) * 60 + int(m.group(2) or 0)


def _trim_blanks(lines):
    while lines and not lines[0].strip():
        lines.pop(0)
    while lines and not lines[-1].strip():
        lines.pop()
    return lines


def card_path(card):
    """Where a card was read from. A deck stores the directory and the id, and
    the id is the filename stem, so this is the one place that puts them back
    together."""
    return Path(card["source"]) / f"{card['id']}.md"


def reload(card, known):
    """`card` read again off disk, after an edit. Follow-ups are kept to the ids
    in `known`, as `build` would, and `parents` is carried over: it is computed
    from every other card, and one edit cannot have changed those. A follow-up
    added or removed here does not show in its target's `parents` until the
    next build."""
    fresh = parse_card(card_path(card))
    fresh.pop("_problems", None)
    fresh["source"] = card["source"]
    fresh["followups"] = [t for t in fresh["followups"] if t in known]
    fresh["parents"] = list(card.get("parents") or [])
    return fresh


def build(config):
    """Every card named by `config`, plus the back-edges and any dangling links.

    Several directories are drilled as one pool, in config order. A card id is
    a filename, so two banks can hold the same one; the first wins and the
    clash is reported rather than resolved. Silently letting the later file win
    would re-point every follow-up link that names it, which is the one way two
    banks can corrupt each other."""
    cards = {}
    problems = []
    for directory in config.cards:
        directory = Path(directory)
        if not directory.is_dir():
            raise DeckError(f"Cannot find the card bank at {directory}")
        for entry in sorted(directory.iterdir()):
            if entry.suffix != ".md" or entry.name.startswith("!"):
                continue
            card = parse_card(entry)
            problems.extend(card.pop("_problems", []))
            if card["id"] in cards:
                problems.append(
                    "{}: two banks hold this card; keeping the one in {}".format(
                        card["id"], cards[card["id"]]["source"]
                    )
                )
                continue
            cards[card["id"]] = card

    # `parents` is computed, never stored in the card files: a "follows from"
    # line would be a second copy of every edge and the two would drift.
    for card in cards.values():
        card["parents"] = []
    for card in cards.values():
        live = []
        for target in card["followups"]:
            if target in cards:
                live.append(target)
                cards[target]["parents"].append(card["id"])
            else:
                problems.append(
                    "{}: follow-up points at a missing card: {}".format(
                        card["id"], target
                    )
                )
        card["followups"] = live

    return {
        "schema": SCHEMA,
        "generated": datetime.datetime.now(datetime.UTC)
        .replace(microsecond=0)
        .isoformat(),
        "deck": config.id,
        "name": config.name,
        "sources": [str(directory) for directory in config.cards],
        "cards": [cards[k] for k in sorted(cards)],
        "problems": problems,
    }


def load(path):
    """A deck JSON back off disk, checked hard enough to give a useful error."""
    with Path(path).expanduser().open(encoding="utf-8") as fh:
        deck = json.load(fh)
    if deck.get("schema") not in READABLE:
        raise DeckError(
            "{} is not a {} deck (found schema {!r})".format(
                path, SCHEMA, deck.get("schema")
            )
        )
    return deck


def index(deck):
    """id -> card, which is what every consumer actually wants."""
    return {c["id"]: c for c in deck["cards"]}


def decks():
    """Every deck in the decks directory, as JSON-ready data. A deck that will
    not load is listed with its error rather than left out."""
    rows = []
    for deck_id, config, error in config_mod.listing():
        if config:
            rows.append(config.describe())
        else:
            path = str(config_mod.decks_home() / deck_id)
            rows.append({"id": deck_id, "path": path, "error": error})
    return {
        "schema": LIST_SCHEMA,
        "home": str(config_mod.decks_home()),
        "decks": rows,
    }


def build_parser():
    """The flags, apart from running them - see `cli.build_parser`."""
    ap = argparse.ArgumentParser(
        description="Read a deck's Markdown cards and emit them as one JSON document."
    )
    ap.add_argument(
        "deck",
        nargs="?",
        default=None,
        help="the deck, by name or by path (see --list)",
    )
    ap.add_argument(
        "--cards",
        metavar="DIR",
        action="append",
        default=None,
        help="a card directory, repeatable; runs without a deck",
    )
    ap.add_argument(
        "--out",
        metavar="FILE",
        default="-",
        help="where to write the deck (default: stdout)",
    )
    ap.add_argument(
        "--check",
        action="store_true",
        help="report dangling follow-ups and drafts, write nothing",
    )
    ap.add_argument(
        "--list",
        action="store_true",
        help="write every deck in the decks directory as JSON, and stop",
    )
    ap.add_argument(
        "--serve",
        action="store_true",
        help="serve the API as JSON-RPC over stdin and stdout, until stdin closes",
    )
    ap.add_argument(
        "--schema",
        action="store_true",
        help="write the API's methods, params and results as JSON Schema",
    )
    return ap


def main(argv=None):
    args = build_parser().parse_args(argv)

    # Imported here: rpc imports api, which imports this module.
    if args.serve:
        from . import rpc

        return rpc.serve()
    if args.schema:
        from . import rpc

        sys.stdout.write(json.dumps(rpc.schema(), indent=2) + "\n")
        return 0

    if args.list:
        sys.stdout.write(json.dumps(decks(), indent=2, ensure_ascii=False) + "\n")
        return 0

    try:
        config = (
            config_mod.from_dirs(args.cards)
            if args.cards
            else config_mod.resolve(args.deck)
        )
        deck = build(config)
    except (DeckError, config_mod.ConfigError) as exc:
        print(str(exc), file=sys.stderr)
        return 1

    if args.check:
        drafts = [c["id"] for c in deck["cards"] if c["draft"]]
        stops = [c["id"] for c in deck["cards"] if not c["followups"]]
        print(
            f"{deck['name']} - {len(deck['cards'])} cards "
            f"from {len(deck['sources'])} bank(s):"
        )
        for directory in deck["sources"]:
            print(f"  {directory}")
        branching = len(deck["cards"]) - len(stops)
        print(f"{branching} offer branches, {len(stops)} stop")
        print(f"{len(drafts)} tagged #draft")
        timed = sum(1 for c in deck["cards"] if c["time"] is not None)
        print(f"{timed} set their own time")
        for problem in deck["problems"]:
            print(f"  {problem}")
        return 1 if deck["problems"] else 0

    text = json.dumps(deck, indent=2, ensure_ascii=False) + "\n"
    if args.out == "-":
        sys.stdout.write(text)
    else:
        Path(args.out).expanduser().write_text(text, encoding="utf-8")
        print(f"Wrote {len(deck['cards'])} cards to {args.out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
