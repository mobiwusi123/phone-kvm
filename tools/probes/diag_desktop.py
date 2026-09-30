# -*- coding: utf-8 -*-
import ctypes
from ctypes import wintypes
u32 = ctypes.WinDLL("user32", use_last_error=True)
k32 = ctypes.WinDLL("kernel32", use_last_error=True)
UOI_NAME = 2
def name_of(h):
    buf = ctypes.create_unicode_buffer(256); n = wintypes.DWORD()
    ok = u32.GetUserObjectInformationW(h, UOI_NAME, buf, ctypes.sizeof(buf), ctypes.byref(n))
    return (ok, buf.value, ctypes.get_last_error())
def probe():
    ws = u32.GetProcessWindowStation(); e_ws = ctypes.get_last_error()
    d = u32.GetThreadDesktop(k32.GetCurrentThreadId()); e_d = ctypes.get_last_error()
    hd = u32.OpenInputDesktop(0, False, 0x0100); e_hd = ctypes.get_last_error()
    out = {}
    out["winsta_handle"] = hex(ws or 0); out["winsta_err"] = e_ws
    out["winsta_name"] = name_of(ws)
    out["desktop_handle"] = hex(d or 0); out["desktop_err"] = e_d
    out["desktop_name"] = name_of(d)
    out["input_desktop_handle"] = hex(hd or 0); out["input_desktop_err"] = e_hd
    out["input_desktop_name"] = name_of(hd) if hd else None
    pt = wintypes.POINT(); ctypes.set_last_error(0)
    ok = u32.GetCursorPos(ctypes.byref(pt))
    out["GetCursorPos"] = (ok, (pt.x, pt.y), ctypes.get_last_error())
    ctypes.set_last_error(0)
    out["GetSystemMetrics_SM_CXSCREEN"] = (u32.GetSystemMetrics(0), ctypes.get_last_error())
    ctypes.set_last_error(0)
    out["GetForegroundWindow"] = (hex(u32.GetForegroundWindow() or 0), ctypes.get_last_error())
    return out
if __name__ == "__main__":
    for k, v in probe().items():
        print("%-28s = %s" % (k, v))
