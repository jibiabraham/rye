package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"rye/internal/compute"
)

type stubProvider struct{}

func (stubProvider) List(context.Context) ([]compute.VM, error) {
	return []compute.VM{{Name: "box", Status: compute.StatusRunning}}, nil
}

func (stubProvider) Get(_ context.Context, name string) (compute.VM, error) {
	if name != "box" {
		return compute.VM{}, compute.ErrNotFound
	}
	return compute.VM{Name: name}, nil
}

// fakeAgentManager answers each request line with reply and records the requests.
func fakeAgentManager(t *testing.T, reply func(request map[string]any) []string) (func(context.Context, string) (io.ReadWriteCloser, error), chan map[string]any) {
	t.Helper()
	requests := make(chan map[string]any, 10)
	dial := func(context.Context, string) (io.ReadWriteCloser, error) {
		ours, theirs := net.Pipe()
		go func() {
			defer func() { _ = theirs.Close() }()
			scanner := bufio.NewScanner(theirs)
			for scanner.Scan() {
				var request map[string]any
				if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
					return
				}
				requests <- request
				for _, line := range reply(request) {
					if _, err := io.WriteString(theirs, line+"\n"); err != nil {
						return
					}
				}
			}
		}()
		return ours, nil
	}
	return dial, requests
}

func serve(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		url         string
		origin      string
		wantStatus  int
		contentType string
	}{
		{"rebinding host", http.MethodGet, "http://evil.example:8080/api/vms", "", http.StatusForbidden, ""},
		{"cross origin", http.MethodPost, "http://localhost:8080/api/vms/box/sessions", "https://evil.example", http.StatusForbidden, "application/problem+json"},
		{"same origin", http.MethodGet, "http://localhost:8080/api/vms", "http://localhost:8080", http.StatusOK, ""},
	}
	dial, _ := fakeAgentManager(t, nil)
	h := New(stubProvider{}, dial, fstest.MapFS{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.url, strings.NewReader("{}"))
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if got := w.Header().Get("Content-Type"); tt.contentType != "" && got != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
		})
	}
}

