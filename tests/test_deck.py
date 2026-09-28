"""The engine, against a bank written here rather than the real one - the
vault's cards are edited constantly and a test that asserts on them is a test
that fails for the wrong reason."""

import json

import pytest

from lernt import config as config_mod
from lernt import deck as deck_mod

CARD = """# Does volatile make something thread-safe?

> [!tip]
> No. It stops the compiler caching the value; it says
> nothing about atomicity.

A 32-bit read on a 16-bit machine is two instructions.

---

**Reference:** [[Notes/C Language#volatile|C Language]]
**Follow-ups:**
1. [[Notes/Questions/Fix count with no atomics|How would you fix it?]]
2. [[Gone missing]]
"""

STOP = """# How would you fix it?

> [!tip]
> Disable the interrupt across the read.

---

#draft #concurrency
"""


@pytest.fixture
def bank(tmp_path):
    (tmp_path / "Does volatile make something thread-safe.md").write_text(CARD)
    (tmp_path / "Fix count with no atomics.md").write_text(STOP)
    (tmp_path / "!Template.md").write_text("# not a card\n")
    return tmp_path


@pytest.fixture
def one_bank(bank):
    return config_mod.from_dirs([str(bank)], name="Test deck")


def card_file(tmp_path, text, name="Card"):
    path = tmp_path / f"{name}.md"
    path.write_text(text)
    return deck_mod.parse_card(str(path))


def test_card_parses_into_its_parts(bank):
    card = deck_mod.parse_card(
        str(bank / "Does volatile make something thread-safe.md")
    )
    assert card["id"] == "Does volatile make something thread-safe"
    assert card["title"] == "Does volatile make something thread-safe?"
    assert card["answer"] == [
        "No. It stops the compiler caching the value; it says",
        "nothing about atomicity.",
    ]
    assert "two instructions" in card["notes"][0]
    assert card["followups"] == ["Fix count with no atomics", "Gone missing"]
    assert card["reference"].startswith("[[Notes/C Language")
    assert (card["tags"], card["draft"]) == ([], False)


def test_a_draft_tag_marks_a_draft(bank):
    card = deck_mod.parse_card(str(bank / "Fix count with no atomics.md"))
    assert card["tags"] == ["draft", "concurrency"]
    assert card["draft"] is True
    assert card["followups"] == []


def test_an_unlabelled_callout_starts_at_its_first_point(tmp_path):
    card = card_file(tmp_path, "# Points?\n\n> [!tip]\n> - one\n> - two\n\n---\n")
    assert card["answer"] == ["- one", "- two"]


def test_any_callout_type_holds_the_answer(tmp_path):
    card = card_file(tmp_path, "# Capital of France?\n\n> [!question]- Paris.\n")
    assert card["answer"] == ["Paris."]


def test_a_card_without_a_callout_is_all_answer(tmp_path):
    card = card_file(tmp_path, "# Capital of France?\n\nParis.\n\nOn the Seine.\n")
    assert card["answer"] == ["Paris.", "", "On the Seine."]
    assert card["notes"] == []


def test_hashes_in_the_answer_are_not_tags(tmp_path):
    card = card_file(
        tmp_path,
        "# Guards?\n\n> [!tip]\n> #ifndef FOO_H\n\n---\n\n**Reference:** [[C#guards]] &#x1f534;\n",
    )
    assert card["tags"] == []


def test_build_drops_dangling_links_and_reports_them(one_bank):
    built = deck_mod.build(one_bank)
    cards = deck_mod.index(built)
    assert set(cards) == {
        "Does volatile make something thread-safe",
        "Fix count with no atomics",
    }  # !Template skipped
    assert cards["Does volatile make something thread-safe"]["followups"] == [
        "Fix count with no atomics"
    ]
    assert any("Gone missing" in p for p in built["problems"])


def test_parents_are_computed_from_the_edges(one_bank):
    cards = deck_mod.index(deck_mod.build(one_bank))
    assert cards["Fix count with no atomics"]["parents"] == [
        "Does volatile make something thread-safe"
    ]


def test_deck_round_trips_through_json(bank, one_bank, tmp_path):
    out = tmp_path / "deck.json"
    assert deck_mod.main(["--cards", str(bank), "--out", str(out)]) == 0
    assert deck_mod.load(str(out))["cards"] == deck_mod.build(one_bank)["cards"]


