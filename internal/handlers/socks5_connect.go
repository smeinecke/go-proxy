package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/auth"
	"github.com/vlourme/go-proxy/internal/nio"
	"github.com/vlourme/go-proxy/internal/proxy"
	"github.com/vlourme/go-proxy/internal/routing"
	"github.com/vlourme/go-proxy/internal/stats"
)

// SOCKS constants
const (
	Version4 = 0x04
	Version5 = 0x05
)

// SOCKS5 Auth Methods
const (
	AuthNoAuth       = 0x00
	AuthGSSAPI       = 0x01
	AuthUsernamePass = 0x02
	AuthNoAcceptable = 0xFF
)

// SOCKS5 Command Codes
const (
	CmdConnect = 0x01
	CmdBind
	CmdUDPAssociate
)

// SOCKS5 Address Types
const (
	AtypIPv4   = 0x01
	AtypDomain = 0x03
	AtypIPv6   = 0x04
)

// SOCKS5 Reply Codes
const (
	RepSuccess = iota
	RepGeneralFailure
	RepConnectionNotAllowed
	RepNetworkUnreachable
	RepHostUnreachable
	RepConnectionRefused
	RepTTLExpired
	RepCmdNotSupported
	RepAddrTypeNotSupported
)

// IsSocks checks if the request is a SOCKS request
func IsSocks(buf *bufio.Reader) bool {
	ver, err := buf.ReadByte()
	if err != nil {
		return false
	}
	defer buf.UnreadByte()
	return ver == Version4 || ver == Version5
}

// HandleSocks handles the SOCKS protocol
func (p *ProxyHandler) HandleSocks(conn net.Conn, buf *bufio.Reader, st *stats.Stats) int64 {
	ver, err := buf.ReadByte()
	if err != nil {
		log.Error().Err(err).Msg("failed to read version")
		return -1
	}

	if ver == Version4 {
		log.Error().Msg("socks4 is not implemented")
		return -1
	}

	return p.HandleSocks5(conn, buf, st)
}

// HandleSocks5 handles the SOCKS5 protocol
func (p *ProxyHandler) HandleSocks5(conn net.Conn, buf *bufio.Reader, st *stats.Stats) int64 {
	methodsCount, err := buf.ReadByte()
	if err != nil {
		log.Error().Err(err).Msg("failed to read methods count")
		return -1
	}

	methods := make([]byte, int(methodsCount))
	if _, err := io.ReadFull(buf, methods); err != nil {
		log.Error().Err(err).Msg("failed to read methods")
		return -1
	}

	if !bytes.Contains(methods, []byte{AuthUsernamePass}) {
		proxy.WriteSocks5Status(conn, RepGeneralFailure)
		log.Error().Msg("socks5: auth required")
		return -1
	}

	conn.Write([]byte{Version5, AuthUsernamePass})

	username, password, err := parseSocksAuth(buf)
	if err != nil {
		conn.Write([]byte{0x01, 0x01})
		log.Error().Err(err).Msg("failed to parse auth")
		return -1
	}

	username, paramStr := auth.SplitParams(username)
	if !p.Authenticator.Verify(username, password) {
		if st != nil {
			st.AuthFailuresTotal.Add(1)
		}
		conn.Write([]byte{0x01, 0x01})
		log.Error().Msg("failed to verify auth")
		return -1
	}
	params := auth.GetParams(paramStr)
	conn.Write([]byte{0x01, 0x00})

	hdr := make([]byte, 4)
	if _, err := io.ReadFull(buf, hdr); err != nil {
		log.Error().Err(err).Msg("failed to read header")
		return -1
	}

	addrType := hdr[3]
	target, err := p.parseAtyp(addrType, buf, st)
	if err != nil {
		proxy.WriteSocks5Status(conn, RepAddrTypeNotSupported)
		log.Error().Err(err).Msg("failed to parse address")
		return -1
	}

	route, err := p.Router.Route(routing.RouteRequest{
		Username: username,
		TargetIP: target.IP,
		Session:  params[auth.ParamSession],
		Timeout:  parseTimeout(params[auth.ParamTimeout]),
		Location: params[auth.ParamLocation],
		Fallback: params[auth.ParamFallback],
	})
	if err != nil {
		if st != nil {
			st.DialFailuresTotal.Add(1)
		}
		proxy.WriteSocks5Status(conn, RepGeneralFailure)
		if errors.Is(err, routing.ErrAddressFamilyMismatch) {
			log.Warn().Err(err).Msg("failed to route")
		} else {
			log.Error().Err(err).Msg("failed to route")
		}
		return -1
	}

	destConn, err := route.Dialer.Dial("tcp", target.Addr())
	if err != nil {
		if st != nil {
			st.DialFailuresTotal.Add(1)
		}
		proxy.WriteSocks5Status(conn, RepHostUnreachable)
		log.Error().Err(err).Msg("failed to dial")
		return -1
	}
	defer destConn.Close()

	proxy.WriteSocks5Status(conn, RepSuccess)
	bytes := nio.CopyBidirectional(destConn, conn, time.Duration(p.Config.IdleTimeout)*time.Second)
	if st != nil {
		st.BytesTotal.Add(uint64(bytes))
	}
	return bytes
}

