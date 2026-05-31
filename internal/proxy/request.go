package proxy

// Request holds the common fields extracted from any proxy protocol.
type Request struct {
	Protocol string // http, connect, socks5
	Username string
	Params   map[string]string
	Target   Target
}
