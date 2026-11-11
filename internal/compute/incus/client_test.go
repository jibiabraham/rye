package incus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rye/internal/compute"
)

func testClient(t *testing.T, handler http.HandlerFunc) *client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &client{http: server.Client(), baseURL: server.URL, project: "rye"}
}

// async answers a request with an operation, and the operation's wait with wait.
func async(wait string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/wait") {
			_, _ = io.WriteString(w, wait)
			return
		}
		_, _ = io.WriteString(w, `{"type":"async","operation":"/1.0/operations/1"}`)
	}
}

func TestClient_Request(t *testing.T) {
	type result struct {
		Name   string `json:"name"`
		Return int    `json:"return"`
	}
	tests := []struct {
		name      string
		method    string
		body      any
		handler   http.HandlerFunc
		want      result
		wantErr   string
		wantErrIs error
	}{
		{
			name:   "sync metadata",
			method: http.MethodGet,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `{"type":"sync","metadata":{"name":"a"}}`)
			},
			want: result{Name: "a"},
		},
		{
			name:   "not found",
			method: http.MethodGet,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"type":"error","error":"Not Found","error_code":404}`)
			},
			wantErrIs: compute.ErrNotFound,
		},
		{
			name:    "operation",
			method:  http.MethodPost,
			body:    map[string]any{},
			handler: async(`{"type":"sync","metadata":{"status_code":200,"metadata":{"return":3}}}`),
			want:    result{Return: 3},
		},
		{
			name:    "failed operation",
			method:  http.MethodDelete,
			handler: async(`{"type":"sync","metadata":{"status_code":400,"err":"boom"}}`),
			wantErr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("project"); got != "rye" {
					t.Errorf("%s %s: project = %q, want %q", r.Method, r.URL.Path, got, "rye")
				}
				tt.handler(w, r)
			})
			var got result
			err := c.request(context.Background(), tt.method, "/1.0/instances/a", tt.body, &got)
			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("request() error = %v, want %v", err, tt.wantErrIs)
				}
			case tt.wantErr != "":
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("request() error = %v, want %q", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("request() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("request() decoded %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClient_Push(t *testing.T) {
	tests := []struct {
		name        string
		file        compute.File
		status      int
		reply       string
		wantHeaders map[string]string
		wantBody    string
		wantLength  int64
		wantErr     string
	}{
		{
			name:   "owner and mode",
			file:   compute.File{Path: "/tmp/x", Body: strings.NewReader("hello"), Size: 5, Mode: 0o640, UID: 1000, GID: 1001},
			status: http.StatusOK,
			reply:  `{"type":"sync","metadata":{}}`,
			wantHeaders: map[string]string{
				"Content-Type": "application/octet-stream", "X-Incus-Type": "file",
				"X-Incus-Write": "overwrite", "X-Incus-Uid": "1000", "X-Incus-Gid": "1001", "X-Incus-Mode": "0640",
			},
			wantBody:   "hello",
			wantLength: 5,
		},
		{
			name:    "incus error",
			file:    compute.File{Path: "/x/y", Body: strings.NewReader(""), Mode: 0o644},
			status:  http.StatusBadRequest,
			reply:   `{"type":"error","error":"no such directory"}`,
			wantErr: "no such directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var headers http.Header
			var body string
			var length int64
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/1.0/instances/a/files" || r.URL.Query().Get("path") != tt.file.Path {
					t.Errorf("request = %s %s, want POST /1.0/instances/a/files?path=%s", r.Method, r.URL, tt.file.Path)
				}
				headers, length = r.Header, r.ContentLength
				data, _ := io.ReadAll(r.Body)
				body = string(data)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.reply)
			})
			err := c.push(context.Background(), "a", tt.file)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("push() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("push() error = %v", err)
			}
			for key, want := range tt.wantHeaders {
				if got := headers.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if body != tt.wantBody || length != tt.wantLength {
				t.Errorf("body = %q, length %d, want %q, length %d", body, length, tt.wantBody, tt.wantLength)
			}
		})
	}
}

func TestProvider_Run(t *testing.T) {
	var exec map[string]any
	deleted := map[string]bool{}
	logs := map[string]string{"/1.0/instances/a/logs/exec-output/e.stdout": "out\n", "/1.0/instances/a/logs/exec-output/e.stderr": "err\n"}
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances/a/exec":
			_ = json.NewDecoder(r.Body).Decode(&exec)
			_, _ = io.WriteString(w, `{"type":"async","operation":"/1.0/operations/1"}`)
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_, _ = io.WriteString(w, `{"type":"sync","metadata":{"status_code":200,"metadata":{"return":3,"output":{`+
				`"1":"/1.0/instances/a/logs/exec-output/e.stdout","2":"/1.0/instances/a/logs/exec-output/e.stderr"}}}}`)
		case r.Method == http.MethodGet && logs[r.URL.Path] != "":
			_, _ = io.WriteString(w, logs[r.URL.Path])
		case r.Method == http.MethodDelete && logs[r.URL.Path] != "":
			deleted[r.URL.Path] = true
			_, _ = io.WriteString(w, `{"type":"sync","metadata":{}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	p := &Provider{api: c}
	got, err := p.Run(context.Background(), "a", compute.Command{Args: []string{"id"}, UID: 1000, GID: 1000, Dir: "/home/dev",
		Env: map[string]string{"HOME": "/home/dev"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := (compute.Result{Code: 3, Stdout: "out\n", Stderr: "err\n"}); got != want {
		t.Errorf("Run() = %+v, want %+v", got, want)
	}
	wantExec := map[string]any{"record-output": true, "wait-for-websocket": false, "user": float64(1000), "cwd": "/home/dev"}
	for key, want := range wantExec {
		if exec[key] != want {
			t.Errorf("exec request %s = %v, want %v", key, exec[key], want)
		}
	}
	if len(deleted) != len(logs) {
		t.Errorf("deleted %v, want every log in %v", deleted, logs)
	}
}

func TestServerTLS(t *testing.T) {
	// httptest's TLS server listens on 127.0.0.1, and its certificate's first DNS name is example.com.
	trusted := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(trusted.Close)

	// other has another certificate for the same DNS name.
	otherPEM, otherKey := testCertFor(t, "example.com")
	other := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	other.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{decodePEM(t, otherPEM)}, PrivateKey: otherKey}}}
	other.StartTLS()
	t.Cleanup(other.Close)

	noNamePEM, _ := testCertFor(t)
	noName, err := x509.ParseCertificate(decodePEM(t, noNamePEM))
	if err != nil {
		t.Fatal(err)
	}
	clientPEM, _ := testCertFor(t)
	pair := tls.Certificate{Certificate: [][]byte{decodePEM(t, clientPEM)}}

	tests := []struct {
		name        string
		server      *x509.Certificate
		url         string
		wantErr     string
		wantDialErr bool
	}{
		{name: "own certificate", server: trusted.Certificate(), url: trusted.URL},
		{name: "same DNS name", server: trusted.Certificate(), url: other.URL, wantDialErr: true},
		{name: "no DNS name", server: noName, wantErr: "no DNS name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := serverTLS(tt.server, pair)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("serverTLS() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("serverTLS() error = %v", err)
			}
			c := &http.Client{Transport: &http.Transport{TLSClientConfig: config}}
			res, err := c.Get(tt.url)
			if err == nil {
				_ = res.Body.Close()
			}
			if gotErr := err != nil; gotErr != tt.wantDialErr {
				t.Errorf("Get(%q) error = %v, want error %v", tt.url, err, tt.wantDialErr)
			}
		})
	}
}

func decodePEM(t *testing.T, data []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("not PEM")
	}
	return block.Bytes
}

func TestProvider_List(t *testing.T) {
	var requests []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path != "/1.0/instances" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"type":"sync","metadata":[{"name":"a","status_code":103,"config":{"user.rye.ready":"true"},`+
			`"devices":{"root":{"size":"30GiB"}}}]}`)
	})
	vms, err := (&Provider{api: c, pool: "default"}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (compute.VM{Name: "a", Status: compute.StatusRunning, Disk: "30GiB"}); len(vms) != 1 || vms[0] != want {
		t.Errorf("List() = %+v, want [%+v]", vms, want)
	}
	if len(requests) != 1 {
		t.Errorf("List() made %d requests, want 1: %q", len(requests), requests)
	}
}
