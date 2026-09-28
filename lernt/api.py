"""The engine's API: everything a frontend can ask of lernt, as typed calls.

Each operation is a method on `Engine` taking one params dataclass and returning
one result dataclass, registered under a dotted name (`run.start`). That is the
whole contract. `rpc` serves it over stdio as JSON-RPC and derives a JSON Schema
from these very annotations; `cli` calls it in-process. Neither adds a rule of
its own, so the terminal walk and the TUI cannot drift apart on what a run is.

A run is stateful and there is one at a time: `run.start` opens it, the card
calls act on the card it is on, `run.end` closes it. Card contents are the
Markdown they were written in - rendering is still the frontend's business.
"""

import datetime
import time
from dataclasses import dataclass, field

from . import author, leitner
from . import config as config_mod
from . import deck as deck_mod
from . import edits as edits_mod
from . import misses as misses_mod
from . import options as options_mod
from .session import SCORES, Session, SessionError
from .text import clean

PROTOCOL = "lernt.api/7"

# Error codes, beside JSON-RPC's own negative ones. A frontend branches on the
# code and shows the message.
NO_DECK = 1  # the deck does not exist or will not load
NO_CARDS = 2  # the options left nothing to drill
NO_MISSES = 3  # a weak run with an empty miss list
NOT_NOW = 4  # the call does not fit where the run is
INVALID = 5  # the call's values are wrong (a score that is not a score)
NO_DUE = 6  # a due run with nothing due today

METHODS = {}


def method(name):
    def register(fn):
        METHODS[name] = fn
        return fn

    return register


class ApiError(Exception):
    """Phrased for the user, with a code for the frontend to branch on."""

    def __init__(self, code, message):
        super().__init__(message)
        self.code = code


# -- types -----------------------------------------------------------------


@dataclass
class NoParams:
    pass


@dataclass
class Info:
    protocol: str
    version: str


@dataclass
class DeckStats:
    """Where a deck stands, for a deck list to show without opening a report."""

    size: int  # cards in the deck
    new: int  # never attempted
    boxes: list[int]  # cards in each Leitner box, box 1 first
    due: int  # due today, new cards included
    missed: int  # on the miss list
    last_run: str  # the day of the last logged attempt; "": never


@dataclass
class DeckInfo:
    """A deck in the decks directory. One that will not load has only id, path
    and error."""

    id: str
    path: str | None = None
    name: str = ""
    description: str = ""  # what the deck is, in a line or two; "": none given
    cards: list[str] = field(default_factory=list)
    count: int | None = None
    limit: int | None = None
    boxes: list[int] = field(default_factory=list)  # days between reviews, per box
    log: str = ""
    edits: str = ""
    error: str = ""
    stats: DeckStats | None = None  # None: the deck or its cards will not load


@dataclass
class DeckList:
    home: str
    decks: list[DeckInfo]


@dataclass
class StartParams:
    deck: str
    count: int | None = None  # None: the deck's default, else 10. 0: no end
    limit: int | None = None  # seconds for every card; 0: untimed; None: per card
    auto: bool = False
    drafts: bool = False
    filter: str = ""
    weak: bool = False
    due: bool = False  # only the cards the Leitner schedule has due, lowest box first
    edit: bool = False
    log: bool = True


@dataclass
class Question:
    id: str
    title: str
    number: int  # its place in the run, from 1
    answer: list[str]
    notes: list[str]
    reference: str
    draft: bool
    limit: int  # the seconds this card gets; 0: untimed
    follows_from: str  # the parent's title, on a card reached by a random jump
    box: int  # its Leitner box; 0: new, never attempted


@dataclass
class Started:
    deck: str
    name: str
    pool: int
    count: int
    limit: int | None  # one limit for every card; None: each card's own
    default_limit: int  # with limit None, for a card with no time; 0: untimed
    timed: int  # cards in the pool with a time of their own
    auto: bool
    edit: bool
    weak: bool
    due: bool
    log: str | None  # None: not recording
    card: Question


@dataclass
class ScoreParams:
    score: str  # pass, shaky or fail
    seconds: int


@dataclass
class EditEntry:
    kind: str
    at: str
    text: str


@dataclass
class Scored:
    finished: bool
    over_limit: bool
    standing: list[EditEntry]  # only filled in an edit-mode run
    was: int  # the card's Leitner box before this score; 0: new
    box: int  # and after it


@dataclass
class MenuItem:
    id: str
    title: str
    draft: bool


