// Package engine is the TUI's only door into lernt: a client for the API that
// `lernt-deck --serve` speaks as JSON-RPC over its stdin and stdout. Nothing
// else in the TUI knows the engine is a separate program, and nothing here
// knows about the screen, so either side can be replaced without the other.
package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

// Error is an error the engine returned: a code from the constants in api.go
// or JSON-RPC's own, and a message phrased for the user.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// Code is err's engine error code, or 0 when err is not one.
func Code(err error) int {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return 0
}

// Client holds one engine process and the run inside it. Calls are serialised.
type Client struct {
	in     io.WriteCloser
	out    *bufio.Reader
	cmd    *exec.Cmd
	stderr *bytes.Buffer
	mu     sync.Mutex
	nextID int
}

// command is how to run the engine. lernt sets LERNT_ENGINE, a JSON array, when
// it starts the TUI; run by hand during development, lernt-deck on PATH.
func command() ([]string, error) {
	raw := os.Getenv("LERNT_ENGINE")
	if raw == "" {
		return []string{"lernt-deck"}, nil
	}
	var argv []string
	if err := json.Unmarshal([]byte(raw), &argv); err != nil || len(argv) == 0 {
		return nil, fmt.Errorf("LERNT_ENGINE is not a JSON array of strings: %q", raw)
	}
	return argv, nil
}

// Start runs the engine and checks it speaks this client's protocol.
func Start() (*Client, error) {
	argv, err := command()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(argv[0], append(argv[1:], "--serve")...)
	// Its own process group: a Ctrl-C typed into an editor the TUI has handed
	// the terminal to must not reach the engine and end the run.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting the engine %s: %w", strings.Join(argv, " "), err)
	}
	c := newClient(in, out)
	c.cmd, c.stderr = cmd, stderr
	info, err := c.Info()
	if err != nil {
		c.Close()
		return nil, err
	}
	if info.Protocol != Protocol {
		c.Close()
		return nil, fmt.Errorf("the engine speaks %s, this TUI %s: rebuild it with `make tui`",
			info.Protocol, Protocol)
	}
	return c, nil
}

func newClient(in io.WriteCloser, out io.Reader) *Client {
	return &Client{in: in, out: bufio.NewReader(out)}
}

// Close ends the engine: it serves until its stdin closes.
func (c *Client) Close() error {
	c.in.Close()
	if c.cmd == nil {
		return nil
	}
	return c.cmd.Wait()
}

func (c *Client) call(method string, params, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": c.nextID, "method": method, "params": params,
	})
	if err != nil {
		return err
	}
	if _, err := c.in.Write(append(request, '\n')); err != nil {
		return c.gone(err)
	}
	line, err := c.out.ReadBytes('\n')
	if err != nil {
		return c.gone(err)
	}
	var response struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return fmt.Errorf("%s: the engine's response is not JSON-RPC: %w", method, err)
	}
	if response.Error != nil {
		return response.Error
	}
	if response.ID != c.nextID {
		return fmt.Errorf("%s: response %d to request %d", method, response.ID, c.nextID)
	}
	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf("%s: reading the result: %w", method, err)
	}
	return nil
}

// gone explains a dead engine with whatever it said on the way out.
func (c *Client) gone(err error) error {
	if c.stderr != nil {
		if said := strings.TrimSpace(c.stderr.String()); said != "" {
			return fmt.Errorf("the engine stopped: %s", said)
		}
	}
	return fmt.Errorf("the engine stopped: %w", err)
}

func (c *Client) Info() (*Info, error) {
	var r Info
	return &r, c.call("engine.info", NoParams{}, &r)
}

func (c *Client) Decks() (*DeckList, error) {
	var r DeckList
	return &r, c.call("decks.list", NoParams{}, &r)
}

func (c *Client) Start(p StartParams) (*Started, error) {
	var r Started
	return &r, c.call("run.start", p, &r)
}

func (c *Client) Score(p ScoreParams) (*Scored, error) {
	var r Scored
	return &r, c.call("run.score", p, &r)
}

func (c *Client) Menu() (*Menu, error) {
	var r Menu
	return &r, c.call("run.menu", NoParams{}, &r)
}

func (c *Client) Next(p NextParams) (*Question, error) {
	var r Question
	return &r, c.call("run.next", p, &r)
}

func (c *Client) End() (*Summary, error) {
	var r Summary
	return &r, c.call("run.end", NoParams{}, &r)
}

func (c *Client) EditCard() (*CardFile, error) {
	var r CardFile
	return &r, c.call("card.edit", NoParams{}, &r)
}

func (c *Client) Reload() (*Reloaded, error) {
	var r Reloaded
	return &r, c.call("card.reload", NoParams{}, &r)
}

func (c *Client) Note(p NoteParams) (*Standing, error) {
	var r Standing
	return &r, c.call("card.note", p, &r)
}

func (c *Client) Flag(p FlagParams) (*Standing, error) {
	var r Standing
	return &r, c.call("card.flag", p, &r)
}

func (c *Client) Done() (*Standing, error) {
	var r Standing
	return &r, c.call("card.done", NoParams{}, &r)
}

func (c *Client) Misses(p MissParams) (*MissReport, error) {
	var r MissReport
	return &r, c.call("report.misses", p, &r)
}

func (c *Client) Boxes(p DeckParams) (*BoxReport, error) {
	var r BoxReport
	return &r, c.call("report.boxes", p, &r)
}

func (c *Client) Edits(p DeckParams) (*EditReport, error) {
	var r EditReport
	return &r, c.call("report.edits", p, &r)
}

func (c *Client) LoadOptions(p DeckParams) (*Options, error) {
	var r Options
	return &r, c.call("options.load", p, &r)
}

func (c *Client) SaveOptions(p SaveOptionsParams) (*Options, error) {
	var r Options
	return &r, c.call("options.save", p, &r)
}

func (c *Client) LoadSettings() (*Settings, error) {
	var r Settings
	return &r, c.call("settings.load", NoParams{}, &r)
}

func (c *Client) SaveSettings(p SaveSettingsParams) (*Settings, error) {
	var r Settings
	return &r, c.call("settings.save", p, &r)
}

func (c *Client) CreateDeck(p NewDeckParams) (*NewDeck, error) {
	var r NewDeck
	return &r, c.call("deck.create", p, &r)
}

func (c *Client) CreateCard(p NewCardParams) (*NewCard, error) {
	var r NewCard
	return &r, c.call("card.create", p, &r)
}

func (c *Client) ListCards(p DeckParams) (*CardList, error) {
	var r CardList
	return &r, c.call("card.list", p, &r)
}

func (c *Client) CheckDeck(p DeckParams) (*Problems, error) {
	var r Problems
	return &r, c.call("deck.check", p, &r)
}
