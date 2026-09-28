"""The frontend, driven the way a person drives it - through the prompts."""

import io
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from lernt import cli, edits, misses

CARD = """# Does volatile make something thread-safe?

> [!tip]
> No - it is about the compiler, not atomicity.

---

**Follow-ups:**
1. [[Questions/Second card|How would you fix it?]]
"""

SECOND = """# How would you fix it?

> [!tip]
> Disable the interrupt across the read.
"""


@pytest.fixture
def bank(tmp_path):
    (tmp_path / "First card.md").write_text(CARD)
    (tmp_path / "Second card.md").write_text(SECOND)
    return tmp_path


@pytest.fixture
def home(tmp_path, monkeypatch):
    """An XDG home of our own - a test must never read or write the real one."""
    monkeypatch.setenv("XDG_DATA_HOME", str(tmp_path / "data"))
    monkeypatch.setenv("XDG_STATE_HOME", str(tmp_path / "state"))
    monkeypatch.delenv("LERNT_DECKS", raising=False)
    return tmp_path


def make_deck(directory, body="[deck]\n", cards=(("First card", CARD),)):
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "deck.toml").write_text(body)
    for name, text in cards:
        (directory / f"{name}.md").write_text(text)
    return directory


def decks(home):
    return home / "data" / "lernt" / "decks"


@pytest.fixture
def keys(monkeypatch):
    """Feeds the prompts a script, the way the tty would."""

    def feed(text):
        monkeypatch.setattr(cli, "_TTY", io.StringIO(text))

    return feed


def test_a_scored_run_writes_one_log_line_per_card(bank, tmp_path, keys, capsys):
    log = str(tmp_path / "attempts.jsonl")
    keys("\nf\nr\n\np\n")
    assert cli.main(["-n", "2", "--cards", str(bank), "--log", log, "-d"]) == 0
    out = capsys.readouterr().out
    assert "pass 1   shaky 0   fail 1   (of 2)" in out
    scored = {(e["card"], e["score"]) for e in misses.load(log)}
    assert len(scored) == 2
    assert "fail" in {s for _, s in scored}


def test_the_model_answer_is_revealed_after_the_answer_not_before(
    bank, tmp_path, keys, capsys
):
    keys("\np\nq\n")
    cli.main(["-n", "2", "--cards", str(bank), "--no-log"])
    out = capsys.readouterr().out
    asked = out.index("(Enter when you have finished answering)")
    # Either card may come up first, so assert on the shape: the reveal lands
    # between the answering prompt and the score prompt, never above them.
    revealed = min(i for i in (out.find("atomicity"), out.find("interrupt")) if i > 0)
    assert asked < revealed < out.index("p)ass")


def test_q_ends_the_run_and_still_scores_it(bank, tmp_path, keys, capsys):
    keys("\np\nq\n")
    cli.main(["-n", "5", "--cards", str(bank), "--no-log"])
    out = capsys.readouterr().out
    assert "Ended early." in out
    assert "(of 1)" in out


def test_ctrl_d_at_the_first_prompt_still_prints_a_summary(bank, keys, capsys):
    keys("")
    assert cli.main(["--cards", str(bank), "--no-log"]) == 0
    assert "Ended early." in capsys.readouterr().out


def test_weak_mode_refuses_to_run_when_nothing_is_outstanding(bank, tmp_path, capsys):
    log = str(tmp_path / "attempts.jsonl")
    misses.append(log, "First card", "q", "pass", 5)
    assert cli.main(["-w", "--cards", str(bank), "--log", log]) == 0
    assert "Nothing is on the miss list" in capsys.readouterr().out


def test_weak_mode_drills_exactly_the_miss_list(bank, tmp_path, keys, capsys):
    log = str(tmp_path / "attempts.jsonl")
    misses.append(log, "Second card", "How would you fix it?", "fail", 40)
    keys("\np\n")
    assert cli.main(["-w", "-n", "1", "--cards", str(bank), "--log", log]) == 0
    out = capsys.readouterr().out
    assert "[1/1] How would you fix it?" in out
    # The card it hangs off is still printed as the "follows from" line, which
    # is context rather than a second question, so only the header is checked.
    assert "[1/1] Does volatile" not in out


