#!/usr/bin/env python3
"""Minimal GRBL G-code sender with line-by-line handshake.

Usage:
  python3 scripts/send-gcode.py --port /dev/ttyUSB0 --baud 115200 file.gcode
  python3 scripts/send-gcode.py --cmd '$$'
"""

from __future__ import annotations

import argparse
from collections import deque
import glob
import os
import re
import select
import sys
import termios
import time


BAUD_MAP = {
    9600: termios.B9600,
    19200: termios.B19200,
    38400: termios.B38400,
    57600: termios.B57600,
    115200: termios.B115200,
    230400: termios.B230400,
}

_RX_BUF = bytearray()


def configure_serial(fd: int, baud: int) -> None:
    if baud not in BAUD_MAP:
        raise ValueError(f"unsupported baud: {baud}")
    attrs = termios.tcgetattr(fd)
    attrs[0] = 0  # iflag
    attrs[1] = 0  # oflag
    attrs[2] = termios.CREAD | termios.CLOCAL | termios.CS8  # cflag
    attrs[3] = 0  # lflag
    attrs[4] = BAUD_MAP[baud]  # ispeed
    attrs[5] = BAUD_MAP[baud]  # ospeed
    attrs[6][termios.VMIN] = 0
    attrs[6][termios.VTIME] = 1  # 100ms
    termios.tcsetattr(fd, termios.TCSANOW, attrs)
    termios.tcflush(fd, termios.TCIOFLUSH)


def read_line(fd: int, timeout_s: float) -> str | None:
    deadline = time.time() + timeout_s
    global _RX_BUF
    if b"\n" in _RX_BUF:
        line, _, rest = _RX_BUF.partition(b"\n")
        _RX_BUF = bytearray(rest)
        return line.decode("utf-8", errors="replace").strip()
    while time.time() < deadline:
        r, _, _ = select.select([fd], [], [], 0.1)
        if not r:
            continue
        chunk = os.read(fd, 256)
        if not chunk:
            continue
        _RX_BUF.extend(chunk)
        if b"\n" in _RX_BUF:
            line, _, rest = _RX_BUF.partition(b"\n")
            _RX_BUF = bytearray(rest)
            return line.decode("utf-8", errors="replace").strip()
    if _RX_BUF:
        line = _RX_BUF.decode("utf-8", errors="replace").strip()
        _RX_BUF.clear()
        return line
    return None


COMMENT_PAREN_RE = re.compile(r"\([^)]*\)")
WORD_RE = re.compile(r"([A-Z])([-+]?(?:\d+(?:\.\d*)?|\.\d+))", re.IGNORECASE)
G_WORD_RE = re.compile(r"G([-+]?(?:\d+(?:\.\d*)?|\.\d+))", re.IGNORECASE)


def clean_gcode_line(line: str) -> str:
    line = COMMENT_PAREN_RE.sub("", line)
    if ";" in line:
        line = line.split(";", 1)[0]
    return line.strip()


def wait_for_ready(fd: int, timeout_s: float) -> None:
    end = time.time() + timeout_s
    while time.time() < end:
        line = read_line(fd, 0.25)
        if not line:
            continue
        print(f"<< {line}")
        if "grbl" in line.lower():
            return


def send_and_wait_ok(fd: int, cmd: str, timeout_s: float, watchdog: LaserWatchdog | None = None) -> None:
    os.write(fd, (cmd + "\n").encode("ascii", errors="ignore"))
    deadline = time.time() + timeout_s
    while True:
        if watchdog is not None and not _is_long_motion_exec_cmd(cmd):
            watchdog.check(cmd)
        line = read_line(fd, 0.1)
        if line is None:
            if time.time() >= deadline:
                raise TimeoutError(f"timeout waiting for response to: {cmd}")
            continue
        if not line:
            continue
        print(f"<< {line}")
        low = line.lower()
        if low == "ok":
            return
        if low.startswith("error") or low.startswith("alarm"):
            raise RuntimeError(f"controller rejected line '{cmd}': {line}")


def _wire_len(cmd: str) -> int:
    return len(cmd.encode("ascii", errors="ignore")) + 1