func TestHandler_Tailnet(t *testing.T) {
	tests := []struct {
		name                string
		host, login, origin string
		want                int
	}{
		{"owner", "rye.tail1.ts.net", "owner@github", "", http.StatusOK},
		{"same origin", "rye.tail1.ts.net", "owner@github", "https://rye.tail1.ts.net", http.StatusOK},
		{"http origin", "rye.tail1.ts.net", "owner@github", "http://rye.tail1.ts.net", http.StatusForbidden},
		{"cross origin", "rye.tail1.ts.net", "owner@github", "https://evil.example", http.StatusForbidden},
		{"other login", "rye.tail1.ts.net", "other@github", "", http.StatusForbidden},
		{"no login", "rye.tail1.ts.net", "", "", http.StatusForbidden},
		{"other host", "localhost", "owner@github", "", http.StatusForbidden},
	}
	dial, _ := fakeAgentManager(t, nil)
	h := New(stubProvider{}, dial, fstest.MapFS{}).Tailnet("rye.tail1.ts.net", "owner@github")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/vms", nil)
			r.Host = tt.host
			if tt.login != "" {
				r.Header.Set("Tailscale-User-Login", tt.login)
			}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestHandler_listVMs(t *testing.T) {
	dial, _ := fakeAgentManager(t, nil)
	w := serve(t, New(stubProvider{}, dial, fstest.MapFS{}), http.MethodGet, "/api/vms", "")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if want := `"name":"box"`; !strings.Contains(w.Body.String(), want) {
		t.Errorf("body = %s, want it to contain %s", w.Body, want)
	}
}

func TestHandler_forward(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		reply       string // agent-manager's reply line; "" means agent-manager is unreachable
		wantStatus  int
		wantBody    string         // a substring of the response body
		wantRequest map[string]any // fields of the agent-manager request; nil means no request is checked
		absent      []string       // fields the agent-manager request leaves out
	}{
		{
			name: "create", method: http.MethodPost, path: "/api/vms/box/sessions", body: `{"cwd":"/home/dev"}`,
			reply: `{"id":1,"session":"s1"}`, wantStatus: http.StatusCreated, wantBody: `{"session":"s1"}`,
			wantRequest: map[string]any{"type": "create", "cwd": "/home/dev"}, absent: []string{"model"},
		},
		{
			name: "approval", method: http.MethodPost, path: "/api/vms/box/sessions/s1/approvals/r1", body: `{"behavior":"allow"}`,
			reply: `{"id":1}`, wantStatus: http.StatusNoContent,
			wantRequest: map[string]any{"type": "answer", "session": "s1", "request": "r1", "result": map[string]any{"behavior": "allow"}},
		},
		{
			name: "agent-manager refusal", method: http.MethodPost, path: "/api/vms/box/sessions/s1/approvals/r1", body: `{}`,
			reply: `{"id":1,"error":"no pending request r1"}`, wantStatus: http.StatusBadRequest, wantBody: "no pending request",
		},
		{
			name: "unreachable agent-manager", method: http.MethodGet, path: "/api/vms/box/sessions",
			wantStatus: http.StatusBadGateway, wantBody: "Connection refused",
		},
		{
			name: "unknown vm", method: http.MethodGet, path: "/api/vms/nope/sessions",
			reply: `{"id":1}`, wantStatus: http.StatusNotFound,
		},
		{
			name: "body not an object", method: http.MethodPost, path: "/api/vms/box/sessions/s1/messages", body: `[1]`,
			reply: `{"id":1}`, wantStatus: http.StatusBadRequest,
		},
		{
			name: "earlier messages", method: http.MethodGet, path: "/api/vms/box/sessions/s1/messages?before=m9",
			reply: `{"id":1,"messages":[],"more":false}`, wantStatus: http.StatusOK, wantBody: `"more":false`,
			wantRequest: map[string]any{"type": "messages", "before": "m9", "limit": float64(pageSize)},
		},
		{
			name: "from a linked message", method: http.MethodGet, path: "/api/vms/box/sessions/s1/messages?before=m9&from=m2",
			reply: `{"id":1,"messages":[],"more":false}`, wantStatus: http.StatusOK,
			wantRequest: map[string]any{"from": "m2", "before": "m9"},
		},
		{
			name: "all messages", method: http.MethodGet, path: "/api/vms/box/sessions/s1/messages?before=m9&all=1",
			reply: `{"id":1,"messages":[],"more":false}`, wantStatus: http.StatusOK,
			wantRequest: map[string]any{"limit": nil},
		},
		{
			name: "models", method: http.MethodGet, path: "/api/vms/box/models",
			reply: `{"id":1,"models":[{"value":"haiku"}]}`, wantStatus: http.StatusOK, wantBody: `"haiku"`,
		},
		{
			name: "effort", method: http.MethodPut, path: "/api/vms/box/sessions/s1/effort", body: `{"effort":"low"}`,
			reply: `{"id":1}`, wantStatus: http.StatusNoContent,
			wantRequest: map[string]any{"type": "effort", "session": "s1", "effort": "low"},
		},
		{
			name: "default effort", method: http.MethodPut, path: "/api/vms/box/sessions/s1/effort", body: `{}`,
			reply: `{"id":1}`, wantStatus: http.StatusNoContent,
			wantRequest: map[string]any{"effort": nil},
		},
		{
			name: "mode", method: http.MethodPut, path: "/api/vms/box/sessions/s1/mode", body: `{"mode":"auto"}`,
			reply: `{"id":1}`, wantStatus: http.StatusNoContent,
			wantRequest: map[string]any{"type": "mode", "session": "s1", "mode": "auto"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dial, requests := fakeAgentManager(t, func(map[string]any) []string { return []string{tt.reply} })
			if tt.reply == "" {
				dial = func(context.Context, string) (io.ReadWriteCloser, error) {
					return nil, errors.New("channel 0: open failed: connect failed: Connection refused")
				}
			}
			w := serve(t, New(stubProvider{}, dial, fstest.MapFS{}), tt.method, tt.path, tt.body)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body %s", w.Code, tt.wantStatus, w.Body)
			}
			if !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", w.Body, tt.wantBody)
			}
			if tt.wantRequest == nil && tt.absent == nil {
				return
			}
			// The fake agent-manager records a request before it replies, and the handler returns after the reply.
			var request map[string]any
			select {
			case request = <-requests:
			default:
				t.Fatal("agent-manager got no request")
			}
			for key, want := range tt.wantRequest {
				if got := request[key]; !reflect.DeepEqual(got, want) {
					t.Errorf("request[%q] = %v, want %v", key, got, want)
				}
			}
			for _, key := range tt.absent {
				if got, ok := request[key]; ok {
					t.Errorf("request[%q] = %v, want it absent", key, got)
				}
			}
		})
	}
}