def test_the_miss_report_names_a_card_that_has_left_the_bank(bank, tmp_path, capsys):
    log = str(tmp_path / "attempts.jsonl")
    misses.append(log, "Renamed away", "An old question?", "fail", 30)
    assert cli.main(["--misses", "--cards", str(bank), "--log", log]) == 0
    out = capsys.readouterr().out
    assert "no longer in the bank" in out
    assert "Renamed away" in out


def test_the_frontend_reads_an_exported_deck(bank, tmp_path, keys, capsys):
    from lernt import deck as deck_mod

    out_file = tmp_path / "deck.json"
    deck_mod.main(["--cards", str(bank), "--out", str(out_file)])
    keys("\np\nq\n")
    assert cli.main(["--json", str(out_file), "-n", "1", "--no-log"]) == 0
    assert "(of 1)" in capsys.readouterr().out


def test_a_deck_names_the_run_and_its_log(home, keys, capsys):
    make_deck(decks(home) / "embedded", '[deck]\nname = "Embedded screens"\n')
    keys("\np\n")
    assert cli.main(["embedded", "-n", "1"]) == 0
    out = capsys.readouterr().out
    assert "Embedded screens - up to 1 card" in out
    # Named by the deck's id, not its title or the directory it read.
    log = home / "state" / "lernt" / "embedded.jsonl"
    assert [e["card"] for e in misses.load(str(log))] == ["First card"]


def test_two_decks_keep_two_histories(tmp_path, home):
    for name in ("embedded", "web"):
        misses.append(
            str(tmp_path / "state" / "lernt" / (f"{name}.jsonl")),
            "shared card name",
            "q",
            "fail" if name == "embedded" else "pass",
            9,
        )
    embedded = misses.load(str(tmp_path / "state" / "lernt" / "embedded.jsonl"))
    web = misses.load(str(tmp_path / "state" / "lernt" / "web.jsonl"))
    assert [e["score"] for e in embedded] == ["fail"]
    assert [e["score"] for e in web] == ["pass"]


def test_run_defaults_come_from_the_deck_and_the_flags_still_win(
    tmp_path, home, keys, capsys
):
    deck = make_deck(
        tmp_path / "configured", '[deck]\nname = "Configured"\ncount = 3\nlimit = 45\n'
    )
    keys("\np\nq\n")
    cli.main([str(deck), "--no-log"])
    assert "up to 3 cards from a pool of 1, 45s each" in capsys.readouterr().out
    keys("\np\nq\n")
    cli.main([str(deck), "-n", "1", "-t", "10", "--no-log"])
    assert "up to 1 card from a pool of 1, 10s each" in capsys.readouterr().out


def test_list_shows_the_decks_and_their_banks(tmp_path, home, capsys):
    (tmp_path / "cards").mkdir()
    make_deck(
        decks(home) / "general",
        '[deck]\nname = "General software"\ncards = ["%s"]\n' % (tmp_path / "cards"),
        cards=(),
    )
    assert cli.main(["--list"]) == 0
    out = capsys.readouterr().out
    assert "general" in out and "General software" in out
    assert str(tmp_path / "cards") in out


def test_list_shows_a_broken_deck_rather_than_hiding_it(tmp_path, home, capsys):
    decks(home).mkdir(parents=True)
    (decks(home) / "stale").symlink_to(tmp_path / "gone")
    assert cli.main(["--list"]) == 0
    assert "[unusable]" in capsys.readouterr().out


def test_run_options_without_a_deck_ask_for_one(home, capsys):
    make_deck(decks(home) / "only")
    assert cli.main(["-n", "3"]) == 1
    err = capsys.readouterr().err
    assert "Name a deck" in err and "only" in err


def test_a_draft_stays_out_of_random_jumps_but_is_marked_on_the_menu(
    bank, keys, capsys
):
    (bank / "Second card.md").write_text(SECOND + "\n---\n\n#draft\n")
    keys("\np\nq\n")
    cli.main(["-n", "2", "--cards", str(bank), "--no-log"])
    out = capsys.readouterr().out
    assert "[1/2] Does volatile" in out  # the only card not a draft
    assert "How would you fix it?   [draft]" in out


# -- bare lernt: the bundled TUI -------------------------------------------


@pytest.fixture
def execs(monkeypatch):
    calls = []
    monkeypatch.setattr(
        cli.os, "execve", lambda path, argv, env: calls.append((path, argv, env))
    )
    return calls