@dataclass
class Menu:
    followups: list[MenuItem]


@dataclass
class NextParams:
    pick: str  # auto (follow a branch if one is left), random, or followup
    card: str = ""  # with followup: which one


@dataclass
class Review:
    score: str
    title: str


@dataclass
class Summary:
    passed: int
    shaky: int
    failed: int
    asked: int
    ended_early: bool
    review: list[Review]
    logged: int
    log: str | None
    promoted: int  # cards this run moved up a Leitner box
    demoted: int  # and down
    edited: int
    noted: int
    flagged: int
    cleared: int
    edits: str


@dataclass
class CardFile:
    path: str


@dataclass
class Reloaded:
    changed: bool
    card: Question
    standing: list[EditEntry]


@dataclass
class NoteParams:
    text: str


@dataclass
class FlagParams:
    reason: str = ""


@dataclass
class Standing:
    standing: list[EditEntry]


@dataclass
class MissParams:
    deck: str
    all: bool = False


@dataclass
class MissRecord:
    card: str
    title: str
    score: str
    misses: int
    attempts: int
    seen: str  # date of the last attempt
    retired: bool  # no longer in the deck


@dataclass
class MissReport:
    name: str
    log: str | None
    attempts: int
    records: list[MissRecord]
    retired: list[str]


@dataclass
class DeckParams:
    deck: str


@dataclass
class BoxRow:
    box: int
    days: int  # between reviews in this box
    cards: int
    due: int  # of those, due today


@dataclass
class BoxReport:
    name: str
    log: str | None
    boxes: list[BoxRow]
    new: int  # never attempted: due, and on their way into box 1
    due: int  # due today, new cards included
    next: str  # with nothing due, the day something is; else ""


@dataclass
class EditItem:
    card: str
    title: str
    path: str
    entries: list[EditEntry]


@dataclass
class EditReport:
    name: str
    log: str
    items: list[EditItem]
    orphaned: list[str]


@dataclass
class Option:
    key: str
    value: str


@dataclass
class Options:
    deck: str
    saved: list[Option]  # by key; empty: nothing saved, use the defaults


@dataclass
class SaveOptionsParams:
    deck: str
    options: list[Option]  # merged into what is saved; keys not sent are kept


@dataclass
class Settings:
    """A frontend's settings for lernt as a whole, as `Options` are for a deck."""

    saved: list[Option]  # by key; empty: nothing saved, use the defaults


@dataclass
class SaveSettingsParams:
    settings: list[Option]  # merged into what is saved; keys not sent are kept


@dataclass
class NewDeckParams:
    name: str  # the deck's title; its id is made from it
    description: str = ""


@dataclass
class NewDeck:
    id: str
    path: str


@dataclass
class NewCardParams:
    deck: str
    title: str  # the question; the card's id is made from it
    answer: str = ""
    notes: str = ""
    reference: str = ""
    time: int | None = None  # seconds of its own; 0: untimed; None: the run's
    follows: list[str] = field(default_factory=list)  # ids of cards it leads to
    draft: bool = False


@dataclass
class NewCard:
    id: str
    path: str  # the file written, for a frontend to open in an editor


@dataclass
class CardSummary:
    id: str
    title: str
    draft: bool


@dataclass
class CardList:
    deck: str
    cards: list[CardSummary]


@dataclass
class Problems:
    """What a deck's cards say is wrong with them: dangling follow-ups, times
    that are not durations, an id two banks both hold."""

    deck: str
    problems: list[str]


# -- the engine ------------------------------------------------------------


class _Run:
    def __init__(
        self, config, session, started_with, log_path, limit, default_limit, boxes
    ):
        self.id = time.strftime("%Y-%m-%dT%H:%M:%S")
        self.config = config
        self.session = session
        self.options = started_with
        self.log_path = log_path
        self.limit = limit
        self.default_limit = default_limit
        self.current = None
        self.followed = False
        self.scored = False
        self.logged = 0
        self.boxes = boxes  # card id -> Leitner record, kept up with each score
        self.moved = {"up": 0, "down": 0}
        self.edited = dict.fromkeys(edits_mod.KINDS, 0)
        self.before = None  # the card file's text when it was handed out


