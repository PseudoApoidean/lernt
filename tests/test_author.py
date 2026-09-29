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


def test_a_deleted_card_is_moved_to_the_trash_not_erased(engine, home):
    new_deck(engine)
    made = new_card(engine, "Capital of France?", answer="Paris.")
    gone = engine.card_delete(api.DeleteCardParams(deck="capital-cities", card=made.id))
    bank = home / "decks" / "capital-cities"
    assert gone.trash == str(bank / ".trash" / "Capital of France.md")
    assert not Path(made.path).exists()
    assert "Paris." in Path(gone.trash).read_text()
    listed = engine.card_list(api.DeckParams(deck="capital-cities"))
    assert listed.cards == []


def test_a_second_card_with_the_same_id_does_not_replace_the_trashed_one(engine):
    new_deck(engine)
    for _ in range(2):
        made = new_card(engine, "Capital of France?", answer="Paris.")
        engine.card_delete(api.DeleteCardParams(deck="capital-cities", card=made.id))
    trash = Path(made.path).parent / ".trash"
    assert sorted(p.name for p in trash.iterdir()) == [
        "Capital of France-2.md",
        "Capital of France.md",
    ]


def test_deleting_a_card_the_deck_does_not_have_is_refused(engine):
    new_deck(engine)
    with pytest.raises(api.ApiError) as exc:
        engine.card_delete(api.DeleteCardParams(deck="capital-cities", card="Nope"))
    assert exc.value.code == api.INVALID


def test_a_deleted_deck_is_moved_out_of_the_list(engine, home):
    new_deck(engine)
    gone = engine.deck_delete(api.DeleteDeckParams(deck="capital-cities"))
    assert gone.trash == str(home / "decks" / ".trash" / "capital-cities")
    assert (Path(gone.trash) / "deck.toml").is_file()
    assert engine.decks_list(api.NoParams()).decks == []
    # and the name is free again
    assert new_deck(engine).id == "capital-cities"


def test_deleting_a_symlinked_deck_moves_the_link_and_leaves_the_cards(engine, home):
    vault = home / "vault"
    vault.mkdir()
    (vault / "deck.toml").write_text('[deck]\nname = "Vaulted"\n')
    (vault / "Q.md").write_text("# Q?\n\n> [!tip]\n> A.\n")
    (home / "decks").mkdir()
    (home / "decks" / "vaulted").symlink_to(vault)
    gone = engine.deck_delete(api.DeleteDeckParams(deck="vaulted"))
    assert Path(gone.trash).is_symlink()
    assert (vault / "Q.md").is_file() and (vault / "deck.toml").is_file()


def test_a_deck_outside_the_decks_directory_is_not_lernts_to_delete(engine, home):
    other = home / "elsewhere" / "capitals"
    other.mkdir(parents=True)
    (other / "deck.toml").write_text("[deck]\n")
    with pytest.raises(api.ApiError) as exc:
        engine.deck_delete(api.DeleteDeckParams(deck=str(other)))
    assert exc.value.code == api.INVALID
    assert other.is_dir()


def test_a_deck_says_whether_deleting_it_takes_its_cards(engine, home):
    new_deck(engine)  # its cards are in its own folder
    vault = home / "vault"
    vault.mkdir()
    (vault / "deck.toml").write_text('[deck]\nname = "Linked"\n')
    (home / "decks" / "linked").symlink_to(vault)
    bank = home / "notes"
    bank.mkdir()
    named = home / "decks" / "named"
    named.mkdir()
    (named / "deck.toml").write_text(f'[deck]\ncards = ["{bank}"]\n')
    by_id = {d.id: d for d in engine.decks_list(api.NoParams()).decks}
    assert by_id["capital-cities"].cards_inside is True
    assert by_id["linked"].cards_inside is False
    assert by_id["named"].cards_inside is False


def update(engine, card, deck_id="capital-cities", **fields):
    fields.setdefault("title", "Capital of France?")
    return engine.card_update(api.UpdateCardParams(deck=deck_id, card=card, **fields))


def test_a_card_is_updated_under_the_id_it_has(engine):
    new_deck(engine)
    made = new_card(engine, "Capital of France?", answer="Paris.")
    done = update(
        engine,
        made.id,
        title="What is France's capital city?",
        answer="Paris.\nOn the Seine.",
        notes="Since 987.",
        reference="[[France]]",
        time=20,
        draft=True,
    )
    assert done.id == made.id and done.path == made.path
    card = deck.parse_card(made.path)
    assert card["id"] == "Capital of France"
    assert card["title"] == "What is France's capital city?"
    assert card["answer"] == ["Paris.", "On the Seine."]
    assert (card["notes"], card["reference"], card["time"]) == (
        ["Since 987."],
        "[[France]]",
        20,
    )
    assert card["draft"] is True


def test_an_update_keeps_the_tags_it_has_and_can_lift_a_draft(engine):
    new_deck(engine)
    made = new_card(engine, "Capital of France?", answer="Paris.", draft=True)
    with open(made.path, "a") as fh:
        fh.write("#geography\n")
    update(engine, made.id, answer="Paris.", draft=False)
    card = deck.parse_card(made.path)
    assert card["tags"] == ["geography"] and card["draft"] is False


