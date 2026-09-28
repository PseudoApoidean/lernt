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
	_, err = tea.NewProgram(newModel(client), tea.WithAltScreen()).Run()
	client.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lernt:", err)
		os.Exit(1)
	}
}

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
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "lernt", path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDone{err} })
}
