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
    do_home = args.home_only or args.home or (not args.no_home and args.gcode is not None)

    lines: list[str] = []
    if not args.home_only and not args.cmd:
        if not args.gcode:
            print("ERROR: gcode file required unless --home-only or --cmd is set", file=sys.stderr)
            return 2
        lines = load_lines(args.gcode, dry_run=args.dry_run)
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
            send_and_wait_ok(fd, cmd, args.line_timeout)
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
