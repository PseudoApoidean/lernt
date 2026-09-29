"""The TUI in a real terminal, with and without tmux.

The screens are tested in Go against a fake engine and a fake terminal, which cannot
say how a terminal really draws a line: tmux draws `1️⃣` in two cells where the
Unicode tables say one, and a line that comes out a cell too long wraps and pushes
the whole page down. These run the built binary, against the real engine, in two
terminals - tmux itself, and a plain pseudo-terminal read through pyte - and paste in
the emoji that have gone wrong, checking the page stays whole: the title on the first
row, the key hints on the last, and no row lost or repeated.

They start real terminals, so they take about ten seconds. They are part of `make test`
(which builds the TUI first), and left out of `make test-fast`; `make test-terminal` runs
only them. They skip where the TUI is not built, or tmux or pyte is missing.
"""

import fcntl
import json
import os
import pty
import select
import shutil
import struct
import subprocess
import sys
import termios
import time
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[1]
TUI = REPO / "lernt" / "_bin" / "lernt-tui"
COLS, ROWS = 110, 32

pyte = pytest.importorskip("pyte")

pytestmark = [
    pytest.mark.terminal,
    pytest.mark.skipif(not TUI.is_file(), reason="the TUI is not built: make tui"),
]

# The emoji that tell the terminals apart: plain, with a skin tone, joined, a flag,
# made wide by a variation selector, a keycap (the one tmux and the tables disagree
# on), a recent one, a lone skin tone, and two text-default symbols.
EMOJI = [
    ("plain", "🔴"),
    ("skin tone", "👍🏽"),
    ("joined family", "👨‍👩‍👧"),
    ("flag", "🇬🇷"),
    ("variation selector", "❤️"),
    ("check", "✔️"),
    ("keycap", "1️⃣"),
    ("recent", "🫠"),
    ("technologist", "🧑‍💻"),
    ("rainbow flag", "🏳️‍🌈"),
    ("lone skin tone", "🏻"),
    ("text-default emoji", "🏋"),
    ("hand", "☝"),
]


@pytest.fixture(scope="module")
def world(tmp_path_factory):
    """A decks directory with one deck and a card, and the environment to run in."""
    tmp_path = tmp_path_factory.mktemp("terminal")
    deck = tmp_path / "decks" / "red"
    deck.mkdir(parents=True)
    (deck / "deck.toml").write_text('[deck]\nname = "Red"\n')
    (deck / "Which.md").write_text("# Which is red?\n\n> [!tip]\n> This one.\n")
    env = {
        "PATH": os.environ["PATH"],
        "HOME": os.environ["HOME"],  # not a temporary one: tmux does not work with it
        "LANG": "en_US.UTF-8",
        "LERNT_DECKS": str(tmp_path / "decks"),
        "XDG_STATE_HOME": str(tmp_path / "state"),
        "PYTHONPATH": str(REPO),
        "LERNT_ENGINE": json.dumps([sys.executable, "-m", "lernt.deck"]),
    }
    return tmp_path, env


class Terminal:
    """What both kinds share: keys in, the screen as rows out, and waiting for what
    the screen is expected to show, since how long a terminal takes depends on the
    machine."""

    kind = ""

    def wait(self, seconds):
        time.sleep(seconds)

    def until(self, wanted, seconds=4):
        """The rows once `wanted` is on the screen, or as they are after `seconds`."""
        end = time.time() + seconds
        while True:
            rows = self.rows()
            if wanted in "\n".join(rows) or time.time() > end:
                return rows
            self.wait(0.05)

    def settled(self, seconds=3):
        """The rows once they have stopped changing: a paste is drawn in pieces."""
        end, last = time.time() + seconds, None
        while True:
            rows = self.rows()
            if rows == last or time.time() > end:
                return rows
            last = rows
            self.wait(0.1)


class Plain(Terminal):
    """The TUI on a pseudo-terminal, its output read through pyte. No tmux: TMUX is
    not set, and the terminal is an xterm."""

    kind = "plain"

    def __init__(self, env):
        self.screen = pyte.Screen(COLS, ROWS)
        self.stream = pyte.ByteStream(self.screen)
        pid, self.fd = pty.fork()
        if pid == 0:
            os.execve(str(TUI), [str(TUI)], {**env, "TERM": "xterm-256color"})
        self.pid = pid
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
        self.wait(0.4)
        # It asks the terminal its background and cursor position before it draws.
        os.write(self.fd, b"\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R")
        self.until("Red")

    def wait(self, seconds):
        end = time.time() + seconds
        while time.time() < end:
            ready, _, _ = select.select([self.fd], [], [], 0.02)
            if ready:
                try:
                    self.stream.feed(os.read(self.fd, 65536))
                except OSError:
                    return

    def keys(self, *keys):
        for key in keys:
            os.write(self.fd, key.encode())
            self.wait(0.12)

    def paste(self, text):
        os.write(self.fd, b"\x1b[200~" + text.encode() + b"\x1b[201~")

    def rows(self):
        self.wait(0.02)
        return [row.rstrip() for row in self.screen.display]

    def close(self):
        os.close(self.fd)
        try:
            os.kill(self.pid, 9)
            os.waitpid(self.pid, 0)
        except OSError:
            pass