@pytest.fixture
def built(tmp_path, monkeypatch):
    tui = tmp_path / "_bin" / "lernt-tui"
    tui.parent.mkdir()
    tui.write_text("#!/bin/sh\n")
    tui.chmod(0o755)
    monkeypatch.setattr(cli, "TUI", tui)
    return tui


def test_bare_lernt_runs_the_bundled_tui_and_hands_it_the_engine(
    home, monkeypatch, built, execs
):
    monkeypatch.setattr(cli, "_interactive", lambda: True)
    assert cli.main([]) == 0
    ((path, argv, env),) = execs
    assert (path, argv) == (built, [str(built)])
    assert json.loads(env["LERNT_ENGINE"]) == [sys.executable, "-m", "lernt.deck"]


def test_bare_lernt_with_the_tui_not_built_lists_the_decks(
    tmp_path, home, monkeypatch, execs, capsys
):
    monkeypatch.setattr(cli, "TUI", tmp_path / "not-built")
    monkeypatch.setattr(cli, "_interactive", lambda: True)
    make_deck(decks(home) / "capitals", '[deck]\nname = "Capitals"\n')
    assert cli.main([]) == 0
    out = capsys.readouterr().out
    assert execs == []
    assert "Capitals" in out and "not built" in out


def test_bare_lernt_outside_a_terminal_lists_rather_than_launching(
    home, monkeypatch, built, execs, capsys
):
    monkeypatch.setattr(cli, "_interactive", lambda: False)
    assert cli.main([]) == 0
    assert execs == []
    assert "No decks in" in capsys.readouterr().out


