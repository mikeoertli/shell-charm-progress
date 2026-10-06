<p align="center">
  <img src="assets/shell-charm-progress-icon.png" alt="shell-charm-progress icon: a purple and pink terminal prompt above a progress bar" width="160" height="160">
</p>

# shell-charm-progress

**A standalone, Charm-styled progress bar you control from inside your shell script.**

Keep your commands and their output. Start a bar with the number of items, then
call `progress_tick` after each item finishes. Normal stdout and stderr scroll
above the animated bar with a blank line separating them. The bar defaults to
60% of the terminal width and adjusts on resize, with scrollback intact. Status
changes do not change the bar length.

```text
Updating gum…
Already up to date.
Updating kube-resource-monitor…
Fast-forward

██████████████████░░░░░░░░░░░░  18/30 · Updating github-pr-monitor
```

The real bar has a **purple → pink gradient**, a pink count, and a muted gray
track. It uses Bubble Tea, Bubbles, and Lip Gloss, with Gum's pink `212` accent.
You do not need Gum installed.

## Install once

Requires **Go 1.26+ to build**, on macOS or Linux. From this repository:

```sh
go install .
```

Go installs `shell-charm-progress` in `GOBIN`, or `$(go env GOPATH)/bin` if `GOBIN`
is unset. Ensure that directory is on your `PATH`. With Go's default location:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
shell-charm-progress demo
```

Alternatively, build a local executable and place it wherever you keep tools:

```sh
make build
mkdir -p "$HOME/.local/bin"
install -m 755 bin/shell-charm-progress "$HOME/.local/bin/shell-charm-progress"
# Ensure ~/.local/bin is on PATH.
```

**Only the executable is needed at runtime.** It embeds its shell functions and
demo; there is no helper file to copy, dotfiles dependency, background service,
Go runtime, or project-specific setup.

### Homebrew

```sh
brew install mikeoertli/tap/shell-charm-progress
brew update
brew upgrade mikeoertli/tap/shell-charm-progress
```

Homebrew installs the dependencies declared by the tap. A compatible prebuilt
bottle avoids local compilation; otherwise Homebrew builds from source.

## Copy this into a script

Works in Bash 3.2+, Zsh, and sh/dash. This complete example simulates three jobs:

```sh
#!/bin/sh
set -eu

eval "$(shell-charm-progress init)"
trap 'progress_stop' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

progress_start 3 "Running jobs"

for job in one two three; do
  progress_label "Working on $job"
  echo "Starting $job…"
  sleep 1                         # Your command goes here.
  echo "Finished $job"
  progress_tick                  # One item finished.
done

progress_stop                    # Optional here: the EXIT trap also stops it.
```

The setup line loads functions **from the installed executable into the current
shell**. No wrapper launches your script: your script still runs all the work.
`progress_start` captures subsequent stdout/stderr so commands need no special
redirections. Call `progress_stop` before interactive prompts or a new phase that
should have ordinary terminal output.

If your script already has an EXIT trap, add `progress_stop` to its cleanup
instead of replacing it; see [cleanup](#cleanup-and-exit-status).

## Git repository updater example

For an existing Bash updater, add initialization and cleanup once, then start
the bar after your existing discovery code has populated `repos`:

```bash
eval "$(shell-charm-progress init)"
trap 'progress_stop' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Your existing discovery logic sets the repos array.
progress_start "${#repos[@]}" "Updating repositories"
failed=0

for repo in "${repos[@]}"; do
  progress_label "Updating ${repo##*/}"
  echo "Updating $repo…"

  if ! git -C "$repo" pull --ff-only; then
    echo "Failed to update $repo" >&2
    failed=$((failed + 1))
  fi

  progress_tick
done

