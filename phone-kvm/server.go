//go:build windows

package main

import (
	"embed"
	"encoding/binary"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed www
var embeddedWeb embed.FS

var webFS, _ = fs.Sub(embeddedWeb, "www")

// 手机 -> 电脑
const (
	cMove   = 0x01 // int16 dx, int16 dy
	cButton = 0x02 // uint8 键位(1左 2右 3中 4后退 5前进), uint8 1按下 0抬起
	cWheel  = 0x03 // int16 垂直, int16 水平（单位 1/120 格）
	cKey    = 0x04 // uint16 虚拟键码, uint8 1按下 0抬起
	cText   = 0x05 // uint16 字节数 + UTF-8
	cPing   = 0x06 // uint32 序号，原样回 sPong
	cCenter = 0x08 // 无载荷：光标回主屏中央
	cQuery  = 0x09 // 无载荷：查询光标 -> sCursor
)

// 电脑 -> 手机
const (
	sPong   = 0x07 // 原样回 cPing 的 4 字节
	sCursor = 0x0A // int32 x, int32 y
)

const maxFrame = 1 << 20

type server struct {
	inj      *injector
	token    string
	sens     float64
	allowVPN bool
	vpnNets  []net.IPNet

	mu      sync.Mutex
	clients map[*wsConn]string
}

func newServer(inj *injector, token string, sens float64, allowVPN bool) *server {
	return &server{
		inj:      inj,
		token:    token,
		sens:     sens,
		allowVPN: allowVPN,
		vpnNets:  vpnNetworks(),
		clients:  make(map[*wsConn]string),
	}
}

func (srv *server) mux() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("/", srv.handleHTTP)
	return m
}

func (srv *server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		writePage(w, http.StatusForbidden, "缺少密钥", "请打开控制台里打印的完整网址（结尾带一串密钥）。")
		return
	}
	// 少了末尾的斜杠，页面里的相对路径（style.css / app.js）会解析到错误的位置
	if r.URL.Path == "/"+srv.token {
		http.Redirect(w, r, "/"+srv.token+"/", http.StatusMovedPermanently)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/"+srv.token)
	if !ok {
		writePage(w, http.StatusForbidden, "密钥不正确", "网址里的密钥无效，请用控制台里打印的那个网址。")
		return
	}
	if rest == "/ws" {
		srv.handleWS(w, r)
		return
	}
	name := strings.TrimPrefix(rest, "/")
	if name == "" {
		name = "index.html"
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFileFS(w, r, webFS, name)
}

func (srv *server) handleWS(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	if !srv.allowed(ip) {
		logf("拒绝来自 %v 的连接（不在允许范围内）", ip)
		writePage(w, http.StatusForbidden, "来源被拒绝", "这台电脑只接受局域网内的连接。")
		return
	}
	ws, err := wsUpgrade(w, r)
	if err != nil {
		logf("WebSocket 握手失败: %v", err)
		return
	}
	srv.addClient(ws, ip)
	defer func() {
		srv.removeClient(ws)
		ws.Close()
	}()
	logf("手机已连接: %v", ip)

	x, y, _ := srv.inj.Cursor(2 * time.Second)
	_ = ws.WriteText(fmt.Sprintf(`{"t":"hello","cursor":[%d,%d],"sens":%g}`, x, y, srv.sens))

	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if err := ws.Ping(); err != nil {
					ws.Close() // 让读循环立刻醒来
					return
				}
			}
		}
	}()

	for {
		op, data, err := ws.readMessage(maxFrame)
		if err != nil {
			break
		}
		if op != opText && op != opBinary {
			continue
		}
		srv.dispatch(ws, data)
	}
	logf("手机已断开: %v", ip)
}

