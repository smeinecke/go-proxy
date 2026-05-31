package handlers

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/auth"
	"github.com/vlourme/go-proxy/internal/http"
	"github.com/vlourme/go-proxy/internal/nio"
	"github.com/vlourme/go-proxy/internal/proxy"
	"github.com/vlourme/go-proxy/internal/routing"
	"github.com/vlourme/go-proxy/internal/stats"
)

// HandleTunneling handles the HTTPS tunneling request.
func (p *ProxyHandler) HandleTunneling(w net.Conn, r *http.Request, st *stats.Stats) int64 {
	username, password, encodedParams := auth.GetCredentials(r)
	if !p.Authenticator.Verify(username, password) {
		if st != nil {
			st.AuthFailuresTotal.Add(1)
		}
		log.Warn().Msg("Invalid credentials")
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
		if st != nil {
			if err == routing.ErrBlocked {
				st.BlockedTotal.Add(1)
			}
			st.DNSFailuresTotal.Add(1)
		}
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
		if st != nil {
			st.DialFailuresTotal.Add(1)
		}
		if errors.Is(err, routing.ErrAddressFamilyMismatch) {
			log.Warn().Err(err).Msg("Error routing")
		} else {
			log.Error().Err(err).Msg("Error routing")
		}
		proxy.WriteError(w, 500, "Internal Server Error")
		return -1
	}

	destConn, err := route.Dialer.Dial("tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		if st != nil {
			st.DialFailuresTotal.Add(1)
		}
		log.Error().Err(err).Msg("Error dialing")
		proxy.WriteError(w, 500, "Internal Server Error")
		return -1
	}
	defer destConn.Close()

	proxy.WriteConnectEstablished(w)
	bytes := nio.CopyBidirectional(w, destConn, time.Duration(p.Config.IdleTimeout)*time.Second)
	if st != nil {
		st.BytesTotal.Add(uint64(bytes))
	}
	return bytes
}