class Engine:
    def __init__(self, rng=None, today=None):
        self.rng = rng
        self.today = today or datetime.date.today
        self.run = None

    @method("engine.info")
    def info(self, params: NoParams) -> Info:
        from . import __version__

        return Info(protocol=PROTOCOL, version=__version__)

    @method("decks.list")
    def decks_list(self, params: NoParams) -> DeckList:
        listed = deck_mod.decks()
        rows = [DeckInfo(**row) for row in listed["decks"]]
        for row in rows:
            if not row.error:
                row.stats = deck_stats(_config(row.id), self.today())
        return DeckList(home=listed["home"], decks=rows)

    # -- a run -------------------------------------------------------------

    @method("run.start")
    def run_start(self, params: StartParams) -> Started:
        config, deck = open_deck(params.deck)
        return self.begin(config, deck, params, config.log_path if params.log else None)

    def begin(self, config, deck, options, log_path):
        """`run.start` for a deck already in hand - how the CLI runs a deck
        read from JSON or from bare directories, with a log of its choosing."""
        if options.count is not None and options.count < 0:
            raise ApiError(INVALID, "The count takes a number (0 = until you stop it).")
        count = _first(options.count, config.count, 10)
        if options.limit is not None and options.limit < 0:
            raise ApiError(INVALID, "The limit takes seconds (0 = untimed).")
        default_limit = _first(config.limit, 30)

        if options.weak and options.due:
            raise ApiError(
                INVALID, "A run is the miss list or the due cards, not both."
            )
        entries = misses_mod.load(log_path) if log_path else []
        known = {c["id"] for c in deck["cards"]}
        boxes = leitner.replay(entries, config.boxes)

        only = rank = None
        if options.weak:
            only = {r["card"] for r in misses_mod.missed(entries, known)}
            if not only:
                raise ApiError(
                    NO_MISSES,
                    "Nothing is on the miss list - every card you have "
                    "attempted, you last got right.",
                )
        if options.due:
            only = leitner.due(boxes, known, self.today(), config.boxes)
            if not only:
                when = leitner.next_due(boxes, known, config.boxes)
                raise ApiError(
                    NO_DUE, f"Nothing is due today. The next card is due {when}."
                )
            # New cards go in with box 1: they are about to enter it.
            rank = {c: max(leitner.box_of(boxes.get(c)), 1) for c in only}
        try:
            session = Session(
                deck,
                count=count,
                include_drafts=options.drafts,
                name_filter=options.filter,
                only=only,
                rank=rank,
                rng=self.rng,
            )
        except SessionError as exc:
            raise ApiError(NO_CARDS, str(exc)) from exc

        self.run = _Run(
            config, session, options, log_path, options.limit, default_limit, boxes
        )
        card = self._present(session.start(), followed=False)
        return Started(
            deck=config.id,
            name=config.name,
            pool=len(session.pool),
            count=count,
            limit=options.limit,
            default_limit=default_limit,
            timed=sum(
                1 for c in session.pool if session.cards[c].get("time") is not None
            ),
            auto=options.auto,
            edit=options.edit,
            weak=options.weak,
            due=options.due,
            log=str(log_path) if log_path else None,
            card=card,
        )

    @method("run.score")
    def run_score(self, params: ScoreParams) -> Scored:
        run = self._running()
        if run.scored:
            raise ApiError(NOT_NOW, "This card is already scored.")
        if params.score not in SCORES:
            raise ApiError(INVALID, f"A score is one of {', '.join(SCORES)}.")
        if params.seconds < 0:
            raise ApiError(INVALID, "Seconds cannot be negative.")
        session, card = run.session, run.current
        session.score(card, params.score)
        run.scored = True
        before = run.boxes.get(card)
        after = leitner.step(before, params.score, self.today(), run.config.boxes)
        run.boxes[card] = after
        was, box = leitner.box_of(before), leitner.box_of(after)
        if box > max(was, 1):
            run.moved["up"] += 1
        elif box < was:
            run.moved["down"] += 1
        if run.log_path:
            misses_mod.append(
                run.log_path,
                card,
                clean(session.cards[card]["title"]),
                params.score,
                params.seconds,
                run=run.id,
                reached="branch" if run.followed else "random",
            )
            run.logged += 1
        return Scored(
            finished=session.finished(),
            over_limit=_over(params.seconds, self._limit(card)),
            standing=self._standing() if run.options.edit else [],
            was=was,
            box=box,
        )

    @method("run.menu")
    def run_menu(self, params: NoParams) -> Menu:
        run = self._running()
        cards = run.session.cards
        return Menu(
            followups=[
                MenuItem(
                    id=t, title=cards[t]["title"], draft=bool(cards[t].get("draft"))
                )
                for t in run.session.followups(run.current)
            ]
        )

    @method("run.next")
    def run_next(self, params: NextParams) -> Question:
        run = self._running()
        session = run.session
        if not run.scored:
            raise ApiError(NOT_NOW, "Score this card before moving on.")
        if session.finished():
            raise ApiError(NOT_NOW, "The run is over.")
        if params.pick == "auto":
            card, followed = session.pick_next(run.current)
        elif params.pick == "random":
            card, followed = session.pick_random(), False
        elif params.pick == "followup":
            if params.card not in session.followups(run.current):
                raise ApiError(INVALID, f"{params.card!r} does not follow this card.")
            card, followed = params.card, True
        else:
            raise ApiError(INVALID, "pick is one of auto, random, followup.")
        return self._present(card, followed)

    @method("run.end")
    def run_end(self, params: NoParams) -> Summary:
        run = self._running()
        session = run.session
        self.run = None
        return Summary(
            passed=session.scores["pass"],
            shaky=session.scores["shaky"],
            failed=session.scores["fail"],
            asked=session.asked,
            ended_early=not (session.finished() and run.scored),
            review=[Review(score=s, title=t) for s, t in session.review],
            logged=run.logged,
            log=str(run.log_path) if run.log_path else None,
            promoted=run.moved["up"],
            demoted=run.moved["down"],
            edited=run.edited["edit"],
            noted=run.edited["note"],
            flagged=run.edited["remove"],
            cleared=run.edited["done"],
            edits=str(run.config.edits_path),
        )

    # -- the card the run is on ----------------------------------------------

    @method("card.edit")
    def card_edit(self, params: NoParams) -> CardFile:
        """The file to open in an editor. The frontend runs the editor - it
        owns the terminal - and calls `card.reload` when it exits."""
        run = self._running()
        path = deck_mod.card_path(run.session.cards[run.current])
        if not path.is_file():
            raise ApiError(NOT_NOW, f"Cannot find the card file at {path}.")
        run.before = path.read_text(encoding="utf-8")
        return CardFile(path=str(path))

    @method("card.reload")
    def card_reload(self, params: NoParams) -> Reloaded:
        """Read the card back after `card.edit`. Only a file that actually
        changed is an edit: an editor quit without saving must not clear a note
        that is still true."""
        run = self._running()
        if run.before is None:
            raise ApiError(NOT_NOW, "The card was not handed out for editing.")
        session = run.session
        card = session.cards[run.current]
        path = deck_mod.card_path(card)
        before, run.before = run.before, None
        changed = path.is_file() and path.read_text(encoding="utf-8") != before
        if changed:
            fresh = deck_mod.reload(card, session.cards)
            session.update(fresh)
            self._log_edit("edit")
        return Reloaded(
            changed=changed, card=self._question(run.current), standing=self._standing()
        )

    @method("card.note")
    def card_note(self, params: NoteParams) -> Standing:
        if not params.text.strip():
            raise ApiError(INVALID, "Nothing noted.")
        self._log_edit("note", params.text)
        return Standing(standing=self._standing())

    @method("card.flag")
    def card_flag(self, params: FlagParams) -> Standing:
        self._log_edit("remove", params.reason)
        return Standing(standing=self._standing())

    @method("card.done")
    def card_done(self, params: NoParams) -> Standing:
        if not self._standing():
            raise ApiError(NOT_NOW, "Nothing stands against this card.")
        self._log_edit("done")
        return Standing(standing=[])

    # -- reports -----------------------------------------------------------

    @method("report.misses")
    def report_misses(self, params: MissParams) -> MissReport:
        config, deck = open_deck(params.deck)
        return miss_report(config.name, config.log_path, deck, params.all)

    @method("report.boxes")
    def report_boxes(self, params: DeckParams) -> BoxReport:
        config, deck = open_deck(params.deck)
        return box_report(
            config.name, config.log_path, deck, config.boxes, self.today()
        )

    @method("report.edits")
    def report_edits(self, params: DeckParams) -> EditReport:
        config, deck = open_deck(params.deck)
        return edit_report(config.name, config.edits_path, deck)

    # -- a frontend's saved options -----------------------------------------

    @method("options.load")
    def options_load(self, params: DeckParams) -> Options:
        config = _config(params.deck)
        return _options(config.id, options_mod.load(config.options_path))

    @method("options.save")
    def options_save(self, params: SaveOptionsParams) -> Options:
        config = _config(params.deck)
        if any(not option.key for option in params.options):
            raise ApiError(INVALID, "An option needs a key.")
        values = {option.key: option.value for option in params.options}
        return _options(config.id, options_mod.save(config.options_path, values))

    # -- a frontend's saved settings ------------------------------------------

    @method("settings.load")
    def settings_load(self, params: NoParams) -> Settings:
        return Settings(saved=_saved(options_mod.load(config_mod.settings_path())))

    @method("settings.save")
    def settings_save(self, params: SaveSettingsParams) -> Settings:
        if any(not setting.key for setting in params.settings):
            raise ApiError(INVALID, "A setting needs a key.")
        values = {setting.key: setting.value for setting in params.settings}
        return Settings(
            saved=_saved(options_mod.save(config_mod.settings_path(), values))
        )

    # -- writing decks and cards -----------------------------------------------

    @method("deck.create")
    def deck_create(self, params: NewDeckParams) -> NewDeck:
        """A new, empty deck in the decks directory, its cards beside its
        `deck.toml`. Its id is made from its name, and an id in use is refused."""
        try:
            path = author.create_deck(
                config_mod.decks_home(), params.name, params.description
            )
        except (author.AuthorError, OSError) as exc:
            raise ApiError(INVALID, str(exc)) from exc
        return NewDeck(id=path.name, path=str(path))

    @method("card.create")
    def card_create(self, params: NewCardParams) -> NewCard:
        """A new card, written into the deck's first card bank. A follow-up has to
        be a card the deck already has, so no link is left dangling by a typo."""
        config, deck = open_deck(params.deck)
        if not config.cards:
            raise ApiError(INVALID, f"{config.name} has no card bank to write to.")
        known = {card["id"]: card["title"] for card in deck["cards"]}
        try:
            card_id, path = author.create_card(
                config.cards[0],
                known,
                params.title,
                params.answer,
                params.notes,
                params.reference,
                params.time,
                params.follows,
                params.draft,
            )
        except (author.AuthorError, OSError) as exc:
            raise ApiError(INVALID, str(exc)) from exc
        return NewCard(id=card_id, path=str(path))

    @method("card.list")
    def card_list(self, params: DeckParams) -> CardList:
        """A deck's cards, for choosing what a new card should lead to."""
        config, deck = open_deck(params.deck)
        return CardList(
            deck=config.id,
            cards=[
                CardSummary(id=c["id"], title=c["title"], draft=c["draft"])
                for c in deck["cards"]
            ],
        )

    @method("deck.check")
    def deck_check(self, params: DeckParams) -> Problems:
        """What is wrong with a deck's cards, as `lernt-deck --check` lists it. A
        frontend runs it after an editor closes on a card it just made."""
        config, deck = open_deck(params.deck)
        return Problems(deck=config.id, problems=list(deck["problems"]))

    # -- helpers -----------------------------------------------------------

    def _running(self):
        if self.run is None:
            raise ApiError(NOT_NOW, "No run is open.")
        return self.run

    def _present(self, card, followed):
        run = self.run
        run.session.visit(card)
        run.current, run.followed, run.scored, run.before = card, followed, False, None
        return self._question(card)

    def _question(self, card_id):
        run = self.run
        session = run.session
        card = session.cards[card_id]
        parent = None if run.followed else session.parent(card_id)
        return Question(
            id=card_id,
            title=card["title"],
            number=len(session.seen),
            answer=list(card["answer"]),
            notes=list(card["notes"]),
            reference=card.get("reference", ""),
            draft=bool(card.get("draft")),
            limit=self._limit(card_id),
            follows_from=session.cards[parent]["title"] if parent else "",
            box=leitner.box_of(run.boxes.get(card_id)),
        )

    def _limit(self, card_id):
        """The seconds a card gets: the run's limit if it set one, else the
        card's own time, else the deck's. 0 is untimed."""
        run = self.run
        if run.limit is not None:
            return run.limit
        own = run.session.cards[card_id].get("time")
        return run.default_limit if own is None else own

    def _standing(self):
        run = self._running()
        rows = edits_mod.standing(edits_mod.load(run.config.edits_path))
        return [_entry(row) for row in rows.get(run.current, [])]

    def _log_edit(self, kind, text=""):
        run = self._running()
        edits_mod.append(
            run.config.edits_path,
            run.current,
            clean(run.session.cards[run.current]["title"]),
            kind,
            text.strip(),
            run=run.id,
        )
        run.edited[kind] += 1


