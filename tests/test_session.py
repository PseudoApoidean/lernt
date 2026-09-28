"""Selection logic. Seeded RNG throughout - a drill that picks at random is
still a thing whose rules must be testable."""

import random

import pytest

from lernt.session import Session, SessionError


def deck(*cards):
    by_id = {}
    for cid, followups, draft in cards:
        by_id[cid] = {
            "id": cid,
            "title": cid + "?",
            "answer": [],
            "notes": [],
            "followups": list(followups),
            "parents": [],
            "reference": "",
            "tags": ["draft"] if draft else [],
            "draft": draft,
        }
    for card in by_id.values():
        for target in card["followups"]:
            by_id[target]["parents"].append(card["id"])
    return {"schema": "drill.deck/3", "cards": list(by_id.values())}


BANK = deck(
    ("a", ["b", "draft"], False),
    ("b", ["c"], False),
    ("c", [], False),
    ("draft", [], True),
)


def session(**kw):
    kw.setdefault("rng", random.Random(0))
    return Session(BANK, **kw)


def test_drafts_stay_out_of_the_pool():
    assert "draft" not in session().pool
    assert "draft" in session(include_drafts=True).pool


def test_filter_narrows_the_pool_and_an_empty_one_is_an_error():
    assert session(name_filter="b").pool == ["b"]
    with pytest.raises(SessionError):
        session(name_filter="nothing matches this")


def test_only_restricts_the_pool_which_is_how_weak_mode_works():
    assert session(only={"c"}).pool == ["c"]


def test_a_branch_is_followed_before_a_fresh_thread_starts():
    s = session()
    s.visit("a")
    card, followed = s.pick_next("a")
    assert (card, followed) == ("b", True)


def test_an_exhausted_line_starts_a_fresh_thread():
    s = session()
    s.visit("a")
    s.visit("b")
    card, followed = s.pick_next("b")  # b's only branch, c... is unasked
    assert (card, followed) == ("c", True)
    s.visit("c")
    assert s.pick_next("c")[1] is False


def test_auto_never_steers_into_a_draft():
    s = session()
    s.visit("a")
    s.visit("b")
    s.visit("c")
    for _ in range(20):
        assert s.pick_next("a")[0] != "draft"


def test_the_menu_still_offers_the_draft_branch():
    # Picking one off the menu is a deliberate act; being steered into one is
    # not - so followups() and pick_next() deliberately disagree here.
    assert "draft" in session().followups("a")


def test_random_prefers_cards_not_yet_asked():
    s = session()
    s.visit("a")
    s.visit("b")
    assert s.pick_random() == "c"


def test_scoring_counts_and_collects_the_re_run_list():
    s = session()
    s.score("a", "pass")
    s.score("b", "fail")
    s.score("c", "shaky")
    assert s.scores == {"pass": 1, "shaky": 1, "fail": 1}
    assert [r[0] for r in s.review] == ["fail", "shaky"]
    assert s.asked == 3
    with pytest.raises(SessionError):
        s.score("a", "maybe")


def test_count_zero_never_finishes_on_its_own():
    s = session(count=0)
    for cid in ("a", "b", "c"):
        s.visit(cid)
    assert not s.finished()
    assert session(count=2).finished() is False


def test_a_card_can_be_replaced_mid_run_but_not_invented():
    s = session()
    cid = s.pool[0]
    s.update({**s.cards[cid], "title": "reworded"})
    assert s.cards[cid]["title"] == "reworded"
    with pytest.raises(SessionError):
        s.update({"id": "never in the deck", "title": "x"})


def test_rank_picks_the_lowest_first_then_the_next():
    ranked = deck(("a", [], False), ("b", [], False), ("c", [], False))
    run = Session(ranked, count=0, rank={"a": 3, "b": 1, "c": 2}, rng=random.Random(0))
    order = []
    for _ in range(3):
        card = run.pick_random()
        run.visit(card)
        order.append(card)
    assert order == ["b", "c", "a"]
