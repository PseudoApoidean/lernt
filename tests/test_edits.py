"""The edit log - in particular what counts as standing, and what clears it."""

import pytest

from lernt import edits


def log(tmp_path):
    return str(tmp_path / "state" / "lernt.edits.jsonl")


def test_an_entry_round_trips(tmp_path):
    path = log(tmp_path)
    edits.append(path, "a card", "A card?", "note", "clarify X", run="r1")
    (entry,) = edits.load(path)
    assert (entry["card"], entry["title"], entry["kind"], entry["text"]) == (
        "a card",
        "A card?",
        "note",
        "clarify X",
    )
    assert entry["v"] == edits.VERSION


def test_an_unknown_kind_is_refused_not_written(tmp_path):
    path = log(tmp_path)
    with pytest.raises(ValueError):
        edits.append(path, "a", "A?", "delete")
    assert edits.load(path) == []


def test_a_torn_final_line_does_not_cost_the_entries_above_it(tmp_path):
    path = log(tmp_path)
    edits.append(path, "a", "A?", "note", "x")
    with open(path, "a") as fh:
        fh.write('{"card": "b", "ki')
    assert [e["card"] for e in edits.load(path)] == ["a"]


def test_notes_and_flags_stand_until_an_edit_or_done_follows_them(tmp_path):
    path = log(tmp_path)
    edits.append(path, "a", "A?", "note", "one")
    edits.append(path, "a", "A?", "remove", "")
    edits.append(path, "b", "B?", "note", "two")
    edits.append(path, "b", "B?", "edit")
    edits.append(path, "c", "C?", "remove", "dup")
    edits.append(path, "c", "C?", "done")
    edits.append(path, "c", "C?", "note", "after the done")
    standing = edits.standing(edits.load(path))
    assert [e["kind"] for e in standing["a"]] == ["note", "remove"]
    assert "b" not in standing
    assert [e["text"] for e in standing["c"]] == ["after the done"]


def test_removal_flags_sort_first_and_gone_cards_leave_the_list(tmp_path):
    path = log(tmp_path)
    edits.append(path, "noted", "N?", "note", "old")
    edits.append(path, "flagged", "F?", "remove", "")
    edits.append(path, "deleted", "D?", "remove", "dup")
    entries = edits.load(path)
    known = {"noted", "flagged"}
    assert [card for card, _ in edits.outstanding(entries, known)] == [
        "flagged",
        "noted",
    ]
    # A flagged card that has gone was acted on; a noted one was renamed away.
    assert edits.orphaned(entries, known) == []
    assert edits.orphaned(entries, {"flagged"}) == ["noted"]
