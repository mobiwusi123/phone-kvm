# -*- coding: utf-8 -*-
"""只读诊断: 当前线程/进程所在的桌面 vs 输入桌面, 并尝试附着(不做任何注入)"""
import ctypes
from ctypes import wintypes
u32 = ctypes.WinDLL("user32", use_last_error=True)
k32 = ctypes.WinDLL("kernel32", use_last_error=True)
GENERIC_ALL_DESKTOP = 0x000F01FF

def name_of(h):
    buf = ctypes.create_unicode_buffer(256); n = wintypes.DWORD()
    u32.GetUserObjectInformationW(h, 2, buf, ctypes.sizeof(buf), ctypes.byref(n))
    return buf.value

def last():  return ctypes.get_last_error()

own = u32.GetThreadDesktop(k32.GetCurrentThreadId())
print("thread desktop :", name_of(own))
ctypes.set_last_error(0)
h = u32.OpenInputDesktop(0, False, GENERIC_ALL_DESKTOP)
print("OpenInputDesktop(GENERIC_ALL) ->", hex(h or 0), "err", last())
if h:
    print("input desktop  :", name_of(h))
    ctypes.set_last_error(0)
    ok = u32.SetThreadDesktop(h)
    print("SetThreadDesktop ->", bool(ok), "err", last())
    if ok:
        print("attached! now on:", name_of(u32.GetThreadDesktop(k32.GetCurrentThreadId())))
        pt = wintypes.POINT(); ctypes.set_last_error(0)
        r = u32.GetCursorPos(ctypes.byref(pt))
        print("GetCursorPos ->", bool(r), (pt.x, pt.y), "err", last())
        print("GetForegroundWindow ->", hex(u32.GetForegroundWindow() or 0))
    else:
        print("=> 不可附着(令牌被沙箱限制)")
else:
    print("=> 连输入桌面句柄都拿不到")
