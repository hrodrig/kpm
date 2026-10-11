// Package identity resolves the sampling client identity ("who samples") via
// the cascade: KPM_CLIENT_ID -> client.id -> hostname -> primary IPv4 -> MAC ->
// literal "unknown". Never anonymous — notification payloads always carry an
// origin tag (same pattern as kwd / pgwd).
package identity

import (
	"net"
	"os"
	"strings"
)

// EnvID is the environment variable that takes precedence over config.
const EnvID = "KPM_CLIENT_ID"

// Resolve returns the client identity using the cascade, or "unknown" if every
// source fails. cfgID is the client.id value from config (may be empty).
func Resolve(cfgID string) string {
	if v := strings.TrimSpace(os.Getenv(EnvID)); v != "" {
		return v
	}
	if v := strings.TrimSpace(cfgID); v != "" {
		return v
	}
	if v, err := os.Hostname(); err == nil && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := primaryIPv4(); v != "" {
		return v
	}
	if v := primaryMAC(); v != "" {
		return v
	}
	return "unknown"
}

func primaryIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ip, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip.IP.IsLoopback() || ip.IP.To4() == nil {
			continue
		}
		return ip.IP.String()
	}
	return ""
}

func primaryMAC() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.HardwareAddr == nil {
			continue
		}
		if mac := iface.HardwareAddr.String(); mac != "" {
			return mac
		}
	}
	return ""
}
