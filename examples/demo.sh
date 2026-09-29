#!/bin/sh
# Simulated work only: no repositories or network access required.
set -eu
eval "$("${SHELL_CHARM_PROGRESS_BIN:-shell-charm-progress}" init)"
trap 'progress_stop' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

progress_start 5 "Updating repositories"
failed=0
for repo in gum kube-resource-monitor github-pr-monitor kube-context-manager bubbletea; do
  progress_label "Updating $repo"
  printf '\nUpdating %s…\n' "$repo"
  sleep 0.5
  if [ "${1-}" = --fail ] && [ "$repo" = github-pr-monitor ]; then
    printf 'Failed to update %s (simulated).\n' "$repo" >&2
    failed=$((failed + 1))
  else
    printf 'Already up to date.\n'
  fi
  progress_tick
  sleep 0.2
done
progress_label "Finished · $failed failures"
printf '\nProcessed 5 repositories; %s failures.\n' "$failed"
[ "$failed" -eq 0 ]