# -- shared by the RPC calls and the CLI --------------------------------------


def _config(wanted):
    try:
        return config_mod.resolve(wanted)
    except config_mod.ConfigError as exc:
        raise ApiError(NO_DECK, str(exc)) from exc


def _options(deck_id, values):
    return Options(deck=deck_id, saved=_saved(values))


def _saved(values):
    return [Option(key=k, value=values[k]) for k in sorted(values)]


def open_deck(wanted):
    try:
        config = config_mod.resolve(wanted)
        return config, deck_mod.build(config)
    except (config_mod.ConfigError, deck_mod.DeckError, OSError) as exc:
        raise ApiError(NO_DECK, str(exc)) from exc


def miss_report(name, log_path, deck, show_all):
    """What you have been getting wrong - or with `show_all`, every card
    attempted, most-missed first. A logged card that has left the deck is
    marked retired rather than dropped."""
    entries = misses_mod.load(log_path) if log_path else []
    known = {c["id"] for c in deck["cards"]}
    if show_all:
        records = sorted(
            misses_mod.history(entries).values(),
            key=lambda r: (-r["misses"], r["title"]),
        )
    else:
        records = misses_mod.missed(entries, known)
    return MissReport(
        name=name,
        log=str(log_path) if log_path else None,
        attempts=len(entries),
        records=[
            MissRecord(
                card=r["card"],
                title=r["title"],
                score=r["last"].get("score", "?"),
                misses=r["misses"],
                attempts=r["attempts"],
                seen=(r["last"].get("at") or "")[:10],
                retired=r["card"] not in known,
            )
            for r in records
        ],
        retired=misses_mod.retired(entries, known),
    )