def test_the_engine_command_the_tui_is_handed_runs(tmp_path):
    deck = tmp_path / "decks" / "capitals"
    make_deck(deck, '[deck]\nname = "Capitals"\n')
    env = {
        **os.environ,
        "LERNT_DECKS": str(tmp_path / "decks"),
        "PYTHONPATH": str(Path(__file__).resolve().parents[1]),
    }
    result = subprocess.run(
        [sys.executable, "-m", "lernt.deck", "--list"],
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    assert [d["id"] for d in json.loads(result.stdout)["decks"]] == ["capitals"]


# -- edit mode -------------------------------------------------------------


@pytest.fixture
def editor(tmp_path, monkeypatch):
    """An $EDITOR that rewrites the question line, the way a person would."""
    script = tmp_path / "editor.py"
    script.write_text(
        f"#!{sys.executable}\n"
        "import pathlib, sys\n"
        "p = pathlib.Path(sys.argv[1])\n"
        "p.write_text(p.read_text().replace('# ', '# EDITED ', 1))\n"
    )
    script.chmod(0o755)
    monkeypatch.delenv("VISUAL", raising=False)
    monkeypatch.setenv("EDITOR", str(script))
    return script


def edit_log(home):
    return home / "state" / "lernt" / "ad-hoc.edits.jsonl"


def test_edit_mode_notes_and_flags_a_card_and_still_scores_it(bank, home, keys, capsys):
    attempts = str(home / "attempts.jsonl")
    keys("\nf\nn\nthe question needs to clarify X\nx\nduplicate\n\n")
    assert cli.main(["-n", "1", "-e", "--cards", str(bank), "--log", attempts]) == 0
    out = capsys.readouterr().out
    assert "fail 1" in out
    assert [e["score"] for e in misses.load(attempts)] == ["fail"]
    rows = edits.load(edit_log(home))
    assert [(e["kind"], e["text"]) for e in rows] == [
        ("note", "the question needs to clarify X"),
        ("remove", "duplicate"),
    ]
    assert "1 noted, 1 flagged for removal" in out


def test_editing_a_card_rewrites_its_file_and_clears_what_stood(
    bank, home, keys, editor, capsys
):
    keys("\np\nn\nclarify X\ne\n\n")
    assert cli.main(["-n", "1", "-e", "--cards", str(bank), "--no-log"]) == 0
    out = capsys.readouterr().out
    (edited,) = [p for p in bank.glob("*.md") if "# EDITED " in p.read_text()]
    assert "EDITED" in out  # the card is shown again as it now reads
    rows = edits.load(edit_log(home))
    assert [e["kind"] for e in rows] == ["note", "edit"]
    assert rows[-1]["card"] == edited.stem
    assert edits.standing(rows) == {}


def test_an_editor_that_changes_nothing_is_not_an_edit(
    bank, home, keys, monkeypatch, capsys
):
    monkeypatch.delenv("VISUAL", raising=False)
    monkeypatch.setenv("EDITOR", "true")
    keys("\np\ne\n\n")
    cli.main(["-n", "1", "-e", "--cards", str(bank), "--no-log"])
    assert "Unchanged." in capsys.readouterr().out
    assert edits.load(edit_log(home)) == []


def test_a_card_comes_up_with_what_stands_against_it(bank, home, keys, capsys):
    for card in ("First card", "Second card"):
        edits.append(edit_log(home), card, card, "note", f"fix {card}")
    keys("\np\nd\n\n")
    cli.main(["-n", "1", "-e", "--cards", str(bank), "--no-log"])
    out = capsys.readouterr().out
    assert "Standing against this card:" in out
    assert len(edits.standing(edits.load(edit_log(home)))) == 1


def test_without_edit_mode_there_is_no_edit_step(bank, home, keys, capsys):
    keys("\np\nq\n")
    cli.main(["-n", "2", "--cards", str(bank), "--no-log"])
    assert "flag for removal" not in capsys.readouterr().out


def test_the_edit_report_lists_what_stands_with_the_file_to_open(bank, home, capsys):
    edits.append(edit_log(home), "Second card", "old title", "note", "clarify X")
    edits.append(edit_log(home), "First card", "old title", "remove", "dup")
    edits.append(edit_log(home), "Gone card", "Gone?", "note", "still true")
    assert cli.main(["--edits", "--cards", str(bank)]) == 0
    out = capsys.readouterr().out
    assert out.index("Does volatile") < out.index("How would you fix it?")
    assert "First card.md" in out
    assert "clarify X" in out
    assert "no longer in the bank" in out
    assert "Gone card" in out


# -- time ------------------------------------------------------------------------

TIMED = "# Quick one?\n\nYes.\n\n---\n\n**Time:** 1m\n"


def test_a_card_with_its_own_time_shows_it(tmp_path, keys, capsys):
    (tmp_path / "Timed.md").write_text(TIMED)
    keys("\np\n")
    assert cli.main(["-n", "1", "--cards", str(tmp_path), "--no-log"]) == 0
    out = capsys.readouterr().out
    assert "each card's own time, else 30s" in out
    assert "[1/1] Quick one?   (60s)" in out


def test_t_off_runs_untimed(tmp_path, keys, capsys):
    (tmp_path / "Timed.md").write_text(TIMED)
    keys("\np\n")
    assert cli.main(["-n", "1", "-t", "off", "--cards", str(tmp_path), "--no-log"]) == 0
    out = capsys.readouterr().out
    assert "from a pool of 1, untimed." in out
    assert "(60s)" not in out


def test_t_refuses_what_is_not_a_time(tmp_path, capsys):
    with pytest.raises(SystemExit):
        cli.main(["-t", "soon", "--cards", str(tmp_path)])
    assert "takes seconds, off or auto" in capsys.readouterr().err


# -- the Leitner boxes -----------------------------------------------------


def test_the_box_report_counts_new_cards_as_due(bank, tmp_path, capsys):
    log = str(tmp_path / "attempts.jsonl")
    assert cli.main(["--boxes", "--cards", str(bank), "--log", log]) == 0
    out = capsys.readouterr().out
    assert "ad hoc, the Leitner boxes" in out
    assert "2 card(s) due today - -l drills exactly those." in out


def test_a_due_run_shows_the_box_and_where_the_score_moved_it(
    bank, tmp_path, keys, capsys
):
    log = str(tmp_path / "attempts.jsonl")
    keys("\np\n")
    assert cli.main(["-l", "-n", "1", "--cards", str(bank), "--log", log]) == 0
    out = capsys.readouterr().out
    assert "The due cards only, lowest Leitner box first." in out
    assert "[new]" in out
    assert "new -> box 2" in out
    assert "1 up a box, 0 back to box 1" in out


def test_a_due_run_with_nothing_due_says_so_and_stops(bank, tmp_path, capsys):
    log = str(tmp_path / "attempts.jsonl")
    for card in ("First card", "Second card"):
        misses.append(log, card, "q", "pass", 5)  # today: due in two days
    assert cli.main(["-l", "--cards", str(bank), "--log", log]) == 0
    assert "Nothing is due today." in capsys.readouterr().out
