"""Saved run options: what a frontend's options screen was last set to, per deck,
and its settings for lernt as a whole.

A flat JSON object of string keys to string values, one file per deck under XDG
state, and one more for the settings. Keyed, never positional: a frontend can
add, drop or reorder its options and every value it still knows comes back. A
save merges into what is there, so a key one frontend version stops sending is
kept rather than lost, in case a later one knows it again. What a value means is
the frontend's business - this module only keeps them.
"""

import json
from pathlib import Path


def load(path):
    """key -> value. Nothing saved, or a file that will not parse, is {}: a
    lost preference falls back to the deck's defaults, it never stops a run."""
    try:
        data = json.loads(Path(path).expanduser().read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}
    if not isinstance(data, dict):
        return {}
    return {
        str(key): _text(value)
        for key, value in data.items()
        if key and value is not None and not isinstance(value, dict | list)
    }


def save(path, values):
    """Merge `values` into the file and return the whole of it. Written to a
    temporary file and renamed over, so a crash mid-write leaves the old file."""
    path = Path(path).expanduser()
    merged = load(path)
    merged.update({str(k): _text(v) for k, v in values.items()})
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(
        json.dumps(merged, indent=2, sort_keys=True, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    temporary.replace(path)
    return merged


def _text(value):
    """Hand-edited files may hold 12 or true; the API speaks strings."""
    if isinstance(value, bool):
        return "on" if value else "off"
    return str(value)
