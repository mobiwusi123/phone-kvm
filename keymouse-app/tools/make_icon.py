"""
生成 App 图标：蓝青渐变底 + 白色鼠标轮廓。
纯标准库实现（zlib + struct 手写 PNG），不依赖 Pillow。
用法：python make_icon.py
输出：../static/logo.png （1024x1024）
"""
import math
import os
import struct
import zlib

W = 1024

# 形状参数（1024 坐标系）
MOUSE_CX, MOUSE_CY = 512.0, 512.0
MOUSE_HW, MOUSE_HH, MOUSE_R = 180.0, 270.0, 180.0

SPLIT_CX, SPLIT_CY = 512.0, 380.0
SPLIT_HW, SPLIT_HH, SPLIT_R = 4.0, 120.0, 4.0

WHEEL_CX, WHEEL_CY = 512.0, 398.0
WHEEL_HW, WHEEL_HH, WHEEL_R = 32.0, 60.0, 32.0

WHITE = (255.0, 255.0, 255.0)
DEEP = (22.0, 84.0, 165.0)

BG_FROM = (47.0, 128.0, 237.0)
BG_TO = (86.0, 204.0, 242.0)


def sd_round_rect(px, py, cx, cy, hw, hh, r):
    """圆角矩形的带符号距离场：<0 在内部，>0 在外部"""
    qx = abs(px - cx) - (hw - r)
    qy = abs(py - cy) - (hh - r)
    ax = qx if qx > 0.0 else 0.0
    ay = qy if qy > 0.0 else 0.0
    outside = math.sqrt(ax * ax + ay * ay)
    inside = qx if qx > qy else qy
    if inside > 0.0:
        inside = 0.0
    return outside + inside - r


def coverage(d):
    """把距离转成 0..1 的覆盖度，做边缘抗锯齿"""
    a = 0.5 - d
    if a <= 0.0:
        return 0.0
    if a >= 1.0:
        return 1.0
    return a


def mix(dst, src, a):
    return (
        dst[0] + (src[0] - dst[0]) * a,
        dst[1] + (src[1] - dst[1]) * a,
        dst[2] + (src[2] - dst[2]) * a,
    )


def build_rows():
    rows = []
    inv = 1.0 / (2.0 * W)
    for y in range(W):
        py = y + 0.5
        row = bytearray()
        row.append(0)  # PNG 行过滤器：None
        for x in range(W):
            px = x + 0.5
            t = (px + py) * inv
            col = (
                BG_FROM[0] + (BG_TO[0] - BG_FROM[0]) * t,
                BG_FROM[1] + (BG_TO[1] - BG_FROM[1]) * t,
                BG_FROM[2] + (BG_TO[2] - BG_FROM[2]) * t,
            )
            a = coverage(sd_round_rect(px, py, MOUSE_CX, MOUSE_CY, MOUSE_HW, MOUSE_HH, MOUSE_R))
            if a > 0.0:
                col = mix(col, WHITE, a)
            a = coverage(sd_round_rect(px, py, SPLIT_CX, SPLIT_CY, SPLIT_HW, SPLIT_HH, SPLIT_R))
            if a > 0.0:
                col = mix(col, DEEP, a)
            a = coverage(sd_round_rect(px, py, WHEEL_CX, WHEEL_CY, WHEEL_HW, WHEEL_HH, WHEEL_R))
            if a > 0.0:
                col = mix(col, DEEP, a)
            row.append(int(col[0] + 0.5))
            row.append(int(col[1] + 0.5))
            row.append(int(col[2] + 0.5))
        rows.append(bytes(row))
    return rows


def chunk(tag, data):
    out = struct.pack(">I", len(data)) + tag + data
    crc = zlib.crc32(tag + data) & 0xFFFFFFFF
    return out + struct.pack(">I", crc)


def main():
    rows = build_rows()
    raw = b"".join(rows)
    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", struct.pack(">IIBBBBB", W, W, 8, 2, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(raw, 9))
    png += chunk(b"IEND", b"")

    here = os.path.dirname(os.path.abspath(__file__))
    out = os.path.join(here, "..", "static", "logo.png")
    out = os.path.normpath(out)
    with open(out, "wb") as f:
        f.write(png)
    print("saved", out, len(png), "bytes")


if __name__ == "__main__":
    main()
