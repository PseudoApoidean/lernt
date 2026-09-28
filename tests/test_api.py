"""The API as a frontend drives it: calls in, typed results out, no terminal."""

import datetime
import random

import pytest

from lernt import api, edits, misses

FIRST = """# Does volatile make something thread-safe?

> [!tip]
> No - it is about the compiler, not atomicity.

A 32-bit read on a 16-bit machine is two instructions.

---

**Follow-ups:**
1. [[Second card|How would you fix it?]]
"""

SECOND = """# How would you fix it?

> [!tip]
> Disable the interrupt across the read.
"""

DRAFT = """# Not ready yet?

Half an answer.

---

#draft
"""


@pytest.fixture
def home(tmp_path, monkeypatch):
    monkeypatch.setenv("LERNT_DECKS", str(tmp_path / "decks"))
    monkeypatch.setenv("XDG_STATE_HOME", str(tmp_path / "state"))
    deck = tmp_path / "decks" / "embedded"
    deck.mkdir(parents=True)
    (deck / "deck.toml").write_text('[deck]\nname = "Embedded screens"\n')
    for name, text in (
        ("First card", FIRST),
        ("Second card", SECOND),
        ("Draft", DRAFT),
    ):
        (deck / f"{name}.md").write_text(text)
    return tmp_path


@pytest.fixture
def engine(home):
    return api.Engine(rng=random.Random(0))


def log(home):
    return home / "state" / "lernt" / "embedded.jsonl"


def edit_log(home):
    return home / "state" / "lernt" / "embedded.edits.jsonl"


def start(engine, **options):
    return engine.run_start(api.StartParams(deck="embedded", **options))


def code(exc_info):
    return exc_info.value.code


def test_a_run_is_started_scored_followed_and_ended(engine, home):
    started = start(engine, count=2, filter="card")
    assert (
        started.name,
        started.pool,
        started.count,
        started.limit,
        started.default_limit,
    ) == ("Embedded screens", 2, 2, None, 30)
    first = started.card
    assert first.number == 1

    scored = engine.run_score(api.ScoreParams(score="fail", seconds=41))
    assert (scored.finished, scored.over_limit, scored.standing) == (False, True, [])

    if first.id == "First card":
        (item,) = engine.run_menu(api.NoParams()).followups
        assert (item.id, item.draft) == ("Second card", False)
        second = engine.run_next(api.NextParams(pick="followup", card="Second card"))
    else:
        second = engine.run_next(api.NextParams(pick="random"))
    assert second.number == 2
    assert engine.run_score(api.ScoreParams(score="pass", seconds=3)).finished

    with pytest.raises(api.ApiError) as over:
        engine.run_next(api.NextParams(pick="random"))
    assert code(over) == api.NOT_NOW

    done = engine.run_end(api.NoParams())
    assert (done.passed, done.failed, done.asked, done.ended_early) == (1, 1, 2, False)
    assert done.review[0].score == "fail"
    assert done.logged == 2
    assert [e["score"] for e in misses.load(log(home))] == ["fail", "pass"]
    assert engine.run is None


def test_a_card_reached_by_a_random_jump_says_what_it_follows(engine):
    card = start(engine, count=0, filter="Second").card
    assert card.follows_from == "Does volatile make something thread-safe?"


def test_a_followed_branch_does_not_repeat_its_parent(engine):
    start(engine, count=0, filter="card")
    engine.run.current = "First card"  # pin the walk to the branching card
    engine.run_score(api.ScoreParams(score="pass", seconds=1))
    card = engine.run_next(api.NextParams(pick="followup", card="Second card"))
    assert card.follows_from == ""


def test_the_run_refuses_calls_out_of_order(engine):
    with pytest.raises(api.ApiError) as none_open:
        engine.run_score(api.ScoreParams(score="pass", seconds=1))
    assert code(none_open) == api.NOT_NOW

    start(engine)
    with pytest.raises(api.ApiError) as unscored:
        engine.run_next(api.NextParams(pick="random"))
    assert code(unscored) == api.NOT_NOW

    engine.run_score(api.ScoreParams(score="pass", seconds=1))
    with pytest.raises(api.ApiError) as twice:
        engine.run_score(api.ScoreParams(score="pass", seconds=1))
    assert code(twice) == api.NOT_NOW


