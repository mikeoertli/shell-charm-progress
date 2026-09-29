package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

//go:embed shell/progress.sh
var shellHelper string

//go:embed examples/demo.sh
var demoScript string

const helpText = `shell-charm-progress — Charm-style progress inside your shell scripts

Usage:
  shell-charm-progress init [bash|zsh|sh]   Print shell functions for eval
  shell-charm-progress demo [--fail]       Run a harmless demonstration
  shell-charm-progress --version

In your script:
  eval "$(shell-charm-progress init)"
  trap 'progress_stop' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  progress_start 3 "Updating repositories"
  # Run commands normally; stdout/stderr scroll above the bar.
  progress_tick "Finished first repository"
  progress_update 3 "Finished"
  progress_stop

Functions:
  progress_start TOTAL [TITLE] [OPTIONS]
  progress_start --total N [--title TEXT] [OPTIONS]
  progress_label TEXT            Change the label without advancing
  progress_tick [TEXT]           Advance by one; optionally change label
  progress_update N [TEXT]       Set an absolute completed count
  progress_stop                 Flush, restore output; preserve exit status

Style options for progress_start:
  --width N|N%                  Bar columns or terminal percentage (default 60%)
  --color COLOR                 Solid fill, or start of a custom gradient
  --color-end COLOR             End of a custom gradient
  --no-color                    Disable color (also honors NO_COLOR)
Colors: #RRGGBB or ANSI palette number 0–255. Default: purple 99 → pink 212.

Bash 3.2+, Zsh, and sh/dash on macOS/Linux. No Go runtime or Gum installation
needed after building. Redirected output is left untouched. The helper changes
no traps: add progress_stop to your existing cleanup if you already have one.
`

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func initialization(executable string) string {
	return "_SCP_BIN=" + shellQuote(executable) + "\n" + shellHelper
}

func cli(args []string, out io.Writer) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")) {
		_, err := io.WriteString(out, helpText)
		return err
	}
	switch args[0] {
	case "--version", "version":
		if len(args) != 1 {
			return fmt.Errorf("version takes no arguments")
		}
		_, err := fmt.Fprintln(out, "shell-charm-progress", Version())
		return err
	case "init":
		if len(args) > 2 || (len(args) == 2 && args[1] != "bash" && args[1] != "zsh" && args[1] != "sh") {
			return fmt.Errorf("usage: shell-charm-progress init [bash|zsh|sh]")
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, initialization(executable))
		return err
	case "demo":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--fail") {
			return fmt.Errorf("usage: shell-charm-progress demo [--fail]")
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		// Replace this process so shell traps receive signals normally.
		script := "SHELL_CHARM_PROGRESS_BIN=" + shellQuote(executable) + "\n" + demoScript
		return syscall.Exec("/bin/sh", append([]string{"sh", "-c", script, "demo"}, args[1:]...), os.Environ())
	case "render":
		return runRenderer(args[1:])
	default:
		return fmt.Errorf("unknown command %q; use --help", args[0])
	}
}
