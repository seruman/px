package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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

	if len(extensions) == 0 {
		extensions = []string{"paths"}
	}

	var matchers []resolvedMatcher
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
		name := words[0]
		if name == "regex" {
			if len(words) != 2 {
				fmt.Fprintf(os.Stderr, "px: regex matcher requires exactly one pattern argument\n")
				os.Exit(1)
			}
			re, err := regexp.Compile(words[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "px: invalid regex %q: %v\n", words[1], err)
				os.Exit(1)
			}
			matchers = append(matchers, resolvedMatcher{
				name:    name,
				builtin: matchRegex(re),
			})
		} else if bin, lookErr := exec.LookPath("px-" + name); lookErr == nil {
			matchers = append(matchers, resolvedMatcher{
				name: name,
				ext:  &extDef{bin, words[1:]},
			})
		} else if fn, ok := builtinMatchers[name]; ok {
			matchers = append(matchers, resolvedMatcher{
				name:    name,
				builtin: fn,
			})
		} else {
			fmt.Fprintf(os.Stderr, "px: unknown matcher %q (no px-%s in PATH and no built-in)\n", name, name)
			os.Exit(1)
		}
	}

	p := newPicker()
	var cancel func()
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()
	selected, err := p.run(func(postEvent func(any)) {
		cancel = startMatchers(lineSeq, matchers, width, postEvent)
	})
	if err != nil {
		if errors.Is(err, errNoMatches) {
			fmt.Fprintf(os.Stderr, "px: %v\n", err)
			os.Exit(0)
		}
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
