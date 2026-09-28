"""The attempt log - in particular the properties it exists for: that adding,
reordering and renaming cards cannot corrupt a history."""

from lernt import misses


def log(tmp_path):
    return str(tmp_path / "state" / "attempts.jsonl")


def write(path, *rows):
    for card, score in rows:
        misses.append(path, card, card + "?", score, 12, run="r1")


def test_an_attempt_round_trips(tmp_path):
    path = log(tmp_path)
    misses.append(path, "a card", "A card?", "fail", 41, run="r1", reached="branch")
    (entry,) = misses.load(path)
    assert entry["card"] == "a card"
    assert entry["title"] == "A card?"
    assert entry["score"] == "fail"
    assert entry["seconds"] == 41
    assert entry["reached"] == "branch"
    assert entry["v"] == misses.VERSION


def test_a_missing_log_is_simply_empty(tmp_path):
    assert misses.load(log(tmp_path)) == []


def test_a_torn_final_line_does_not_cost_the_history_above_it(tmp_path):
    path = log(tmp_path)
    write(path, ("a", "fail"))
    with open(path, "a") as fh:
        fh.write('{"card": "b", "sco')  # killed mid-write
    assert [e["card"] for e in misses.load(path)] == ["a"]


def test_the_miss_list_is_the_last_score_not_a_running_tally(tmp_path):
    path = log(tmp_path)
    write(path, ("a", "fail"), ("a", "fail"), ("a", "pass"))
    assert misses.missed(misses.load(path)) == []
    write(path, ("a", "shaky"))
    assert [r["card"] for r in misses.missed(misses.load(path))] == ["a"]


def test_fails_sort_above_shakies_and_the_most_missed_above_the_rest(tmp_path):
    path = log(tmp_path)
    write(
        path,
        ("shaky one", "shaky"),
        ("occasional", "pass"),
        ("occasional", "fail"),
        ("chronic", "fail"),
        ("chronic", "fail"),
    )
    assert [r["card"] for r in misses.missed(misses.load(path))] == [
        "chronic",
        "occasional",
        "shaky one",
    ]


def test_history_counts_attempts_misses_and_the_closing_pass_streak(tmp_path):
    path = log(tmp_path)
    write(path, ("a", "fail"), ("a", "pass"), ("a", "pass"))
    rec = misses.history(misses.load(path))["a"]
    assert (rec["attempts"], rec["misses"], rec["streak"]) == (3, 1, 2)


def test_adding_and_reordering_cards_leaves_the_history_alone(tmp_path):
    # Nothing in the log is positional, so "the deck grew and everything after
    # card 7 now means a different question" cannot happen.
    path = log(tmp_path)
    write(path, ("b", "fail"))
    before = misses.load(path)
    known = {"a", "b", "c"}  # a and c added after the fact, sorted around b
    assert [r["card"] for r in misses.missed(before, known)] == ["b"]
    assert misses.retired(before, known) == []


def test_a_renamed_card_is_named_as_retired_not_silently_dropped(tmp_path):
    path = log(tmp_path)
    write(path, ("old name", "fail"))
    entries = misses.load(path)
    known = {"new name"}
    assert misses.missed(entries, known) == []  # not drilled any more
    assert misses.retired(entries, known) == ["old name"]
    assert misses.history(entries)["old name"]["title"] == "old name?"  # still readable
