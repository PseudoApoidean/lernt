package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// serveFake answers requests in-process with `reply`, which gets each request
// and returns the JSON for its response's result or error members.
func serveFake(t *testing.T, reply func(method string, params json.RawMessage) string) *Client {
	t.Helper()
	requests, requestWriter := io.Pipe()
	responseReader, responses := io.Pipe()
	go func() {
		defer responses.Close()
		scanner := bufio.NewScanner(requests)
		for scanner.Scan() {
			var req struct {
				ID     int             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				t.Errorf("the client sent something that is not JSON: %s", scanner.Text())
				return
			}
			id, _ := json.Marshal(req.ID)
			var line bytes.Buffer
			raw := `{"jsonrpc": "2.0", "id": ` + string(id) + `, ` + reply(req.Method, req.Params) + "}"
			if err := json.Compact(&line, []byte(raw)); err != nil {
				t.Errorf("the fake's own reply is not JSON: %s", raw)
				return
			}
			line.WriteByte('\n')
			responses.Write(line.Bytes())
		}
	}()
	c := newClient(requestWriter, responseReader)
	t.Cleanup(func() { c.Close() })
	return c
}

func TestACallSendsTypedParamsAndDecodesTheResult(t *testing.T) {
	var sent StartParams
	c := serveFake(t, func(method string, params json.RawMessage) string {
		if method != "run.start" {
			t.Errorf("method %q", method)
		}
		json.Unmarshal(params, &sent)
		return `"result": {"name": "Capitals", "count": 3, "log": null,
			"card": {"id": "France", "title": "Capital of France?", "answer": ["Paris."]}}`
	})
	count := 3
	started, err := c.Start(StartParams{Deck: "capitals", Count: &count, Log: true})
	if err != nil {
		t.Fatal(err)
	}
	if sent.Deck != "capitals" || *sent.Count != 3 || !sent.Log {
		t.Fatalf("sent %+v", sent)
	}
	if started.Name != "Capitals" || started.Log != nil || started.Card.Answer[0] != "Paris." {
		t.Fatalf("decoded %+v", started)
	}
}

func TestAnEngineErrorKeepsItsCode(t *testing.T) {
	c := serveFake(t, func(string, json.RawMessage) string {
		return `"error": {"code": 3, "message": "Nothing is on the miss list."}`
	})
	_, err := c.Start(StartParams{Deck: "capitals", Weak: true})
	if Code(err) != NoMisses || err.Error() != "Nothing is on the miss list." {
		t.Fatalf("err %v, code %d", err, Code(err))
	}
}

func TestADeadEngineIsAnError(t *testing.T) {
	c := serveFake(t, func(string, json.RawMessage) string { return `"result": {}` })
	c.out = bufio.NewReader(strings.NewReader("")) // the engine has gone
	if _, err := c.Decks(); err == nil || !strings.Contains(err.Error(), "engine stopped") {
		t.Fatalf("err %v", err)
	}
}

func TestAMalformedEngineCommandIsAnError(t *testing.T) {
	t.Setenv("LERNT_ENGINE", "lernt-deck --serve")
	if _, err := Start(); err == nil {
		t.Fatal("a non-JSON LERNT_ENGINE was accepted")
	}
}
