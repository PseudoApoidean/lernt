"""Writing decks and cards: what goes in the file is what parses back out."""

import tomllib
from pathlib import Path

import pytest

from lernt import api, author, config, deck


@pytest.fixture
def home(tmp_path, monkeypatch):
    monkeypatch.setenv("LERNT_DECKS", str(tmp_path / "decks"))
    monkeypatch.setenv("XDG_STATE_HOME", str(tmp_path / "state"))
    return tmp_path


@pytest.fixture
def engine(home):
    return api.Engine()


def new_deck(engine, name="Capital cities", description=""):
    return engine.deck_create(api.NewDeckParams(name=name, description=description))


def new_card(engine, title, deck_id="capital-cities", **fields):
    return engine.card_create(api.NewCardParams(deck=deck_id, title=title, **fields))


def test_a_deck_is_a_directory_named_from_its_title(engine, home):
    made = new_deck(engine, "Capital cities", "Where the governments sit.")
    assert made.id == "capital-cities"
    path = home / "decks" / "capital-cities"
    assert made.path == str(path)
    data = tomllib.loads((path / "deck.toml").read_text())
    assert data == {
        "deck": {"name": "Capital cities", "description": "Where the governments sit."}
    }
    loaded = config.resolve("capital-cities")
    assert (loaded.name, loaded.cards) == ("Capital cities", [path.resolve()])


def test_a_title_with_quotes_survives_the_toml(engine):
    new_deck(engine, 'The "C" screen \\ 2', 'It says "hello".')
    loaded = config.resolve("the-c-screen-2")
    assert loaded.name == 'The "C" screen \\ 2'
    assert loaded.description == 'It says "hello".'


def test_a_deck_never_replaces_one_that_is_there(engine):
    new_deck(engine)
    with pytest.raises(api.ApiError) as exc:
        new_deck(engine, "Capital Cities")
    assert exc.value.code == api.INVALID
    assert "capital-cities" in str(exc.value)


@pytest.mark.parametrize("name", ["", "   ", "???"])
def test_a_deck_needs_a_name_with_something_in_it(engine, name):
    with pytest.raises(api.ApiError) as exc:
        new_deck(engine, name)
    assert exc.value.code == api.INVALID


def test_a_card_is_written_in_the_format_it_is_read_in(engine, home):
    new_deck(engine)
    made = new_card(
        engine,
        "What is the capital of France?",
        answer="Paris.\nOn the Seine.",
        notes="It has been since 987.",
        reference="[[France]]",
        time=45,
        draft=True,
    )
    assert made.id == "What is the capital of France"
    card = deck.parse_card(made.path)
    assert card["id"] == made.id
    assert card["title"] == "What is the capital of France?"
    assert card["answer"] == ["Paris.", "On the Seine."]
    assert card["notes"] == ["It has been since 987."]
    assert card["reference"] == "[[France]]"
    assert card["time"] == 45
    assert card["draft"] is True


def test_a_card_can_be_started_empty_for_an_editor(engine):
    new_deck(engine)
    made = new_card(engine, "What is the capital of Germany?")
    card = deck.parse_card(made.path)
    assert (card["title"], card["answer"], card["time"]) == (
        "What is the capital of Germany?",
        [],
        None,
    )
    assert engine.deck_check(api.DeckParams(deck="capital-cities")).problems == []


def test_an_untimed_card_says_so(engine):
    new_deck(engine)
    made = new_card(engine, "Recite pi?", answer="3.14", time=0)
    assert deck.parse_card(made.path)["time"] == 0


def test_a_follow_up_is_a_link_to_a_card_that_is_there(engine):
    new_deck(engine)
    new_card(engine, "Capital of France?", answer="Paris.")
    new_card(engine, "Capital of Germany?", answer="Berlin.")
    made = new_card(
        engine,
        "Which is bigger?",
        answer="Berlin.",
        follows=["Capital of France", "Capital of Germany"],
    )
    card = deck.parse_card(made.path)
    assert card["followups"] == ["Capital of France", "Capital of Germany"]
    text = Path(made.path).read_text()
    assert "1. [[Capital of France|Capital of France?]]" in text
    assert engine.deck_check(api.DeckParams(deck="capital-cities")).problems == []


def test_a_follow_up_that_is_not_a_card_is_refused_and_nothing_is_written(engine, home):
    new_deck(engine)
    with pytest.raises(api.ApiError) as exc:
        new_card(engine, "Which is bigger?", follows=["Nothing here"])
    assert exc.value.code == api.INVALID
    assert "Nothing here" in str(exc.value)
    assert list((home / "decks" / "capital-cities").glob("*.md")) == []


def test_a_card_never_replaces_one_that_is_there(engine):
    new_deck(engine)
    new_card(engine, "Capital of France?", answer="Paris.")
    with pytest.raises(api.ApiError) as exc:
        new_card(engine, "Capital of France", answer="Lyon.")
    assert exc.value.code == api.INVALID


def test_a_question_with_path_characters_makes_a_safe_id(engine):
    new_deck(engine)
    made = new_card(engine, "Why does a/b: [[c]] #d?", answer="Because.")
    assert made.id == "Why does a b c d"
    assert deck.parse_card(made.path)["title"] == "Why does a/b: [[c]] #d?"


@pytest.mark.parametrize("title", ["", "  ", "///"])
def test_a_card_needs_a_question_with_something_in_it(engine, title):
    new_deck(engine)
    with pytest.raises(api.ApiError) as exc:
        new_card(engine, title)
    assert exc.value.code == api.INVALID


def test_a_negative_time_is_refused(engine):
    new_deck(engine)
    with pytest.raises(api.ApiError) as exc:
        new_card(engine, "Q?", time=-1)
    assert exc.value.code == api.INVALID


def test_a_card_goes_in_the_decks_first_bank(engine, home):
    bank = home / "vault" / "Questions"
    bank.mkdir(parents=True)
    other = home / "decks" / "vaulted"
    (other / "elsewhere").mkdir(parents=True)
    (other / "deck.toml").write_text(f'[deck]\ncards = ["{bank}", "elsewhere"]\n')
    made = new_card(engine, "Q?", deck_id="vaulted", answer="A.")
    assert made.path == str(bank / "Q.md")


def test_a_card_for_an_unknown_deck_is_no_deck(engine):
    with pytest.raises(api.ApiError) as exc:
        new_card(engine, "Q?", deck_id="nope")
    assert exc.value.code == api.NO_DECK


def test_the_cards_of_a_deck_are_listed_for_choosing_follow_ups(engine):
    new_deck(engine)
    new_card(engine, "Capital of France?", answer="Paris.")
    new_card(engine, "Capital of Germany?", answer="Berlin.", draft=True)
    listed = engine.card_list(api.DeckParams(deck="capital-cities"))
    assert [(c.id, c.title, c.draft) for c in listed.cards] == [
        ("Capital of France", "Capital of France?", False),
        ("Capital of Germany", "Capital of Germany?", True),
    ]


def test_a_check_reports_what_a_hand_edit_broke(engine):
    new_deck(engine)
    made = new_card(engine, "Capital of France?", answer="Paris.")
    with open(made.path, "a") as fh:
        fh.write("\n---\n\n**Follow-ups:**\n1. [[Gone]]\n")
    problems = engine.deck_check(api.DeckParams(deck="capital-cities")).problems
    assert len(problems) == 1 and "Gone" in problems[0]


def test_card_ids_are_bounded(engine):
    assert len(author.card_id("word " * 100)) <= author.MAX_ID
