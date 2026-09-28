"""The saved-options file: keys and values, merged, never positional."""

from lernt import options


def test_nothing_saved_is_empty(tmp_path):
    assert options.load(tmp_path / "none.json") == {}


def test_a_save_merges_and_keeps_keys_it_was_not_sent(tmp_path):
    path = tmp_path / "deck.options.json"
    options.save(path, {"count": "12", "retired-option": "x"})
    assert options.save(path, {"count": "20", "limit": "auto"}) == {
        "count": "20",
        "limit": "auto",
        "retired-option": "x",
    }
    assert options.load(path)["retired-option"] == "x"


def test_a_file_that_will_not_parse_is_empty_and_is_replaced(tmp_path):
    path = tmp_path / "deck.options.json"
    path.write_text("{torn")
    assert options.load(path) == {}
    options.save(path, {"count": "5"})
    assert options.load(path) == {"count": "5"}


def test_hand_edited_values_come_back_as_text(tmp_path):
    path = tmp_path / "deck.options.json"
    path.write_text('{"count": 12, "edit": true, "gone": null, "nested": {"a": 1}}')
    assert options.load(path) == {"count": "12", "edit": "on"}
