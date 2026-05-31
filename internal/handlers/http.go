package handlers

import (
	"bufio"
	"context"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/auth"
	"github.com/vlourme/go-proxy/internal/http"
	"github.com/vlourme/go-proxy/internal/nio"
	"github.com/vlourme/go-proxy/internal/proxy"
	"github.com/vlourme/go-proxy/internal/routing"
)

// HandleHTTP handles the HTTP request.
func (p *ProxyHandler) HandleHTTP(w net.Conn, buf *bufio.Reader, r *http.Request) int64 {
	username, password, encodedParams := auth.GetCredentials(r)
	if !p.Authenticator.Verify(username, password) {
		p.Stats.AuthFailuresTotal.Add(1)
		log.Error().Msg("Invalid credentials")
		proxy.WriteAuthRequired(w)
		return -1
	}

	params := auth.GetParams(encodedParams)

	for _, header := range p.Config.DeletedHeaders {
		r.DeleteHeader(header)
	}
	r.DeleteHeader("Proxy-Authorization")
	r.DeleteHeader("Proxy-Connection")

	port, err := strconv.Atoi(string(r.Port))
	if err != nil {
		port = 80
	}

	host := string(r.Host)
	ip, err := p.Resolver.Resolve(context.Background(), host)
	if err != nil {
		p.Stats.DNSFailuresTotal.Add(1)
		log.Error().Err(err).Str("host", host).Msg("Error resolving hostname")
		proxy.WriteError(w, 500, "Internal Server Error")
		return -1
	}

	route, err := p.Router.Route(routing.RouteRequest{
		Username: username,
		TargetIP: ip,
		Session:  params[auth.ParamSession],
		Timeout:  parseTimeout(params[auth.ParamTimeout]),
		Location: params[auth.ParamLocation],
		Fallback: params[auth.ParamFallback],
	})
	if err != nil {
		p.Stats.DialFailuresTotal.Add(1)
		log.Error().Err(err).Msg("Error routing")
		proxy.WriteError(w, 500, "Internal Server Error")
		return -1
	}

	destConn, err := route.Dialer.Dial("tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		p.Stats.DialFailuresTotal.Add(1)
		log.Error().Err(err).Msg("Error dialing")
		proxy.WriteError(w, 500, "Internal Server Error")
		return -1
	}
	defer destConn.Close()

	_, err = r.WriteTo(destConn, buf)
	if err != nil {
		log.Error().Err(err).Msg("Error writing request")
		proxy.WriteError(w, 500, "Internal Server Error")
		return -1
	}

	bytes := nio.CopyBidirectional(w, destConn, time.Duration(p.Config.IdleTimeout)*time.Second)
	p.Stats.BytesUp.Add(uint64(bytes))
	return bytes
}

func parseTimeout(s string) time.Duration {
	if s == "" {
		return 0
	}
	m, err := strconv.Atoi(s)
	if err != nil || m <= 0 {
		return 0
	}
	return time.Duration(m) * time.Minute
}

// parseAddrPort parses a host string and port into a netip.Addr.
func parseAddrPort(host string, portStr string) (netip.Addr, uint16, error) {
	port, _ := strconv.ParseUint(portStr, 10, 16)
	if port == 0 {
		port = 80
	}

	addr, err := netip.ParseAddr(host)
	if err == nil {
		return addr, uint16(port), nil
	}

	return netip.Addr{}, uint16(port), nil
}
