"""Decks: a directory with a deck.toml, found by name or by path, logging under
its own id."""

import os
from pathlib import Path

import pytest

from lernt import config as config_mod


@pytest.fixture
def home(tmp_path, monkeypatch):
    """XDG homes of our own - these tests must never read the real ones."""
    monkeypatch.setenv("XDG_DATA_HOME", str(tmp_path / "data"))
    monkeypatch.setenv("XDG_STATE_HOME", str(tmp_path / "state"))
    monkeypatch.delenv("LERNT_DECKS", raising=False)
    return tmp_path


def decks(home):
    return home / "data" / "lernt" / "decks"


def bank(directory):
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "A card.md").write_text("# A card?\n")
    return directory


def make_deck(directory, body="[deck]\n"):
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "deck.toml").write_text(body)
    return directory


def test_cards_default_to_the_deck_directory(tmp_path):
    deck = make_deck(bank(tmp_path / "capitals"))
    config = config_mod.load(deck)
    assert config.cards == [deck]
    assert (config.id, config.name) == ("capitals", "capitals")


def test_card_paths_resolve_against_the_deck_not_the_cwd(tmp_path, monkeypatch):
    bank(tmp_path / "vault" / "Questions")
    make_deck(tmp_path / "vault", '[deck]\nname = "Embedded"\ncards = ["Questions"]\n')
    monkeypatch.chdir(tmp_path)  # somewhere else entirely
    config = config_mod.load(str(tmp_path / "vault"))
    assert config.cards == [tmp_path / "vault" / "Questions"]
    assert config.name == "Embedded"


def test_the_deck_file_itself_may_be_given(tmp_path):
    deck = make_deck(bank(tmp_path / "capitals"))
    assert config_mod.load(deck / "deck.toml").id == "capitals"


def test_cards_may_live_anywhere_and_in_several_banks(tmp_path):
    far = bank(tmp_path / "elsewhere" / "embedded")
    near = bank(tmp_path / "web")
    deck = make_deck(
        tmp_path / "decks" / "general",
        f'[deck]\ncards = ["{far}", "../../web"]\n',
    )
    assert config_mod.load(deck).cards == [far, near]


def test_a_single_path_may_be_written_without_the_list(tmp_path):
    bank(tmp_path / "deck" / "Questions")
    deck = make_deck(tmp_path / "deck", '[deck]\ncards = "Questions"\n')
    assert config_mod.load(deck).cards == [tmp_path / "deck" / "Questions"]


def test_an_empty_deck_file_is_a_deck_of_defaults(tmp_path):
    deck = make_deck(bank(tmp_path / "plain"), "")
    assert config_mod.load(deck).cards == [deck]


def test_a_deck_file_without_a_deck_section_is_an_error(tmp_path):
    deck = make_deck(bank(tmp_path / "wrong"), 'name = "Wrong"\n')
    with pytest.raises(config_mod.ConfigError) as exc:
        config_mod.load(deck)
    assert "[deck]" in str(exc.value)


def test_a_directory_without_a_deck_file_is_not_a_deck(tmp_path):
    with pytest.raises(config_mod.ConfigError) as exc:
        config_mod.load(bank(tmp_path / "loose"))
    assert "deck.toml" in str(exc.value)


def test_a_deck_naming_a_missing_bank_is_an_error(tmp_path):
    deck = make_deck(tmp_path / "gone", '[deck]\ncards = ["nowhere"]\n')
    with pytest.raises(config_mod.ConfigError) as exc:
        config_mod.load(deck)
    assert "does not exist" in str(exc.value)


def test_the_logs_are_named_by_the_deck_directory_not_its_title(tmp_path, home):
    deck = make_deck(bank(tmp_path / "embedded"), '[deck]\nname = "Embedded Screens"\n')
    config = config_mod.load(deck)
    # Retitling a deck must not start its history over.
    assert config.log_path.parts[-2:] == ("lernt", "embedded.jsonl")
    assert config.edits_path.name == "embedded.edits.jsonl"
    assert config.edits_path.parent == config.log_path.parent


def test_a_deck_may_name_its_own_logs_and_run_defaults(tmp_path, home):
    deck = make_deck(
        bank(tmp_path / "web"),
        '[deck]\ncount = 25\nlimit = 45\nlog = "~/web.jsonl"\nedits = "~/web-edits.jsonl"\n',
    )
    config = config_mod.load(deck)
    assert (config.count, config.limit) == (25, 45)
    assert config.log_path == Path("~/web.jsonl").expanduser()
    assert config.edits_path == Path("~/web-edits.jsonl").expanduser()


def test_resolve_takes_a_bare_name_from_the_decks_directory(home):
    make_deck(bank(decks(home) / "capitals"), '[deck]\nname = "Capitals"\n')
    assert config_mod.resolve("capitals").name == "Capitals"
    assert "capitals" in config_mod.available()


