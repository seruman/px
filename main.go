package main

import (
	"cmp"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/term"
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

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
	var extensions stringSlice
	widthFlag := flag.String("width", "", "terminal width for soft-wrap rejoining (\"auto\" or integer)")
	flag.Var(&extensions, "e", "extension to run (e.g. -e url or -e 'ip --v6'); repeatable")
	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "px: unexpected argument %q (use -e to specify extensions)\n", flag.Args()[0])
		os.Exit(1)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Usage: <command> | px [-e <ext>]...\n")
		fmt.Fprintf(os.Stderr, "Reads piped text, finds matches, and presents an interactive picker.\n")
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

	lineSeq := unwrap(scanLines(os.Stdin), width)

	var lines []string
	var spans []Span
	if len(extensions) > 0 {
		var defs []extDef
		for _, ext := range extensions {
			words, err := shellSplit(ext)
			if err != nil {
				fmt.Fprintf(os.Stderr, "px: invalid -e value %q: %v\n", ext, err)
				os.Exit(1)
			}
			if len(words) == 0 {
				fmt.Fprintf(os.Stderr, "px: empty -e value\n")
				os.Exit(1)
			}
			bin, err := exec.LookPath("px-" + words[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "px: %v\n", err)
				os.Exit(1)
			}
			defs = append(defs, extDef{bin, words[1:]})
		}

		var err error
		lines, spans, err = runExtensions(lineSeq, defs, width)
		if err != nil {
			fmt.Fprintf(os.Stderr, "px: %v\n", err)
			os.Exit(1)
		}
		slices.SortFunc(spans, func(a, b Span) int {
			if c := cmp.Compare(a.Line, b.Line); c != 0 {
				return c
			}
			return cmp.Compare(a.Start, b.Start)
		})
	} else {
		lines = slices.Collect(lineSeq)
		spans = findPaths(lines)
	}

	if len(spans) == 0 {
		fmt.Fprintf(os.Stderr, "px: no matches found in input\n")
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

	for _, s := range selected {
		fmt.Println(s)
	}
}
