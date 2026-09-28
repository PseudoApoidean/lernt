"""completions/_lernt against the parsers it describes. A flag added to argparse
and not to the completion file - or left in the file after it was removed -
fails here rather than at somebody's Tab key."""

import re
import shutil
import subprocess
from pathlib import Path

import pytest

from lernt import cli, deck

COMPLETION = Path(__file__).resolve().parents[1] / "completions" / "_lernt"

# An option at a word boundary: "-w", "--no-log", but not the "-log" inside
# "--no-log" or the "_lernt" in a helper's name.
_OPTION = re.compile(r"(?<![\w-])--?[A-Za-z][\w-]*")


def parser_flags(parser):
    return {flag for action in parser._actions for flag in action.option_strings}


def completion_flags(service):
    """The flags declared in one `case` branch. Only the part of each spec before
    its "[description]" is read, so an action's own switches (`_files -g`) and
    positional specs (no description) are not mistaken for flags."""
    text = COMPLETION.read_text()
    branch = re.search(rf"^\s*{re.escape(service)}\)\n(.*?)^\s*;;", text, re.M | re.S)
    assert branch, f"no {service}) branch in {COMPLETION.name}"
    flags = set()
    for line in branch.group(1).splitlines():
        head, bracket, _ = line.partition("[")
        if bracket:
            flags.update(_OPTION.findall(head))
    return flags


@pytest.mark.parametrize(
    ("service", "parser"),
    [("lernt", cli.build_parser), ("lernt-deck", deck.build_parser)],
)
def test_the_completion_file_offers_exactly_the_parsers_flags(service, parser):
    declared = completion_flags(service)
    real = parser_flags(parser())
    assert real - declared == set(), f"missing from {COMPLETION.name}"
    assert declared - real == set(), f"in {COMPLETION.name} but not the parser"


def test_the_completion_file_names_both_commands():
    first = COMPLETION.read_text().splitlines()[0]
    assert first.split() == ["#compdef", "lernt", "lernt-deck"]


@pytest.mark.skipif(shutil.which("zsh") is None, reason="zsh is not installed")
def test_the_completion_file_is_valid_zsh():
    result = subprocess.run(
        ["zsh", "-n", str(COMPLETION)], capture_output=True, text=True, check=False
    )
    assert result.returncode == 0, result.stderr
