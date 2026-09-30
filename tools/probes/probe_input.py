# -*- coding: utf-8 -*-
"""probe_input.py - 无管理员权限驱动鼠标/键盘的最小验证 (ctypes 标准库, 无第三方依赖)
仅使用 SendInput: 鼠标 MOUSEEVENTF_MOVE (+30 / -30 px), 键盘 VK_SHIFT 单纯修饰键 down/up。"""
import ctypes
from ctypes import wintypes

u32 = ctypes.WinDLL("user32", use_last_error=True)
k32 = ctypes.WinDLL("kernel32", use_last_error=True)
INPUT_MOUSE, INPUT_KEYBOARD = 0, 1
MOUSEEVENTF_MOVE = 0x0001
KEYEVENTF_KEYUP = 0x0002
VK_SHIFT = 0x10
DESKTOP_GENERIC_ALL = 0x000F01FF

class MOUSEINPUT(ctypes.Structure):   # x64: 32B (含 time)
    _fields_ = [("dx", wintypes.LONG), ("dy", wintypes.LONG),
                ("mouseData", wintypes.DWORD), ("dwFlags", wintypes.DWORD),
                ("time", wintypes.DWORD), ("dwExtraInfo", ctypes.c_void_p)]

class KEYBDINPUT(ctypes.Structure):   # x64: 24B
    _fields_ = [("wVk", wintypes.WORD), ("wScan", wintypes.WORD),
                ("dwFlags", wintypes.DWORD), ("time", wintypes.DWORD),
                ("dwExtraInfo", ctypes.c_void_p)]

class _U(ctypes.Union):
    _fields_ = [("mi", MOUSEINPUT), ("ki", KEYBDINPUT)]

class INPUT(ctypes.Structure):        # x64: 40B, cbSize 必须传 40
    _fields_ = [("type", wintypes.DWORD), ("u", _U)]

def desk_name(h):
    buf = ctypes.create_unicode_buffer(256)
    u32.GetUserObjectInformationW(h, 2, buf, ctypes.sizeof(buf), None)
    return buf.value

def ensure_input_desktop():
    tid = k32.GetCurrentThreadId()
    cur = u32.GetThreadDesktop(tid)
    ind = u32.OpenInputDesktop(0, False, DESKTOP_GENERIC_ALL)
    if not ind:
        return "OpenInputDesktop failed err=%d" % ctypes.get_last_error()
    if desk_name(cur) == desk_name(ind):
        return "desktop %s (== input desktop, no attach needed)" % desk_name(ind)
    if not u32.SetThreadDesktop(ind):
        return "SetThreadDesktop failed err=%d" % ctypes.get_last_error()
    return "desktop %s -> attached to input desktop %s" % (desk_name(cur), desk_name(ind))

def get_pos():
    pt = wintypes.POINT(); ctypes.set_last_error(0)
    ok = u32.GetCursorPos(ctypes.byref(pt))
    return ok, (pt.x, pt.y), ctypes.get_last_error()

def send(*inputs):
    arr = (INPUT * len(inputs))(*inputs); ctypes.set_last_error(0)
    n = u32.SendInput(len(inputs), arr, ctypes.sizeof(INPUT))
    return n, ctypes.get_last_error()

def move(dx, dy):
    return send(INPUT(type=INPUT_MOUSE, u=_U(mi=MOUSEINPUT(dx=dx, dy=dy, mouseData=0,
                 dwFlags=MOUSEEVENTF_MOVE, time=0, dwExtraInfo=None))))

def shift(up):
    return send(INPUT(type=INPUT_KEYBOARD, u=_U(ki=KEYBDINPUT(
        wVk=VK_SHIFT, wScan=0, dwFlags=KEYEVENTF_KEYUP if up else 0, time=0, dwExtraInfo=None))))

if __name__ == "__main__":
    print("sizeof INPUT=%d MOUSEINPUT=%d KEYBDINPUT=%d | admin=%s | session=%d" % (
        ctypes.sizeof(INPUT), ctypes.sizeof(MOUSEINPUT), ctypes.sizeof(KEYBDINPUT),
        bool(ctypes.windll.shell32.IsUserAnAdmin()), k32.WTSGetActiveConsoleSessionId()))
    print("desktop:", ensure_input_desktop())

    ok0, before, e0 = get_pos()
    print("1) GetCursorPos     -> ok=%s pos=%s GetLastError=%d" % (ok0, before, e0))

    n1, e1 = move(30, 0)
    mid = get_pos()[1]
    print("2) SendInput MOVE +30 -> ret=%d GetLastError=%d | pos=%s 实测 dx=%d" % (n1, e1, mid, mid[0]-before[0]))

    n2, e2 = move(-30, 0)
    ok3, after, e3 = get_pos()
    print("3) SendInput MOVE -30 -> ret=%d GetLastError=%d" % (n2, e2))
    print("4) GetCursorPos     -> ok=%s pos=%s GetLastError=%d | 对称移动后 delta=%s"
          % (ok3, after, e3, (after[0]-before[0], after[1]-before[1])))
    if after != before:
        ctypes.set_last_error(0)
        if u32.SetCursorPos(before[0], before[1]):
            after = get_pos()[1]
            print("   SetCursorPos 精确还原 -> pos=%s GetLastError=%d" % (after, ctypes.get_last_error()))
    print("   RESTORED =", after == before)

    n4, e4 = shift(False); n5, e5 = shift(True)
    print("5) SendInput VK_SHIFT down/up -> ret=%d/%d GetLastError=%d/%d" % (n4, n5, e4, e5))
