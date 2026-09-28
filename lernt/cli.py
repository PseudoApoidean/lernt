"""The terminal frontend: a deck in, a timed walk through its cards out.

USAGE: lernt
       lernt DECK [-n COUNT] [-t SECONDS] [-a] [-d] [-f FILTER] [-w | -l] [-e]
       lernt DECK --misses [--all]
       lernt DECK --boxes
       lernt DECK --edits
       lernt --list
       lernt --new-deck NAME [--about TEXT]
       lernt DECK --new QUESTION [--answer TEXT] [--follows ID]... [--time SECONDS]
       lernt --cards DIR... | --json FILE  [run options]

DECK is a directory holding a deck.toml: by bare name from the decks directory
(~/.local/share/lernt/decks), or by path. See config.py.

`lernt` alone hands the terminal to the TUI that ships in this package
(built from tui/ by `make`), or lists the decks where it is not built or this
is not a terminal.

This half owns the terminal and nothing else. A run is driven through `api` -
the same calls the TUI makes over JSON-RPC - so where the walk goes next, what
is logged and what a miss is are decided once, there, for both frontends.

Edit mode (-e) is a normal run with one more step after each score: open the
card in $EDITOR, leave a note on it, or flag it for removal (see `edits`). The
engine hands out the card's path and reads the file back; this module only runs
the editor in between, because it owns the terminal.

Authoring is the same: `--new-deck` and `--new` call the API's `deck.create` and
`card.create`, and a card started without an answer is opened in $EDITOR to be
written, then checked. A follow-up is refused unless it names a card the deck has.
"""

import argparse
import datetime
import json
import os
import shlex
import subprocess
import sys
import time
from pathlib import Path

from . import api
from . import config as config_mod
from . import deck as deck_mod
from .text import clean, wrap

RULE = "-------------------------------------------------------------"
THICK = "============================================================="
IND = "      "

# The TUI ships inside the package, built from tui/ by `make`. One path, no search.
TUI = Path(__file__).resolve().parent / "_bin" / "lernt-tui"


def main(argv=None):
    try:
        return _run_main(argv)
    except BrokenPipeError:
        # `lernt --misses | head` closes the pipe under us. Nothing is wrong
        # and a traceback would say otherwise; devnull keeps the interpreter's
        # own shutdown flush from raising it again.
        os.dup2(os.open(os.devnull, os.O_WRONLY), sys.stdout.fileno())
        return 0


