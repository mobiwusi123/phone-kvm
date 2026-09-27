# -*- coding: utf-8 -*-
"""沙箱适配层: 把当前线程附到输入桌面(Default)后, 原样运行 probe_input.py 的 __main__"""
import ctypes, runpy, sys
u32 = ctypes.WinDLL("user32", use_last_error=True)
DESKTOP_GENERIC_ALL = 0x000F01FF
h = u32.OpenInputDesktop(0, False, DESKTOP_GENERIC_ALL)
if not h:
    sys.exit("OpenInputDesktop failed err=%d" % ctypes.get_last_error())
if not u32.SetThreadDesktop(h):
    sys.exit("SetThreadDesktop failed err=%d" % ctypes.get_last_error())
print("[wrapper] attached to input desktop (Default)\n")
runpy.run_path(sys.argv[1], run_name="__main__")
