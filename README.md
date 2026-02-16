# px

A terminal picker for paths, URLs, and other patterns in piped text.

## Why

I use [fzf](https://github.com/junegunn/fzf) daily and love it, but I wanted a picker that preserves the original input.

[PathPicker](https://github.com/facebook/PathPicker) does this well, but it executes commands rather than outputting to stdout. I just wanted something that dumps to stdout.

> [!WARNING]
> Just me learning TUIs with Claude, nothing to see here. Don't use this, it's probably full of bugs.

## Usage

```
<command> | px [-e <matcher>]...
```

Pipe any command output to px. It displays the original input with matches highlighted, preserving terminal colors and formatting. Navigate to select matches, then confirm to print your selection to stdout.

```bash
# Find paths in build output
make 2>&1 | px

# Find URLs in logs
curl -v https://example.com 2>&1 | px -e urls

# Find multiple patterns
git log --oneline | px -e sha -e paths

# Open selected files in editor
make 2>&1 | px | xargs $EDITOR

# Copy selection to clipboard
git log --oneline | px -e sha | pbcopy

# Pick from current tmux pane
tmux capture-pane -p | px

# Stage selected files
git status | px | xargs git add
```

## Built-in Matchers

- `paths` - file paths, default
- `urls` - URLs
- `ips` - IPv4 and IPv6 addresses
- `sha` - git SHAs, 7+ hex chars
- `email` - email addresses
- `regex <pattern>` - custom regex

## Keys

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up |
| `Ctrl+D` / `Ctrl+U` | Half page down / up |
| `Ctrl+F` / `Ctrl+B` | Full page down / up |
| `gg` / `G` | First / last match |
| `Tab` | Toggle selection |
| `f` | Hint mode, type label to select |
| `/` | Search |
| `n` / `N` | Next / previous search hit |
| `Enter` | Confirm |
| `Esc` / `q` | Cancel |

## Extensions

External matchers can be added by placing a `px-<name>` executable in PATH. Use with `-e name`.

The extension receives input lines on stdin and outputs matched spans on stdout, one per line:

```
LINE:START:END
```

Where LINE is 1-indexed line number, START and END are byte offsets within that line.

Example extension that matches numbers:

```bash
#!/bin/bash
# As px-numbers in PATH, chmod +x, etc. etc...
line=0
while IFS= read -r text; do
  ((line++))
  echo "$text" | grep -bo '[0-9]\+' | while IFS=: read -r start match; do
    echo "$line:$start:$((start + ${#match}))"
  done
done
```

```bash
echo "port 8080 and 443" | px -e numbers
```

## Install

```
go install code.selman.me/px@latest
```

