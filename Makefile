# The Python engine and the Go TUI build and install together. The TUI is
# compiled into the package, at the one path `lernt` runs it from.

TUI := lernt/_bin/lernt-tui
GO_SOURCES := $(shell find tui -name '*.go') tui/go.mod

# Where the man page and zsh completion go, following the same XDG rule as the
# engine's deck directory.
DATA  := $(or $(XDG_DATA_HOME),$(HOME)/.local/share)
STATE := $(or $(XDG_STATE_HOME),$(HOME)/.local/state)
MAN   := $(DATA)/man/man1/lernt.1
COMP  := $(DATA)/zsh/site-functions/_lernt

.PHONY: all tui install uninstall test clean

all: tui

tui: $(TUI)

$(TUI): $(GO_SOURCES)
	cd tui && go mod tidy && go build -o ../$(TUI) .

install: tui
	uv tool install --editable . --reinstall
	install -Dm644 lernt.1 $(MAN)
	install -Dm644 completions/_lernt $(COMP)

# Removes what install put down, and nothing of the user's: decks may be
# symlinks into a notes vault, and the logs are the history.
uninstall:
	-uv tool uninstall lernt
	rm -f $(MAN) $(COMP)
	@echo "Left in place: your decks ($(or $(LERNT_DECKS),$(DATA)/lernt/decks)) and"
	@echo "your history ($(STATE)/lernt). To delete the history: rm -r $(STATE)/lernt"

test:
	uv run --with pytest pytest -q
	cd tui && go test -count=1 ./...  # uncached: the engine it runs is not a Go input

clean:
	rm -f $(TUI)