def box_report(name, log_path, deck, intervals, today):
    """Where every card in the deck stands in the Leitner schedule. Cards that
    have left the deck are not counted: they will never come due again."""
    entries = misses_mod.load(log_path) if log_path else []
    state = leitner.replay(entries, intervals)
    known = {c["id"] for c in deck["cards"]}
    rows = [
        BoxRow(box=n, days=days, cards=0, due=0) for n, days in enumerate(intervals, 1)
    ]
    new = 0
    for card in known:
        record = state.get(card)
        if record is None:
            new += 1
            continue
        row = rows[record["box"] - 1]
        row.cards += 1
        if leitner.is_due(record, today, intervals):
            row.due += 1
    due = new + sum(row.due for row in rows)
    when = None if due else leitner.next_due(state, known, intervals)
    return BoxReport(
        name=name,
        log=str(log_path) if log_path else None,
        boxes=rows,
        new=new,
        due=due,
        next=when.isoformat() if when else "",
    )


def deck_stats(config, today):
    """The figures a deck list shows beside a deck: its size, its boxes, what is
    due and missed, and when it last ran. None if its cards will not load."""
    try:
        deck = deck_mod.build(config)
    except (deck_mod.DeckError, OSError):
        return None
    entries = misses_mod.load(config.log_path) if config.log_path else []
    known = {c["id"] for c in deck["cards"]}
    state = leitner.replay(entries, config.boxes)
    boxes = [0] * len(config.boxes)
    for card in known & state.keys():
        boxes[state[card]["box"] - 1] += 1
    days = [day for day in map(leitner.day_of, entries) if day]
    return DeckStats(
        size=len(known),
        new=len(known) - sum(boxes),
        boxes=boxes,
        due=len(leitner.due(state, known, today, config.boxes)),
        missed=len(misses_mod.missed(entries, known)),
        last_run=max(days).isoformat() if days else "",
    )


def edit_report(name, edits_path, deck):
    """The notes and removal flags still standing, with the file to open."""
    cards = deck_mod.index(deck)
    entries = edits_mod.load(edits_path)
    return EditReport(
        name=name,
        log=str(edits_path),
        items=[
            EditItem(
                card=card_id,
                title=cards[card_id]["title"],
                path=str(deck_mod.card_path(cards[card_id])),
                entries=[_entry(row) for row in rows],
            )
            for card_id, rows in edits_mod.outstanding(entries, cards)
        ],
        orphaned=edits_mod.orphaned(entries, cards),
    )


def _entry(row):
    return EditEntry(
        kind=row["kind"], at=(row.get("at") or "")[:10], text=row.get("text") or ""
    )


def _over(seconds, limit):
    return 0 < limit < seconds


def _first(*values):
    return next((v for v in values if v is not None), None)
