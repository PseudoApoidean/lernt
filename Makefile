# The Python engine and the Go TUI build and install together. The TUI is
# compiled into the package, at the one path `lernt` runs it from.

TUI := lernt/_bin/lernt-tui
GO_SOURCES := $(shell find tui -name '*.go') tui/go.mod

.PHONY: all tui install test clean

all: tui

tui: $(TUI)

$(TUI): $(GO_SOURCES)
	cd tui && go mod tidy && go build -o ../$(TUI) .

install: tui
	uv tool install --editable . --reinstall

test:
	uv run --with pytest pytest -q
	cd tui && go test -count=1 ./...  # uncached: the engine it runs is not a Go input

clean:
	rm -f $(TUI)