def send_job_buffered(
    fd: int,
    lines: list[str],
    line_timeout_s: float,
    watchdog: LaserWatchdog | None,
    stream_buffer_bytes: int,
) -> None:
    total = len(lines)
    pending: deque[list[object]] = deque()  # [1-based line number, cmd, wire bytes, watchdog_applied]
    queued_bytes = 0
    next_idx = 0
    acked = 0
    last_rx = time.monotonic()

    while acked < total:
        # Fill planner/RX window.
        while next_idx < total:
            cmd = lines[next_idx]
            n = _wire_len(cmd)
            if pending and queued_bytes+n > stream_buffer_bytes:
                break
            os.write(fd, (cmd + "\n").encode("ascii", errors="ignore"))
            pending.append([next_idx + 1, cmd, n, False])
            queued_bytes += n
            next_idx += 1
            if next_idx == total:
                break

        if pending and watchdog is not None:
            head = pending[0]
            head_cmd = head[1]
            if not head[3]:
                watchdog.pre_send(head_cmd)
                head[3] = True
            if not _is_long_motion_exec_cmd(head_cmd):
                watchdog.check(head_cmd)

        line = read_line(fd, 0.05)
        if line is None:
            if pending and time.monotonic()-last_rx >= line_timeout_s:
                raise TimeoutError(f"timeout waiting for response to: {pending[0][1]}")
            continue
        if not line:
            continue

        print(f"<< {line}")
        low = line.lower()
        last_rx = time.monotonic()
        if low == "ok":
            if not pending:
                continue
            idx, cmd, n, _ = pending.popleft()
            queued_bytes -= n
            acked = idx
            if acked == 1 or acked % 100 == 0 or acked == total:
                print(f"[{acked}/{total}] {cmd}")
            continue
        if low.startswith("error") or low.startswith("alarm"):
            failing = pending[0][1] if pending else "<unknown>"
            raise RuntimeError(f"controller rejected line '{failing}': {line}")


class LaserWatchdog:
    """Tracks predicted laser-on state from outgoing G-code and enforces a max on-duration."""

    def __init__(self, max_on_seconds: float) -> None:
        self.max_on_seconds = max_on_seconds
        self.mode: str | None = None
        self.power = 0.0
        self.motion_mode: int | None = None
        self.on_since: float | None = None

    def _update_on_timer(self, now: float) -> None:
        laser_on = self._predicted_laser_on
        if laser_on:
            if self.on_since is None:
                self.on_since = now
        else:
            self.on_since = None

    @property
    def _predicted_laser_on(self) -> bool:
        return getattr(self, "_laser_on_now", False)

    def pre_send(self, cmd: str) -> None:
        if self.max_on_seconds <= 0:
            return
        up = cmd.upper()
        words = WORD_RE.findall(up)
        m_vals = [int(float(v)) for k, v in words if k.upper() == "M"]
        s_vals = [float(v) for k, v in words if k.upper() == "S"]
        g_vals = [int(float(v)) for k, v in words if k.upper() == "G"]
        has_axis_word = any(k.upper() in ("X", "Y", "Z") for k, _ in words)

        # Track modal motion mode so axis-only lines can be interpreted.
        for g in g_vals:
            if g in (0, 1, 2, 3):
                self.motion_mode = g

        if 5 in m_vals:
            self.mode = None
            self.power = 0.0
        else:
            if 3 in m_vals:
                self.mode = "M3"
            elif 4 in m_vals:
                self.mode = "M4"
            if s_vals:
                self.power = s_vals[-1]

        # Determine whether this outgoing command can keep the beam actively on.
        laser_on_now = False
        if self.mode == "M3":
            # Constant-power mode can be on while stationary.
            laser_on_now = self.power > 0.0
        elif self.mode == "M4":
            # Dynamic mode should only be considered "on" during burn motion.
            burn_motion = any(g in (1, 2, 3) for g in g_vals)
            if not burn_motion and has_axis_word and self.motion_mode in (1, 2, 3):
                burn_motion = True
            laser_on_now = self.power > 0.0 and burn_motion
        self._laser_on_now = laser_on_now
        self._update_on_timer(time.monotonic())

    def check(self, cmd: str) -> None:
        if self.max_on_seconds <= 0 or self.on_since is None:
            return
        elapsed = time.monotonic() - self.on_since
        if elapsed > self.max_on_seconds:
            raise RuntimeError(
                f"laser watchdog tripped after {elapsed:.2f}s (> {self.max_on_seconds:.2f}s) while waiting for: {cmd}"
            )


