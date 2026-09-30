//go:build windows

package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// runInjectSelfTest 只测注入层本身，不碰网络。
// 安全约定：只移动光标、只按修饰键 VK_SHIFT，绝不发送任何会输入字符的按键。
func runInjectSelfTest(sens float64) {
	inj := newInjector(sens)
	time.Sleep(300 * time.Millisecond)
	x0, y0, ok := inj.Cursor(2 * time.Second)
	if !ok {
		fmt.Println("自检失败：读不到光标位置")
		os.Exit(1)
	}
	fmt.Printf("初始光标 = (%d, %d)\n", x0, y0)

	// 关键验证：绝对定位 + 自建曲线意味着"同样的位移必然走出同样的距离"，
	// 不像相对的 MOUSEEVENTF_MOVE 那样被系统加速放大成不可预测的值。
	inj.Move(20, 0)
	inj.Move(-20, 0)
	time.Sleep(120 * time.Millisecond)
	bx, by, _ := inj.Cursor(time.Second)
	fmt.Printf("往返 +20/-20 后 = (%d, %d)  偏差 = (%d, %d)  [期望 0,0]\n", bx, by, bx-x0, by-y0)
	// 绝对坐标归一化存在 1 像素的量化误差（实测上限就是 1，且完全可复现），
	// 它不是累积漂移，所以这里给 2 像素的容差。
	roundTrip := absDiff(bx, x0) <= 2 && absDiff(by, y0) <= 2

	inj.Move(30, 0)
	time.Sleep(150 * time.Millisecond)
	gx, _, _ := inj.Cursor(time.Second)
	fmt.Printf("单次 +30 后实际位移 = %d  [带加速应大于 30]\n", gx-bx)

	inj.SetPos(x0, y0)
	time.Sleep(120 * time.Millisecond)
	rx, ry, _ := inj.Cursor(time.Second)
	fmt.Printf("显式还原后 = (%d, %d)  还原到 ±2 像素内 = %v\n", rx, ry, absDiff(rx, x0) <= 2 && absDiff(ry, y0) <= 2)
	restored := absDiff(rx, x0) <= 2 && absDiff(ry, y0) <= 2

	inj.Key(0x10, true)
	inj.Key(0x10, false)
	fmt.Println("已发送一次 VK_SHIFT 按下/抬起")

	if roundTrip && restored {
		fmt.Println("注入自检通过")
		return
	}
	fmt.Println("注入自检未通过")
	os.Exit(1)
}

// ------------------------------------------------------------ 端到端自检

type wsTestClient struct {
	conn net.Conn
	br   *bufio.Reader
}

func dialWS(rawURL string) (*wsTestClient, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if host == "" {
		return nil, fmt.Errorf("地址里没有主机名: %s", rawURL)
	}
	path := strings.TrimSuffix(u.Path, "/") + "/ws"
	conn, err := net.DialTimeout("tcp", host, 3*time.Second)
	if err != nil {
		return nil, err
	}
	keyRaw := make([]byte, 16)
	if _, err := rand.Read(keyRaw); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyRaw)
	req := "GET " + path + " HTTP/1.1\r\nHost: " + host + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		conn.Close()
		return nil, fmt.Errorf("握手被拒: %s", strings.TrimSpace(status))
	}
	for {
		h, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		if h == "\r\n" {
			break
		}
	}
	return &wsTestClient{conn: conn, br: br}, nil
}