// dispatch 解析手机发来的事件流。一条 WebSocket 帧里可以拼多个消息，
// 这样每帧只付一次 WebSocket 头开销。
func (srv *server) dispatch(ws *wsConn, b []byte) {
	for len(b) > 0 {
		switch b[0] {
		case cMove:
			if len(b) < 5 {
				return
			}
			srv.inj.Move(float64(int16(le16(b[1:]))), float64(int16(le16(b[3:]))))
			b = b[5:]
		case cButton:
			if len(b) < 3 {
				return
			}
			srv.inj.Button(b[1], b[2] != 0)
			b = b[3:]
		case cWheel:
			if len(b) < 5 {
				return
			}
			srv.inj.Wheel(int32(int16(le16(b[1:]))), int32(int16(le16(b[3:]))))
			b = b[5:]
		case cKey:
			if len(b) < 4 {
				return
			}
			srv.inj.Key(le16(b[1:]), b[3] != 0)
			b = b[4:]
		case cText:
			if len(b) < 3 {
				return
			}
			n := int(le16(b[1:]))
			if len(b) < 3+n {
				return
			}
			srv.inj.Text(string(b[3 : 3+n]))
			b = b[3+n:]
		case cCenter:
			srv.inj.Center()
			b = b[1:]
		case cQuery:
			if x, y, ok := srv.inj.Cursor(time.Second); ok {
				out := make([]byte, 9)
				out[0] = sCursor
				binary.LittleEndian.PutUint32(out[1:5], uint32(x))
				binary.LittleEndian.PutUint32(out[5:9], uint32(y))
				if err := ws.WriteBinary(out); err != nil {
					return
				}
			}
			b = b[1:]
		case cPing:
			if len(b) < 5 {
				return
			}
			out := make([]byte, 5)
			out[0] = sPong
			copy(out[1:], b[1:5])
			if err := ws.WriteBinary(out); err != nil {
				return
			}
			b = b[5:]
		default:
			return // 未知消息：丢掉这一帧的剩余部分
		}
	}
}

func le16(b []byte) uint16 { return binary.LittleEndian.Uint16(b) }

func (srv *server) addClient(ws *wsConn, ip net.IP) {
	srv.mu.Lock()
	srv.clients[ws] = ip.String()
	srv.mu.Unlock()
}

func (srv *server) removeClient(ws *wsConn) {
	srv.mu.Lock()
	delete(srv.clients, ws)
	srv.mu.Unlock()
}

func (srv *server) broadcast(msg string) {
	srv.mu.Lock()
	list := make([]*wsConn, 0, len(srv.clients))
	for c := range srv.clients {
		list = append(list, c)
	}
	srv.mu.Unlock()
	for _, c := range list {
		_ = c.WriteText(msg)
	}
}

// allowed 是"只接受局域网来源"的实现：VPN 网段默认拒绝，其余只放行私有地址。
func (srv *server) allowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, n := range srv.vpnNets {
		if n.Contains(ip) {
			return srv.allowVPN
		}
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

func writePage(w http.ResponseWriter, code int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8">`+
		`<meta name="viewport" content="width=device-width,initial-scale=1">`+
		`<title>%s</title><body style="font:16px/1.7 system-ui,sans-serif;padding:2rem;background:#12141a;color:#e8eaf0">`+
		`<h2 style="margin:0 0 .6rem">%s</h2><p style="color:#9aa3b2">%s</p>`,
		html.EscapeString(title), html.EscapeString(title), html.EscapeString(body))
}

func isVPNName(name string) bool {
	n := strings.ToLower(name)
	for _, k := range []string{"radmin", "vpn", "tailscale", "zerotier", "wireguard", "openvpn", "hamachi", "easyconnect"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// vpnNetworks 找出本机的 VPN 网段，外加 Radmin VPN 的默认网段兜底。
func vpnNetworks() []net.IPNet {
	out := []net.IPNet{{IP: net.IPv4(26, 0, 0, 0), Mask: net.CIDRMask(8, 32)}}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if !isVPNName(ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if v4 := ipn.IP.To4(); v4 != nil {
				out = append(out, net.IPNet{IP: v4.Mask(ipn.Mask), Mask: ipn.Mask})
			}
		}
	}
	return out
}

type localAddr struct {
	iface string
	ip    net.IP
	vpn   bool
}

func localAddresses() []localAddr {
	var out []localAddr
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if v4 := ipn.IP.To4(); v4 != nil {
				out = append(out, localAddr{iface: ifc.Name, ip: v4, vpn: isVPNName(ifc.Name)})
			}
		}
	}
	return out
}

// isPrivateIP 判断 RFC1918 私有地址。169.254 之类的链路本地地址手机连不上，不算私有。
func isPrivateIP(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 10:
		return true
	case v4[0] == 172 && v4[1]&0xF0 == 16:
		return true
	case v4[0] == 192 && v4[1] == 168:
		return true
	}
	return false
}

// isHotspotName 判断“本地连接* N / Local Area Connection* N”这类 Wi-Fi Direct
// 虚拟网卡，也就是系统“移动热点”用的那块网卡（默认地址 192.168.137.1）。
func isHotspotName(name string) bool {
	n := strings.ToLower(name)
	if strings.Contains(n, "wi-fi direct") || strings.Contains(n, "wifi direct") {
		return true
	}
	if !strings.Contains(n, "*") {
		return false
	}
	return strings.Contains(n, "本地连接") || strings.Contains(n, "local area connection")
}

// pickHotspotAddr 找电脑自己开的热点地址（返回下标，没有则 -1）。
// 手机连电脑热点和手机连同一个 Wi-Fi 是两条不同的路，所以热点单独画一张码。
func pickHotspotAddr(addrs []localAddr) int {
	for i := range addrs {
		if !addrs[i].vpn && isHotspotName(addrs[i].iface) {
			return i
		}
	}
	return -1
}

