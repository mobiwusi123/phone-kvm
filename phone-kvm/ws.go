// 极简 WebSocket 服务端：只做这个项目需要的事，零第三方依赖。
// 支持：握手、文本/二进制帧、分片重组、ping/pong、close。
package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
	mu   sync.Mutex
	dead bool
}

func headerContains(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !headerContains(r.Header, "Connection", "upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("不是 WebSocket 升级请求")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, errors.New("只支持 WebSocket 版本 13")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("缺少 Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("当前服务器不支持连接劫持")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true) // 延迟敏感：别让 Nagle 把事件攒起来
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, resp); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Time{})
	return &wsConn{conn: conn, br: rw.Reader}, nil
}

func (c *wsConn) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dead {
		c.dead = true
		_ = c.conn.Close()
	}
}

// readMessage 返回一个完整的消息（自动重组分片、自动回 pong）。
func (c *wsConn) readMessage(max int) (byte, []byte, error) {
	var (
		opcode  byte
		payload []byte
	)
	for {
		var head [2]byte
		if _, err := io.ReadFull(c.br, head[:]); err != nil {
			return 0, nil, err
		}
		fin := head[0]&0x80 != 0
		op := head[0] & 0x0F
		masked := head[1]&0x80 != 0
		length := int64(head[1] & 0x7F)
		switch length {
		case 126:
			var ext [2]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return 0, nil, err
			}
			length = int64(binary.BigEndian.Uint16(ext[:]))
		case 127:
			var ext [8]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return 0, nil, err
			}
			length = int64(binary.BigEndian.Uint64(ext[:]))
		}
		if length < 0 || length > int64(max) {
			return 0, nil, fmt.Errorf("帧过大: %d 字节", length)
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(c.br, mask[:]); err != nil {
				return 0, nil, err
			}
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(c.br, body); err != nil {
			return 0, nil, err
		}
		if masked {
			for i := range body {
				body[i] ^= mask[i&3]
			}
		}
		switch op {
		case opContinuation:
			payload = append(payload, body...)
			if len(payload) > max {
				return 0, nil, errors.New("分片消息过大")
			}
			if fin {
				return opcode, payload, nil
			}
		case opText, opBinary:
			if fin {
				return op, body, nil
			}
			opcode, payload = op, append(payload, body...)
		case opClose:
			_ = c.writeFrame(opClose, nil)
			c.Close()
			return 0, nil, io.EOF
		case opPing:
			if err := c.writeFrame(opPong, body); err != nil {
				return 0, nil, err
			}
		case opPong:
			// 保活回执，无需处理
		default:
			return 0, nil, fmt.Errorf("不支持的操作码 0x%x", op)
		}
	}
}

func (c *wsConn) writeFrame(op byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead {
		return errors.New("连接已关闭")
	}
	n := len(payload)
	buf := make([]byte, 0, n+10)
	buf = append(buf, 0x80|op)
	switch {
	case n < 126:
		buf = append(buf, byte(n))
	case n <= 0xFFFF:
		buf = append(buf, 126, byte(n>>8), byte(n))
	default:
		buf = append(buf, 127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	buf = append(buf, payload...)
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.conn.Write(buf) // 头和载荷一次写出，避免两次系统调用
	return err
}

func (c *wsConn) WriteText(s string) error   { return c.writeFrame(opText, []byte(s)) }
func (c *wsConn) WriteBinary(b []byte) error { return c.writeFrame(opBinary, b) }
func (c *wsConn) Ping() error                { return c.writeFrame(opPing, nil) }