@pytest.mark.parametrize(
    ("call", "params"),
    [
        ("run_score", api.ScoreParams(score="great", seconds=1)),
        ("run_score", api.ScoreParams(score="pass", seconds=-1)),
        ("run_next", api.NextParams(pick="sideways")),
        ("run_next", api.NextParams(pick="followup", card="Draft")),
    ],
)
def test_bad_values_are_invalid(engine, call, params):
    start(engine, count=0)
    if call == "run_next":
        engine.run_score(api.ScoreParams(score="pass", seconds=1))
    with pytest.raises(api.ApiError) as bad:
        getattr(engine, call)(params)
    assert code(bad) == api.INVALID


def test_ending_before_the_last_card_is_scored_is_ending_early(engine):
    start(engine, count=1)
    assert engine.run_end(api.NoParams()).ended_early


def test_drafts_stay_out_unless_asked_for(engine):
    assert start(engine, count=0).pool == 2
    assert start(engine, count=0, drafts=True).pool == 3


def test_deck_defaults_apply_and_options_win(engine, home):
    (home / "decks" / "embedded" / "deck.toml").write_text(
        "[deck]\ncount = 4\nlimit = 45\n"
    )
    assert (start(engine).count, start(engine).default_limit) == (4, 45)
    assert start(engine, count=1, limit=5).limit == 5


@pytest.mark.parametrize(
    ("options", "expected"),
    [
        ({"weak": True}, api.NO_MISSES),
        ({"filter": "nothing matches this"}, api.NO_CARDS),
        ({"count": -1}, api.INVALID),
        ({"limit": -1}, api.INVALID),
    ],
)
def test_a_run_that_cannot_start_says_why(engine, options, expected):
    with pytest.raises(api.ApiError) as refused:
        start(engine, **options)
    assert code(refused) == expected


def test_a_weak_run_is_the_miss_list(engine, home):
    misses.append(log(home), "Second card", "How would you fix it?", "shaky", 9)
    started = start(engine, weak=True)
    assert (started.pool, started.card.id) == (1, "Second card")


def test_an_unknown_deck_is_no_deck(engine):
    with pytest.raises(api.ApiError) as missing:
        engine.run_start(api.StartParams(deck="nope"))
    assert code(missing) == api.NO_DECK


def test_a_run_without_a_log_writes_none(engine, home):
    started = start(engine, log=False)
    engine.run_score(api.ScoreParams(score="fail", seconds=1))
    assert started.log is None
    assert engine.run_end(api.NoParams()).logged == 0
    assert not log(home).exists()


# -- the card the run is on ----------------------------------------------------


def test_only_a_changed_file_is_an_edit(engine, home):
    card = start(engine, filter="Second").card
    path = engine.card_edit(api.NoParams()).path
    assert path.endswith("Second card.md")
    assert engine.card_reload(api.NoParams()).changed is False
    assert edits.load(edit_log(home)) == []

    engine.card_note(api.NoteParams(text="clarify X"))
    engine.card_edit(api.NoParams())
    with open(path, "w") as fh:
        fh.write(SECOND.replace("fix it", "fix that"))
    reloaded = engine.card_reload(api.NoParams())
    assert reloaded.changed
    assert reloaded.card.title == "How would you fix that?"
    assert reloaded.card.number == card.number
    assert reloaded.standing == []
    assert [e["kind"] for e in edits.load(edit_log(home))] == ["note", "edit"]


def test_reload_needs_the_card_handed_out_first(engine):
    start(engine)
    with pytest.raises(api.ApiError) as unasked:
        engine.card_reload(api.NoParams())
    assert code(unasked) == api.NOT_NOW


def test_notes_and_flags_stand_until_done(engine, home):
    start(engine, edit=True)
    with pytest.raises(api.ApiError) as blank:
        engine.card_note(api.NoteParams(text="   "))
    assert code(blank) == api.INVALID

    engine.card_note(api.NoteParams(text=" clarify X "))
    standing = engine.card_flag(api.FlagParams(reason="duplicate")).standing
    assert [(e.kind, e.text) for e in standing] == [
        ("note", "clarify X"),
        ("remove", "duplicate"),
    ]
    assert len(engine.run_score(api.ScoreParams(score="pass", seconds=1)).standing) == 2
    assert engine.card_done(api.NoParams()).standing == []
    with pytest.raises(api.ApiError) as nothing:
        engine.card_done(api.NoParams())
    assert code(nothing) == api.NOT_NOW

    done = engine.run_end(api.NoParams())
    assert (done.noted, done.flagged, done.cleared) == (1, 1, 1)


# -- reports -------------------------------------------------------------------


