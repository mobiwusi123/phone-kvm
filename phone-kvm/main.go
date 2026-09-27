//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// tokenAlphabet 去掉了 0 o 1 l i 这些容易看错的字符，方便手动输入网址。
const tokenAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

func logf(format string, a ...any) {
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
}

func randomToken(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = tokenAlphabet[rand.IntN(len(tokenAlphabet))]
	}
	return string(b)
}

func main() {
	enableUTF8Console()

	port := flag.Int("port", 8123, "监听端口")
	token := flag.String("token", "", "访问密钥（默认每次随机生成）")
	sens := flag.Float64("sens", 1.0, "指针灵敏度倍数")
	allowVPN := flag.Bool("allow-vpn", false, "允许来自 VPN 网段的连接")
	selftest := flag.Bool("selftest", false, "只做键鼠注入自检然后退出")
	selftestNet := flag.String("selftest-net", "", "对运行中的服务端做端到端自检，参数形如 http://127.0.0.1:8123/<密钥>")
	flag.Parse()

	if *selftest {
		runInjectSelfTest(*sens)
		return
	}
	if *selftestNet != "" {
		runNetSelfTest(*selftestNet)
		return
	}

	tk := *token
	if tk == "" {
		tk = randomToken(8)
	}
	inj := newInjector(*sens)
	srv := newServer(inj, tk, *sens, *allowVPN)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		logf("监听 %d 端口失败: %v", *port, err)
		os.Exit(1)
	}
	srv.report(*port)

	httpSrv := &http.Server{Handler: srv.mux(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("HTTP 服务异常退出: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	logf("正在退出…")
	srv.broadcast(`{"t":"bye"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	logf("已退出")
}
