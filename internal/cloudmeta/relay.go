package cloudmeta

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultUpstream = "https://daily-cloudcode-pa.googleapis.com"
	RelayTimeout    = 30 * time.Second
	MaxBodySize     = 16 << 20
)

var requestHopHeaders = map[string]bool{
	"connection": true, "content-length": true, "host": true, "transfer-encoding": true,
	"expect": true, "accept-encoding": true, "proxy-connection": true, "keep-alive": true,
}
var responseHopHeaders = map[string]bool{
	"connection": true, "content-length": true, "transfer-encoding": true,
	"keep-alive": true, "proxy-authenticate": true, "proxy-authorization": true,
	"trailer": true, "upgrade": true,
}

// Relay is a running local Cloud Code HTTP relay.
type Relay struct {
	URL       string
	Collector *Collector
	server    *http.Server
	listener  net.Listener
}

// Start starts a relay on an ephemeral 127.0.0.1 port. An empty upstream uses DefaultUpstream.
func Start(upstream string) (*Relay, error) {
	if upstream == "" {
		upstream = DefaultUpstream
	}
	upstream = strings.TrimRight(upstream, "/")
	u, err := url.Parse(upstream)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("cloudmeta: invalid upstream URL")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	collector := NewCollector()
	r := &Relay{URL: "http://" + ln.Addr().String(), Collector: collector, listener: ln}
	// Constructing a fresh transport avoids retaining request headers or bodies in relay state.
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DisableCompression: true, ResponseHeaderTimeout: RelayTimeout, TLSHandshakeTimeout: RelayTimeout, DialContext: (&net.Dialer{Timeout: RelayTimeout}).DialContext}
	r.server = &http.Server{Handler: r.handler(upstream, transport), ReadTimeout: RelayTimeout, ReadHeaderTimeout: RelayTimeout, WriteTimeout: RelayTimeout, IdleTimeout: RelayTimeout, MaxHeaderBytes: 1 << 20}
	go func() { _ = r.server.Serve(ln) }()
	return r, nil
}

// NewRelay is an alias for Start.
func NewRelay(upstream string) (*Relay, error) { return Start(upstream) }

func (r *Relay) handler(upstream string, transport *http.Transport) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Host != "127.0.0.1" && req.Host != "localhost" && !strings.HasPrefix(req.Host, "127.0.0.1:") && !strings.HasPrefix(req.Host, "localhost:") {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if req.Method != http.MethodGet && req.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, MaxBodySize+1))
		if err != nil || int64(len(body)) > MaxBodySize {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		target, err := url.Parse(upstream + req.URL.RequestURI())
		if err != nil {
			http.Error(w, "bad upstream URL", http.StatusBadGateway)
			return
		}
		var src io.Reader
		if req.Method == http.MethodPost {
			src = bytes.NewReader(body)
		}
		out, err := http.NewRequestWithContext(req.Context(), req.Method, target.String(), src)
		if err != nil {
			http.Error(w, "bad upstream request", http.StatusBadGateway)
			return
		}
		for name, values := range req.Header {
			if requestHopHeaders[strings.ToLower(name)] {
				continue
			}
			for _, value := range values {
				out.Header.Add(name, value)
			}
		}
		if req.Method == http.MethodPost {
			out.ContentLength = int64(len(body))
		}
		resp, err := transport.RoundTrip(out)
		if err != nil {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize+1))
		if err != nil || int64(len(respBody)) > MaxBodySize {
			http.Error(w, "upstream response too large", http.StatusBadGateway)
			return
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			r.Collector.Observe(req.URL.Path, respBody)
		}
		for name, values := range resp.Header {
			if responseHopHeaders[strings.ToLower(name)] {
				continue
			}
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.Header().Set("Content-Length", stringSize(len(respBody)))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(respBody)
	})
}

func stringSize(n int) string { return strconv.Itoa(n) }

// Close stops accepting requests and releases the loopback listener.
func (r *Relay) Close() error {
	if r == nil || r.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), RelayTimeout)
	defer cancel()
	return r.server.Shutdown(ctx)
}

// Meta returns a current snapshot of observed metadata.
func (r *Relay) Meta() CloudMeta {
	if r == nil || r.Collector == nil {
		return CloudMeta{ModelTags: map[string]string{}}
	}
	return r.Collector.Snapshot()
}

// LocalAddr exposes the bound loopback address.
func (r *Relay) LocalAddr() net.Addr {
	if r == nil || r.listener == nil {
		return nil
	}
	return r.listener.Addr()
}
