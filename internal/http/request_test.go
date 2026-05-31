package http

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestParseRequest(t *testing.T) {
	request := "GET http://api.ipquery.io/?format=json HTTP/1.1\r\nHost: api.ipquery.io\r\nUser-Agent: curl/8.5.0\r\nAccept: */*\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(req.Method) != "GET" {
		t.Fatalf("expected method GET, got %s", req.Method)
	}

	if string(req.Host) != "api.ipquery.io" {
		t.Fatalf("expected Host api.ipquery.io, got %s", req.Host)
	}

	if string(req.Port) != "80" {
		t.Fatalf("expected Port 80, got %s", req.Port)
	}

	if string(req.URL) != "http://api.ipquery.io/?format=json" {
		t.Fatalf("expected URL http://api.ipquery.io/?format=json, got %s", req.URL)
	}

	if string(req.Version) != "HTTP/1.1" {
		t.Fatalf("expected version HTTP/1.1, got %s", req.Version)
	}

	if string(req.Header["Host"]) != "api.ipquery.io" {
		t.Fatalf("expected Host header api.ipquery.io, got %s", req.Header["Host"])
	}

	if string(req.Header["User-Agent"]) != "curl/8.5.0" {
		t.Fatalf("expected User-Agent header curl/8.5.0, got %s", req.Header["User-Agent"])
	}

	if string(req.Header["Accept"]) != "*/*" {
		t.Fatalf("expected Accept header */*, got %s", req.Header["Accept"])
	}
}

func TestParseRequestConnect(t *testing.T) {
	request := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(req.Host) != "example.com" {
		t.Fatalf("expected Host example.com, got %s", req.Host)
	}

	if string(req.Port) != "443" {
		t.Fatalf("expected Port 443, got %s", req.Port)
	}

	if string(req.Method) != "CONNECT" {
		t.Fatalf("expected method CONNECT, got %s", req.Method)
	}

	if string(req.URL) != "example.com:443" {
		t.Fatalf("expected URL example.com:443, got %s", req.URL)
	}

	if string(req.Version) != "HTTP/1.1" {
		t.Fatalf("expected version HTTP/1.1, got %s", req.Version)
	}

	if string(req.Header["Connection"]) != "close" {
		t.Fatalf("expected Connection header close, got %s", req.Header["Connection"])
	}
}

func TestParseRequestConnectIPv6(t *testing.T) {
	request := "CONNECT [2001:db8::1]:443 HTTP/1.1\r\nHost: example.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(req.Host) != "2001:db8::1" {
		t.Fatalf("expected Host 2001:db8::1, got %s", req.Host)
	}

	if string(req.Port) != "443" {
		t.Fatalf("expected Port 443, got %s", req.Port)
	}
}

func TestParseRequestConnectIPv4(t *testing.T) {
	request := "CONNECT 1.2.3.4:443 HTTP/1.1\r\nHost: example.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(req.Host) != "1.2.3.4" {
		t.Fatalf("expected Host 1.2.3.4, got %s", req.Host)
	}

	if string(req.Port) != "443" {
		t.Fatalf("expected Port 443, got %s", req.Port)
	}
}

func TestParseRequestIPv6URLNoPort(t *testing.T) {
	request := "GET http://[2001:db8::1]/path HTTP/1.1\r\nHost: [2001:db8::1]\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(req.Host) != "2001:db8::1" {
		t.Fatalf("expected Host 2001:db8::1, got %s", req.Host)
	}
	if string(req.Port) != "80" {
		t.Fatalf("expected Port 80, got %s", req.Port)
	}
}

func TestParseRequestIPv6URLQueryOnly(t *testing.T) {
	request := "GET http://[2001:db8::1]?x=1 HTTP/1.1\r\nHost: [2001:db8::1]\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(req.Host) != "2001:db8::1" {
		t.Fatalf("expected Host 2001:db8::1, got %s", req.Host)
	}
	if string(req.Port) != "80" {
		t.Fatalf("expected Port 80, got %s", req.Port)
	}
	if string(req.URL) != "http://[2001:db8::1]?x=1" {
		t.Fatalf("expected URL http://[2001:db8::1]?x=1, got %s", req.URL)
	}
}

func TestParseRequestQueryOnlyURL(t *testing.T) {
	request := "GET http://example.com?x=1 HTTP/1.1\r\nHost: example.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(req.Host) != "example.com" {
		t.Fatalf("expected Host example.com, got %s", req.Host)
	}
	if string(req.Port) != "80" {
		t.Fatalf("expected Port 80, got %s", req.Port)
	}
	if string(req.URL) != "http://example.com?x=1" {
		t.Fatalf("expected URL http://example.com?x=1, got %s", req.URL)
	}
}