func (t *wsTestClient) send(op byte, payload []byte) error {
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	n := len(payload)
	buf := []byte{0x80 | op}
	switch {
	case n < 126:
		buf = append(buf, 0x80|byte(n))
	case n <= 0xFFFF:
		buf = append(buf, 0x80|126, byte(n>>8), byte(n))
	default:
		buf = append(buf, 0x80|127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	buf = append(buf, mask[0], mask[1], mask[2], mask[3])
	for i := 0; i < n; i++ {
		buf = append(buf, payload[i]^mask[i&3])
	}
	_, err := t.conn.Write(buf)
	return err
}

func (t *wsTestClient) recvBinary() ([]byte, error) {
	for {
		var head [2]byte
		if _, err := io.ReadFull(t.br, head[:]); err != nil {
			return nil, err
		}
		op := head[0] & 0x0F
		n := int64(head[1] & 0x7F)
		switch n {
		case 126:
			var ext [2]byte
			if _, err := io.ReadFull(t.br, ext[:]); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint16(ext[:]))
		case 127:
			var ext [8]byte
			if _, err := io.ReadFull(t.br, ext[:]); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint64(ext[:]))
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(t.br, body); err != nil {
			return nil, err
		}
		if op == opBinary || op == opText {
			return body, nil
		}
	}
}

func (t *wsTestClient) move(dx, dy int16) error {
	b := make([]byte, 5)
	b[0] = cMove
	binary.LittleEndian.PutUint16(b[1:], uint16(dx))
	binary.LittleEndian.PutUint16(b[3:], uint16(dy))
	return t.send(opBinary, b)
}

func (t *wsTestClient) queryCursor() (int32, int32, error) {
	if err := t.send(opBinary, []byte{cQuery}); err != nil {
		return 0, 0, err
	}
	for i := 0; i < 8; i++ {
		body, err := t.recvBinary()
		if err != nil {
			return 0, 0, err
		}
		if len(body) >= 9 && body[0] == sCursor {
			return int32(binary.LittleEndian.Uint32(body[1:5])), int32(binary.LittleEndian.Uint32(body[5:9])), nil
		}
	}
	return 0, 0, fmt.Errorf("没有收到光标位置")
}

// runNetSelfTest 走完整链路：HTTP 鉴权 -> WebSocket 握手 -> 协议解析 -> 注入。
func runNetSelfTest(rawURL string) {
	base := strings.TrimSuffix(rawURL, "/")
	root := base
	if i := strings.LastIndex(base, "/"); i >= 0 {
		root = base[:i]
	}

	// 局域网自检不能被系统/环境里的 HTTP_PROXY 拐走（实测过：本机设了代理时
	// 连 192.168.137.1 会被代理拒掉），所以显式关掉代理。
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(root + "/wrongtoken000/ws")
	if err != nil {
		fmt.Println("自检失败：连不上服务端 ", err)
		os.Exit(1)
	}
	resp.Body.Close()
	rejected := resp.StatusCode == http.StatusForbidden
	fmt.Printf("错误密钥 -> HTTP %d  被拒绝 = %v  [期望拒绝]\n", resp.StatusCode, rejected)

	c, err := dialWS(base)
	if err != nil {
		fmt.Println("自检失败：", err)
		os.Exit(1)
	}
	defer c.conn.Close()
	_ = c.conn.SetDeadline(time.Now().Add(15 * time.Second))
	hello, err := c.recvBinary()
	if err != nil {
		fmt.Println("自检失败：没有收到握手消息 ", err)
		os.Exit(1)
	}
	fmt.Printf("收到握手消息: %s\n", string(hello))

	start := time.Now()
	if err := c.send(opBinary, []byte{cPing, 9, 9, 9, 9}); err != nil {
		fmt.Println("自检失败：发送 ping 出错 ", err)
		os.Exit(1)
	}
	pongOK := false
	for i := 0; i < 8 && !pongOK; i++ {
		body, err := c.recvBinary()
		if err != nil {
			break
		}
		if len(body) >= 5 && body[0] == sPong && body[1] == 9 && body[4] == 9 {
			pongOK = true
		}
	}
	fmt.Printf("ping/pong 往返 = %v  本机耗时 %.2f ms\n", pongOK, float64(time.Since(start).Microseconds())/1000)

	x0, y0, err := c.queryCursor()
	if err != nil {
		fmt.Println("自检失败：", err)
		os.Exit(1)
	}
	fmt.Printf("初始光标 = (%d, %d)\n", x0, y0)
	_ = c.move(20, 0)
	_ = c.move(-20, 0)
	time.Sleep(120 * time.Millisecond)
	x1, y1, err := c.queryCursor()
	if err != nil {
		fmt.Println("自检失败：", err)
		os.Exit(1)
	}
	fmt.Printf("往返 +20/-20 后 = (%d, %d)  偏差 = (%d, %d)  [期望 0,0]\n", x1, y1, x1-x0, y1-y0)

	_ = c.move(30, 0)
	time.Sleep(150 * time.Millisecond)
	x2, _, err := c.queryCursor()
	if err != nil {
		fmt.Println("自检失败：", err)
		os.Exit(1)
	}
	fmt.Printf("单次 +30 后实际位移 = %d  [带加速应大于 30]\n", x2-x1)

	_ = c.send(opBinary, []byte{opClose})
	if rejected && pongOK && absDiff(x1, x0) <= 2 && absDiff(y1, y0) <= 2 {
		fmt.Println("端到端自检通过")
		return
	}
	fmt.Println("端到端自检未通过")
	os.Exit(1)
}

func absDiff(a, b int32) int32 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}
