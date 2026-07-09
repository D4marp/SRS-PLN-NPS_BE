package utils

import (
	"fmt"
	"net"
	"strings"
)

// LocalIPv4Addresses returns private/LAN IPv4 addresses (skips loopback).
func LocalIPv4Addresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var ips []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
}

// LocalAPIURLs builds http://<ip>:<port> for each LAN IPv4 address.
func LocalAPIURLs(port string) []string {
	ips := LocalIPv4Addresses()
	urls := make([]string, 0, len(ips))
	for _, ip := range ips {
		urls = append(urls, fmt.Sprintf("http://%s:%s", ip, port))
	}
	return urls
}

// BaseURLUsesLoopback reports whether BASE_URL points at localhost (unreachable from phone).
func BaseURLUsesLoopback(baseURL string) bool {
	u := strings.ToLower(baseURL)
	return strings.Contains(u, "://localhost") ||
		strings.Contains(u, "://127.0.0.1") ||
		strings.Contains(u, "://[::1]")
}
