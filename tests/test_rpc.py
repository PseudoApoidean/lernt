"""The API over the wire: JSON-RPC lines in, JSON-RPC lines out."""

import io
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from lernt import api, rpc

CARD = "# Capital of France?\n\n> [!tip]\n> Paris.\n"


@pytest.fixture
def home(tmp_path, monkeypatch):
    monkeypatch.setenv("LERNT_DECKS", str(tmp_path / "decks"))
    monkeypatch.setenv("XDG_STATE_HOME", str(tmp_path / "state"))
    deck = tmp_path / "decks" / "capitals"
    deck.mkdir(parents=True)
    (deck / "deck.toml").write_text(
        '[deck]\nname = "Capitals"\ndescription = "Capital cities."\n'
    )
    (deck / "France.md").write_text(CARD)
    return tmp_path


def request(method, params=None, request_id=1):
    message = {"jsonrpc": "2.0", "id": request_id, "method": method}
    if params is not None:
        message["params"] = params
    return json.dumps(message)


def call(engine, method, params=None):
    return rpc.handle(engine, request(method, params))


def error_code(response):
    return response["error"]["code"]


def test_a_call_returns_its_result_as_json(home):
    response = call(api.Engine(), "engine.info")
    assert response == {
        "jsonrpc": "2.0",
        "id": 1,
        "result": {"protocol": api.PROTOCOL, "version": response["result"]["version"]},
    }


def test_a_run_over_the_wire(home):
    engine = api.Engine()
    started = call(engine, "run.start", {"deck": "capitals", "count": 1})["result"]
    assert started["card"]["answer"] == ["Paris."]
    assert started["log"].endswith("capitals.jsonl")
    scored = call(engine, "run.score", {"score": "pass", "seconds": 2})["result"]
    assert scored["finished"] is True
    assert call(engine, "run.end")["result"]["passed"] == 1


@pytest.mark.parametrize(
    ("line", "expected"),
    [
        ("{not json", rpc.PARSE_ERROR),
        ('{"id": 1, "method": "engine.info"}', rpc.INVALID_REQUEST),
        ('["jsonrpc", "2.0"]', rpc.INVALID_REQUEST),
        (request("engine.nope"), rpc.METHOD_NOT_FOUND),
        (request("run.start", {}), rpc.INVALID_PARAMS),
        (request("run.start", {"deck": 3}), rpc.INVALID_PARAMS),
        (request("run.start", {"deck": "capitals", "count": True}), rpc.INVALID_PARAMS),
        (
            request("run.start", {"deck": "capitals", "colour": "red"}),
            rpc.INVALID_PARAMS,
        ),
        (request("run.start", ["capitals"]), rpc.INVALID_PARAMS),
        (request("run.start", {"deck": "nope"}), api.NO_DECK),
        (request("run.score", {"score": "pass", "seconds": 1}), api.NOT_NOW),
    ],
)
def test_errors_carry_a_code(home, line, expected):
    assert error_code(rpc.handle(api.Engine(), line)) == expected


def test_a_null_is_a_missing_optional(home):
    engine = api.Engine()
    started = call(engine, "run.start", {"deck": "capitals", "count": None})
    assert started["result"]["count"] == 10


def test_a_notification_gets_no_response(home):
    line = json.dumps({"jsonrpc": "2.0", "method": "engine.info"})
    assert rpc.handle(api.Engine(), line) is None


def test_a_crash_in_one_call_is_an_error_not_the_end(home, monkeypatch):
    def broken(self, params: api.NoParams) -> api.Info:
        raise RuntimeError("boom")

    monkeypatch.setitem(api.METHODS, "engine.info", broken)
    stdout = io.StringIO()
    lines = request("engine.info") + "\n" + request("decks.list", request_id=2) + "\n"
    rpc.serve(stdin=io.StringIO(lines), stdout=stdout)
    first, second = (json.loads(line) for line in stdout.getvalue().splitlines())
    assert (
        error_code(first) == rpc.INTERNAL_ERROR and "boom" in first["error"]["message"]
    )
    assert second["result"]["decks"][0]["id"] == "capitals"


def test_a_stray_print_cannot_corrupt_the_stream(home, monkeypatch):
    def chatty(self, params: api.NoParams) -> api.Info:
        print("debugging output")
        return api.Info(protocol="p", version="v")

    monkeypatch.setitem(api.METHODS, "engine.info", chatty)
    stdout = io.StringIO()
    rpc.serve(stdin=io.StringIO(request("engine.info") + "\n"), stdout=stdout)
    assert json.loads(stdout.getvalue())["result"] == {"protocol": "p", "version": "v"}


def test_the_schema_describes_every_method():
    described = rpc.schema()
    assert described["protocol"] == api.PROTOCOL
    assert set(described["methods"]) == set(api.METHODS)
    start = described["methods"]["run.start"]["params"]
    assert start["required"] == ["deck"]
    assert start["properties"]["count"] == {
        "anyOf": [{"type": "integer"}, {"type": "null"}]
    }
    card = described["methods"]["run.start"]["result"]["properties"]["card"]
    assert card["properties"]["answer"] == {
        "type": "array",
        "items": {"type": "string"},
    }


def test_the_served_engine_runs_as_a_process(home):
    env = {**os.environ, "PYTHONPATH": str(Path(__file__).resolve().parents[1])}
    lines = "\n".join([request("engine.info"), request("decks.list", request_id=2)])
    result = subprocess.run(
        [sys.executable, "-m", "lernt.deck", "--serve"],
        input=lines + "\n",
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    responses = [json.loads(line) for line in result.stdout.splitlines()]
    assert [r["id"] for r in responses] == [1, 2]
    assert responses[1]["result"]["decks"][0]["name"] == "Capitals"
    assert responses[1]["result"]["decks"][0]["description"] == "Capital cities."
