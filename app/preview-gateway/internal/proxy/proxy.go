package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"preview-gateway/internal/routing"
)

type bufferPool struct {
	pool sync.Pool
}

func newBufferPool(size int) *bufferPool {
	return &bufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, size)
			},
		},
	}
}

func (b *bufferPool) Get() []byte  { return b.pool.Get().([]byte) }
func (b *bufferPool) Put(v []byte) { b.pool.Put(v) }

var sharedTransport = &http.Transport{
	MaxIdleConns:          256,
	MaxIdleConnsPerHost:   64,
	IdleConnTimeout:       90 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	ForceAttemptHTTP2:     true,
	DisableCompression:    false,
}

type Gateway struct {
	proxies map[string]*httputil.ReverseProxy
	bp      *bufferPool
}

func New() *Gateway {
	g := &Gateway{
		proxies: make(map[string]*httputil.ReverseProxy),
		bp:      newBufferPool(32 * 1024),
	}
	for _, u := range routing.GetAllUpstreams() {
		g.proxies[u.Name] = g.newProxy(u)
	}
	return g
}

func (g *Gateway) newProxy(u *routing.Upstream) *httputil.ReverseProxy {
	targetURL, _ := url.Parse(u.Target)
	p := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			if u.PathRewrite != nil {
				req.URL.Path = u.PathRewrite(req.URL.Path)
			}
			req.URL.RawQuery = req.URL.RawQuery

			req.Header.Set("X-Gw-Source", u.Name)
			req.Header.Set("Origin", "http://localhost:16666")

			if proto := getForwardedProto(req); proto != "" {
				req.Header.Set("X-Forwarded-Proto", proto)
			}
		},
		FlushInterval: -1,
		BufferPool:    g.bp,
		ModifyResponse: func(resp *http.Response) error {
			return modifyResponse(resp)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			errorHandler(w, r, err, u)
		},
		Transport: sharedTransport,
	}
	return p
}

func getForwardedProto(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return ""
}

func modifyResponse(resp *http.Response) error {
	source := resp.Header.Get("X-Gw-Source")
	resp.Header.Del("X-Gw-Source")

	if source == "plugin-openlist-web" {
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(strings.ToLower(ct), "text/html") {
			cookie := "__plugin_spa=1; Path=/; SameSite=Lax; Max-Age=3600"
			existing := resp.Header["Set-Cookie"]
			resp.Header["Set-Cookie"] = append(existing, cookie)
		}
	}
	return nil
}

func errorHandler(w http.ResponseWriter, r *http.Request, err error, u *routing.Upstream) {
	log.Printf("[gateway] upstream error: %s %s -> %s: %v", r.Method, r.URL.String(), u.Name, err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Upstream", u.Name)
	w.WriteHeader(http.StatusBadGateway)
	payload := map[string]interface{}{
		"ok":       false,
		"error":    "upstream unreachable",
		"detail":   err.Error(),
		"upstream": u.Name,
		"hint":     u.Hint,
	}
	if b, e := json.Marshal(payload); e == nil {
		_, _ = w.Write(b)
	}
}

func (g *Gateway) ProxyFor(u *routing.Upstream) *httputil.ReverseProxy {
	if p, ok := g.proxies[u.Name]; ok {
		return p
	}
	return g.proxies[routing.EncvGoUpstream.Name]
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isWebSocketUpgrade(r) {
		g.handleWS(w, r)
		return
	}
	referer := r.Referer()
	cookie := r.Header.Get("Cookie")
	u := routing.PickUpstream(r.URL.RequestURI(), referer, cookie)
	g.ProxyFor(u).ServeHTTP(w, r)
}

func isWebSocketUpgrade(r *http.Request) bool {
	upgrade := r.Header.Get("Upgrade")
	connection := r.Header.Get("Connection")
	return strings.EqualFold(upgrade, "websocket") &&
		strings.Contains(strings.ToLower(connection), "upgrade")
}

func (g *Gateway) handleWS(w http.ResponseWriter, r *http.Request) {
	referer := r.Referer()
	cookie := r.Header.Get("Cookie")
	u := routing.PickUpstream(r.URL.RequestURI(), referer, cookie)
	proxyWS(w, r, u)
}

func proxyWS(w http.ResponseWriter, r *http.Request, u *routing.Upstream) {
	target, _ := url.Parse(u.WsTarget)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}

	backend, err := net.DialTimeout("tcp", target.Host, 5*time.Second)
	if err != nil {
		errorHandler(w, r, err, u)
		return
	}
	defer backend.Close()

	outReq := r.Clone(r.Context())
	outReq.URL.Scheme = "http"
	outReq.URL.Host = target.Host
	if u.PathRewrite != nil {
		outReq.URL.Path = u.PathRewrite(r.URL.Path)
	}
	outReq.RequestURI = ""
	outReq.Header.Set("X-Gw-Source", u.Name)
	outReq.Header.Set("Origin", "http://localhost:16666")

	if proto := getForwardedProto(r); proto != "" {
		outReq.Header.Set("X-Forwarded-Proto", proto)
	}

	if err := outReq.Write(backend); err != nil {
		return
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	done := make(chan struct{}, 2)
	go func() { io.Copy(backend, clientConn); done <- struct{}{} }()
	go func() { io.Copy(clientConn, backend); done <- struct{}{} }()
	<-done
}