def test_the_miss_report(engine, home):
    misses.append(log(home), "First card", "Does volatile...?", "fail", 30)
    misses.append(log(home), "Second card", "How would you fix it?", "pass", 3)
    misses.append(log(home), "Renamed away", "Old?", "fail", 3)

    listed = engine.report_misses(api.MissParams(deck="embedded"))
    assert [r.card for r in listed.records] == ["First card"]
    assert (listed.attempts, listed.retired) == (3, ["Renamed away"])

    every = engine.report_misses(api.MissParams(deck="embedded", all=True))
    assert {r.card: r.retired for r in every.records} == {
        "First card": False,
        "Renamed away": True,
        "Second card": False,
    }


def test_the_edit_report(engine, home):
    edits.append(edit_log(home), "Second card", "t", "note", "clarify X")
    edits.append(edit_log(home), "Gone card", "t", "note", "still true")
    listed = engine.report_edits(api.DeckParams(deck="embedded"))
    (item,) = listed.items
    assert (item.card, item.entries[0].text) == ("Second card", "clarify X")
    assert item.path.endswith("Second card.md")
    assert listed.orphaned == ["Gone card"]


def test_decks_are_listed_with_the_broken_ones(engine, home):
    (home / "decks" / "stale").mkdir()
    listed = engine.decks_list(api.NoParams())
    rows = {d.id: d for d in listed.decks}
    assert rows["embedded"].name == "Embedded screens"
    assert rows["stale"].error


# -- time --------------------------------------------------------------------------


def test_each_card_gets_its_own_time_unless_the_run_sets_one(engine, home):
    card = home / "decks" / "embedded" / "Second card.md"
    card.write_text(SECOND + "\n---\n\n**Time:** 1m\n")
    started = start(engine, filter="Second")
    assert (started.limit, started.timed, started.card.limit) == (None, 1, 60)
    assert not engine.run_score(api.ScoreParams(score="pass", seconds=59)).over_limit
    assert start(engine, filter="Second", limit=10).card.limit == 10
    assert start(engine, filter="Second", limit=0).card.limit == 0
    assert not engine.run_score(api.ScoreParams(score="pass", seconds=999)).over_limit


def test_a_card_without_a_time_gets_the_decks(engine, home):
    (home / "decks" / "embedded" / "deck.toml").write_text("[deck]\nlimit = 0\n")
    started = start(engine, filter="First")
    assert (started.default_limit, started.timed, started.card.limit) == (0, 0, 0)
    assert not engine.run_score(api.ScoreParams(score="fail", seconds=500)).over_limit


# -- saved options ------------------------------------------------------------------


def options(listed):
    return {o.key: o.value for o in listed.saved}


def test_options_are_saved_per_deck_by_key(engine, home):
    other = home / "decks" / "capitals"
    other.mkdir()
    (other / "deck.toml").write_text("[deck]\n")
    assert engine.options_load(api.DeckParams(deck="embedded")).saved == []

    engine.options_save(
        api.SaveOptionsParams(
            deck="embedded",
            options=[api.Option("limit", "45"), api.Option("count", "12")],
        )
    )
    engine.options_save(
        api.SaveOptionsParams(deck="embedded", options=[api.Option("count", "20")])
    )
    listed = engine.options_load(api.DeckParams(deck="embedded"))
    assert [o.key for o in listed.saved] == ["count", "limit"]
    assert options(listed) == {"count": "20", "limit": "45"}
    assert (home / "state" / "lernt" / "embedded.options.json").is_file()

    assert engine.options_load(api.DeckParams(deck="capitals")).saved == []


def test_options_for_no_deck_or_with_no_key_are_refused(engine):
    with pytest.raises(api.ApiError) as missing:
        engine.options_load(api.DeckParams(deck="nope"))
    assert code(missing) == api.NO_DECK
    with pytest.raises(api.ApiError) as keyless:
        engine.options_save(
            api.SaveOptionsParams(deck="embedded", options=[api.Option("", "x")])
        )
    assert code(keyless) == api.INVALID


def test_settings_are_saved_for_lernt_as_a_whole(engine, home):
    assert engine.settings_load(api.NoParams()).saved == []
    engine.settings_save(
        api.SaveSettingsParams(settings=[api.Option("mode", "leitner")])
    )
    engine.settings_save(api.SaveSettingsParams(settings=[api.Option("other", "x")]))
    listed = engine.settings_load(api.NoParams())
    assert options(listed) == {"mode": "leitner", "other": "x"}
    assert (home / "state" / "lernt" / "settings.json").is_file()
    with pytest.raises(api.ApiError) as keyless:
        engine.settings_save(api.SaveSettingsParams(settings=[api.Option("", "x")]))
    assert code(keyless) == api.INVALID


# -- the Leitner boxes ---------------------------------------------------------

TODAY = datetime.date(2026, 9, 10)


