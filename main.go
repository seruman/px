package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

func ttyWidth() (int, error) {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return 0, fmt.Errorf("open /dev/tty: %w", err)
	}
	defer f.Close()

	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, fmt.Errorf("get terminal size: %w", err)
	}
	return w, nil
}

func main() {
	widthFlag := flag.String("width", "", "terminal width for soft-wrap rejoining (\"auto\" or integer)")
	flag.Parse()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Usage: <command> | px\n")
		fmt.Fprintf(os.Stderr, "Reads piped text, finds file paths, and presents an interactive picker.\n")
		os.Exit(1)
	}

	var width int
	switch *widthFlag {
	case "":
		// no width specified
	case "auto":
		w, err := ttyWidth()
		if err != nil {
			fmt.Fprintf(os.Stderr, "px: detect terminal width: %v\n", err)
			os.Exit(1)
		}
		width = w
	default:
		w, err := strconv.Atoi(*widthFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "px: invalid --width value %q: %v\n", *widthFlag, err)
			os.Exit(1)
		}
		width = w
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "px: read stdin: %v\n", err)
		os.Exit(1)
	}

	if len(data) == 0 {
		os.Exit(0)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if width > 0 {
		lines = unwrapLines(lines, width)
	}

	spans := findPaths(lines)
	if len(spans) == 0 {
		fmt.Fprintf(os.Stderr, "px: no paths found in input\n")
		os.Exit(0)
	}

	p := newPicker(lines, spans)
	selected, err := p.run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "px: %v\n", err)
		os.Exit(1)
	}

	if selected == nil {
		os.Exit(130)
	}

	for _, path := range selected {
		fmt.Println(path)
	}
}
