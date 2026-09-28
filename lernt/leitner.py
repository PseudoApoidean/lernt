"""The Leitner schedule: which box each card is in, and which cards are due.

Nothing here is stored. A card's box is replayed from the attempt log, the way
the miss list is, so the log stays the one record of what happened and a box can
never disagree with it. A history logged before this module existed turns into
boxes the first time it is read.

The rules, one attempt at a time:

  - A card never attempted is new. It is due, and it enters box 1.
  - A pass on a card that is due moves it up a box, to the last box at most.
  - A pass on a card that is not due yet changes nothing. Running the miss list
    three times in an afternoon must not carry a card up three boxes.
  - A fail sends it back to box 1, due or not: forgetting it early is still
    forgetting it.
  - A shaky keeps it in its box and starts its wait again.

Box n comes due `intervals[n - 1]` days after the card was last moved. Days are
calendar days in local time, read off the log's timestamps, so a card passed
late one evening is not due again until the day it should be.
"""

import datetime

DEFAULT_INTERVALS = (1, 2, 4, 8, 16)


def day_of(entry):
    """The local calendar day of an attempt, or None if it has no usable time."""
    try:
        return datetime.date.fromisoformat((entry.get("at") or "")[:10])
    except ValueError:
        return None


def step(record, score, day, intervals):
    """`record` - {"box", "moved"}, or None for a new card - after one attempt
    on `day`. Returns the new record; the old one is not changed."""
    if record is None:
        box, moved, due = 1, None, True
    else:
        box, moved = record["box"], record["moved"]
        due = day >= moved + datetime.timedelta(days=intervals[box - 1])
    if score == "fail":
        return {"box": 1, "moved": day}
    if score == "pass":
        if not due:
            return {"box": box, "moved": moved}
        return {"box": min(box + 1, len(intervals)), "moved": day}
    return {"box": box, "moved": day}


def replay(entries, intervals=DEFAULT_INTERVALS):
    """card id -> {"box", "moved"} for every card with a usable attempt. An
    attempt with no date cannot be placed in the schedule and is passed over."""
    state = {}
    for entry in entries:
        day = day_of(entry)
        if day is None:
            continue
        card = entry["card"]
        state[card] = step(state.get(card), entry.get("score"), day, intervals)
    return state


def due_on(record, intervals):
    """The day `record`'s card comes due. None for a new card: it already is."""
    if record is None:
        return None
    return record["moved"] + datetime.timedelta(days=intervals[record["box"] - 1])


def is_due(record, today, intervals):
    when = due_on(record, intervals)
    return when is None or today >= when


def box_of(record):
    """The card's box; 0 for a new card, which has not entered one yet."""
    return 0 if record is None else record["box"]


def due(state, known, today, intervals=DEFAULT_INTERVALS):
    """The cards in `known` that are due on `today`, new ones included."""
    return {card for card in known if is_due(state.get(card), today, intervals)}


def next_due(state, known, intervals=DEFAULT_INTERVALS):
    """The earliest day any card in `known` comes due, or None if one is new."""
    days = [due_on(state.get(card), intervals) for card in known]
    if not days or None in days:
        return None
    return min(days)


def check_intervals(value):
    """`boxes` from a deck.toml as a tuple of days, or a message saying why not."""
    if (
        not isinstance(value, list)
        or not value
        or not all(isinstance(v, int) and not isinstance(v, bool) for v in value)
        or any(v < 1 for v in value)
    ):
        return None, "boxes must be a list of days, each 1 or more"
    return tuple(value), None