// parseAtyp parses the address type and returns a Target.
func (p *ProxyHandler) parseAtyp(atyp byte, buf *bufio.Reader, st *stats.Stats) (proxy.Target, error) {
	switch atyp {
	case AtypIPv4:
		addr := make([]byte, 6)
		if _, err := io.ReadFull(buf, addr); err != nil {
			return proxy.Target{}, fmt.Errorf("read IPv4: %w", err)
		}
		ip := netip.AddrFrom4([4]byte(addr[:4]))
		port := binary.BigEndian.Uint16(addr[4:])
		return proxy.Target{IP: ip, Port: port}, nil

	case AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(buf, lenBuf); err != nil {
			return proxy.Target{}, fmt.Errorf("read domain length: %w", err)
		}
		domainLen := int(lenBuf[0])
		domainBuf := make([]byte, domainLen+2)
		if _, err := io.ReadFull(buf, domainBuf); err != nil {
			return proxy.Target{}, fmt.Errorf("read domain+port: %w", err)
		}

		host := string(domainBuf[:domainLen])
		port := binary.BigEndian.Uint16(domainBuf[domainLen:])
		ip, err := p.Resolver.Resolve(context.Background(), host)
		if err != nil {
			if st != nil && err == routing.ErrBlocked {
				st.BlockedTotal.Add(1)
			}
			return proxy.Target{}, fmt.Errorf("resolve hostname: %w", err)
		}
		return proxy.Target{Host: host, IP: ip, Port: port}, nil

	case AtypIPv6:
		addr := make([]byte, 18)
		if _, err := io.ReadFull(buf, addr); err != nil {
			return proxy.Target{}, fmt.Errorf("read IPv6: %w", err)
		}
		ip := netip.AddrFrom16([16]byte(addr[:16]))
		port := binary.BigEndian.Uint16(addr[16:])
		return proxy.Target{IP: ip, Port: port}, nil

	default:
		return proxy.Target{}, fmt.Errorf("unsupported address type: %d", atyp)
	}
}

// parseSocksAuth parses the username and password from the SOCKS5 authentication request
func parseSocksAuth(buf *bufio.Reader) (string, string, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(buf, header); err != nil {
		return "", "", fmt.Errorf("read auth header: %w", err)
	}

	ulen := int(header[1])
	ubuf := make([]byte, ulen)
	if _, err := io.ReadFull(buf, ubuf); err != nil {
		return "", "", fmt.Errorf("read username: %w", err)
	}

	if _, err := io.ReadFull(buf, header[:1]); err != nil {
		return "", "", fmt.Errorf("read password length: %w", err)
	}
	plen := int(header[0])
	pbuf := make([]byte, plen)
	if _, err := io.ReadFull(buf, pbuf); err != nil {
		return "", "", fmt.Errorf("read password: %w", err)
	}

	return string(ubuf), string(pbuf), nil
}
