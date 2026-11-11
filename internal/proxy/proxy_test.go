package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"rye/internal/integration"
)

type upstreamRequest struct {
	authorization string
	path          string
	host          string
}

func setup(t *testing.T) (*Handler, *upstreamRequest) {
	t.Helper()
	seen := &upstreamRequest{}
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		*seen = upstreamRequest{r.Header.Get("Authorization"), r.URL.Path, r.Host}
	}))
	t.Cleanup(upstream.Close)

	dir := t.TempDir()
	storePath := filepath.Join(dir, "integrations.json")
	store, err := integration.Load(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add("forgejo", integration.Integration{URL: upstream.URL, Header: "Authorization", Value: "token stored"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Attach("forgejo", "attached"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(storePath); err != nil {
		t.Fatal(err)
	}
	keysPath := filepath.Join(dir, "authorized_keys")
	if err := os.WriteFile(keysPath, []byte("key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Handler{
		KeysPath:  keysPath,
		StorePath: storePath,
		FindVM: func(_ context.Context, address string) (string, error) {
			switch address {
			case "198.18.0.2":
				return "attached", nil
			case "198.18.0.3":
				return "other", nil
			}
			return "", errors.New("no VM")
		},
	}, seen
}

func get(h http.Handler, host, address string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/v1/user", nil)
	request.RemoteAddr = address + ":40000"
	request.Header.Set("Authorization", "Bearer guest")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		address  string
		wantCode int
		wantBody string
		// wantAuth is the Authorization header upstream gets, or "" when the request mustn't reach upstream.
		wantAuth     string
		wantDocsPath string
	}{
		{name: "attached VM", host: "forgejo.int.rye", address: "198.18.0.2", wantCode: http.StatusOK, wantAuth: "token stored"},
		{name: "unattached VM", host: "forgejo.int.rye", address: "198.18.0.3", wantCode: http.StatusForbidden},
		{name: "unknown address", host: "forgejo.int.rye", address: "198.18.0.1", wantCode: http.StatusForbidden},
		{name: "unknown integration", host: "missing.int.rye", address: "198.18.0.2", wantCode: http.StatusForbidden},
		{name: "keys host", host: "keys.int.rye", address: "198.18.0.3", wantCode: http.StatusOK, wantBody: "key\n"},
		// 198.18.0.11 is the admin VM, which FindVM doesn't know.
		{name: "docs host", host: "docs.int.rye", address: "198.18.0.11", wantCode: http.StatusOK, wantBody: "docs", wantDocsPath: "/api/v1/user"},
		{name: "other host", host: "example.com", address: "198.18.0.2", wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, seen := setup(t)
			var docsPath string
			h.Docs = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				docsPath = r.URL.Path
				_, _ = io.WriteString(w, "docs")
			})
			response := get(h, tt.host, tt.address)
			if response.Code != tt.wantCode {
				t.Errorf("GET %s from %s: code = %d, want %d", tt.host, tt.address, response.Code, tt.wantCode)
			}
			if tt.wantBody != "" && response.Body.String() != tt.wantBody {
				t.Errorf("GET %s from %s: body = %q, want %q", tt.host, tt.address, response.Body.String(), tt.wantBody)
			}
			if docsPath != tt.wantDocsPath {
				t.Errorf("GET %s from %s: docs path = %q, want %q", tt.host, tt.address, docsPath, tt.wantDocsPath)
			}
			if tt.wantAuth == "" {
				if *seen != (upstreamRequest{}) {
					t.Errorf("GET %s from %s: upstream got %+v, want no request", tt.host, tt.address, *seen)
				}
				return
			}
			if seen.authorization != tt.wantAuth || seen.path != "/api/v1/user" {
				t.Errorf("upstream got Authorization %q, path %q, want %q, %q", seen.authorization, seen.path, tt.wantAuth, "/api/v1/user")
			}
		})
	}
}

// TestHandler_vmTransport proxies an integration on a VM through the VM's loopback, over one reused connection.
func TestHandler_vmTransport(t *testing.T) {
	h, _ := setup(t)
	var seen upstreamRequest
	forge := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = upstreamRequest{r.Header.Get("Authorization"), r.URL.Path, r.Host}
	}))
	t.Cleanup(forge.Close)
	var dialed []string
	h.DialVM = func(ctx context.Context, vm, port string) (io.ReadWriteCloser, error) {
		dialed = append(dialed, vm+" "+port)
		return (&net.Dialer{}).DialContext(ctx, "tcp", forge.Listener.Addr().String())
	}
	store, err := integration.Load(h.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add("forge", integration.Integration{URL: "http://forge.rye:3000", Header: "Authorization", Value: "token bot"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Attach("forge", "attached"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(h.StorePath); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if response := get(h, "forge.int.rye", "198.18.0.2"); response.Code != http.StatusOK {
			t.Fatalf("code = %d, want %d", response.Code, http.StatusOK)
		}
	}
	if want := (upstreamRequest{"token bot", "/api/v1/user", "forge.rye:3000"}); seen != want {
		t.Errorf("upstream got %+v, want %+v", seen, want)
	}
	if want := []string{"forge 3000"}; !slices.Equal(dialed, want) {
		t.Errorf("dialed = %q, want %q", dialed, want)
	}
}

func TestHandler_ServeHTTP_trace(t *testing.T) {
	h, seen := setup(t)
	request := httptest.NewRequest(http.MethodTrace, "http://forgejo.int.rye/", nil)
	request.RemoteAddr = "198.18.0.2:40000"
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("ServeHTTP(TRACE) code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if *seen != (upstreamRequest{}) {
		t.Errorf("ServeHTTP(TRACE) upstream request = %+v, want none", *seen)
	}
}
