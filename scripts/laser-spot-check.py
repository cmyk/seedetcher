#!/usr/bin/env python3
"""Interactive low-power spot-check walker for GRBL lasers.

Default path per step:
  (0,0) -> (150,0) -> (150,150) -> (0,150) -> (75,75) -> (0,0)

Press Enter to advance to next point. Ctrl+C (or 'q'+Enter) exits and sends M5.
"""

from __future__ import annotations

import argparse
import glob
import os
import select
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


def detect_default_port() -> str | None:
    by_id = sorted(glob.glob("/dev/serial/by-id/*"))
    if by_id:
        return by_id[0]
    candidates = []
    candidates.extend(sorted(glob.glob("/dev/ttyACM*")))
    candidates.extend(sorted(glob.glob("/dev/ttyUSB*")))
    candidates.extend(sorted(glob.glob("/dev/ttyAMA*")))
    candidates.extend(sorted(glob.glob("/dev/ttyS*")))
    return candidates[0] if candidates else None


def configure_serial(fd: int, baud: int) -> None:
    if baud not in BAUD_MAP:
        raise ValueError(f"unsupported baud: {baud}")
    attrs = termios.tcgetattr(fd)
    attrs[0] = 0
    attrs[1] = 0
    attrs[2] = termios.CREAD | termios.CLOCAL | termios.CS8
    attrs[3] = 0
    attrs[4] = BAUD_MAP[baud]
    attrs[5] = BAUD_MAP[baud]
    attrs[6][termios.VMIN] = 0
    attrs[6][termios.VTIME] = 1
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


def send_realtime(fd: int, byte_value: bytes) -> None:
    if len(byte_value) != 1:
        raise ValueError("realtime command must be exactly 1 byte")
    os.write(fd, byte_value)


def clamp(v: float, lo: float, hi: float) -> float:
    if v < lo:
        return lo
    if v > hi:
        return hi
    return v


def nudge_point(x: float, y: float, bed: float, nudge_mm: float) -> tuple[float, float]:
    if nudge_mm <= 0:
        return x, y
    if x+nudge_mm <= bed:
        return x + nudge_mm, y
    if x-nudge_mm >= 0:
        return x - nudge_mm, y
    if y+nudge_mm <= bed:
        return x, y + nudge_mm
    if y-nudge_mm >= 0:
        return x, y - nudge_mm
    return x, y


def parse_point(spec: str) -> tuple[float, float]:
    parts = [p.strip() for p in spec.split(",", 1)]
    if len(parts) != 2:
        raise ValueError(f"invalid point '{spec}', expected x,y")
    return float(parts[0]), float(parts[1])


