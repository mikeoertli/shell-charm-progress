# Embedded by shell-charm-progress init. Compatible with Bash 3.2+, Zsh and dash.
# No traps or shell options are changed. Descriptors 8/9 are reserved while active.

_scp_number() {
  case ${1-} in ''|*[!0-9]*|0?*) return 1 ;; esac
  [ "${#1}" -le 9 ]
}

_scp_width() {
  local number=${1%\%}
  case $1 in
    auto) return 0 ;;
    *%) _scp_number "$number" && [ "$number" -ge 1 ] && [ "$number" -le 100 ] ;;
    *) _scp_number "$number" && [ "$number" -ge 3 ] ;;
  esac
}

_scp_dispose() {
  if [ -n "${_SCP_PID-}" ]; then
    kill -KILL "$_SCP_PID" 2>/dev/null || true
    wait "$_SCP_PID" 2>/dev/null || true
  fi
  [ -z "${_SCP_DIR-}" ] || rm -rf -- "$_SCP_DIR"
  _SCP_PID='' _SCP_DIR='' _SCP_ACTIVE=0
}

progress_start() {
  local total='' title='Working' width=60% color='' color_end='' no_color=false renderer i startup_error
  if [ "${_SCP_STARTED-0}" = 1 ]; then
    printf '%s\n' 'shell-charm-progress: a session is already started; call progress_stop first.' >&2
    return 2
  fi
  case ${1-} in
    ''|--*) ;;
    *) total=$1; shift
       case ${1-} in ''|--*) ;; *) title=$1; shift ;; esac ;;
  esac
  while [ "$#" -gt 0 ]; do
    case $1 in
      --total|--title|--width|--color|--color-end)
        if [ "$#" -lt 2 ]; then
          printf 'shell-charm-progress: missing value for %s\n' "$1" >&2
          return 2
        fi
        case $1 in
          --total) total=$2 ;; --title) title=$2 ;; --width) width=$2 ;;
          --color) color=$2 ;; --color-end) color_end=$2 ;;
        esac
        shift 2 ;;
      --no-color) no_color=true; shift ;;
      *) printf 'shell-charm-progress: unknown option: %s\n' "$1" >&2; return 2 ;;
    esac
  done
  if ! _scp_number "$total"; then
    printf '%s\n' 'shell-charm-progress: total must be 0–999999999, without leading zeros.' >&2
    return 2
  fi
  if ! _scp_width "$width"; then
    printf '%s\n' 'shell-charm-progress: width must be 1%–100% or 3–999999999 columns.' >&2
    return 2
  fi
  _SCP_TOTAL=$total _SCP_CURRENT=0 _SCP_STARTED=1
  # Preserve both streams exactly in pipelines, CI, redirected runs and zero work.
  [ "$total" != 0 ] && [ -t 1 ] && [ -t 2 ] && [ -n "${TERM-}" ] && [ "${TERM-}" != dumb ] || return 0
  renderer=${SHELL_CHARM_PROGRESS_BIN:-${_SCP_BIN-}}
  if [ ! -x "$renderer" ]; then
    printf 'shell-charm-progress: renderer unavailable (%s); continuing without progress.\n' "$renderer" >&2
    return 0
  fi
  # Subshells contain fatal redirection errors in POSIX shells.
  if ( : >&8 ) 2>/dev/null || ( : >&9 ) 2>/dev/null; then
    printf '%s\n' 'shell-charm-progress: descriptors 8/9 are in use; continuing without progress.' >&2
    return 0
  fi
  if ! _SCP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/shell-charm-progress.XXXXXXXX"); then
    printf '%s\n' 'shell-charm-progress: cannot create session; continuing without progress.' >&2
    return 0
  fi
  if ! { : >"$_SCP_DIR/logs" && printf 'label\000%s\000' "$title" >"$_SCP_DIR/control"; }; then
    _scp_dispose
    return 0
  fi
  exec 8>&1 9>&2
  "$renderer" render --session "$_SCP_DIR" --total "$total" --parent "$$" \
    --width "$width" --color "$color" --color-end "$color_end" --no-color="$no_color" \
    </dev/null >&8 2>"$_SCP_DIR/renderer-error" 8>&- 9>&- &
  _SCP_PID=$!
  i=0
  while [ "$i" -lt 100 ]; do
    [ ! -f "$_SCP_DIR/ready" ] || break
    kill -0 "$_SCP_PID" 2>/dev/null || break
    sleep 0.02
    i=$((i + 1))
  done
  if [ ! -f "$_SCP_DIR/ready" ] || ! kill -0 "$_SCP_PID" 2>/dev/null; then
    startup_error=$(cat "$_SCP_DIR/renderer-error")
    _scp_dispose
    printf '\033[0m\033[?25h\r\033[2Kshell-charm-progress: renderer failed to start; continuing without progress.\n' >&9
    [ -z "$startup_error" ] || printf '%s\n' "$startup_error" >&9
    exec 8>&- 9>&-
    return 0
  fi
  _SCP_ACTIVE=1
  # A shared append journal avoids pipe backpressure if the renderer crashes.
  exec >>"$_SCP_DIR/logs" 2>&1
  return 0
}