class Tmux(Terminal):
    """The TUI inside a real tmux, read with capture-pane."""

    kind = "tmux"
    KEYS = {"\x1b[C": "Right", "\r": "Enter", "\x1b": "Escape"}

    def __init__(self, env, tmp_path):
        self.sock = f"lernt-test-{os.getpid()}-{time.time_ns()}"
        self.tmp = tmp_path
        # -e adds to the environment tmux gives the pane, which has TMUX in it; a
        # cleared environment would take that out, and the TUI would not know.
        variables = [arg for k, v in env.items() for arg in ("-e", f"{k}={v}")]
        self.tm(
            "new-session", "-d", "-s", "term", "-x", str(COLS), "-y", str(ROWS),
            *variables, str(TUI),
        )  # fmt: skip
        self.until("Red", seconds=8)

    def tm(self, *args):
        return subprocess.run(
            ["tmux", "-L", self.sock, *args],
            capture_output=True,
            text=True,
            check=False,
        ).stdout

    def keys(self, *keys):
        for key in keys:
            self.tm("send-keys", "-t", "term", self.KEYS.get(key, key))
            self.wait(0.15)

    def paste(self, text):
        buf = self.tmp / "paste"
        buf.write_text(text)
        self.tm("load-buffer", str(buf))
        self.tm("paste-buffer", "-p", "-t", "term")

    def rows(self):
        rows = self.tm("capture-pane", "-p", "-t", "term").split("\n")[:ROWS]
        return [row.rstrip() for row in rows] + [""] * (ROWS - len(rows))

    def close(self):
        self.tm("kill-server")
        # A killed server leaves its socket file; it is this test's own, so it goes.
        socket = (
            Path(os.environ.get("TMUX_TMPDIR", "/tmp"))
            / f"tmux-{os.getuid()}"
            / self.sock
        )
        socket.unlink(missing_ok=True)


@pytest.fixture(scope="module", params=["tmux", "plain"])
def terminal(request, world):
    """One terminal of each kind for the whole module: starting the TUI takes a
    couple of seconds, so it is started once and the tests go on from the deck list,
    each leaving it there."""
    tmp_path, env = world
    if request.param == "tmux":
        if shutil.which("tmux") is None:
            pytest.skip("tmux is not installed")
        term = Tmux(env, tmp_path)
    else:
        term = Plain(env)
    yield term
    term.close()


def text(rows):
    return "\n".join(rows)


def test_the_deck_list_draws_with_the_hints_on_the_last_line(terminal):
    rows = terminal.until("change screen")
    assert "lernt" in rows[0] and "1 Decks" in rows[0]
    assert "Red" in text(rows)
    assert "1-3 change screen" in rows[ROWS - 1]


def test_the_tmux_warning_is_in_tmux_and_only_there_and_only_on_the_main_screen(
    terminal,
):
    warning = "In tmux, lines containing emoji may look misaligned."
    assert (warning in text(terminal.rows())) == (terminal.kind == "tmux")
    terminal.keys("2")
    assert warning not in text(terminal.until("→ cards"))
    terminal.keys("1")
    terminal.until("enter run")


def test_pasted_emoji_do_not_break_the_page(terminal):
    """Each pasted in turn into the box of a new card, the page checked after each:
    a line that came out too long would push the title off the top and repeat the
    hints. Then all at once, and out of the box."""
    terminal.keys(
        "2", "\x1b[C", "n", "\r"
    )  # the Library, a new card, typing the question
    terminal.until("done typing")
    for name, emoji in EMOJI:
        terminal.paste(f"x{emoji}y")
        rows = terminal.settled()
        shown = text(rows)
        assert "lernt" in rows[0], (
            f"{name}: the title bar is not the first row:\n{shown}"
        )
        assert "done typing" in rows[ROWS - 1], (
            f"{name}: the hints are not the last row:\n{shown}"
        )
        assert shown.count("done typing") == 1, (
            f"{name}: the hints are repeated:\n{shown}"
        )
        assert shown.count("2 Library") == 1, (
            f"{name}: the title bar is repeated:\n{shown}"
        )
    terminal.paste("".join(e for _, e in EMOJI) * 2)
    terminal.settled()
    terminal.keys("\x1b")  # esc: out of the box
    rows = terminal.until("esc cancel")
    assert "lernt" in rows[0] and "esc cancel" in rows[ROWS - 1], text(rows)
    assert "Card options" in text(rows) and "Actions" in text(rows)
    # Back to the deck list for whatever comes next: out of the form, the pane, the screen.
    terminal.keys("\x1b", "\x1b", "\x1b", "1")
    terminal.until("enter run")


def test_a_selected_setting_is_one_unbroken_bar(terminal):
    """The bar on a selected row is the highlight colour from one edge of the card to the
    other. A part of the row styled on its own (a swatch, a faint value) ends the bar
    where it ends, so this reads the colour of every cell of the row."""
    if terminal.kind != "plain":
        pytest.skip("cell colours are read from the pyte screen, not from tmux")
    terminal.keys("3", "\x1b[C", "\x1b[C")  # the settings, and their Visual tab
    terminal.until("Colour mode")
    terminal.keys("\x1b[B", "\r")  # the theme colour row, selected
    rows = terminal.until("‹")
    y = next(i for i, row in enumerate(rows) if "Theme colour" in row)
    # The bar is inside the card's border and its cell of padding on each side.
    start, end = rows[y].index("│") + 2, rows[y].rindex("│") - 1
    colours = {terminal.screen.buffer[y][x].bg for x in range(start, end)}
    assert len(colours) == 1 and "default" not in colours, (
        f"the bar is not one colour across the row: {colours}\n{rows[y]}"
    )
    terminal.keys("\r", "\x1b", "1")  # let go of the row, leave the settings
    terminal.until("enter run")
