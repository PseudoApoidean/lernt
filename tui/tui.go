// The lernt TUI: the decks, a timed walk through one, and its reports. It is a
// view - every rule of a run lives in the engine, reached through tui/engine.
package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"lernt/tui/engine"
)

func main() {
	client, err := engine.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lernt:", err)
		os.Exit(1)
	}
	// A line wider than the terminal draws it, because a character took more cells
	// than we counted, would wrap and push the whole page down. With wrapping off it
	// is cut at the edge instead, and only that line is out.
	fmt.Fprint(os.Stdout, wrapOff)
	_, err = tea.NewProgram(newModel(client), tea.WithAltScreen()).Run()
	fmt.Fprint(os.Stdout, wrapOn)
	client.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lernt:", err)
		os.Exit(1)
	}
}

// The terminal's automatic wrapping, off while the TUI draws and on for anything
// else that has the terminal: an editor expects it, and so does the shell after.
const (
	wrapOff = "\x1b[?7l"
	wrapOn  = "\x1b[?7h"
)

// runEditor hands the terminal to $VISUAL or $EDITOR on path, through the shell
// so an editor with arguments ("code -w") works as it would typed.
func runEditor(path string) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// Wrapping is on for the editor and off again when it returns.
	script := `printf '\033[?7h'; ` + editor + ` "$1"; status=$?; printf '\033[?7l'; exit $status`
	cmd := exec.Command("sh", "-c", script, "lernt", path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDone{err} })
}