def test_a_foreign_json_is_refused(tmp_path):
    out = tmp_path / "other.json"
    out.write_text(json.dumps({"schema": "something else", "cards": []}))
    with pytest.raises(deck_mod.DeckError):
        deck_mod.load(str(out))


def test_several_banks_are_drilled_as_one_pool(bank, tmp_path):
    other = tmp_path / "web"
    other.mkdir()
    (other / "Explain the box model.md").write_text("# Explain the box model.\n")
    deck = deck_mod.build(config_mod.from_dirs([str(bank), str(other)], name="General"))
    assert "Explain the box model" in deck_mod.index(deck)
    assert "Does volatile make something thread-safe" in deck_mod.index(deck)
    assert deck["name"] == "General"
    assert deck["deck"] == "ad-hoc"
    assert deck["sources"] == [str(bank), str(other)]


def test_a_card_in_two_banks_keeps_the_first_and_says_so(bank, tmp_path):
    other = tmp_path / "web"
    other.mkdir()
    (other / "Does volatile make something thread-safe.md").write_text("# Other.\n")
    deck = deck_mod.build(config_mod.from_dirs([str(bank), str(other)]))
    card = deck_mod.index(deck)["Does volatile make something thread-safe"]
    assert card["source"] == str(bank)  # config order decides, not disk order
    assert any("two banks hold this card" in p for p in deck["problems"])


def test_a_card_reloads_from_its_file_keeping_its_parents(bank, one_bank):
    deck = deck_mod.build(one_bank)
    cards = deck_mod.index(deck)
    stop = cards["Fix count with no atomics"]
    path = deck_mod.card_path(stop)
    assert path == bank / "Fix count with no atomics.md"
    path.write_text(STOP.replace("How would you fix it?", "How would you fix that?"))
    fresh = deck_mod.reload(stop, cards)
    assert fresh["title"] == "How would you fix that?"
    assert (
        fresh["parents"]
        == stop["parents"]
        == ["Does volatile make something thread-safe"]
    )


def test_the_decks_are_listed_as_json(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("LERNT_DECKS", str(tmp_path / "decks"))
    deck = tmp_path / "decks" / "capitals"
    deck.mkdir(parents=True)
    (deck / "deck.toml").write_text('[deck]\nname = "Capitals"\n')
    (tmp_path / "decks" / "broken").mkdir()
    assert deck_mod.main(["--list"]) == 0
    listed = json.loads(capsys.readouterr().out)
    assert listed["schema"] == deck_mod.LIST_SCHEMA
    rows = {row["id"]: row for row in listed["decks"]}
    assert rows["capitals"]["name"] == "Capitals"
    assert "error" in rows["broken"]


def test_no_deck_named_is_an_error(tmp_path, monkeypatch, capsys):
    monkeypatch.setenv("LERNT_DECKS", str(tmp_path / "decks"))
    assert deck_mod.main([]) == 1
    assert "Name a deck" in capsys.readouterr().err


@pytest.mark.parametrize(
    ("written", "seconds"),
    [("45", 45), ("45s", 45), ("2m", 120), ("1m30s", 90), ("1m 30s", 90), ("off", 0)],
)
def test_a_card_sets_its_own_time(tmp_path, written, seconds):
    card = card_file(tmp_path, f"# Q?\n\nA.\n\n---\n\n**Time:** {written}\n")
    assert card["time"] == seconds


def test_a_card_without_a_time_leaves_it_to_the_run(tmp_path):
    assert card_file(tmp_path, "# Q?\n\nA.\n")["time"] is None


def test_a_time_that_is_not_a_duration_is_a_problem(tmp_path):
    (tmp_path / "Q.md").write_text("# Q?\n\nA.\n\n---\n\n**Time:** soon\n")
    deck = deck_mod.build(config_mod.from_dirs([str(tmp_path)]))
    (card,) = deck["cards"]
    assert card["time"] is None and "_problems" not in card
    assert any("not a duration" in p for p in deck["problems"])


def test_an_older_deck_json_still_loads(tmp_path):
    out = tmp_path / "old.json"
    out.write_text(json.dumps({"schema": "drill.deck/3", "cards": []}))
    assert deck_mod.load(str(out))["cards"] == []
