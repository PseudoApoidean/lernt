"""Lernt: decks of Markdown flash cards, read once and driven by any frontend.

Two halves, deliberately separable (see README):

  deck    - the engine. Reads a deck's Markdown cards and emits one JSON
            document: every card, its answer, and the branches out of it.
  cli     - one frontend. Takes that JSON and runs a timed walk in a terminal,
            or hands a bare `lernt` over to a TUI.

Nothing in `deck` knows about a terminal, and nothing in `cli` parses Markdown,
so a second frontend is a second consumer of the same JSON rather than a fork.
"""

__version__ = "2.0.0"