def best_effort_safe_stop(fd: int, line_timeout_s: float) -> None:
    print("!! Safety stop: attempting M5/hold/reset")
    stop_timeout = max(1.0, min(3.0, line_timeout_s))
    try:
        send_and_wait_ok(fd, "M5", stop_timeout)
        print("!! Safety stop: M5 acknowledged")
        return
    except Exception as e:
        print(f"!! Safety stop: M5 not acknowledged ({e})")

    # Feed hold (real-time command) can help halt queued motion if firmware supports it.
    try:
        os.write(fd, b"!")
        time.sleep(0.1)
    except Exception as e:
        print(f"!! Safety stop: feed-hold write failed ({e})")

    # Soft reset (Ctrl-X) reinitializes GRBL and usually forces laser off.
    try:
        os.write(fd, b"\x18")
        time.sleep(0.5)
        termios.tcflush(fd, termios.TCIFLUSH)
        wait_for_ready(fd, 2.0)
    except Exception as e:
        print(f"!! Safety stop: soft-reset failed ({e})")

    try:
        send_and_wait_ok(fd, "M5", stop_timeout)
        print("!! Safety stop: M5 acknowledged after reset")
    except Exception as e:
        print(f"!! Safety stop: final M5 still not acknowledged ({e})")


def to_dry_run_line(cmd: str) -> str:
    c = cmd.strip()
    if not c:
        return ""
    up = c.upper()
    if up.startswith("M3") or up.startswith("M4"):
        return "M5"
    # Remove spindle power words from motion lines.
    c = re.sub(r"\sS[-+]?(?:\d+(?:\.\d*)?|\.\d+)\b", "", c, flags=re.IGNORECASE)
    return c.strip()


def parse_offset(spec: str) -> tuple[float, float]:
    parts = [p.strip() for p in spec.split(",", 1)]
    if len(parts) != 2:
        raise ValueError("offset must be 'x,y'")
    try:
        dx = float(parts[0])
        dy = float(parts[1])
    except ValueError as e:
        raise ValueError(f"invalid offset '{spec}': {e}") from e
    return dx, dy


def _is_motion_line(cmd: str) -> bool:
    for m in G_WORD_RE.finditer(cmd):
        g = int(float(m.group(1)))
        if g in (0, 1, 2, 3):
            return True
    return False


def _is_long_motion_exec_cmd(cmd: str) -> bool:
    """True for commands that can legitimately take long to acknowledge."""
    up = cmd.upper()
    words = WORD_RE.findall(up)
    if not words:
        return False
    g_vals = [int(float(v)) for k, v in words if k.upper() == "G"]
    has_xyz = any(k.upper() in ("X", "Y", "Z") for k, _ in words)
    has_arc = any(k.upper() in ("I", "J", "K", "R") for k, _ in words)
    for g in g_vals:
        if g in (0, 1, 2, 3) and (has_xyz or has_arc):
            return True
    return False


def _apply_xy_shift(cmd: str, dx: float, dy: float) -> str:
    def repl(m: re.Match[str]) -> str:
        axis = m.group(1)
        raw = m.group(2)
        dec = len(raw.split(".", 1)[1]) if "." in raw else 0
        delta = dx if axis.upper() == "X" else dy
        v = float(raw) + delta
        if dec > 0:
            out = f"{v:.{dec}f}"
        else:
            if abs(v - round(v)) <= 1e-9:
                out = str(int(round(v)))
            else:
                out = f"{v:.3f}"
        return axis + out

    return re.sub(r"([XY])([-+]?(?:\d+(?:\.\d*)?|\.\d+))", repl, cmd, flags=re.IGNORECASE)


def apply_offset(lines: list[str], dx: float, dy: float) -> list[str]:
    if abs(dx) <= 1e-12 and abs(dy) <= 1e-12:
        return lines
    out: list[str] = []
    abs_mode = True
    for cmd in lines:
        line_abs = abs_mode
        for m in G_WORD_RE.finditer(cmd):
            g = int(float(m.group(1)))
            if g == 90:
                line_abs = True
            elif g == 91:
                line_abs = False
        shifted = cmd
        up = cmd.upper()
        if line_abs and _is_motion_line(cmd) and ("X" in up or "Y" in up):
            shifted = _apply_xy_shift(cmd, dx, dy)
        out.append(shifted)
        abs_mode = line_abs
    return out


