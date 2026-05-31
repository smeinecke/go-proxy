package handlers

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/auth"
	"github.com/vlourme/go-proxy/internal/http"
	"github.com/vlourme/go-proxy/internal/nio"
	"github.com/vlourme/go-proxy/internal/proxy"
	"github.com/vlourme/go-proxy/internal/routing"
)

// HandleTunneling handles the HTTPS tunneling request.
func (p *ProxyHandler) HandleTunneling(w net.Conn, r *http.Request) int64 {
	username, password, encodedParams := auth.GetCredentials(r)
	if !p.Authenticator.Verify(username, password) {
		p.Stats.AuthFailuresTotal.Add(1)
		log.Error().Msg("Invalid credentials")
		proxy.WriteAuthRequired(w)
		return -1
	}

	params := auth.GetParams(encodedParams)

	port, err := strconv.Atoi(string(r.Port))
	if err != nil {
		port = 443
	}

	host := string(r.Host)
	ip, err := p.Resolver.Resolve(context.Background(), host)
	if err != nil {
		if err == routing.ErrBlocked {
			p.Stats.BlockedTotal.Add(1)
		}
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

	proxy.WriteConnectEstablished(w)
	bytes := nio.CopyBidirectional(w, destConn, time.Duration(p.Config.IdleTimeout)*time.Second)
	p.Stats.BytesTotal.Add(uint64(bytes))
	return bytes
}