def build_parser():
    """The flags, apart from running them - `tests/test_completion.py` holds
    `completions/_lernt` to this."""
    ap = argparse.ArgumentParser(
        description="Drill a deck of Markdown flash cards: a question, your "
        "answer, the card's answer, a score, and where to go next. With no "
        "arguments at all, start the TUI."
    )
    ap.add_argument(
        "deck",
        nargs="?",
        default=None,
        help="the deck to run, by name or by path (see --list)",
    )
    ap.add_argument(
        "-n",
        dest="count",
        type=int,
        default=None,
        metavar="COUNT",
        help="cards in the run (0 runs until you stop it; default 10)",
    )
    ap.add_argument(
        "-t",
        dest="limit",
        type=limit_arg,
        default=None,
        metavar="SECONDS",
        help="seconds per answer for every card, or off (default: each card's "
        "own time, else the deck's, else 30)",
    )
    ap.add_argument(
        "-a",
        dest="auto",
        action="store_true",
        help="follow the cards' branches automatically instead of a menu",
    )
    ap.add_argument(
        "-d",
        "--drafts",
        action="store_true",
        help="random jumps may also land on cards tagged #draft",
    )
    ap.add_argument(
        "-f",
        dest="filter",
        default="",
        metavar="FILTER",
        help="restrict the pool to card ids matching FILTER",
    )
    ap.add_argument(
        "-w",
        "--weak",
        action="store_true",
        help="drill only the cards you last got shaky or wrong",
    )
    ap.add_argument(
        "-l",
        "--due",
        action="store_true",
        help="drill only the cards the Leitner schedule has due today, lowest "
        "box first",
    )
    ap.add_argument(
        "-e",
        "--edit",
        action="store_true",
        help="after each score, offer to edit the card, note it, or flag it "
        "for removal",
    )
    ap.add_argument(
        "--misses",
        action="store_true",
        help="print what you have been getting wrong, and stop",
    )
    ap.add_argument(
        "--all",
        action="store_true",
        help="with --misses, show every card attempted, not just the misses",
    )
    ap.add_argument(
        "--boxes",
        action="store_true",
        help="print how many cards sit in each Leitner box and what is due, and stop",
    )
    ap.add_argument(
        "--edits",
        action="store_true",
        help="print the notes and removal flags standing against cards, and stop",
    )
    ap.add_argument(
        "--json",
        metavar="FILE",
        default=None,
        help="run from a deck JSON written by lernt-deck, instead of the cards",
    )
    ap.add_argument(
        "--cards",
        metavar="DIR",
        action="append",
        default=None,
        help="a card directory, repeatable; runs without a deck",
    )
    ap.add_argument("--list", action="store_true", help="list the decks and stop")
    ap.add_argument(
        "--new-deck",
        metavar="NAME",
        default=None,
        help="make a new, empty deck called NAME in the decks directory, and stop",
    )
    ap.add_argument(
        "--about",
        metavar="TEXT",
        default="",
        help="with --new-deck, a line on what the deck is",
    )
    ap.add_argument(
        "--new",
        metavar="QUESTION",
        default=None,
        help="add a card asking QUESTION to DECK, then open it in $EDITOR unless "
        "--answer gives it one",
    )
    ap.add_argument(
        "--answer",
        metavar="TEXT",
        default="",
        help="with --new, the card's answer",
    )
    ap.add_argument(
        "--follows",
        metavar="CARD",
        action="append",
        default=[],
        help="with --new, the id of a card in the deck to lead to (repeatable)",
    )
    ap.add_argument(
        "--time",
        dest="card_time",
        type=time_arg,
        default=None,
        metavar="SECONDS",
        help="with --new, the card's own time, or off to answer it untimed",
    )
    ap.add_argument(
        "--as-draft",
        action="store_true",
        help="with --new, tag the card #draft",
    )
    ap.add_argument(
        "--log",
        metavar="FILE",
        default=None,
        help="attempt log (default: $XDG_STATE_HOME/lernt/DECK.jsonl)",
    )
    ap.add_argument("--no-log", action="store_true", help="do not record this run")
    return ap


