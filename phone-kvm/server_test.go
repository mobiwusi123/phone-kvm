//go:build windows

package main

import (
	"net"
	"testing"
)

func addrFor(iface, ip string) localAddr {
	return localAddr{iface: iface, ip: net.ParseIP(ip).To4(), vpn: isVPNName(iface)}
}

// TestIsHotspotName covers the adapter names Windows uses for the mobile
// hotspot: "Local Area Connection* N" / "本地连接* N" and the Wi-Fi Direct
// adapters. Everything else must stay a normal interface.
func TestIsHotspotName(t *testing.T) {
	cases := map[string]bool{
		"Local Area Connection* 10":                true,
		"本地连接* 10":                                 true,
		"Microsoft Wi-Fi Direct Virtual Adapter":   true,
		"Microsoft WiFi Direct Virtual Adapter #2": true,
		"WLAN":       false,
		"WLAN 2":     false,
		"Ethernet":   false,
		"本地连接":       false,
		"Radmin VPN": false,
	}
	for name, want := range cases {
		if got := isHotspotName(name); got != want {
			t.Errorf("isHotspotName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestPickAddrs(t *testing.T) {
	wlan := addrFor("WLAN 2", "10.253.86.17")
	hotspot := addrFor("本地连接* 10", "192.168.137.1")
	vpn := addrFor("Radmin VPN", "26.163.198.217")
	linkLocal := addrFor("Ethernet", "169.254.10.20")

	cases := []struct {
		name      string
		addrs     []localAddr
		allowVPN  bool
		wantMain  int
		wantHotsp int
	}{
		{"wifi + hotspot + vpn", []localAddr{vpn, hotspot, wlan}, false, 2, 1},
		{"only wifi", []localAddr{wlan}, false, 0, -1},
		{"only hotspot", []localAddr{hotspot}, false, 0, 0},
		{"vpn only, rejected", []localAddr{vpn}, false, -1, -1},
		{"vpn only, allowed", []localAddr{vpn}, true, 0, -1},
		{"link local beats vpn", []localAddr{vpn, linkLocal}, false, 1, -1},
		{"hotspot beats link local", []localAddr{linkLocal, hotspot}, false, 1, 1},
		{"no addresses", nil, false, -1, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickMainAddr(c.addrs, c.allowVPN); got != c.wantMain {
				t.Errorf("pickMainAddr = %d, want %d", got, c.wantMain)
			}
			if got := pickHotspotAddr(c.addrs); got != c.wantHotsp {
				t.Errorf("pickHotspotAddr = %d, want %d", got, c.wantHotsp)
			}
		})
	}
}

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{
		"WLAN 2":     6,
		"本地连接* 10":   12, // 4 个汉字 = 8 列，加 "* 10" 4 列
		"Radmin VPN": 10,
		"":           0,
	}
	for s, want := range cases {
		if got := displayWidth(s); got != want {
			t.Errorf("displayWidth(%q) = %d, want %d", s, got, want)
		}
	}
	if got := padRight("WLAN", 6); got != "WLAN  " {
		t.Errorf("padRight = %q", got)
	}
	if got := padRight("本地连接* 10", 4); got != "本地连接* 10" {
		t.Errorf("padRight must not truncate: %q", got)
	}
}
