"""The API served over stdio: JSON-RPC 2.0, one message per line.

    lernt-deck --serve      requests on stdin, responses on stdout
    lernt-deck --schema     every method's params and result, as JSON Schema

Nothing here decides anything. A method name picks an `api.Engine` method, its
params are decoded into that method's params dataclass - checked against the
annotations, so a wrong type is refused here rather than half-used there - and
its result dataclass is encoded back. The schema is derived from the same
annotations, which is what lets another language's client be tested against
the API rather than against someone's memory of it.
"""

import dataclasses
import json
import sys
import types
import typing

from . import api

# JSON-RPC's own codes.
PARSE_ERROR = -32700
INVALID_REQUEST = -32600
METHOD_NOT_FOUND = -32601
INVALID_PARAMS = -32602
INTERNAL_ERROR = -32603


class ParamsError(Exception):
    pass


def signature(fn):
    hints = typing.get_type_hints(fn)
    return hints["params"], hints["return"]


# -- decoding and encoding -------------------------------------------------------


def decode(cls, data, where="params"):
    if not isinstance(data, dict):
        raise ParamsError(f"{where} must be an object")
    fields = {f.name: f for f in dataclasses.fields(cls)}
    unknown = sorted(set(data) - set(fields))
    if unknown:
        raise ParamsError(f"{where}: unknown field(s) {', '.join(unknown)}")
    hints = typing.get_type_hints(cls)
    values = {}
    for name, f in fields.items():
        if name in data:
            values[name] = _decode_value(hints[name], data[name], f"{where}.{name}")
        elif (
            f.default is dataclasses.MISSING
            and f.default_factory is dataclasses.MISSING
        ):
            raise ParamsError(f"{where}.{name} is required")
    return cls(**values)


def _decode_value(tp, value, where):
    inner = _optional(tp)
    if inner is not None:
        return None if value is None else _decode_value(inner, value, where)
    if typing.get_origin(tp) is list:
        if not isinstance(value, list):
            raise ParamsError(f"{where} must be an array")
        (item,) = typing.get_args(tp)
        return [_decode_value(item, v, f"{where}[{i}]") for i, v in enumerate(value)]
    if dataclasses.is_dataclass(tp):
        return decode(tp, value, where)
    # bool is an int in Python and must not pass for one.
    ok = isinstance(value, tp) and not (tp is int and isinstance(value, bool))
    if not ok:
        raise ParamsError(f"{where} must be {_JSON_TYPES[tp]}")
    return value


def encode(value):
    return dataclasses.asdict(value)


def _optional(tp):
    """X for `X | None`, else None."""
    if typing.get_origin(tp) in (types.UnionType, typing.Union):
        args = [a for a in typing.get_args(tp) if a is not type(None)]
        if len(args) == 1 and len(typing.get_args(tp)) == 2:
            return args[0]
    return None


# -- the schema ----------------------------------------------------------------

_JSON_TYPES = {str: "string", int: "integer", bool: "boolean", float: "number"}


def schema():
    return {
        "protocol": api.PROTOCOL,
        "methods": {
            name: {
                "params": _type_schema(params),
                "result": _type_schema(result),
            }
            for name, (params, result) in (
                (name, signature(fn)) for name, fn in sorted(api.METHODS.items())
            )
        },
    }


def _type_schema(tp):
    inner = _optional(tp)
    if inner is not None:
        return {"anyOf": [_type_schema(inner), {"type": "null"}]}
    if typing.get_origin(tp) is list:
        (item,) = typing.get_args(tp)
        return {"type": "array", "items": _type_schema(item)}
    if dataclasses.is_dataclass(tp):
        hints = typing.get_type_hints(tp)
        fields = dataclasses.fields(tp)
        return {
            "title": tp.__name__,
            "type": "object",
            "properties": {f.name: _type_schema(hints[f.name]) for f in fields},
            "required": [
                f.name
                for f in fields
                if f.default is dataclasses.MISSING
                and f.default_factory is dataclasses.MISSING
            ],
            "additionalProperties": False,
        }
    return {"type": _JSON_TYPES[tp]}


# -- serving -------------------------------------------------------------------


def handle(engine, line):
    """One request line to one response dict - or None, for a notification."""
    try:
        message = json.loads(line)
    except ValueError as exc:
        return _error(None, PARSE_ERROR, f"Not JSON: {exc}")
    if (
        not isinstance(message, dict)
        or message.get("jsonrpc") != "2.0"
        or not isinstance(message.get("method"), str)
    ):
        return _error(
            message.get("id") if isinstance(message, dict) else None,
            INVALID_REQUEST,
            "Not a JSON-RPC 2.0 request",
        )
    request_id = message.get("id")
    notification = "id" not in message

    fn = api.METHODS.get(message["method"])
    if fn is None:
        response = _error(
            request_id, METHOD_NOT_FOUND, f"No method {message['method']!r}"
        )
    else:
        response = _call(engine, fn, message.get("params", {}), request_id)
    return None if notification else response


def _call(engine, fn, raw, request_id):
    params_type, _ = signature(fn)
    try:
        params = decode(params_type, raw)
    except ParamsError as exc:
        return _error(request_id, INVALID_PARAMS, str(exc))
    try:
        result = fn(engine, params)
    except api.ApiError as exc:
        return _error(request_id, exc.code, str(exc))
    except Exception as exc:  # noqa: BLE001 - one bad call must not end the session
        return _error(request_id, INTERNAL_ERROR, f"{type(exc).__name__}: {exc}")
    return {"jsonrpc": "2.0", "id": request_id, "result": encode(result)}


def _error(request_id, code, message):
    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "error": {"code": code, "message": message},
    }


def serve(stdin=None, stdout=None, engine=None):
    """Until stdin closes. Anything else that writes to stdout would corrupt the
    stream, so for the duration stdout *is* stderr, and responses go to the
    stream captured before that."""
    stdin = stdin or sys.stdin
    out = stdout or sys.stdout
    engine = engine or api.Engine()
    saved, sys.stdout = sys.stdout, sys.stderr
    try:
        for line in stdin:
            if not line.strip():
                continue
            response = handle(engine, line)
            if response is not None:
                out.write(json.dumps(response, ensure_ascii=False) + "\n")
                out.flush()
    finally:
        sys.stdout = saved
    return 0
