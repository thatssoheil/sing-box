package transport

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// URLHost builds the host field of an HTTP-based DNS transport URL from the
// configured server address and port. A zero port means the implied default of
// 443.
//
// The configured address is not necessarily a bare host: brackets and an
// explicit port may already be present, both from the option value and from a
// user supplied Host header. They are normalized here, so net.JoinHostPort
// never receives an already bracketed or already ported host. A non-default
// port replaces an explicit one. When the default port is implied, a bare IPv6
// literal is bracketed because net/url rejects a URL whose host is a bare IPv6
// literal ("invalid port after host").
func URLHost(host string, port uint16) string {
	hostPort := ""
	if bareHost, explicitPort, err := net.SplitHostPort(host); err == nil {
		host, hostPort = bareHost, explicitPort
	} else {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if port != 0 && port != 443 {
		return net.JoinHostPort(host, strconv.Itoa(int(port)))
	}
	if hostPort != "" {
		return net.JoinHostPort(host, hostPort)
	}
	addr, parseErr := netip.ParseAddr(host)
	if parseErr == nil && addr.Is6() {
		return "[" + host + "]"
	}
	return host
}
