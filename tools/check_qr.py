"""\u628a Go \u7aef\u81ea\u5199 QR \u7f16\u7801\u5668\u7684\u77e9\u9635\u4e0e segno \u9010\u6a21\u5757\u6bd4\u5bf9\u3002

\u4fee\u6b63\u4e86 segno 1.6.6 \u7684\u4e00\u4e2a\u504f\u5dee\uff1aencoder.write_padding_bits \u7528 `8 - (length % 8)`\uff0c
\u5f53\u4f4d\u6d41\u5df2\u843d\u5728\u7801\u5b57\u8fb9\u754c\u65f6\u4f1a\u591a\u8865\u4e00\u4e2a 0x00 \u5b57\u8282\u3002ISO/IEC 18004 7.4.10 \u539f\u6587\u662f
"if the bit stream length is such that it does not end at a codeword boundary, padding bits ... shall be added"\uff0c
\u5373\u5df2\u5bf9\u9f50\u5c31\u4e0d\u8865\u3002Nayuki \u5b9e\u73b0\u7528 `(i + len) % 8 != 0` \u5faa\u73af\uff0c\u4e0e Go \u7aef\u4e00\u81f4\u3002
\u8fd9\u4e2a\u5dee\u5f02\u5bf9\u626b\u7801\u65e0\u5f71\u54cd\uff08\u89e3\u7801\u5668\u8bfb\u5b8c\u6570\u636e\u5c31\u505c\uff09\uff0c\u4f46\u4f1a\u8ba9\u77e9\u9635\u9010\u4f4d\u5bf9\u4e0d\u4e0a\uff0c\u6240\u4ee5\u5148\u628a segno \u8865\u9f50\u5230\u89c4\u8303\u884c\u4e3a\u3002
"""
import sys

import segno
import segno.encoder as se
from segno import consts

_orig_write_padding_bits = se.write_padding_bits


def _spec_write_padding_bits(buff, version, length):
    if version not in (consts.VERSION_M1, consts.VERSION_M3):
        buff.extend([0] * ((8 - (length % 8)) % 8))


se.write_padding_bits = _spec_write_padding_bits


def main(dump_path):
    lines = open(dump_path, encoding="utf-8").read().split("\n")
    while lines and lines[-1] == "":
        lines.pop()

    cases = []
    i = 0
    while i < len(lines):
        payload, ver, ecl, mask = lines[i].split("|")
        ver, mask = int(ver), int(mask)
        size = ver * 4 + 17
        rows = lines[i + 1:i + 1 + size]
        i += 1 + size
        cases.append((payload, ver, ecl, mask, rows))

    print("cases:", len(cases))
    bad = 0
    for payload, ver, ecl, mask, rows in cases:
        qr = segno.make_qr(payload, error=ecl, version=ver, mask=mask,
                           boost_error=False, mode="byte")
        ref = qr.matrix
        n = len(ref)
        if n != ver * 4 + 17:
            print("SIZE MISMATCH", ver, mask, n)
            bad += 1
            continue
        diffs = [(x, y) for y in range(n) for x in range(n)
                 if (rows[y][x] == "1") != bool(ref[y][x])]
        if diffs:
            bad += 1
        print("%s ver=%2d mask=%d %-44s diffs=%d" % (
            "OK " if not diffs else "BAD", ver, mask, payload[:44], len(diffs)))
        if diffs:
            print("     first 16:", diffs[:16])
    print("FAILED:", bad, "/", len(cases))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "qr_dump.txt"))