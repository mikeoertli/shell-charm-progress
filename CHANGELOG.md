# Changelog

## 1.0.0 — 2026-09-29

First stable release.

- Provide a standalone executable with embedded shell helpers for Bash, Zsh,
  and sh/dash, initialized from any script with `shell-charm-progress init`.
- Add start, label, tick, absolute-count update, and stop functions while keeping
  the calling script in control of its commands and exit status.
- Display stdout/stderr above an animated progress bar with a persistent blank
  separator, Gum-inspired purple-to-pink styling, and custom color options.
- Default the bar to 60% of terminal width; support percentage and fixed-column
  widths, live resizing, stable bar length across status changes, and truncation
  of long labels.
- Preserve redirected streams, support monochrome output, and leave the actual
  completed count visible after normal completion or an early exit.
- Recover captured output after renderer failures and handle interruption
  cleanup, bounded shutdown, and repeated stop calls.
- Include a built-in demo using public project names, copy-paste integration
  examples, and a project icon displayed in the README.
- Embed `VERSION` for every build path and document the versioning workflow.
- Cover rendering, shell integration, resizing, failures, and cleanup with Go
  tests and real-terminal integration tests across supported shells.