def logged(home, card, score, on):
    """An attempt on a day of the test's choosing - misses.append stamps now."""
    path = log(home)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a") as fh:
        fh.write(
            f'{{"card": "{card}", "score": "{score}", "at": "{on}T09:00:00+0100"}}\n'
        )


@pytest.fixture
def dated(home):
    return api.Engine(rng=random.Random(0), today=lambda: TODAY)


def test_a_due_run_is_the_new_cards_and_the_ones_whose_wait_is_over(dated, home):
    logged(home, "First card", "pass", "2026-09-09")  # box 2: due the 11th
    logged(home, "Second card", "fail", "2026-09-09")  # box 1: due the 10th
    started = start(dated, due=True, drafts=True)
    assert started.due
    assert started.pool == 2  # Second card, and the never-attempted Draft
    assert started.card.box in (0, 1)


def test_a_due_run_asks_the_lowest_box_first(dated, home):
    logged(home, "First card", "pass", "2026-09-01")  # box 2, long due
    logged(home, "Second card", "fail", "2026-09-01")  # box 1, long due
    assert start(dated, due=True).card.id == "Second card"


def test_nothing_due_says_when_something_will_be(dated, home):
    for card in ("First card", "Second card", "Draft"):
        logged(home, card, "pass", "2026-09-10")
    with pytest.raises(api.ApiError) as refused:
        start(dated, due=True)
    assert code(refused) == api.NO_DUE
    assert "2026-09-12" in str(refused.value)


def test_a_run_is_not_both_the_miss_list_and_the_due_cards(dated):
    with pytest.raises(api.ApiError) as refused:
        start(dated, due=True, weak=True)
    assert code(refused) == api.INVALID


def test_a_score_moves_the_card_between_boxes_and_the_summary_counts_it(dated, home):
    logged(home, "Second card", "pass", "2026-09-01")
    logged(home, "Second card", "pass", "2026-09-03")  # box 3, due the 7th
    card = start(dated, filter="Second").card
    assert card.box == 3
    scored = dated.run_score(api.ScoreParams(score="pass", seconds=3))
    assert (scored.was, scored.box) == (3, 4)
    summary = dated.run_end(api.NoParams())
    assert (summary.promoted, summary.demoted) == (1, 0)

    # And the log it wrote replays to the same box.
    report = dated.report_boxes(api.DeckParams(deck="embedded"))
    assert [row.cards for row in report.boxes] == [0, 0, 0, 1, 0]


def test_a_fail_is_a_demotion(dated, home):
    logged(home, "Second card", "pass", "2026-09-01")
    start(dated, filter="Second")
    scored = dated.run_score(api.ScoreParams(score="fail", seconds=3))
    assert (scored.was, scored.box) == (2, 1)
    assert dated.run_end(api.NoParams()).demoted == 1


def test_the_box_report(dated, home):
    logged(home, "First card", "pass", "2026-09-09")  # box 2, due the 11th
    logged(home, "Second card", "fail", "2026-09-09")  # box 1, due the 10th
    report = dated.report_boxes(api.DeckParams(deck="embedded"))
    assert (report.new, report.due, report.next) == (1, 2, "")
    assert [(r.box, r.days, r.cards, r.due) for r in report.boxes] == [
        (1, 1, 1, 1),
        (2, 2, 1, 0),
        (3, 4, 0, 0),
        (4, 8, 0, 0),
        (5, 16, 0, 0),
    ]


def test_the_deck_list_says_where_each_deck_stands(dated, home):
    logged(home, "First card", "pass", "2026-09-09")  # box 2, due the 11th
    logged(home, "Second card", "fail", "2026-09-08")  # box 1, due the 9th
    (home / "decks" / "stale").mkdir()
    rows = {d.id: d for d in dated.decks_list(api.NoParams()).decks}
    stats = rows["embedded"].stats
    assert (stats.size, stats.new, stats.due, stats.missed) == (3, 1, 2, 1)
    assert stats.boxes == [1, 1, 0, 0, 0]
    assert stats.last_run == "2026-09-09"
    assert rows["stale"].stats is None


def test_a_deck_never_run_has_every_card_new(dated):
    (row,) = dated.decks_list(api.NoParams()).decks
    assert (row.stats.new, row.stats.due, row.stats.last_run) == (3, 3, "")


def test_the_box_report_says_when_nothing_is_due(dated, home):
    for card in ("First card", "Second card", "Draft"):
        logged(home, card, "pass", "2026-09-10")
    report = dated.report_boxes(api.DeckParams(deck="embedded"))
    assert (report.due, report.next) == (0, "2026-09-12")