def _run_main(argv=None):
    argv = sys.argv[1:] if argv is None else argv
    if not argv:
        return launch_tui()
    args = build_parser().parse_args(argv)

    if args.list:
        return list_decks()
    if args.new_deck is not None:
        return new_deck(args)
    if args.new is not None:
        return new_card(args)

    if args.count is not None and args.count < 0:
        print("-n takes a number (0 = until you stop it).", file=sys.stderr)
        return 2

    # A deck read from JSON carries its own id and name; a deck built here
    # needs the config that named the banks. Either way the run is identified
    # by the deck, never by the directory it happened to read.
    try:
        if args.cards:
            config = config_mod.from_dirs(args.cards)
        elif args.json:
            config = None
        else:
            config = config_mod.resolve(args.deck)
        deck = deck_mod.load(args.json) if args.json else deck_mod.build(config)
    except (deck_mod.DeckError, config_mod.ConfigError, OSError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        return 1

    if config is None:
        config = config_mod.Config(
            deck_id=deck.get("deck") or "ad-hoc",
            name=deck.get("name") or "ad hoc",
            cards=[],
        )
    log_path = None if args.no_log else Path(args.log or config.log_path).expanduser()

    if args.misses:
        return report(api.miss_report(config.name, log_path, deck, args.all), args.all)
    if args.edits:
        return edit_report(api.edit_report(config.name, config.edits_path, deck))
    if args.boxes:
        return box_report(
            api.box_report(
                config.name, log_path, deck, config.boxes, datetime.date.today()
            )
        )

    engine = api.Engine()
    options = api.StartParams(
        deck=config.id,
        count=args.count,
        limit=args.limit,
        auto=args.auto,
        drafts=args.drafts,
        filter=args.filter,
        weak=args.weak,
        due=args.due,
        edit=args.edit,
        log=log_path is not None,
    )
    try:
        started = engine.begin(config, deck, options, log_path)
    except api.ApiError as exc:
        if exc.code in (api.NO_MISSES, api.NO_DUE):
            print(f"\n{exc}\n")
            return 0
        hint = " Is -f too narrow?" if exc.code == api.NO_CARDS else ""
        print(f"{exc}{hint}", file=sys.stderr)
        return 1

    return run(engine, started)


# -- the run -------------------------------------------------------------


def run(engine, started):
    """The walk, as prompts. Every decision - what comes next, what is logged,
    whether a run is over - is the engine's; this only asks and prints."""
    banner(started)
    card = started.card

    try:
        while True:
            print(RULE)
            if card.follows_from:
                print(f"{IND}    (follows from: {clean(card.follows_from)})")
            total = "" if started.count == 0 else f"/{started.count}"
            timed = f"   ({card.limit}s)" if card.limit else ""
            box = f"   [{box_label(card.box)}]"
            print(f"[{card.number}{total}] {clean(card.title)}{timed}{box}")
            print()

            start = time.monotonic()
            if prompt(f"{IND}(Enter when you have finished answering) ") is None:
                break
            elapsed = int(time.monotonic() - start)

            reveal(card)
            print()
            if card.limit and elapsed > card.limit:
                print(f"{IND}{elapsed}s - over the {card.limit}s limit.")
            else:
                print(f"{IND}{elapsed}s")
            print()

            score = ask_score()
            if score is None:
                break
            scored = engine.run_score(api.ScoreParams(score=score, seconds=elapsed))
            print(f"{IND}{box_move(scored.was, scored.box)}")

            if started.edit and not edit_step(engine, scored.standing):
                break
            if scored.finished:
                break

            if started.auto:
                card = engine.run_next(api.NextParams(pick="auto"))
                print()
                continue

            choice = navigate(engine)
            if choice is None:
                break
            card = engine.run_next(choice)
            print()
    except KeyboardInterrupt:
        print()

    done = engine.run_end(api.NoParams())
    summary(done)
    if started.edit:
        edit_summary(done)
    return 0


def banner(started):
    print()
    if started.count == 0:
        print(
            f"{started.name} - a pool of {started.pool} cards, "
            f"{pace(started)}, until you stop it."
        )
    else:
        plural = "" if started.count == 1 else "s"
        print(
            f"{started.name} - up to {started.count} card{plural} from a pool of "
            f"{started.pool}, {pace(started)}."
        )
    if started.weak:
        print("The miss list only: cards you last got shaky or wrong.")
    if started.due:
        print("The due cards only, lowest Leitner box first.")
    if started.edit:
        print("Edit mode: after each score, edit, note or flag the card.")
    print("Answer, then Enter to see the card's answer.")
    if started.auto:
        print("It follows the branches for you. Ctrl-C to stop and score.")
    else:
        print("After scoring, pick where to go next.")
    if not started.log:
        print("Not recording this run.")
    print()


def pace(started):
    """How the run is timed, for the banner: "30s each", "untimed", or "each
    card's own time, else 30s"."""
    if started.limit is not None:
        return f"{started.limit}s each" if started.limit else "untimed"
    fallback = f"{started.default_limit}s" if started.default_limit else "untimed"
    if not started.timed:
        return f"{fallback} each" if started.default_limit else "untimed"
    return f"each card's own time, else {fallback}"


def reveal(card):
    print()
    answer = [clean(line) for line in card.answer]
    if answer:
        print("\n".join(wrap(answer)))
    notes = [clean(line) for line in card.notes if line.strip()]
    if notes:
        print()
        print("\n".join(wrap(notes)))


def ask_score():
    while True:
        answer = prompt(f"{IND}p)ass  s)haky  f)ail  > ")
        if answer is None:
            return None
        choice = answer.strip().lower()
        if choice in ("p", "s", "f"):
            return {"p": "pass", "s": "shaky", "f": "fail"}[choice]
        print(f"{IND}p, s or f.")


def navigate(engine):
    """The menu. Returns the NextParams to move on with, or None to end the run."""
    fups = engine.run_menu(api.NoParams()).followups
    print()
    print(f"{IND}Where next?")
    for i, item in enumerate(fups, 1):
        mark = "   [draft]" if item.draft else ""
        print(f"{IND}  {i}  {clean(item.title)}{mark}")
    if not fups:
        print(f"{IND}  -  nothing follows this one")
    print(f"{IND}  r  random card        q  end the run")

    while True:
        answer = prompt(f"{IND}> ")
        if answer is None:
            return None
        choice = answer.strip().lower()
        if choice in ("", "r"):
            return api.NextParams(pick="random")
        if choice == "q":
            return None
        if choice.isdigit() and 1 <= int(choice) <= len(fups):
            return api.NextParams(pick="followup", card=fups[int(choice) - 1].id)
        if fups:
            print(f"{IND}1-{len(fups)}, r, or q.")
        else:
            print(f"{IND}r or q.")


def summary(done):
    print()
    print(THICK)
    print(
        f"  pass {done.passed}   shaky {done.shaky}   "
        f"fail {done.failed}   (of {done.asked})"
    )
    if done.ended_early:
        print("  Ended early.")
    if done.promoted or done.demoted:
        print(f"  {done.promoted} up a box, {done.demoted} back to box 1")
    print()

    if done.review:
        print("  Worth another pass:")
        for item in done.review:
            label = "FAIL: " if item.score == "fail" else "shaky:"
            print(f"    {label} {clean(item.title)}")
        print()
        print("  Re-run the fails once - -w is exactly that list.")
        print("  Not the whole set - the third run is worse than the second.")
    else:
        print("  Clean run. Stop here rather than going again.")
    if done.log and done.logged:
        print()
        print(f"  {done.logged} attempt(s) logged to {tilde(done.log)}")
    print()


def box_label(box):
    return "new" if box == 0 else f"box {box}"


def box_move(was, box):
    """What a score did to the card's box, in a few words."""
    if box > max(was, 1):
        return f"{box_label(was)} -> box {box}"
    if box < was:
        return f"box {was} -> box {box}, back to the start"
    if was == 0:
        return f"new -> box {box}"
    return f"stays in box {box}"


# -- edit mode -----------------------------------------------------------

EDITED = {
    "edit": "Edited - anything standing against it is cleared.",
    "note": "Noted.",
    "remove": "Flagged for removal.",
    "done": "Cleared.",
}


def edit_step(engine, standing):
    """After the score, before the walk moves on. Loops, so a card can be noted
    and then edited in one visit; Enter moves on. False ends the run."""
    if standing:
        print()
        print(f"{IND}Standing against this card:")
        for entry in standing:
            print(f"{IND}  {edit_line(entry)}")

    while True:
        print()
        options = "e)dit  n)ote  x) flag for removal"
        if standing:
            options += "  d)one"
        answer = prompt(f"{IND}{options}  Enter to go on > ")
        if answer is None:
            return False
        choice = answer.strip().lower()
        if choice == "":
            return True

        try:
            if choice == "e":
                if not edit_card(engine):
                    continue
                kind, standing = "edit", []
            elif choice == "n":
                text = prompt(f"{IND}Note > ")
                if text is None:
                    return False
                kind = "note"
                standing = engine.card_note(api.NoteParams(text=text)).standing
            elif choice == "x":
                text = prompt(f"{IND}Why remove it? (Enter for no reason) > ")
                if text is None:
                    return False
                kind = "remove"
                standing = engine.card_flag(api.FlagParams(reason=text)).standing
            elif choice == "d" and standing:
                kind = "done"
                standing = engine.card_done(api.NoParams()).standing
            else:
                print(f"{IND}e, n, x{', d' if standing else ''}, or Enter.")
                continue
        except api.ApiError as exc:
            print(f"{IND}{exc}")
            continue
        print(f"{IND}{EDITED[kind]}")


def new_deck(args):
    """`--new-deck`: an empty deck, and how to put a card in it."""
    try:
        made = api.Engine().deck_create(
            api.NewDeckParams(name=args.new_deck, description=args.about)
        )
    except api.ApiError as exc:
        print(exc, file=sys.stderr)
        return 1
    print(f"Made the deck {made.id} in {tilde(made.path)}.")
    print(f'Add a card with: lernt {made.id} --new "What is ...?"')
    return 0


def new_card(args):
    """`--new`: write the card, open it to be written unless it came with an
    answer, and say what the deck now thinks of its cards."""
    engine = api.Engine()
    try:
        made = engine.card_create(
            api.NewCardParams(
                deck=args.deck or "",
                title=args.new,
                answer=args.answer,
                time=args.card_time,
                follows=args.follows,
                draft=args.as_draft,
            )
        )
    except api.ApiError as exc:
        print(exc, file=sys.stderr)
        return 1
    print(f"Made the card {made.id} in {tilde(made.path)}.")
    if not args.answer:
        if not _interactive():
            print("It has no answer yet: open it in an editor to write one.")
            return 0
        failed = run_editor(made.path)
        if failed:
            print(f"{failed}; the card is as it was made.")
            return 1
    problems = engine.deck_check(api.DeckParams(deck=args.deck)).problems
    for problem in problems:
        print(f"  {problem}")
    return 1 if problems else 0


def run_editor(path):
    """The file in the user's editor. "" when it ran and exited cleanly, else
    what went wrong, phrased to finish a sentence."""
    editor = os.environ.get("VISUAL") or os.environ.get("EDITOR") or "vi"
    try:
        result = subprocess.run(
            [*shlex.split(editor), str(path)], stdin=_child_stdin(), check=False
        )
    except OSError as exc:
        return f"Could not run {editor!r}: {exc}"
    if result.returncode != 0:
        return f"{editor} exited with {result.returncode}"
    return ""


def edit_card(engine):
    """The card's file in the user's editor, then read back into the run. True
    only if the engine saw the file change."""
    try:
        path = Path(engine.card_edit(api.NoParams()).path)
    except api.ApiError as exc:
        print(f"{IND}{exc}")
        return False

    if failed := run_editor(path):
        print(f"{IND}{failed}; taking no edit.")
        return False

    reloaded = engine.card_reload(api.NoParams())
    if not reloaded.changed:
        print(f"{IND}Unchanged.")
        return False
    print()
    print(clean(reloaded.card.title))
    reveal(reloaded.card)
    return True


def _child_stdin():
    """The editor reads the terminal the prompts read, not a stdin that may be
    a pipe. None - inherit - where the prompts are not reading a real file."""
    try:
        _TTY.fileno()
    except (AttributeError, OSError, ValueError):
        return None
    return _TTY


def edit_line(entry):
    if entry.kind == "remove":
        return (
            f"remove  {entry.at}  {entry.text}" if entry.text else f"remove  {entry.at}"
        )
    return f"note    {entry.at}  {entry.text}"


def edit_summary(done):
    parts = [
        f"{count} {label}"
        for count, label in (
            (done.edited, "edited"),
            (done.noted, "noted"),
            (done.flagged, "flagged for removal"),
            (done.cleared, "cleared"),
        )
        if count
    ]
    if not parts:
        return
    print(f"  {', '.join(parts)} - in {tilde(done.edits)}")
    print("  --edits lists everything still standing.")
    print()


def edit_report(listed):
    print()
    if not listed.items:
        print(f"Nothing standing against any card in {listed.name}.")
        print()
        orphan_note(listed.orphaned)
        return 0

    print(
        f"{listed.name}, the edit list - {len(listed.items)} card(s), "
        f"from {tilde(listed.log)}"
    )
    for item in listed.items:
        print()
        print(f"  {clean(item.title)}")
        print(f"    {tilde(item.path)}")
        for entry in item.entries:
            print(f"    {edit_line(entry)}")
    print()
    print("  Editing a card from an -e run, or d)one there, clears what stands.")
    print()
    orphan_note(listed.orphaned)
    return 0


def orphan_note(gone):
    """As with retired attempts: a renamed card takes its notes with it, and
    those notes are still worth reading."""
    if not gone:
        return
    print(f"  {len(gone)} card(s) with notes are no longer in the bank:")
    for card in gone:
        print(f"    {card}")
    print()


# -- the Leitner boxes ---------------------------------------------------


def box_report(listed):
    print()
    print(f"{listed.name}, the Leitner boxes")
    if listed.log is None:
        print("  (logging is off, so every card counts as new)")
    print()
    print(f"  {'':<6} {'every':>7}  {'cards':>5}  {'due':>4}")
    print(f"  {'new':<6} {'':>7}  {listed.new:>5}  {listed.new:>4}")
    for row in listed.boxes:
        every = f"{row.days}d"
        print(f"  {f'box {row.box}':<6} {every:>7}  {row.cards:>5}  {row.due:>4}")
    print()
    if listed.due:
        print(f"  {listed.due} card(s) due today - -l drills exactly those.")
    elif listed.next:
        print(f"  Nothing due today. The next card is due {listed.next}.")
    else:
        print("  The deck has no cards.")
    print()
    return 0


# -- the miss report -----------------------------------------------------


def report(listed, show_all):
    if not listed.attempts:
        print(
            "\nNothing logged yet for {}{}.\n".format(
                listed.name, "" if listed.log else " (logging is off)"
            )
        )
        return 0

    print()
    if not listed.records:
        print("No misses standing - every card you have attempted, you last got right.")
        print()
        retired_note(listed.retired)
        return 0

    which = "every card attempted" if show_all else "the miss list"
    print(
        f"{listed.name}, {which} - {listed.attempts} attempt(s) logged in "
        f"{tilde(listed.log)}"
    )
    print()
    for rec in listed.records:
        state = rec.score + (", retired" if rec.retired else "")
        counts = f"{rec.misses:>2}/{rec.attempts:<2}"
        print(f"  {state:<7} {counts}  {rec.seen}  {rec.title}")
    print()
    print("  columns: last score, misses/attempts, last seen, question")
    if not show_all:
        print("  -w drills exactly this list; answering one well clears it.")

    print()
    retired_note(listed.retired)
    return 0


def retired_note(gone):
    """Named rather than dropped: a card that was renamed takes its history out
    of every other view here, and silence would read as "you never missed it"."""
    if not gone:
        return
    print(
        f"  {len(gone)} logged card(s) are no longer in the bank - renamed or deleted:"
    )
    for card in gone:
        print(f"    {card}")
    print()


_TTY = None


def list_decks():
    """What `lernt` will accept as a DECK. A deck that will not load is still
    listed, with its error, because a broken deck you can see beats one that is
    simply absent from the list."""
    rows = config_mod.listing()
    home = config_mod.decks_home()
    print()
    if not rows:
        print(f"No decks in {tilde(home)}.")
        print()
        print(config_mod.how_to())
        print()
        return 0
    print(f"Decks in {tilde(home)}:")
    print()
    for deck_id, config, error in rows:
        if config is None:
            print(f"  {deck_id:<12} [unusable] {error}")
            continue
        banks = ", ".join(tilde(d) for d in config.cards)
        print(f"  {deck_id:<12} {config.name}")
        if config.description:
            print(f"  {'':<12}   {config.description}")
        print(f"  {'':<12}   {banks}")
        print(f"  {'':<12}   log: {tilde(config.log_path)}")
    print()
    return 0


def launch_tui():
    """`lernt` with nothing after it. The TUI is a separate program driving the
    same API over JSON-RPC, so it is exec'd rather than imported: it owns the terminal,
    and its exit status is the command's. It is told how to run the engine
    rather than left to find it, so it needs nothing on PATH - and replacing
    tui/ with another frontend changes nothing on this side."""
    if not _interactive():
        return list_decks()
    if not TUI.is_file():
        list_decks()
        print(f"The TUI is not built: `make tui` in {tilde(TUI.parents[2])}.")
        print("`lernt DECK` runs a deck in this terminal meanwhile.")
        print()
        return 0
    env = {
        **os.environ,
        "LERNT_ENGINE": json.dumps([sys.executable, "-m", "lernt.deck"]),
    }
    try:
        os.execve(TUI, [str(TUI)], env)
    except OSError as exc:
        print(f"Could not start the TUI at {tilde(TUI)}: {exc}", file=sys.stderr)
        return 1
    return 0  # only reached when execve is replaced, as the tests do


def _interactive():
    return sys.stdin.isatty() and sys.stdout.isatty()


def first_not_none(*values):
    for value in values:
        if value is not None:
            return value
    return None


def limit_arg(text):
    """-t: seconds, "off" (0: untimed) or "auto" (each card's own time)."""
    if text == "auto":
        return None
    if text == "off":
        return 0
    try:
        seconds = int(text)
    except ValueError:
        seconds = -1
    if seconds < 0:
        raise argparse.ArgumentTypeError("takes seconds, off or auto")
    return seconds


def time_arg(text):
    """--time: seconds, or "off" (0: untimed)."""
    if text == "off":
        return 0
    try:
        seconds = int(text)
    except ValueError:
        seconds = -1
    if seconds < 0:
        raise argparse.ArgumentTypeError("takes seconds or off")
    return seconds


def prompt(text):
    """None on Ctrl-D. Reads the tty rather than stdin, so a deck piped in on
    stdin still takes its answers from the person; falls back to stdin where
    there is no controlling terminal, which is how the tests drive a run."""
    global _TTY
    if _TTY is None:
        try:
            # Held open for the life of the run rather than per prompt, and
            # so deliberately outside a context manager.
            _TTY = Path("/dev/tty").open()  # noqa: SIM115
        except OSError:
            _TTY = sys.stdin
    sys.stdout.write(text)
    sys.stdout.flush()
    line = _TTY.readline()
    if not line:
        print()
        return None
    return line.rstrip("\n")


def tilde(path):
    """`~/.local/state/...` rather than the full path. Cosmetic, and only for
    display - nothing round-trips through this."""
    path = Path(path)
    try:
        return f"~/{path.relative_to(Path.home())}"
    except ValueError:
        return str(path)
