"""The Leitner schedule, replayed from attempts. Every date is written out: the
schedule is about calendar days, and a test that leaned on today would pass or
fail by the clock."""

import datetime

import pytest

from lernt import leitner

DAY = datetime.date(2026, 9, 1)


def attempt(card, score, days=0, at=None):
    when = at if at is not None else (DAY + datetime.timedelta(days=days)).isoformat()
    return {"card": card, "score": score, "at": f"{when}T09:00:00+0100"}


def boxes(*entries):
    return {card: record["box"] for card, record in leitner.replay(entries).items()}


def test_a_new_card_passed_goes_to_box_2():
    assert boxes(attempt("a", "pass")) == {"a": 2}


def test_a_new_card_failed_or_shaky_enters_box_1():
    assert boxes(attempt("a", "fail"), attempt("b", "shaky")) == {"a": 1, "b": 1}


def test_passes_on_the_day_they_are_due_climb_a_box_each():
    # Box 2 waits 2 days, box 3 waits 4.
    climbed = boxes(
        attempt("a", "pass", 0), attempt("a", "pass", 2), attempt("a", "pass", 6)
    )
    assert climbed == {"a": 4}


def test_a_pass_before_the_card_is_due_changes_nothing():
    # Re-running a card the same afternoon must not carry it up three boxes.
    assert boxes(*(attempt("a", "pass", 0) for _ in range(3))) == {"a": 2}
    record = leitner.replay([attempt("a", "pass", 0), attempt("a", "pass", 1)])["a"]
    assert record["moved"] == DAY  # and its clock is not restarted either


def test_a_fail_goes_back_to_box_1_due_or_not():
    entries = [
        attempt("a", "pass", 0),
        attempt("a", "pass", 2),
        attempt("a", "fail", 3),
    ]
    assert boxes(*entries) == {"a": 1}


def test_a_shaky_keeps_the_box_and_restarts_the_wait():
    entries = [attempt("a", "pass", 0), attempt("a", "shaky", 1)]
    record = leitner.replay(entries)["a"]
    assert record == {"box": 2, "moved": DAY + datetime.timedelta(days=1)}


def test_the_last_box_is_as_high_as_it_goes():
    entries = [attempt("a", "pass", 0)]
    day = 0
    for interval in leitner.DEFAULT_INTERVALS[1:] * 2:
        day += interval
        entries.append(attempt("a", "pass", day))
    assert boxes(*entries) == {"a": len(leitner.DEFAULT_INTERVALS)}


def test_an_attempt_without_a_date_is_passed_over():
    assert boxes(attempt("a", "pass", at=""), attempt("b", "fail", at="garbage")) == {}


def test_due_is_new_cards_and_cards_whose_wait_is_over():
    state = leitner.replay(
        [attempt("waited", "pass", 0), attempt("waiting", "pass", 1)]
    )
    known = {"waited", "waiting", "new"}
    on = DAY + datetime.timedelta(days=2)
    assert leitner.due(state, known, on) == {"waited", "new"}


def test_next_due_is_the_earliest_wait_and_none_while_a_card_is_new():
    state = leitner.replay([attempt("a", "pass", 0), attempt("b", "fail", 0)])
    assert leitner.next_due(state, {"a", "b"}) == DAY + datetime.timedelta(days=1)
    assert leitner.next_due(state, {"a", "new"}) is None


def test_intervals_are_a_decks_to_set():
    entries = [attempt("a", "pass", 0), attempt("a", "pass", 3)]
    # With box 2 waiting 5 days, the second pass is early and does nothing.
    assert leitner.replay(entries, (1, 5, 10))["a"]["box"] == 2
    assert leitner.replay(entries, (1, 3, 10))["a"]["box"] == 3


@pytest.mark.parametrize("value", [[], [0, 1], [1, "2"], [1.5], "1, 2", [True, 2]])
def test_intervals_that_are_not_days_are_refused(value):
    intervals, problem = leitner.check_intervals(value)
    assert intervals is None
    assert "boxes" in problem


def test_intervals_that_are_days_are_kept():
    assert leitner.check_intervals([1, 3, 7]) == ((1, 3, 7), None)
