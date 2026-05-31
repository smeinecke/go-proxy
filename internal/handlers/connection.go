package handlers

import (
	"bufio"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"
	httpParse "github.com/vlourme/go-proxy/internal/http"
)

// HandleConnection handles an incoming connection using the handler's dependencies.
func (p *ProxyHandler) HandleConnection(workerId int, conn net.Conn) {
	defer conn.Close()
	st := p.Stats.Shard(workerId)
	if st != nil {
		st.RequestsTotal.Add(1)
	}

	reader := bufio.NewReader(conn)

	if IsSocks(reader) {
		written := p.HandleSocks(conn, reader, st)
		if written == -1 {
			log.Warn().Int("worker_id", workerId).Msg("Request failed")
		} else {
			log.Trace().Int("worker_id", workerId).Int64("written", written).Msg("Request handled")
		}
		return
	}

	for {
		req, err := httpParse.ParseRequest(reader)
		if err != nil {
			break
		}

		var written int64
		if string(req.Method) == http.MethodConnect {
			if st != nil {
				st.ConnectTotal.Add(1)
			}
			written = p.HandleTunneling(conn, req, st)
		} else {
			if st != nil {
				st.HTTPRequestsTotal.Add(1)
			}
			written = p.HandleHTTP(conn, reader, req, st)
		}

		url := string(req.URL)
		req.Release()

		if written == -1 {
			log.Warn().Int("worker_id", workerId).Str("url", url).Msg("Request failed")
			break
		}
		log.Trace().Int("worker_id", workerId).Str("url", url).Int64("written", written).Msg("Request handled")
	}
}
