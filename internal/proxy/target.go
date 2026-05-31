package proxy

import (
	"net"
	"net/netip"
	"strconv"
)

// Target represents the destination of a proxy request.
type Target struct {
	Host string
	IP   netip.Addr
	Port uint16
}

// Addr returns the network address string for dialing (host:port or [host]:port).
func (t Target) Addr() string {
	if t.IP.IsValid() {
		return net.JoinHostPort(t.IP.String(), strconv.Itoa(int(t.Port)))
	}
	return net.JoinHostPort(t.Host, strconv.Itoa(int(t.Port)))
}

// HostPort returns the host:port suitable for HTTP Host header or CONNECT target.
func (t Target) HostPort() string {
	return t.Addr()
}