func TestParseRequestIPv6InURL(t *testing.T) {
	request := "GET http://[2001:db8::1]:8080/path?q=1 HTTP/1.1\r\nHost: [2001:db8::1]:8080\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(req.Host) != "2001:db8::1" {
		t.Fatalf("expected Host 2001:db8::1, got %s", req.Host)
	}

	if string(req.Port) != "8080" {
		t.Fatalf("expected Port 8080, got %s", req.Port)
	}
}

func TestCaseInsensitiveHeaders(t *testing.T) {
	request := "GET http://example.com/ HTTP/1.1\r\nhost: example.com\r\nPROXY-AUTHORIZATION: Basic dXNlcjpwYXNz\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(req.GetHeader("Host")) != "example.com" {
		t.Fatalf("expected Host header via case-insensitive lookup")
	}

	if string(req.GetHeader("proxy-authorization")) != "Basic dXNlcjpwYXNz" {
		t.Fatalf("expected Proxy-Authorization header via case-insensitive lookup, got %s", req.GetHeader("Proxy-Authorization"))
	}

	req.DeleteHeader("proxy-authorization")
	if req.GetHeader("Proxy-Authorization") != nil {
		t.Fatalf("expected Proxy-Authorization to be deleted")
	}
}

func TestOriginForm(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://example.com/path?q=1", "/path?q=1"},
		{"http://example.com", "/"},
		{"http://example.com/", "/"},
		{"https://example.com:443/foo", "/foo"},
		{"/path?q=1", "/path?q=1"},
		{"http://example.com?x=1", "/?x=1"},
		{"http://example.com?x=1&y=2", "/?x=1&y=2"},
	}

	for _, tc := range tests {
		result := string(originForm([]byte(tc.input)))
		if result != tc.expected {
			t.Errorf("originForm(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestWriteToOriginForm(t *testing.T) {
	request := "GET http://api.ipquery.io/?format=json HTTP/1.1\r\nHost: api.ipquery.io\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedLine := "GET /?format=json HTTP/1.1\r\n"
	output := buf.String()
	if !strings.HasPrefix(output, expectedLine) {
		t.Fatalf("expected request line %q, got %q", expectedLine, output)
	}
}

func TestWriteToRemovesProxyAuth(t *testing.T) {
	request := "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: Basic dXNlcjpwYXNz\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	req.DeleteHeader("Proxy-Authorization")

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if strings.Contains(buf.String(), "Proxy-Authorization") {
		t.Fatalf("Proxy-Authorization should not be forwarded")
	}
}

func TestWriteToNormalizesHostIPv6NoPort(t *testing.T) {
	request := "GET http://[2001:db8::1]/path HTTP/1.1\r\nHost: wrong.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Host: [2001:db8::1]\r\n") {
		t.Fatalf("expected bracketed IPv6 Host header, got:\n%s", output)
	}
}

func TestWriteToNormalizesHostIPv6QueryOnly(t *testing.T) {
	request := "GET http://[2001:db8::1]?x=1 HTTP/1.1\r\nHost: wrong.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Host: [2001:db8::1]\r\n") {
		t.Fatalf("expected bracketed IPv6 Host header for query-only URL, got:\n%s", output)
	}
}

func TestWriteToNormalizesHostIPv6WithPort(t *testing.T) {
	request := "GET http://[2001:db8::1]:8080/path HTTP/1.1\r\nHost: wrong.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Host: [2001:db8::1]:8080\r\n") {
		t.Fatalf("expected bracketed IPv6 Host header with port, got:\n%s", output)
	}
}

func TestWriteToNormalizesHost(t *testing.T) {
	request := "GET http://example.com:8080/path HTTP/1.1\r\nHost: wrong.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Host: example.com:8080\r\n") {
		t.Fatalf("expected normalized Host header, got:\n%s", output)
	}
}

func TestWriteToRejectsChunked(t *testing.T) {
	request := "POST http://example.com/ HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, nil)
	if err == nil {
		t.Fatalf("expected error for chunked transfer-encoding")
	}
	if strings.Contains(buf.String(), "\r\n\r\n") {
		t.Fatalf("should not write anything to upstream before rejecting chunked")
	}
}

func TestWriteToPOSTWithBody(t *testing.T) {
	body := "foo=bar&baz=qux"
	request := "POST http://example.com/submit HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	reader := bufio.NewReader(strings.NewReader(request))
	req, err := ParseRequest(reader)
	defer req.Release()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var buf bytes.Buffer
	_, err = req.WriteTo(&buf, reader)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "foo=bar&baz=qux") {
		t.Fatalf("expected body in output, got %q", output)
	}
}

func BenchmarkParseRequest(b *testing.B) {
	request := "GET http://api.ipquery.io/?format=json HTTP/1.1\r\nHost: api.ipquery.io\r\nUser-Agent: curl/8.5.0\r\nAccept: */*\r\n\r\n"
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(request))
		req, err := ParseRequest(reader)
		if err != nil {
			b.Fatalf("expected no error, got %v", err)
		}
		req.Release()
	}
}
