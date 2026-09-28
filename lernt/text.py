"""Markdown to something a terminal can print.

Only "**" is stripped, never a lone "*". Emphasis markers survive as cosmetic
noise, which is much cheaper than mangling "const char *p" into "const char p"
- a C question whose whole point is the pointer star.
"""

import html
import re
import textwrap

_PIPED = re.compile(r"\[\[[^\]|]*\|([^\]]*)\]\]")
_PLAIN = re.compile(r"\[\[([^\]]*)\]\]")


def clean(text):
    text = _PIPED.sub(r"\1", text)
    text = _PLAIN.sub(r"\1", text)
    return html.unescape(text).replace("**", "").replace("`", "")


def wrap(lines, width=74, indent="      "):
    """Wrapped to the terminal, indented under the question. Blank lines are
    kept - a note paragraph reads as a separate beat from the answer."""
    out = []
    for line in lines:
        if not line.strip():
            out.append("")
            continue
        out.extend(
            textwrap.wrap(
                line, width=width, initial_indent=indent, subsequent_indent=indent
            )
            or [indent]
        )
    return out