def parse_points(spec: str) -> list[tuple[float, float]]:
    out: list[tuple[float, float]] = []
    for raw in spec.split(";"):
        s = raw.strip()
        if not s:
            continue
        out.append(parse_point(s))
    if not out:
        raise ValueError("empty points list")
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description="Interactive low-power waypoint spot-check")
    ap.add_argument("--port", default=detect_default_port(), help="serial port (default autodetect)")
    ap.add_argument("--baud", type=int, default=115200, help="serial baud (default 115200)")
    ap.add_argument("--line-timeout", type=float, default=10.0, help="timeout per command")
    ap.add_argument("--startup-wait", type=float, default=2.0, help="seconds to wait after wake/reset")
    ap.add_argument("--unlock", action="store_true", help="send $X before setup")
    ap.add_argument("--home", action="store_true", help="send $H before starting loop")
    ap.add_argument("--power", type=int, default=50, help="beam power S value (default 50)")
    ap.add_argument("--mode", choices=("m3", "m4"), default="m3", help="laser mode command (default m3)")
    ap.add_argument("--move-feed", type=float, default=600.0, help="move feed mm/min (default 600)")
    ap.add_argument("--nudge-feed", type=float, default=120.0, help="nudge feed mm/min (default 120)")
    ap.add_argument("--nudge-mm", type=float, default=0.05, help="tiny post-move nudge for motion-gated beam (default 0.05)")
    ap.add_argument("--nudge-return", action="store_true", help="extra post-hit dither (nudge away and back after the final hit)")
    ap.add_argument("--bed-mm", type=float, default=150.0, help="workspace max X/Y for clamping (default 150)")
    ap.add_argument(
        "--points",
        default="0,0;150,0;150,150;0,150;75,75;0,0",
        help="semicolon-separated waypoint list x,y;... (default standard corner/center loop)",
    )
    args = ap.parse_args()

    if not args.port:
        print("ERROR: no serial port found; pass --port explicitly")
        return 2
    if args.power <= 0:
        print("ERROR: --power must be > 0")
        return 2
    if args.move_feed <= 0 or args.nudge_feed <= 0:
        print("ERROR: feeds must be > 0")
        return 2
    if args.bed_mm <= 0:
        print("ERROR: --bed-mm must be > 0")
        return 2

    try:
        points = parse_points(args.points)
    except Exception as e:
        print(f"ERROR: {e}")
        return 2

    clamped: list[tuple[float, float]] = []
    for x, y in points:
        cx = clamp(x, 0.0, args.bed_mm)
        cy = clamp(y, 0.0, args.bed_mm)
        clamped.append((cx, cy))
    points = clamped

    fd = os.open(args.port, os.O_RDWR | os.O_NOCTTY | os.O_SYNC)
    try:
        configure_serial(fd, args.baud)

        os.write(fd, b"\r\n\r\n")
        time.sleep(args.startup_wait)
        termios.tcflush(fd, termios.TCIFLUSH)
        wait_for_ready(fd, 2.0)

        if args.unlock:
            send_and_wait_ok(fd, "$X", args.line_timeout)
        if args.home:
            send_and_wait_ok(fd, "$H", max(args.line_timeout, 60.0))

        send_and_wait_ok(fd, "G21", args.line_timeout)
        send_and_wait_ok(fd, "G90", args.line_timeout)
        send_and_wait_ok(fd, f"G1 F{args.move_feed:.1f}", args.line_timeout)
        # Some K1 firmware states remain in Hold after homing/door events until resume.
        send_realtime(fd, b"~")
        time.sleep(0.05)

        print("")
        print("Enter: next point | q + Enter: quit | Ctrl+C: quit")
        print(f"Beam mode/power: {args.mode.upper()} S{args.power}")
        print(f"Nudge: {args.nudge_mm:.3f}mm @F{args.nudge_feed:.1f} post-dither={'on' if args.nudge_return else 'off'}")
        print(f"Loop points: {', '.join(f'({x:.3f},{y:.3f})' for x, y in points)}")
        print("")

        idx = 0
        while True:
            raw = input(f"[{idx + 1}/{len(points)}] Press Enter to move > ").strip().lower()
            if raw in {"q", "quit", "exit"}:
                break

            x, y = points[idx]
            send_realtime(fd, b"~")
            time.sleep(0.02)
            send_and_wait_ok(fd, f"{args.mode.upper()} S{args.power}", args.line_timeout)

            nx, ny = nudge_point(x, y, args.bed_mm, args.nudge_mm)
            if nx != x or ny != y:
                send_and_wait_ok(fd, f"G1 F{args.nudge_feed:.1f}", args.line_timeout)
                send_and_wait_ok(fd, f"G1 X{nx:.3f} Y{ny:.3f}", args.line_timeout)
            # Always finish exactly on the requested waypoint.
            send_and_wait_ok(fd, f"G1 F{args.move_feed:.1f}", args.line_timeout)
            send_and_wait_ok(fd, f"G1 X{x:.3f} Y{y:.3f}", args.line_timeout)
            if args.nudge_return and (nx != x or ny != y):
                send_and_wait_ok(fd, f"G1 F{args.nudge_feed:.1f}", args.line_timeout)
                send_and_wait_ok(fd, f"G1 X{nx:.3f} Y{ny:.3f}", args.line_timeout)
                send_and_wait_ok(fd, f"G1 X{x:.3f} Y{y:.3f}", args.line_timeout)
                send_and_wait_ok(fd, f"G1 F{args.move_feed:.1f}", args.line_timeout)

            idx = (idx + 1) % len(points)

        send_and_wait_ok(fd, "M5", args.line_timeout)
        print("Done.")
        return 0
    except KeyboardInterrupt:
        try:
            send_and_wait_ok(fd, "M5", max(1.0, args.line_timeout))
        except Exception:
            pass
        print("")
        print("Stopped.")
        return 130
    except Exception as e:
        try:
            send_and_wait_ok(fd, "M5", max(1.0, args.line_timeout))
        except Exception:
            pass
        print(f"ERROR: {e}")
        return 1
    finally:
        os.close(fd)


if __name__ == "__main__":
    raise SystemExit(main())