echo "Processed ${#repos[@]} repositories; $failed failures."
progress_label "Finished · $failed failures"
[ "$failed" -eq 0 ]
```

Use your own Git update policy in place of `pull --ff-only`. Count **processed**
items, including failures or skips, so the bar represents work completed. Your
script owns error reporting and its exit status. An early exit leaves the actual
partial count; the utility never invents 100% completion.

## Shell API

| Function | Meaning |
| --- | --- |
| `progress_start TOTAL [TITLE] [OPTIONS]` | Start a session; title defaults to `Working`. |
| `progress_start --total N --title TEXT [OPTIONS]` | Equivalent named-argument form. |
| `progress_label "Updating repo"` | Change the label without advancing. |
| `progress_tick [LABEL]` | Advance by one, optionally changing the label. |
| `progress_update N [LABEL]` | Set the absolute completed count, optionally changing the label. |
| `progress_stop` | Flush logs, leave the final bar, restore streams, clean up. Safe to repeat. |

Counts are decimal integers from zero to the total, at most `999999999`, with no
leading zeros. Invalid counts return status `2`; backwards updates are allowed.
A zero total displays nothing. Calls before starting or after stopping are
harmless. Starting another session before stopping the first returns `2`.

Use these functions in the owning shell, not pipeline subshells, command
substitutions, or parallel workers. With parallel jobs, report completion from
the parent shell. Wait for workers whose output you want captured before stopping.

## Styling

The default gradient uses ANSI palette **99 → 212**, with **240** for the empty
track. These colors coordinate with Gum. The gradient spans the filled portion,
so pink is visible even before completion. Colors adapt to terminal capabilities.

```sh
progress_start 20 "Updating repositories"                 # Default: 60% width
progress_start 20 "Updating repositories" --width '75%'   # 75% of terminal
progress_start 20 "Updating repositories" --width 40      # 40-column bar
progress_start 20 "Updating repositories" --color 212     # Solid Gum pink
progress_start 20 "Updating repositories" \
  --color '#7D56F4' --color-end '#FF87D7' --width 40         # Custom gradient
progress_start 20 "Updating repositories" --no-color      # Monochrome
```

These are alternative starts; stop a session before starting the next.

`--width '60%'` is the default. Whole percentages from **1% to 100%** use that
fraction of the terminal's full column count, rounded down, with a minimum bar
width of three cells. The bar updates on resize even while the script is idle.
`--width 40` requests a **40-column bar**; integer widths of three or more
(up to 999999999) are accepted. Fixed widths stay fixed when the terminal grows.

Both forms shrink only when necessary to fit the terminal and item count.
The count has reserved space, and long status text is truncated to what remains;
changing status text or count digits does not change the bar length. At very
narrow widths, the count takes priority over the bar and label. `--width auto`
remains an alias for the default `60%`.

A blank separator stays above the bar while output scrolls and is retained with
the final progress line.

`--color` accepts `#RRGGBB` or an ANSI palette number 0–255.
Set `--color-end` to make a gradient. A nonempty
`NO_COLOR` environment variable also disables color.

The initial title is the first label; `progress_label` replaces it. Long labels
are truncated and whitespace is flattened to keep the live display on one line.

## Cleanup and exit status

The helper installs **no traps** and changes **no shell options**. Use the traps
in the quick start for a new script. For existing cleanup:

```sh
cleanup() {
  cleanup_status=$?
  progress_stop || true
  # Your other cleanup operations go here.
  return "$cleanup_status"
}
trap cleanup EXIT
```

Keep your existing signal policy, or use `trap 'exit 130' INT` and
`trap 'exit 143' TERM` so interruptions trigger EXIT cleanup. `progress_stop`
returns the status that preceded it. Use it early in cleanup to restore normal
output before printing more messages. After an explicit stop, subsequent `echo`
and other commands behave normally.

## Output and reliability

- **Redirected output stays untouched.** If either stdout or stderr is not a
  terminal, or `TERM` is unset/`dumb`, no capture occurs. Stdout and stderr keep
  their original bytes and separation. No bar or escape sequences enter logs.
- **Interactive display combines both streams.** They share an append-only log,
  and the renderer prints lines above the bar. Original ANSI styling and cursor
  controls in command output are stripped; tabs/carriage returns become spaces.
  Line-oriented output is supported, including a final line without a newline.
