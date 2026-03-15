#!/usr/bin/env python3
"""Minimal GRBL G-code sender with line-by-line handshake.

Usage:
  python3 scripts/send-gcode.py --port /dev/ttyUSB0 --baud 115200 file.gcode
  python3 scripts/send-gcode.py --cmd '$$'
"""

from __future__ import annotations

import argparse
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


def send_and_wait_ok(fd: int, cmd: str, timeout_s: float) -> None:
    os.write(fd, (cmd + "\n").encode("ascii", errors="ignore"))
    while True:
        line = read_line(fd, timeout_s)
        if line is None:
            raise TimeoutError(f"timeout waiting for response to: {cmd}")
        if not line:
            continue
        print(f"<< {line}")
        low = line.lower()
        if low == "ok":
            return
        if low.startswith("error") or low.startswith("alarm"):
            raise RuntimeError(f"controller rejected line '{cmd}': {line}")


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
    out: list[str] = []
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
    if dry_run:
        out.insert(0, "M5")
        out.append("M5")
    return out


def detect_default_port() -> str | None:
    candidates = []
    candidates.extend(sorted(glob.glob("/dev/ttyACM*")))
    candidates.extend(sorted(glob.glob("/dev/ttyUSB*")))
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
    ap.add_argument("--preview-s", type=int, default=20, help="laser power for --preview-bounds (default 20)")
    ap.add_argument("--preview-feed", type=float, default=600.0, help="feed rate for --preview-bounds trace (default 600)")
    ap.add_argument("--preview-margin", type=float, default=0.0, help="extra margin in mm around bounds for --preview-bounds")
    ap.add_argument("--offset", default="0,0", help="optional XY offset in mm applied at send time, format 'x,y' (for example '0,25')")
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
        if args.preview_bounds:
            source_lines = load_lines(args.gcode, dry_run=False)
            lines = preview_bounds_lines(source_lines, args.preview_s, args.preview_feed, args.preview_margin, offset_x, offset_y)
        else:
            lines = load_lines(args.gcode, dry_run=args.dry_run)
            lines = apply_offset(lines, offset_x, offset_y)
        if not lines:
            print("No G-code lines to send.", file=sys.stderr)
            return 2

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
        for i, cmd in enumerate(lines, start=1):
            try:
                send_and_wait_ok(fd, cmd, args.line_timeout)
            except Exception as e:
                is_preview_final_m5 = args.preview_bounds and i == total and cmd.strip().upper() == "M5"
                if is_preview_final_m5:
                    print(f"WARN: timed out waiting for final preview M5 ack: {e}", file=sys.stderr)
                    best_effort_safe_stop(fd, args.line_timeout)
                    print("Preview completed with non-fatal final M5 timeout.")
                    return 0
                # Burn mode must fail hard and attempt best-effort stop sequence.
                if not args.dry_run:
                    best_effort_safe_stop(fd, args.line_timeout)
                raise
            if i == 1 or i % 100 == 0 or i == total:
                print(f"[{i}/{total}] {cmd}")
        print("Done.")
        return 0
    except Exception as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1
    finally:
        os.close(fd)


if __name__ == "__main__":
    raise SystemExit(main())
