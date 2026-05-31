package http

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

type Request struct {
	Host    []byte
	Port    []byte
	Method  []byte
	URL     []byte
	Version []byte
	Header  map[string][]byte
}

var requestPool = sync.Pool{
	New: func() any {
		return &Request{
			Header: make(map[string][]byte, 16),
		}
	},
}

func getRequest() *Request {
	req := requestPool.Get().(*Request)
	for k := range req.Header {
		delete(req.Header, k)
	}
	return req
}

// canonicalHeaderKey returns the canonical form of a header key.
func canonicalHeaderKey(key []byte) string {
	return textproto.CanonicalMIMEHeaderKey(string(key))
}

// GetHeader returns the header value for the given key, case-insensitively.
func (req *Request) GetHeader(key string) []byte {
	return req.Header[textproto.CanonicalMIMEHeaderKey(key)]
}

// SetHeader sets a header value using the canonical key form.
func (req *Request) SetHeader(key string, value []byte) {
	req.Header[textproto.CanonicalMIMEHeaderKey(key)] = value
}

// DeleteHeader removes a header by key, case-insensitively.
func (req *Request) DeleteHeader(key string) {
	delete(req.Header, textproto.CanonicalMIMEHeaderKey(key))
}

// originForm returns the origin-form path for forwarding.
// For absolute-form URLs like "http://host/path?q=1", it returns "/path?q=1".
// If the path is empty but a query exists, it returns "/?query".
// Otherwise it returns "/".
func originForm(rawURL []byte) []byte {
	var path []byte

	switch {
	case bytes.HasPrefix(rawURL, []byte("http://")):
		path = rawURL[len("http://"):]
	case bytes.HasPrefix(rawURL, []byte("https://")):
		path = rawURL[len("https://"):]
	default:
		return rawURL
	}

	slash := bytes.IndexByte(path, '/')
	if slash != -1 {
		return path[slash:]
	}

	query := bytes.IndexByte(path, '?')
	if query != -1 {
		return append([]byte("/"), path[query:]...)
	}

	return []byte("/")
}

func (req *Request) WriteTo(w io.Writer, src *bufio.Reader) (total int64, err error) {
	if isChunked(req.Header) {
		return 0, fmt.Errorf("chunked transfer-encoding is not supported")
	}

	var reqLine []byte
	if bytes.Equal(req.Method, []byte("CONNECT")) {
		reqLine = bytes.Join([][]byte{req.Method, req.URL, req.Version}, []byte(" "))
	} else {
		reqLine = bytes.Join([][]byte{req.Method, originForm(req.URL), req.Version}, []byte(" "))
	}

	// Normalize Host header to match the parsed request target.
	// Bracketed IPv6 is required in the Host header even without a port.
	if !bytes.Equal(req.Method, []byte("CONNECT")) {
		host := string(req.Host)
		port := string(req.Port)
		if port != "" && port != "80" && port != "443" {
			host = net.JoinHostPort(host, port)
		} else if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
			host = "[" + host + "]"
		}
		req.SetHeader("Host", []byte(host))
	}

	buf := bytes.NewBuffer(nil)
	buf.Write(reqLine)
	buf.Write([]byte("\r\n"))

	for k, v := range req.Header {
		buf.Write([]byte(k))
		buf.Write([]byte(": "))
		buf.Write(v)
		buf.Write([]byte("\r\n"))
	}

	buf.Write([]byte("\r\n"))

	total, err = buf.WriteTo(w)
	if err != nil {
		return total, err
	}

	if clStr, ok := req.Header["Content-Length"]; ok {
		cl, err := strconv.Atoi(string(clStr))
		if err != nil {
			return total, fmt.Errorf("invalid Content-Length: %v", err)
		}

		n, err := io.CopyN(w, src, int64(cl))
		if err != nil {
			return total, err
		}
		total += n
	}

	return total, nil
}

func isChunked(headers map[string][]byte) bool {
	te := textproto.CanonicalMIMEHeaderKey("Transfer-Encoding")
	val, ok := headers[te]
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(string(val)), "chunked")
}

func (req *Request) Release() {
	requestPool.Put(req)
}

func ParseRequest(r *bufio.Reader) (*Request, error) {
	line, err := r.ReadSlice('\n')
	if err != nil {
		return nil, err
	}
	line = bytes.TrimSpace(line)

	// METHOD
	method, line, found := bytes.Cut(line, []byte(" "))
	if !found {
		return nil, fmt.Errorf("invalid request line")
	}

	// URL
	url, version, found := bytes.Cut(line, []byte(" "))
	if !found {
		return nil, fmt.Errorf("invalid request line")
	}

	req := getRequest()
	req.Method = append([]byte(nil), method...)
	// If buffer > 4096, the slice will move and the URL will be invalid
	// This is kept only for logging purposes, it works without but
	// the URL in the log will be invalid due to the buffer re-use.
	req.URL = bytes.Clone(url)
	req.Version = append([]byte(nil), version...)

	req.Host, req.Port, err = extractHostPort(req.Method, req.URL)
	if err != nil {
		return nil, err
	}

	for {
		line, err := r.ReadSlice('\n')
		if err != nil {
			return nil, err
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			break
		}

		key, value, found := bytes.Cut(line, []byte(":"))
		if !found {
			continue
		}

		req.Header[canonicalHeaderKey(key)] = append([]byte(nil), bytes.TrimSpace(value)...)
	}

	return req, nil
}

func extractHostPort(method, rawURL []byte) ([]byte, []byte, error) {
	if bytes.Equal(method, []byte("CONNECT")) {
		// Use net.SplitHostPort to handle IPv6 literals like [2001:db8::1]:443
		hostPort := string(rawURL)
		host, port, err := net.SplitHostPort(hostPort)
		if err != nil {
			// No port specified; default to 443
			return rawURL, []byte("443"), nil
		}
		return []byte(host), []byte(port), nil
	}

	// Strip scheme manually for GET/POST/...
	const httpPrefix = "http://"
	const httpsPrefix = "https://"

	var authority []byte
	var defaultPort []byte

	switch {
	case bytes.HasPrefix(rawURL, []byte(httpPrefix)):
		defaultPort = []byte("80")
		raw := rawURL[len(httpPrefix):]
		authority = cutAt(raw, '/', '?')

	case bytes.HasPrefix(rawURL, []byte(httpsPrefix)):
		defaultPort = []byte("443")
		raw := rawURL[len(httpsPrefix):]
		authority = cutAt(raw, '/', '?')

	default:
		return nil, nil, fmt.Errorf("invalid absolute URL in request line: %s", rawURL)
	}

	// Use net.SplitHostPort for proper IPv6 bracket handling
	authStr := string(authority)
	host, port, err := net.SplitHostPort(authStr)
	if err != nil {
		// No port specified: could be hostname or bracketed IPv6
		if strings.HasPrefix(authStr, "[") && strings.HasSuffix(authStr, "]") {
			return []byte(authStr[1 : len(authStr)-1]), defaultPort, nil
		}
		return authority, defaultPort, nil
	}

	return []byte(host), []byte(port), nil
}

// cutAt returns the portion of s before the first occurrence of any of the given cut bytes.
func cutAt(s []byte, cut ...byte) []byte {
	min := -1
	for _, b := range cut {
		if idx := bytes.IndexByte(s, b); idx != -1 && (min == -1 || idx < min) {
			min = idx
		}
	}
	if min == -1 {
		return s
	}
	return s[:min]
}
