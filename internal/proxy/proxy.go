// Package proxy serves guests the key endpoint and their attached integrations.
package proxy

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"rye/internal/integration"
)

const (
	hostSuffix = ".int.rye"
	// vmSuffix marks an integration URL on a VM. The proxy reaches it through SSH on the VM's loopback interface.
	vmSuffix = ".rye"
)

// Handler routes guest requests by host name.
type Handler struct {
	// KeysPath is the authorized_keys file behind keys.int.rye.
	KeysPath string
	// Docs serves docs.int.rye to every VM.
	Docs http.Handler
	// StorePath is the integrations file.
	StorePath string
	// FindVM returns the name of the VM at an address.
	FindVM func(ctx context.Context, address string) (string, error)
	// DialVM opens a connection to 127.0.0.1:<port> in a VM, for integration URLs on <vm>.rye.
	DialVM func(ctx context.Context, vm, port string) (io.ReadWriteCloser, error)

	transports sync.Map
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutSuffix(hostname(r.Host), hostSuffix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if name == "keys" {
		http.ServeFile(w, r, h.KeysPath)
		return
	}
	if name == "docs" {
		h.Docs.ServeHTTP(w, r)
		return
	}
	// An upstream that answers TRACE echoes the request, credential included.
	if r.Method == http.MethodTrace {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	address, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	vm, err := h.FindVM(r.Context(), address)
	if err != nil {
		slog.Error("find VM", "address", address, "err", err)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	store, err := integration.Load(h.StorePath)
	if err != nil {
		slog.Error("load integrations", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	target, ok := store.Integrations[name]
	if !ok || !store.Attached(name, vm) {
		slog.Warn("integration not attached", "vm", vm, "integration", name)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	upstream, err := url.Parse(target.URL)
	if err != nil {
		slog.Error("integration URL", "integration", name, "err", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	proxy := reverseProxy(vm, upstream, target)
	if guest, ok := strings.CutSuffix(upstream.Hostname(), vmSuffix); ok && !strings.HasSuffix(guest, ".int") {
		proxy.Transport = h.vmTransport(guest, port(upstream))
	}
	proxy.ServeHTTP(w, r)
}

// vmTransport reuses connections to one port on one VM.
func (h *Handler) vmTransport(vm, port string) http.RoundTripper {
	key := vm + ":" + port
	if t, ok := h.transports.Load(key); ok {
		return t.(http.RoundTripper)
	}
	t, _ := h.transports.LoadOrStore(key, &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			rw, err := h.DialVM(ctx, vm, port)
			if err != nil {
				return nil, err
			}
			return streamConn{rw}, nil
		},
		IdleConnTimeout: time.Minute,
	})
	return t.(http.RoundTripper)
}

func port(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// streamConn is a net.Conn over an SSH channel. Its addresses are placeholders and its deadlines do nothing.
type streamConn struct{ io.ReadWriteCloser }

func (streamConn) LocalAddr() net.Addr              { return stubAddr{} }
func (streamConn) RemoteAddr() net.Addr             { return stubAddr{} }
func (streamConn) SetDeadline(time.Time) error      { return nil }
func (streamConn) SetReadDeadline(time.Time) error  { return nil }
func (streamConn) SetWriteDeadline(time.Time) error { return nil }

type stubAddr struct{}

func (stubAddr) Network() string { return "ssh" }
func (stubAddr) String() string  { return "ssh" }

func reverseProxy(vm string, upstream *url.URL, target integration.Integration) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(upstream)
			request.Out.Header.Del("Authorization")
			request.Out.Header.Set(target.Header, target.Value)
		},
		ModifyResponse: func(response *http.Response) error {
			slog.Info("proxied", "vm", vm, "upstream", upstream.Host, "status", response.StatusCode)
			return nil
		},
	}
}

func hostname(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		return name
	}
	return host
}