def parse_xy_bounds(lines: list[str]) -> tuple[float, float, float, float]:
    abs_mode = True
    unit_scale = 1.0  # mm
    motion_mode = ""
    cur_x: float | None = None
    cur_y: float | None = None
    min_x = float("inf")
    max_x = float("-inf")
    min_y = float("inf")
    max_y = float("-inf")

    for cmd in lines:
        words = WORD_RE.findall(cmd)
        if not words:
            continue
        tokens = {k.upper(): v for k, v in words}

        # Modal state updates.
        if "G90" in cmd.upper():
            abs_mode = True
        if "G91" in cmd.upper():
            abs_mode = False
        if "G20" in cmd.upper():
            unit_scale = 25.4
        if "G21" in cmd.upper():
            unit_scale = 1.0

        g_words = [v for k, v in words if k.upper() == "G"]
        for gv in g_words:
            g = int(float(gv))
            if g in (0, 1, 2, 3):
                motion_mode = f"G{g}"

        has_x = "X" in tokens
        has_y = "Y" in tokens
        if not (has_x or has_y):
            continue
        if motion_mode not in ("G0", "G1", "G2", "G3"):
            continue

        x_word = float(tokens["X"]) * unit_scale if has_x else None
        y_word = float(tokens["Y"]) * unit_scale if has_y else None
        if cur_x is None and has_x:
            cur_x = 0.0 if not abs_mode else x_word
        if cur_y is None and has_y:
            cur_y = 0.0 if not abs_mode else y_word
        if cur_x is None or cur_y is None:
            continue

        if abs_mode:
            if x_word is not None:
                cur_x = x_word
            if y_word is not None:
                cur_y = y_word
        else:
            if x_word is not None:
                cur_x += x_word
            if y_word is not None:
                cur_y += y_word

        if cur_x < min_x:
            min_x = cur_x
        if cur_x > max_x:
            max_x = cur_x
        if cur_y < min_y:
            min_y = cur_y
        if cur_y > max_y:
            max_y = cur_y

    if min_x == float("inf") or min_y == float("inf"):
        raise ValueError("no XY motion coordinates found in gcode")
    return min_x, min_y, max_x, max_y


def bounds_out_of_workspace(min_x: float, min_y: float, max_x: float, max_y: float, bed_mm: float) -> str | None:
    if bed_mm <= 0:
        return None
    if min_x < 0 or min_y < 0 or max_x > bed_mm or max_y > bed_mm:
        return (
            f"motion bounds out of workspace: X[{min_x:.3f}..{max_x:.3f}] "
            f"Y[{min_y:.3f}..{max_y:.3f}] workspace=0..{bed_mm:.3f}"
        )
    return None


def preview_bounds_lines(lines: list[str], s_value: int, feed: float, margin: float, dx: float, dy: float) -> list[str]:
    if s_value <= 0:
        raise ValueError("--preview-s must be > 0")
    if feed <= 0:
        raise ValueError("--preview-feed must be > 0")
    if margin < 0:
        raise ValueError("--preview-margin must be >= 0")
    min_x, min_y, max_x, max_y = parse_xy_bounds(lines)
    min_x += dx
    max_x += dx
    min_y += dy
    max_y += dy
    min_x -= margin
    min_y -= margin
    max_x += margin
    max_y += margin
    if max_x <= min_x or max_y <= min_y:
        raise ValueError("invalid preview bounds: empty rectangle")

    print(
        f"Preview bounds: X[{min_x:.3f}..{max_x:.3f}] "
        f"Y[{min_y:.3f}..{max_y:.3f}] S{s_value} F{feed:.1f}"
    )
    return [
        "M5",
        "G21",
        "G90",
        f"G0 X{min_x:.3f} Y{min_y:.3f}",
        f"M3 S{s_value}",
        f"G1 F{feed:.1f}",
        f"G1 X{max_x:.3f} Y{min_y:.3f}",
        f"G1 X{max_x:.3f} Y{max_y:.3f}",
        f"G1 X{min_x:.3f} Y{max_y:.3f}",
        f"G1 X{min_x:.3f} Y{min_y:.3f}",
        "M5",
    ]


def load_lines(path: str, dry_run: bool = False) -> list[str]:
    if os.path.isdir(path):
        candidates = sorted(
            glob.glob(os.path.join(path, "*.gcode"))
            + glob.glob(os.path.join(path, "*.gc"))
            + glob.glob(os.path.join(path, "*.nc"))
        )
        hint = ""
        if candidates:
            sample = ", ".join(os.path.basename(p) for p in candidates[:3])
            if len(candidates) > 3:
                sample += ", ..."
            hint = f" Available files: {sample}"
        raise ValueError(f"gcode path is a directory: {path}. Pass a file path instead.{hint}")
    if not os.path.exists(path):
        raise ValueError(f"gcode file does not exist: {path}")
    if not os.path.isfile(path):
        raise ValueError(f"gcode path is not a regular file: {path}")
    out: list[str] = []
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            for raw in f:
                line = clean_gcode_line(raw)
                if not line:
                    continue
                if dry_run:
                    line = to_dry_run_line(line)
                    if not line:
                        continue
                out.append(line)
    except OSError as e:
        raise ValueError(f"cannot read gcode file '{path}': {e}") from e
    if dry_run:
        out.insert(0, "M5")
        out.append("M5")
    return out


