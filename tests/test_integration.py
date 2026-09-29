#!/usr/bin/env python3
"""Integration checks using a real PTY; Python 3 standard library only.

Build with make build, then run this file from any directory.
Set TEST_SHELL to test another installed shell; no network access is needed.
"""
import errno
import fcntl
import os
from pathlib import Path
import pty
import re
import select
import shlex
import shutil
import signal
import struct
import subprocess
import tempfile
import termios
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
BINARY = Path(os.environ.get("TEST_BINARY", ROOT / "bin/shell-charm-progress"))
SHELL = os.environ.get("TEST_SHELL", "/bin/bash")
SOURCE = f'set -eu; eval "$({shlex.quote(str(BINARY))} init)"; '
TRAPS = "trap 'progress_stop' EXIT; trap 'exit 130' INT; trap 'exit 143' TERM; "


def terminal(script, *, columns=80, action=None, timeout=10, env_extra=None):
    with tempfile.TemporaryDirectory(prefix="progress-test-") as tmp:
        pid, master = pty.fork()
        if pid == 0:
            env = dict(os.environ, TERM="xterm-256color", TMPDIR=tmp,
                       SHELL_CHARM_PROGRESS_BIN=str(BINARY), COLORTERM="truecolor")
            env.pop("NO_COLOR", None)
            env.update(env_extra or {})
            os.execve(SHELL, [SHELL, "-c", SOURCE + script], env)
        fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 24, columns, 0, 0))
        output = bytearray()
        started = time.monotonic()
        acted = False
        status = None
        eof = False
        try:
            while not (eof and status is not None):
                if time.monotonic() - started > timeout:
                    raise AssertionError(f"PTY timed out: {bytes(output)!r}")
                if action and not acted and b"READY" in output:
                    action(pid, master)
                    acted = True
                ready, _, _ = select.select([master] if not eof else [], [], [], 0.02)
                if ready:
                    try:
                        data = os.read(master, 65536)
                        if data: output.extend(data)
                        else: eof = True
                    except OSError as exc:
                        if exc.errno != errno.EIO: raise
                        eof = True
                if status is None:
                    found, result = os.waitpid(pid, os.WNOHANG)
                    if found: status = os.waitstatus_to_exitcode(result)
            leftovers = list(Path(tmp).glob("shell-charm-progress.*"))
            if leftovers:
                raise AssertionError(f"Leaked session directories: {leftovers}")
            return status, bytes(output)
        finally:
            if status is None:
                os.killpg(pid, signal.SIGKILL)
                os.waitpid(pid, 0)
            os.close(master)