func TestHandler_events(t *testing.T) {
	dial, requests := fakeAgentManager(t, func(request map[string]any) []string {
		switch request["type"] {
		case "subscribe":
			return []string{`{"id":1,"live":true,"pending":[]}`}
		case "messages":
			// The fake agent-manager closes the stream after an event, which ends the response.
			return []string{`{"id":2,"messages":[]}`, `{"session":"s1","message":{"type":"result"}}`}
		}
		return nil
	})
	server := httptest.NewServer(New(stubProvider{}, dial, fstest.MapFS{}))
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)+"/api/vms/box/sessions/s1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if got, want := response.Header.Get("Content-Type"), "text/event-stream"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}
	reader := bufio.NewReader(response.Body)
	var events []string
	for len(events) < 3 {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "event: "); ok {
			events = append(events, name)
		}
	}
	if got, want := strings.Join(events, ","), "state,history,message"; got != want {
		t.Errorf("events = %s, want %s", got, want)
	}
	if first := <-requests; first["type"] != "subscribe" {
		t.Errorf("first request type = %v, want subscribe", first["type"])
	}
}

func TestHandler_page(t *testing.T) {
	web := fstest.MapFS{
		"index.html":           {Data: []byte("<title>rye</title>")},
		"assets/index-abc1.js": {Data: []byte("app()")},
	}
	dial, _ := fakeAgentManager(t, nil)
	h := New(stubProvider{}, dial, web)
	tests := []struct {
		name string
		path string
		want int
	}{
		{"root", "/", http.StatusOK},
		{"vms", "/vms", http.StatusOK},
		{"vm", "/vms/box", http.StatusOK},
		{"session", "/vms/box/sessions/s1", http.StatusOK},
		{"unknown path", "/elsewhere", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(t, h, http.MethodGet, tt.path, "")
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
			if tt.want != http.StatusOK {
				return
			}
			if !strings.Contains(w.Body.String(), "<title>rye</title>") {
				t.Errorf("body = %s, want the page", w.Body)
			}
			if got := w.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
			}
			if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
				t.Errorf("Content-Security-Policy = %q, want frame-ancestors 'none'", got)
			}
		})
	}
}

func TestHandler_asset(t *testing.T) {
	web := fstest.MapFS{"assets/index-abc1.js": {Data: []byte("app()")}}
	dial, _ := fakeAgentManager(t, nil)
	w := serve(t, New(stubProvider{}, dial, web), http.MethodGet, "/assets/index-abc1.js", "")
	if w.Code != http.StatusOK || w.Body.String() != "app()" {
		t.Fatalf("GET /assets/index-abc1.js = %d %q, want 200 with the file", w.Code, w.Body)
	}
	if got, want := w.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}