def test_the_decks_directory_can_be_moved(tmp_path, monkeypatch):
    monkeypatch.setenv("LERNT_DECKS", str(tmp_path / "mine"))
    make_deck(bank(tmp_path / "mine" / "capitals"))
    assert config_mod.resolve("capitals").id == "capitals"


def test_resolve_takes_a_path(tmp_path, home):
    deck = make_deck(bank(tmp_path / "portable"))
    assert config_mod.resolve(str(deck)).id == "portable"


def test_resolve_never_picks_a_deck_for_you(home):
    make_deck(bank(decks(home) / "only"))
    # Even with one deck: a run logs against the deck it ran, so it is named.
    with pytest.raises(config_mod.ConfigError) as exc:
        config_mod.resolve(None)
    assert "Name a deck" in str(exc.value)
    assert "only" in str(exc.value)


def test_an_unknown_name_lists_what_there_is(home):
    make_deck(bank(decks(home) / "embedded"))
    with pytest.raises(config_mod.ConfigError) as exc:
        config_mod.resolve("webdev")
    assert "embedded" in str(exc.value)


def test_no_decks_at_all_explains_how_to_make_one(home):
    with pytest.raises(config_mod.ConfigError) as exc:
        config_mod.resolve("anything")
    assert "[deck]" in str(exc.value)


def test_a_symlinked_deck_reads_its_target_but_keeps_its_link_name(tmp_path, home):
    real = tmp_path / "vault" / "Screens"
    bank(real / "Questions")
    make_deck(real, '[deck]\ncards = ["Questions"]\n')
    link = decks(home) / "embedded"
    link.parent.mkdir(parents=True)
    os.symlink(real, link)
    config = config_mod.resolve("embedded")
    assert config.cards == [real / "Questions"]
    assert config.id == "embedded"  # the log follows the link name


def test_listing_shows_a_broken_deck_rather_than_hiding_it(tmp_path, home):
    decks(home).mkdir(parents=True)
    os.symlink(tmp_path / "gone", decks(home) / "stale")
    make_deck(bank(decks(home) / "fine"))
    rows = {deck_id: (config, error) for deck_id, config, error in config_mod.listing()}
    assert rows["fine"][0] is not None
    assert rows["stale"][0] is None and rows["stale"][1]


def test_from_dirs_is_a_deck_that_was_never_a_directory(tmp_path):
    directory = bank(tmp_path / "Questions")
    config = config_mod.from_dirs([str(directory)])
    assert config.cards == [directory]
    assert (config.id, config.path) == ("ad-hoc", None)
    with pytest.raises(config_mod.ConfigError):
        config_mod.from_dirs([str(tmp_path / "nope")])


def test_describe_is_what_another_frontend_needs(tmp_path, home):
    deck = make_deck(bank(tmp_path / "capitals"), '[deck]\nname = "Capitals"\n')
    described = config_mod.load(deck).describe()
    assert described["id"] == "capitals"
    assert described["cards"] == [str(deck)]
    assert described["log"].endswith("capitals.jsonl")


def test_a_deck_may_say_what_it_is(tmp_path):
    deck = make_deck(
        bank(tmp_path / "capitals"),
        '[deck]\ndescription = """\nThe capitals\n  of Europe.\n"""\n',
    )
    config = config_mod.load(deck)
    assert config.description == "The capitals of Europe."
    assert config.describe()["description"] == "The capitals of Europe."
    assert config_mod.load(make_deck(bank(tmp_path / "bare"))).description == ""


def test_a_description_that_is_not_a_string_is_an_error(tmp_path):
    deck = make_deck(bank(tmp_path / "capitals"), "[deck]\ndescription = 3\n")
    with pytest.raises(config_mod.ConfigError, match="description"):
        config_mod.load(deck)


def test_a_deck_may_set_its_leitner_boxes(tmp_path):
    deck = make_deck(bank(tmp_path / "capitals"), "[deck]\nboxes = [1, 3, 9]\n")
    config = config_mod.load(deck)
    assert config.boxes == (1, 3, 9)
    assert config.describe()["boxes"] == [1, 3, 9]


def test_a_deck_without_boxes_gets_the_default_schedule(tmp_path):
    deck = make_deck(bank(tmp_path / "capitals"))
    assert config_mod.load(deck).boxes == (1, 2, 4, 8, 16)


def test_boxes_that_are_not_days_are_an_error(tmp_path):
    deck = make_deck(bank(tmp_path / "capitals"), "[deck]\nboxes = [1, 0]\n")
    with pytest.raises(config_mod.ConfigError, match="boxes"):
        config_mod.load(deck)