def detect_default_port() -> str | None:
    candidates = []
    candidates.extend(sorted(glob.glob("/dev/ttyACM*")))
    candidates.extend(sorted(glob.glob("/dev/ttyUSB*")))
    candidates.extend(sorted(glob.glob("/dev/ttyAMA*")))
    candidates.extend(sorted(glob.glob("/dev/ttyS*")))
    return candidates[0] if candidates else None


def main() -> int:
    default_port = detect_default_port()
    ap = argparse.ArgumentParser(description="Send G-code to GRBL with handshake")
    ap.add_argument("gcode", nargs="?", help="path to .gcode file")
    ap.add_argument("--cmd", help="send a single GRBL/controller command and exit (for example '$#', '$$', '?')")
    ap.add_argument("--port", default=default_port, help="serial port, default: autodetect (/dev/ttyACM0, /dev/ttyUSB0)")
    ap.add_argument("--baud", type=int, default=115200, help="serial baud (default 115200)")
    ap.add_argument("--startup-wait", type=float, default=2.0, help="seconds to wait after wakeup")
    ap.add_argument("--line-timeout", type=float, default=10.0, help="timeout per line response")
    ap.add_argument("--unlock", action="store_true", help="send $X after wakeup")
    ap.add_argument("--home", action="store_true", help="send $H before job (default behavior for job sends)")
    ap.add_argument("--no-home", action="store_true", help="do not home before sending a job")
    ap.add_argument("--home-only", action="store_true", help="connect, optionally unlock, home, then exit")
    ap.add_argument("--dry-run", action="store_true", help="force laser-off: rewrite M3/M4 to M5 and strip S words")
    ap.add_argument("--preview-bounds", action="store_true", help="send only a low-power perimeter preview of gcode XY bounds")
    ap.add_argument("--preview-s", type=int, default=10, help="laser power for --preview-bounds (default 10)")
    ap.add_argument("--preview-feed", type=float, default=1500.0, help="feed rate for --preview-bounds trace (default 1500)")
    ap.add_argument("--preview-margin", type=float, default=0.0, help="extra margin in mm around bounds for --preview-bounds")
    ap.add_argument("--offset", default="0,0", help="optional XY offset in mm applied at send time, format 'x,y' (for example '0,25')")
    ap.add_argument("--bed-mm", type=float, default=150.0, help="workspace max X/Y in mm for preflight bounds checks (set <=0 to disable)")
    ap.add_argument(
        "--stream-buffer-bytes",
        type=int,
        default=96,
        help="queued command bytes before waiting for GRBL acks (default 96, set <=0 for legacy line-by-line)",
    )
    ap.add_argument(
        "--max-laser-on-seconds",
        type=float,
        default=3.0,
        help="safety watchdog: max continuous predicted laser-on time before forced stop (set <=0 to disable, default 3.0)",
    )
    args = ap.parse_args()

    if not args.port:
        print("ERROR: no serial port found; pass --port explicitly", file=sys.stderr)
        return 2
    if args.home_only and args.no_home:
        print("ERROR: --home-only and --no-home conflict", file=sys.stderr)
        return 2
    if args.cmd and args.gcode:
        print("ERROR: pass either a gcode file or --cmd, not both", file=sys.stderr)
        return 2
    if args.cmd and args.preview_bounds:
        print("ERROR: --cmd and --preview-bounds conflict", file=sys.stderr)
        return 2
    try:
        offset_x, offset_y = parse_offset(args.offset)
    except ValueError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 2
    do_home = args.home_only or args.home or (not args.no_home and args.gcode is not None)

    lines: list[str] = []
    if not args.home_only and not args.cmd:
        if not args.gcode:
            print("ERROR: gcode file required unless --home-only or --cmd is set", file=sys.stderr)
            return 2
        try:
            if args.preview_bounds:
                source_lines = load_lines(args.gcode, dry_run=False)
                lines = preview_bounds_lines(source_lines, args.preview_s, args.preview_feed, args.preview_margin, offset_x, offset_y)
            else:
                lines = load_lines(args.gcode, dry_run=args.dry_run)
                lines = apply_offset(lines, offset_x, offset_y)
        except ValueError as e:
            print(f"ERROR: {e}", file=sys.stderr)
            return 2
        if not lines:
            print("No G-code lines to send.", file=sys.stderr)
            return 2
        try:
            min_x, min_y, max_x, max_y = parse_xy_bounds(lines)
            bounds_err = bounds_out_of_workspace(min_x, min_y, max_x, max_y, args.bed_mm)
            if bounds_err:
                print(f"ERROR: {bounds_err}", file=sys.stderr)
                return 2
        except ValueError:
            # No XY motion in this file/preview; nothing to validate.
            pass

    fd = os.open(args.port, os.O_RDWR | os.O_NOCTTY | os.O_SYNC)
    try:
        configure_serial(fd, args.baud)

        # Wake/reset sequence for GRBL.
        os.write(fd, b"\r\n\r\n")
        time.sleep(args.startup_wait)
        termios.tcflush(fd, termios.TCIFLUSH)
        wait_for_ready(fd, 2.0)

        if args.unlock:
            send_and_wait_ok(fd, "$X", args.line_timeout)
        if do_home:
            send_and_wait_ok(fd, "$H", max(args.line_timeout, 60.0))
        if args.home_only:
            print("Home-only complete.")
            return 0
        if args.cmd:
            cmd = args.cmd.strip()
            if not cmd:
                print("ERROR: empty --cmd", file=sys.stderr)
                return 2
            os.write(fd, (cmd + "\n").encode("ascii", errors="ignore"))
            deadline = time.time() + max(args.line_timeout, 2.0)
            saw_output = False
            while time.time() < deadline:
                line = read_line(fd, 0.25)
                if line is None:
                    continue
                if not line:
                    continue
                print(f"<< {line}")
                saw_output = True
                low = line.lower()
                if low == "ok":
                    return 0
                if low.startswith("error") or low.startswith("alarm"):
                    raise RuntimeError(f"controller rejected command '{cmd}': {line}")
                if cmd == "?":
                    return 0
            if saw_output:
                return 0
            raise TimeoutError(f"timeout waiting for response to: {cmd}")

        total = len(lines)
        print(f"Sending {total} lines...")
        watchdog: LaserWatchdog | None
        if args.preview_bounds or args.dry_run:
            watchdog = None
        else:
            watchdog = LaserWatchdog(args.max_laser_on_seconds)
        if not args.preview_bounds and args.stream_buffer_bytes > 0:
            try:
                send_job_buffered(fd, lines, args.line_timeout, watchdog, args.stream_buffer_bytes)
            except Exception:
                if not args.dry_run:
                    best_effort_safe_stop(fd, args.line_timeout)
                raise
        else:
            for i, cmd in enumerate(lines, start=1):
                if watchdog is not None:
                    watchdog.pre_send(cmd)
                cmd_timeout = args.line_timeout
                is_preview_final_m5 = args.preview_bounds and i == total and cmd.strip().upper() == "M5"
                if is_preview_final_m5:
                    # Final preview M5 is queued after the entire perimeter motion.
                    # Use a long timeout here instead of status-poll loops/resets.
                    cmd_timeout = max(args.line_timeout, 120.0)
                try:
                    send_and_wait_ok(fd, cmd, cmd_timeout, watchdog=watchdog)
                except Exception as e:
                    if is_preview_final_m5:
                        print(f"WARN: timed out waiting for final preview M5 ack: {e}", file=sys.stderr)
                        print("Preview completed with non-fatal final M5 timeout (no forced reset in preview mode).")
                        return 0
                    if not args.dry_run:
                        best_effort_safe_stop(fd, args.line_timeout)
                    raise
                if i == 1 or i % 100 == 0 or i == total:
                    print(f"[{i}/{total}] {cmd}")
        print("Done.")
        return 0
    except KeyboardInterrupt:
        print("WARN: interrupted by user (Ctrl-C); attempting emergency laser stop", file=sys.stderr)
        try:
            best_effort_safe_stop(fd, args.line_timeout)
        except Exception:
            # Keep Ctrl-C path resilient even if serial link is unstable.
            pass
        return 130
    except Exception as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1
    finally:
        os.close(fd)


if __name__ == "__main__":
    raise SystemExit(main())