def test_an_update_can_link_and_unlink_cards(engine):
    new_deck(engine)
    new_card(engine, "Capital of France?", answer="Paris.")
    made = new_card(engine, "Which is bigger?", answer="Berlin.")
    update(engine, made.id, title="Which is bigger?", follows=["Capital of France"])
    assert deck.parse_card(made.path)["followups"] == ["Capital of France"]
    update(engine, made.id, title="Which is bigger?", follows=[])
    assert deck.parse_card(made.path)["followups"] == []


def test_an_update_refuses_a_follow_up_that_is_not_a_card_or_is_itself(engine):
    new_deck(engine)
    made = new_card(engine, "Capital of France?", answer="Paris.")
    before = Path(made.path).read_text()
    for follows in (["Nope"], [made.id]):
        with pytest.raises(api.ApiError) as exc:
            update(engine, made.id, answer="Lyon.", follows=follows)
        assert exc.value.code == api.INVALID
    assert Path(made.path).read_text() == before


def test_updating_a_card_the_deck_does_not_have_is_refused(engine):
    new_deck(engine)
    with pytest.raises(api.ApiError) as exc:
        update(engine, "Nope")
    assert exc.value.code == api.INVALID


def test_the_card_list_carries_the_whole_card_for_a_form(engine):
    new_deck(engine)
    new_card(engine, "Capital of France?", answer="Paris.")
    made = new_card(
        engine,
        "Which is bigger?",
        answer="Berlin.\nBy far.",
        notes="Population.",
        reference="[[Cities]]",
        time=0,
        follows=["Capital of France"],
    )
    listed = {
        c.id: c for c in engine.card_list(api.DeckParams(deck="capital-cities")).cards
    }
    card = listed[made.id]
    assert card.answer == ["Berlin.", "By far."]
    assert (card.notes, card.reference, card.time) == (["Population."], "[[Cities]]", 0)
    assert card.follows == ["Capital of France"]
    assert listed["Capital of France"].time is None


def rename(engine, **fields):
    return engine.deck_update(api.UpdateDeckParams(deck="capital-cities", **fields))


def test_a_deck_is_renamed_and_described_in_its_toml(engine, home):
    new_deck(engine, description="Old.")
    toml = home / "decks" / "capital-cities" / "deck.toml"
    toml.write_text(toml.read_text() + "count = 5\n\n[other]\nname = 'kept'\n")
    done = rename(engine, name="Capitals of Europe", description="New.")
    assert done.id == "capital-cities"
    data = tomllib.loads(toml.read_text())
    assert data["deck"] == {
        "name": "Capitals of Europe",
        "description": "New.",
        "count": 5,
    }
    assert data["other"] == {"name": "kept"}
    assert config.resolve("capital-cities").name == "Capitals of Europe"


def test_an_empty_description_removes_the_line_and_a_missing_one_is_added(engine, home):
    new_deck(engine, description="Old.")
    toml = home / "decks" / "capital-cities" / "deck.toml"
    rename(engine, name="Capital cities", description="")
    assert "description" not in tomllib.loads(toml.read_text())["deck"]
    rename(engine, name="Capital cities", description="Back.")
    assert tomllib.loads(toml.read_text())["deck"]["description"] == "Back."


def test_a_deck_needs_a_name_when_renamed(engine):
    new_deck(engine)
    with pytest.raises(api.ApiError) as exc:
        rename(engine, name="  ")
    assert exc.value.code == api.INVALID


def test_a_deck_toml_the_form_cannot_edit_safely_is_refused_untouched(engine, home):
    new_deck(engine)
    toml = home / "decks" / "capital-cities" / "deck.toml"
    toml.write_text('[deck]\nname = "X"\ndescription = """\nlong\n"""\n')
    with pytest.raises(api.ApiError) as exc:
        rename(engine, name="Y", description="Z")
    assert exc.value.code == api.INVALID
    assert 'name = "X"' in toml.read_text()


def test_a_newline_in_a_one_line_field_is_flattened_not_written(engine, home):
    new_deck(engine, name="Capital\ncities", description="Where\nthe governments\nsit.")
    assert config.resolve("capital-cities").name == "Capital cities"
    assert config.resolve("capital-cities").description == "Where the governments sit."
    made = new_card(
        engine,
        "Capital\nof France?",
        answer="Paris.",
        reference="[[France]]\n[[Paris]]",
    )
    card = deck.parse_card(made.path)
    assert card["title"] == "Capital of France?"
    assert card["reference"] == "[[France]] [[Paris]]"
    assert card["followups"] == [] and card["time"] is None
    update(
        engine, made.id, title="Capital of France?", answer="Paris.", reference="a\n\nb"
    )
    assert deck.parse_card(made.path)["reference"] == "a b"
    rename(engine, name="Capitals\nof Europe")
    assert config.resolve("capital-cities").name == "Capitals of Europe"