_scp_send() {
  [ "${_SCP_ACTIVE-0}" = 1 ] || return 0
  if ! kill -0 "$_SCP_PID" 2>/dev/null; then
    progress_stop
    return 0
  fi
  # NUL framing carries whitespace and Unicode without eval or JSON escaping.
  if ! printf '%s\000%s\000' "$1" "$2" >>"$_SCP_DIR/control"; then
    progress_stop
  fi
  return 0
}

progress_label() {
  [ "${_SCP_STARTED-0}" = 1 ] || return 0
  if [ "$#" -ne 1 ]; then
    printf '%s\n' 'shell-charm-progress: progress_label expects one quoted label.' >&2
    return 2
  fi
  _scp_send label "$1"
}

progress_update() {
  [ "${_SCP_STARTED-0}" = 1 ] || return 0
  if [ "$#" -lt 1 ] || [ "$#" -gt 2 ] || ! _scp_number "${1-}" || [ "$1" -gt "$_SCP_TOTAL" ]; then
    printf '%s\n' 'shell-charm-progress: progress_update expects a count from zero to the total and an optional label.' >&2
    return 2
  fi
  _SCP_CURRENT=$1
  if [ "$#" -eq 2 ]; then _scp_send label "$2"; fi
  _scp_send count "$1"
}

progress_tick() {
  [ "${_SCP_STARTED-0}" = 1 ] || return 0
  if [ "$#" -gt 1 ]; then
    printf '%s\n' 'shell-charm-progress: progress_tick accepts one optional label.' >&2
    return 2
  fi
  progress_update "$((_SCP_CURRENT + 1))" "$@"
}

progress_stop() {
  local caller_status=$? i
  _SCP_STARTED=0
  [ "${_SCP_ACTIVE-0}" = 1 ] || return "$caller_status"
  _SCP_ACTIVE=0
  # Stop capture, then drain a bounded snapshot. Later background writes aren't included.
  exec 1>&8 2>&9 8>&- 9>&-
  printf 'stop\000\000' >>"$_SCP_DIR/control" || true
  i=0
  while [ "$i" -lt 250 ]; do
    kill -0 "$_SCP_PID" 2>/dev/null || break
    sleep 0.02
    i=$((i + 1))
  done
  if kill -0 "$_SCP_PID" 2>/dev/null; then
    kill -KILL "$_SCP_PID" 2>/dev/null || true
  fi
  wait "$_SCP_PID" 2>/dev/null || true
  _SCP_PID=''
  if [ ! -f "$_SCP_DIR/finished" ]; then
    printf '\033[0m\033[?25h\r\033[2K\nshell-charm-progress: renderer stopped; replaying captured output (some lines may repeat).\n' >&2
    cat "$_SCP_DIR/logs"
    cat "$_SCP_DIR/renderer-error" >&2
  fi
  _scp_dispose
  return "$caller_status"
}