// pickMainAddr 挑一条最可能被手机访问到的地址（返回下标，挑不出来返回 -1），
// 用来生成二维码。顺序：非 VPN 的普通私有地址 > 热点虚拟网卡 > 其它非 VPN 地址 >
// 最后才轮到 VPN（仅当 -allow-vpn 打开）。
func pickMainAddr(addrs []localAddr, allowVPN bool) int {
	for _, hotspot := range []bool{false, true} {
		for i := range addrs {
			a := &addrs[i]
			if a.vpn || isHotspotName(a.iface) != hotspot || !isPrivateIP(a.ip) {
				continue
			}
			return i
		}
	}
	for i := range addrs {
		if !addrs[i].vpn {
			return i
		}
	}
	if allowVPN {
		for i := range addrs {
			return i
		}
	}
	return -1
}

// displayWidth 估算字符串在终端里占几列：中日韩全角字符算两列。
// 网卡名里有中文（“本地连接* 10”），不按列数对齐会挤歪。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r >= 0x1100 && r <= 0x115F,
			r >= 0x2E80 && r <= 0xA4CF,
			r >= 0xAC00 && r <= 0xD7A3,
			r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFE30 && r <= 0xFE6F,
			r >= 0xFF00 && r <= 0xFF60,
			r >= 0xFFE0 && r <= 0xFFE6:
			w += 2
		default:
			w++
		}
	}
	return w
}

func padRight(s string, width int) string {
	if pad := width - displayWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func intIn(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func (srv *server) report(port int) {
	fmt.Println()
	fmt.Println("================================================================")
	fmt.Println(" 手机虚拟键鼠 · 已就绪")
	fmt.Println("================================================================")
	addrs := localAddresses()
	if len(addrs) == 0 {
		fmt.Println(" 没找到可用的 IPv4 地址，请先检查网络连接。")
	}
	// 最多画两张码：一张连同一个 Wi-Fi 用，一张连电脑热点用。
	var qrIdx []int
	for _, i := range []int{pickMainAddr(addrs, srv.allowVPN), pickHotspotAddr(addrs)} {
		if i >= 0 && !intIn(qrIdx, i) {
			qrIdx = append(qrIdx, i)
		}
	}
	ansi := enableANSI()
	for n, idx := range qrIdx {
		a := &addrs[idx]
		url := fmt.Sprintf("http://%s:%d/%s/", a.ip, port, srv.token)
		when := "手机连同一个 Wi-Fi 时用这张"
		if isHotspotName(a.iface) {
			when = "手机连的是电脑热点时用这张"
		}
		fmt.Println()
		if len(qrIdx) > 1 {
			fmt.Printf(" 二维码 %d/%d：%s —— %s\n", n+1, len(qrIdx), a.iface, when)
		} else {
			fmt.Printf(" 手机相机扫这张码（%s）：\n", when)
		}
		fmt.Println()
		if qr, _, err := qrMake(url, -1); err != nil {
			logf("二维码生成失败：%v（请手动输入网址）", err)
		} else {
			fmt.Print(qrText(qr, ansi))
			fmt.Println()
		}
		fmt.Printf("   %s\n", url)
	}
	fmt.Println()
	if len(qrIdx) > 0 {
		fmt.Println(" 也可以手动输入，挑手机能连到的那条（* / + 是上面二维码的地址）：")
	} else {
		fmt.Println(" 也可以手动输入，挑手机能连到的那条：")
	}
	// 先把画了二维码的两条列出来，顺序和上面两张码一致，再列其余的。
	order := append([]int{}, qrIdx...)
	for i := range addrs {
		if !intIn(qrIdx, i) {
			order = append(order, i)
		}
	}
	for _, i := range order {
		a := &addrs[i]
		tag := ""
		if a.vpn {
			if srv.allowVPN {
				tag = "  <= VPN 网段"
			} else {
				tag = "  <= 已拒绝此网段来访"
			}
		}
		mark := "  "
		for n, j := range qrIdx {
			if j == i {
				mark = "* "
				if n > 0 {
					mark = "+ "
				}
			}
		}
		fmt.Printf("   %s%s http://%s:%d/%s/%s\n", mark, padRight(a.iface, 18), a.ip, port, srv.token, tag)
	}
	fmt.Println()
	fmt.Println(" 手机必须和电脑连同一个 Wi-Fi；首次运行 Windows 会弹防火墙，请点“允许”。")
	fmt.Println(" 退出：在本窗口按 Ctrl+C。")
	fmt.Println()
}
