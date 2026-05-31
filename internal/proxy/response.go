package proxy

import (
	"fmt"
	"io"
)

// WriteError writes a generic HTTP error response.
func WriteError(w io.Writer, status int, reason string) {
	fmt.Fprintf(w, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", status, reason)
}

// WriteAuthRequired writes a 407 Proxy Authentication Required response.
func WriteAuthRequired(w io.Writer) {
	w.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"proxy\"\r\n\r\n"))
}

// WriteConnectEstablished writes a 200 Connection Established response.
func WriteConnectEstablished(w io.Writer) {
	w.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
}

// WriteSocks5Status writes a SOCKS5 reply.
func WriteSocks5Status(w io.Writer, reply byte) {
	resp := []byte{
		0x05, reply, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, // Dummy IP
		0x00, 0x00, // Dummy port
	}
	w.Write(resp)
}
