using System;
using System.Runtime.InteropServices;
[StructLayout(LayoutKind.Sequential)] public struct MOUSEINPUT { public int dx; public int dy; public uint mouseData; public uint dwFlags; public uint time; public IntPtr dwExtraInfo; }
[StructLayout(LayoutKind.Sequential)] public struct KEYBDINPUT { public ushort wVk; public ushort wScan; public uint dwFlags; public uint time; public IntPtr dwExtraInfo; }
[StructLayout(LayoutKind.Explicit)] public struct INPUT { [FieldOffset(0)] public uint type; [FieldOffset(8)] public MOUSEINPUT mi; [FieldOffset(8)] public KEYBDINPUT ki; }
[StructLayout(LayoutKind.Sequential)] public struct POINT { public int X; public int Y; }
public class Probe {
  [DllImport("user32.dll", SetLastError=true)] static extern uint SendInput(uint n, INPUT[] p, int cb);
  [DllImport("user32.dll", SetLastError=true)] static extern bool GetCursorPos(out POINT p);
  [DllImport("user32.dll", SetLastError=true)] static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll", SetLastError=true)] static extern IntPtr OpenInputDesktop(uint flags, bool inherit, uint access);
  [DllImport("user32.dll", SetLastError=true)] static extern bool SetThreadDesktop(IntPtr h);
  static INPUT Mouse(int dx,int dy){ INPUT i=new INPUT(); i.type=0; i.mi.dx=dx; i.mi.dy=dy; i.mi.dwFlags=0x0001; return i; }
  static INPUT Shift(bool up){ INPUT i=new INPUT(); i.type=1; i.ki.wVk=0x10; i.ki.dwFlags=(uint)(up?2:0); return i; }
  public static void Main(){
    Console.WriteLine("Marshal.SizeOf: INPUT={0} MOUSEINPUT={1} KEYBDINPUT={2}", Marshal.SizeOf(typeof(INPUT)), Marshal.SizeOf(typeof(MOUSEINPUT)), Marshal.SizeOf(typeof(KEYBDINPUT)));
    POINT b;
    if (!GetCursorPos(out b)) {
      Console.WriteLine("GetCursorPos err={0} -> attach to input desktop", Marshal.GetLastWin32Error());
      IntPtr h = OpenInputDesktop(0, false, 0x000F01FF);
      Console.WriteLine("OpenInputDesktop={0} SetThreadDesktop={1} err={2}", h, SetThreadDesktop(h), Marshal.GetLastWin32Error());
    }
    bool ok = GetCursorPos(out b);
    Console.WriteLine("1) GetCursorPos ok={0} pos=({1},{2}) err={3}", ok, b.X, b.Y, Marshal.GetLastWin32Error());
    uint n = SendInput(1, new[]{Mouse(30,0)}, Marshal.SizeOf(typeof(INPUT)));
    Console.WriteLine("2) SendInput +30 cb=40 ret={0} err={1}", n, Marshal.GetLastWin32Error());
    POINT m; GetCursorPos(out m); Console.WriteLine("   pos=({0},{1}) observed dx={2}", m.X, m.Y, m.X-b.X);
    n = SendInput(1, new[]{Mouse(-30,0)}, Marshal.SizeOf(typeof(INPUT)));
    Console.WriteLine("3) SendInput -30 cb=40 ret={0} err={1}", n, Marshal.GetLastWin32Error());
    POINT a; ok = GetCursorPos(out a);
    Console.WriteLine("4) pos=({0},{1}) restored={2}", a.X, a.Y, a.X==b.X&&a.Y==b.Y);
    if (a.X!=b.X||a.Y!=b.Y) { SetCursorPos(b.X,b.Y); GetCursorPos(out a); Console.WriteLine("   SetCursorPos restore -> ({0},{1})", a.X, a.Y); }
    uint n2 = SendInput(1, new[]{Shift(false)}, Marshal.SizeOf(typeof(INPUT)));
    uint n3 = SendInput(1, new[]{Shift(true)}, Marshal.SizeOf(typeof(INPUT)));
    Console.WriteLine("5) VK_SHIFT down/up ret={0}/{1} err={2}", n2, n3, Marshal.GetLastWin32Error());
  }
}