- **This is not a PTY for your commands.** Programs may buffer output or disable
  their own colors when captured. Stop progress before password prompts,
  fullscreen programs, or commands that need to control the terminal. Stdin is
  left available, but interactive echo can conflict with the display.
- **The bar is the last output line**, with a blank separator above it, not a fullscreen footer. No alternate
  screen is used. Narrow terminals retain the count when there is no room for a bar.
- **Renderer failures don't block producers.** Private temporary journals avoid
  pipe backpressure. At the next update or stop, a dead renderer triggers output
  restoration and journal replay; some lines can repeat, rather than be lost.
- Startup failure falls back to ordinary output with a diagnostic. A stalled
  renderer has a bounded shutdown wait (about five seconds plus process overhead)
  before journal replay. Ordinary output draining is performed before exit.
- Descriptors **8 and 9** are reserved while active. If already open, the helper
  falls back to ordinary output without overwriting them.
- Journals live in a private directory under `TMPDIR` (or `/tmp`), grow with
  captured output, and are removed on stop. Output from unrelated background
  writers after the stop snapshot is not included. Machine crashes or killing
  both shell and renderer uncatchably can leave temporary files.

`SHELL_CHARM_PROGRESS_BIN` can select another renderer by absolute path for
testing. Normally `init` embeds the absolute path of its own executable, so
changing directory or `PATH` afterward does not break a running script.

## Versioning and changelog

`VERSION` is the sole source of the application version. It is embedded at
compile time for Make builds and direct `go build`, `go install`, and `go run`
commands. Edit `VERSION` and rebuild to change the reported version; no Make
variable or linker override is needed.

```sh
shell-charm-progress --version
# shell-charm-progress 1.1.0
```

Record changes under the matching `in progress` heading in [CHANGELOG.md](CHANGELOG.md).
When bumping the version, date the previous heading (`YYYY-MM-DD`), update
`VERSION`, and add a new `## <version> — in progress` section. Use semantic
versioning; development versions and changelog entries do not require a Git tag
or a published release.

### Homebrew releases

[`.github/workflows/homebrew.yml`](.github/workflows/homebrew.yml) notifies
[`mikeoertli/homebrew-tap`](https://github.com/mikeoertli/homebrew-tap) when a
stable `vMAJOR.MINOR.PATCH` tag is pushed. Keep the tag aligned with `VERSION`
and include the workflow in the tagged commit. For the current version, the
release tag is `v1.1.0`.

The workflow uses the **TAP_DISPATCH_TOKEN** repository secret to trigger the
tap's release updater for `shell-charm-progress`. The token needs **Actions: Read and write**
access to the tap repository. See the [tap's setup instructions](https://github.com/mikeoertli/homebrew-tap#notify-the-tap-when-a-project-is-tagged)
for token configuration and the publishing process.

The tap opens an update PR, builds and tests its packages, and provides a
separate bottle-publishing step. The notification does not publish the package
on its own. Prerelease tags are skipped, and existing tags are not retriggered.
After the tap update is published, use `brew update` and `brew upgrade` on each
computer to install it. Ordinary branch pushes do not change the packaged version.

## Demo and development

```sh
shell-charm-progress demo          # Harmless simulated repository work
shell-charm-progress demo --fail   # Demonstrates stderr and exit status 1
shell-charm-progress --help

make test                         # Build, Go race tests, real-terminal tests
make check                        # go vet + ShellCheck (install ShellCheck first)
TEST_SHELL=/bin/zsh python3 tests/test_integration.py
TEST_SHELL=/bin/dash python3 tests/test_integration.py
```

The Python integration tests require Python 3 and use real pseudo-terminals.
They exercise cleanup, failures, signals, resizing, shell portability, styling,
relocation of the executable, and redirected output. They do not update repos
or access the network.

The internal `render` command and journal format are implementation details;
use `init` and the shell functions as the public interface.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles), and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).
Inspired by [Gum](https://github.com/charmbracelet/gum).
