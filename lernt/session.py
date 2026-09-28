"""The walk: which card comes next, and what the run scored.

Pure state over a deck dict - no printing, no reading, no clock. A terminal
frontend and a GUI can both drive one of these, which is the reason it is not
simply part of the CLI.
"""

import random

SCORES = ("pass", "shaky", "fail")


class SessionError(Exception):
    pass


class Session:
    def __init__(
        self,
        deck,
        count=10,
        include_drafts=False,
        name_filter="",
        only=None,
        rank=None,
        rng=None,
    ):
        """`only` restricts the pool to a set of card ids - how --weak feeds
        the miss log back in without this module knowing what a miss log is.
        `rank` orders random picks, card id -> number, lowest first - how a
        Leitner run asks the lowest box first without this module knowing what
        a box is."""
        self.cards = {c["id"]: c for c in deck["cards"]}
        self.count = count
        self.include_drafts = include_drafts
        self.rank = rank or {}
        self.rng = rng or random.Random()
        self.pool = [
            cid
            for cid, card in sorted(self.cards.items())
            if (not name_filter or name_filter.lower() in cid.lower())
            and (include_drafts or not card.get("draft"))
            and (only is None or cid in only)
        ]
        if not self.pool:
            raise SessionError("No cards matched.")
        self.seen = []
        self.scores = dict.fromkeys(SCORES, 0)
        self.review = []
        self.ended_early = False

    # -- picking ---------------------------------------------------------

    def pick_random(self):
        """A card not yet asked this run; once the pool is exhausted, anything.
        Unseen-first matters more than it looks: with a pool of a hundred-odd
        cards a naive shuffle repeats inside ten. Within that, the lowest rank
        goes first, and cards of one rank are picked at random."""
        candidates = [c for c in self.pool if c not in self.seen] or self.pool
        if self.rank:
            lowest = min(self.rank.get(c, 0) for c in candidates)
            candidates = [c for c in candidates if self.rank.get(c, 0) == lowest]
        return self.rng.choice(candidates)

    def pick_next(self, current):
        """(card_id, followed). Follows an unasked branch off `current` where
        there is one, and otherwise starts a fresh thread: down a line until it
        is exhausted, then on.

        Drafts stay out of reach here unless they were let into the pool, the
        same rule random jumps follow. Choosing one off a menu is a deliberate
        act; being steered into one is not."""
        candidates = [
            t
            for t in self.cards[current]["followups"]
            if t in self.cards
            and t not in self.seen
            and (self.include_drafts or not self.cards[t].get("draft"))
        ]
        if candidates:
            return self.rng.choice(candidates), True
        return self.pick_random(), False

    def followups(self, current):
        """The branches to offer on a menu - every real one, drafts included,
        because picking one off a menu is a deliberate choice."""
        return [t for t in self.cards[current]["followups"] if t in self.cards]

    def parent(self, current):
        parents = self.cards[current].get("parents") or []
        return parents[0] if parents else None

    # -- running ---------------------------------------------------------

    def start(self):
        return self.pick_random()

    def visit(self, card):
        self.seen.append(card)

    def score(self, card, score):
        if score not in SCORES:
            raise SessionError(f"Unknown score {score!r}")
        self.scores[score] += 1
        if score != "pass":
            self.review.append((score, self.cards[card]["title"]))

    def update(self, card):
        """Replace a card mid-run with a re-read copy, keyed by its id. What it
        was scored under stays as scored: the review list keeps the title it
        was asked by."""
        if card["id"] not in self.cards:
            raise SessionError(f"No card {card['id']!r} in this run")
        self.cards[card["id"]] = card

    def finished(self):
        return self.count != 0 and len(self.seen) >= self.count

    @property
    def asked(self):
        return sum(self.scores.values())