class ProgressTests(unittest.TestCase):
    def test_idle_terminal_resize_changes_bar_width(self):
        for option, percent in (("", 60), ("--width '75%'", 75), ("--width 40", None)):
            for before, after in ((40, 120), (120, 40)):
                with self.subTest(option=option, before=before, after=after):
                    def resize(pid, fd):
                        # Natural SIGWINCH while no progress updates occur.
                        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 24, after, 0, 0))
                    status, out = terminal(TRAPS + f"progress_start --total 2 --title '' {option} --no-color; "
                                           "echo READY; sleep 0.5; progress_stop",
                                           columns=before, action=resize)
                    self.assertEqual(status, 0)
                    widths = [len(run) for run in re.findall("░+", out.decode())]
                    expected = min(after - 6, after * percent // 100 if percent else 40)
                    self.assertIn(expected, widths)
                    self.assertEqual(widths[-1], expected)

    def test_width_validation_and_legacy_auto_alias(self):
        script = SOURCE + """
for width in 0 2 '0%' '101%' '60%%' '-1' '1.5%' '060%' '040' '+40' '60% ' 9999999999; do
  if progress_start 1 --width "$width"; then exit 9; fi
done
for width in '1%' '60%' '100%' 3 40 300 999999999 auto; do
  progress_start 1 --width "$width"
  progress_stop
done
"""
        result = subprocess.run([SHELL, "-c", script], capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        status, out = terminal(TRAPS + "progress_start 2 Short --width auto --no-color; "
                               "progress_label 'A status long enough to otherwise shrink the bar considerably'; "
                               "sleep 0.1; progress_stop", columns=100)
        self.assertEqual(status, 0)
        self.assertEqual(len(re.findall("░+", out.decode())[-1]), 60)

    def test_tick_optional_labels_and_session_restart(self):
        status, out = terminal(TRAPS + """
progress_start 3 'First session'
progress_tick 'First item'
progress_update 2 'Second item'
progress_tick 'All items finished'
if progress_tick; then exit 9; fi
progress_stop
progress_tick; progress_update 999; progress_label ignored
progress_start --total 1 --title 'Second session'
if progress_start 1; then exit 9; fi
progress_tick 'Restarted successfully'
progress_stop
""")
        self.assertEqual(status, 0)
        for text in (b"3/3", b"All items finished", b"1/1", b"Restarted successfully"):
            self.assertIn(text, out)
        self.assertNotIn(b"replaying", out)

    def test_no_color_environment_and_custom_theme(self):
        status, out = terminal("progress_start 2; progress_tick; progress_stop",
                               env_extra={"NO_COLOR": "1"})
        self.assertEqual(status, 0)
        self.assertNotIn(b"38;", out)
        self.assertNotIn("▌".encode(), out)
        status, out = terminal("progress_start 2 --color '#123456' --color-end '#abcdef' "
                               "--width 40; progress_tick; progress_stop")
        self.assertEqual(status, 0)
        self.assertIn(b"38;2;171;205;239", out)

    def test_redirect_one_stream_and_dumb_terminal(self):
        status, out = terminal("""
exec 2>"$TMPDIR/error.log"
progress_start 2; echo ORDINARY; echo ERROR >&2; progress_tick; progress_stop
cat "$TMPDIR/error.log"
""")
        self.assertEqual((status, out), (0, b"ORDINARY\r\nERROR\r\n"))
        status, out = terminal("progress_start 1; echo DUMB; progress_tick; progress_stop",
                               env_extra={"TERM": "dumb"})
        self.assertEqual((status, out), (0, b"DUMB\r\n"))

    def test_single_binary_relocated_with_quoted_path(self):
        with tempfile.TemporaryDirectory(prefix="progress-install-") as tmp:
            binary = Path(tmp) / "a 'quoted' $name executable"
            shutil.copy2(BINARY, binary)
            init = f'eval "$({shlex.quote(str(binary))} init)"; '
            # No helper file is installed beside this executable.
            status, out = terminal(init + TRAPS + "progress_start 1 'Relocated'; "
                                   "cd /; echo PORTABLE; progress_tick; progress_stop",
                                   env_extra={"SHELL_CHARM_PROGRESS_BIN": ""})
            self.assertEqual(status, 0)
            self.assertIn(b"PORTABLE", out)
            self.assertIn(b"1/1", out)
            self.assertNotIn(b"continuing without progress", out)

    def test_demo_embedded_in_binary(self):
        for args, expected in (([], 0), (["--fail"], 1)):
            run = subprocess.run([str(BINARY), "demo", *args], capture_output=True, timeout=8)
            self.assertEqual(run.returncode, expected)
            self.assertIn(b"Processed 5 repositories", run.stdout)
            self.assertNotIn(b"\x1b[", run.stdout + run.stderr)
            if expected:
                self.assertIn(b"Failed to update github-pr-monitor", run.stderr)

    def test_redirected_streams_and_zero_work(self):
        script = SOURCE + TRAPS + """
progress_label before; progress_update 1
progress_start --total 2 --title Test
echo stdout; echo stderr >&2; progress_update 1; progress_stop
progress_start --total 0 --title Empty; progress_update 0; progress_stop
"""
        result = subprocess.run([SHELL, "-c", script], capture_output=True)
        self.assertEqual((result.returncode, result.stdout, result.stderr), (0, b"stdout\n", b"stderr\n"))

    def test_output_counts_tail_and_repeat_stop(self):
        status, out = terminal(TRAPS + """
progress_start --total 3 --title Test
for n in 1 2 3; do
  progress_label "$(printf '项目\twith spaces\n🐚')"
  echo "success-$n"; echo "error-$n" >&2
  progress_update "$n"; sleep 0.08
done
printf tail-without-newline
progress_stop; progress_stop; echo RESTORED
""")
        self.assertEqual(status, 0)
        for n in range(1, 4):
            self.assertEqual(out.count(f"success-{n}".encode()), 1)
            self.assertEqual(out.count(f"error-{n}".encode()), 1)
        self.assertIn(b"tail-without-newline", out)
        self.assertIn(b"3/3", out)
        self.assertIn(b"RESTORED", out)
        self.assertNotIn(b"\x1b[?1049h", out)
        self.assertNotIn(b"replaying", out)

    def test_early_exit_and_existing_cleanup(self):
        status, out = terminal("trap 'rc=$?; progress_stop; echo CLEANUP-$rc' EXIT; "
                               "progress_start --total 5; progress_update 2; echo partial; exit 7")
        self.assertEqual(status, 7)
        self.assertIn(b"2/5", out)
        self.assertNotIn(b"5/5", out)
        self.assertIn(b"CLEANUP-7", out)

    def test_preserves_stop_status(self):
        status, out = terminal("progress_start --total 1; set +e; false; progress_stop; "
                               "rc=$?; echo STATUS-$rc; exit $rc")
        self.assertEqual(status, 1)
        self.assertIn(b"STATUS-1", out)

    def test_color_preference_including_final_line(self):
        for option in ("", "--no-color"):
            status, out = terminal(f"progress_start --total 1 {option}; progress_update 1; progress_stop")
            self.assertEqual(status, 0)
            self.assertEqual(b"\x1b[38;" in out, option == "")

    def test_errexit_cleans_up(self):
        status, out = terminal(TRAPS + "progress_start --total 2; echo before-error; false; echo NEVER")
        self.assertEqual(status, 1)
        self.assertIn(b"before-error", out)
        self.assertNotIn(b"NEVER", out)

    def test_renderer_crash_and_no_pipe_backpressure(self):
        status, out = terminal(TRAPS + """
progress_start --total 1
kill -KILL "$_SCP_PID"
wait "$_SCP_PID" 2>/dev/null || true
n=0; while [ "$n" -lt 3000 ]; do echo captured-output; n=$((n + 1)); done
printf final-tail
progress_update 1
echo RESTORED
""")
        self.assertEqual(status, 0)
        self.assertIn(b"replaying captured output", out)
        self.assertIn(b"final-tail", out)
        self.assertIn(b"RESTORED", out)

    def test_startup_failure_missing_renderer_and_zero(self):
        for renderer in ("/usr/bin/false", "/missing/progress"):
            status, out = terminal(f"SHELL_CHARM_PROGRESS_BIN={renderer}; "
                                   "progress_start --total 1; echo WORK; progress_stop")
            self.assertEqual(status, 0)
            self.assertIn(b"continuing without progress", out)
            self.assertIn(b"WORK", out)
        status, out = terminal("progress_start --total 0; echo ZERO; progress_stop")
        self.assertEqual((status, out), (0, b"ZERO\r\n"))

    def test_invalid_counts_and_reserved_descriptors(self):
        status, out = terminal("""
if progress_start --total 01; then exit 9; fi
progress_start --total 2
if progress_update 3; then exit 9; fi
if progress_update -1; then exit 9; fi
if progress_update 99999999999999999999; then exit 9; fi
progress_stop
exec 8>&1
progress_start --total 2; echo FD; progress_stop
""")
        self.assertEqual(status, 0)
        self.assertIn(b"descriptors 8/9 are in use", out)

    def test_signals(self):
        for sig, expected in ((signal.SIGINT, 130), (signal.SIGTERM, 143)):
            with self.subTest(sig=sig):
                status, out = terminal(TRAPS + "progress_start --total 2; echo READY; sleep 3",
                                       action=lambda pid, fd: os.write(fd, b"\x03") if sig == signal.SIGINT
                                       else os.killpg(pid, sig))
                self.assertEqual(status, expected)
                self.assertNotIn(b"2/2", out)

    def test_stalled_renderer_has_bounded_shutdown(self):
        status, out = terminal(TRAPS + """
progress_start --total 1
kill -STOP "$_SCP_PID"
echo retained-on-timeout
progress_stop
echo RESTORED
""", timeout=12)
        self.assertEqual(status, 0)
        self.assertIn(b"replaying captured output", out)
        self.assertIn(b"retained-on-timeout", out)
        self.assertIn(b"RESTORED", out)

    def test_background_writer_does_not_delay_stop(self):
        status, out = terminal(TRAPS + """
progress_start --total 1
sleep 30 &
child=$!
progress_stop
echo STOPPED
kill "$child"; wait "$child" || true
""", timeout=8)
        self.assertEqual(status, 0)
        self.assertIn(b"STOPPED", out)

    def test_resize_and_stdin_is_available(self):
        def resize_and_type(pid, fd):
            fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 12, 0, 0))
            os.killpg(pid, signal.SIGWINCH)
            os.write(fd, b"user-input\n")
        status, out = terminal(TRAPS + "progress_start --total 2; echo READY; "
                               "read -r answer; echo GOT-$answer; progress_update 1; sleep 0.2",
                               columns=100, action=resize_and_type)
        self.assertEqual(status, 0)
        self.assertIn(b"GOT-user-input", out)



if __name__ == "__main__":
    unittest.main(verbosity=2)
